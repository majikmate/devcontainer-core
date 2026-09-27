package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/majikmate/devcontainer-core/internal/all" // registers the layers and features
	"github.com/majikmate/devcontainer-core/internal/registry"
	"github.com/majikmate/devcontainer-core/pkg/debian"
	"github.com/majikmate/devcontainer-core/pkg/devcontainer"
	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
	"github.com/majikmate/devcontainer-core/pkg/versions"
)

// Labels in which an image records the inputs of its build.
const (
	LabelInputs      = "devenv.inputs"
	LabelFingerprint = "devenv.fingerprint"
)

// defaultConfigPaths are the configuration paths of an image repository: the
// Dev Container files and the README, which GitHub shows on the package page.
var defaultConfigPaths = []string{".devcontainer", "README.md"}

// PlanOptions are the settings of a release plan.
type PlanOptions struct {
	Dir         string   // root of the image repository
	Image       string   // for example ghcr.io/majikmate/devcontainer-base
	ConfigPaths []string // paths whose content is an input (default defaultConfigPaths)
	Major       int      // major version
	MaxAgeDays  int      // rebuild when the newest image is older
	Force       bool     // release without a change
	Bump        string   // auto, patch, minor, major
	Event       string   // GITHUB_EVENT_NAME
	Ref         string   // GITHUB_REF
	PRNumber    string   // number of the pull request
}

// Plan is the result: the inputs of the image and what to do with them.
type Plan struct {
	Inputs      map[string]string
	Build       bool
	Release     bool
	Version     string
	Tags        []string
	Reason      string
	Created     string
	BuildArgs   map[string]string // tool versions as build arguments
	ToolVersion map[string]string // expected tool versions (name -> version)
	Updates     []debian.Update   // pending Debian updates that are the job of this image
}

// Project describes the image of a repository (from its devcontainer.json).
type Project struct {
	Dir             string
	DevcontainerDir string
	Dockerfile      string
	Context         string
	// Tools overrides the release configuration of the features for this
	// image (customizations.devenv.<tool>).
	Tools map[string]ToolConfig
}

// ToolConfig is the release configuration of a tool in the devcontainer.json
// of the image that installs its layer. It overrides the configuration of
// the feature:
//
//	"customizations": {"devenv": {"deno": {"pin": "2", "channel": "stable"}}}
//
// "pin": "" removes the pin of the feature (the newest release).
type ToolConfig struct {
	Pin     *string `json:"pin"`
	Channel string  `json:"channel"`
}

// apply returns the configuration of the feature with the overrides of the
// image.
func (c ToolConfig) apply(feature layer.Config) layer.Config {
	if c.Pin != nil {
		feature.Pin = *c.Pin
	}
	if c.Channel != "" {
		feature.Channel = c.Channel
	}
	return feature
}

// ReadProject reads build.dockerfile and build.context of .devcontainer/devcontainer.json.
func ReadProject(dir string) (*Project, error) {
	devDir := filepath.Join(dir, ".devcontainer")
	data, err := os.ReadFile(filepath.Join(devDir, "devcontainer.json"))
	if err != nil {
		return nil, err
	}
	var config struct {
		Build struct {
			Dockerfile string `json:"dockerfile"`
			Context    string `json:"context"`
		} `json:"build"`
		Customizations struct {
			Devenv map[string]ToolConfig `json:"devenv"`
		} `json:"customizations"`
	}
	if err := json.Unmarshal(devcontainer.StripJSONC(data), &config); err != nil {
		return nil, fmt.Errorf("devcontainer.json: %w", err)
	}
	if config.Build.Dockerfile == "" {
		return nil, fmt.Errorf("devcontainer.json: build.dockerfile is missing")
	}
	if config.Build.Context == "" {
		config.Build.Context = "."
	}
	return &Project{
		Dir:             dir,
		DevcontainerDir: devDir,
		Dockerfile:      filepath.Join(devDir, config.Build.Dockerfile),
		Context:         filepath.Clean(filepath.Join(devDir, config.Build.Context)),
		Tools:           config.Customizations.Devenv,
	}, nil
}

// MakePlan collects the inputs and decides whether to build and release.
func MakePlan(o PlanOptions) (*Plan, error) {
	project, err := ReadProject(o.Dir)
	if err != nil {
		return nil, err
	}
	dockerfile, err := ReadDockerfile(project.Dockerfile)
	if err != nil {
		return nil, err
	}
	current := inspectImage(o.Image + ":latest")
	previous := parseInputs(current.Labels[LabelInputs])

	p := &Plan{Inputs: map[string]string{}, BuildArgs: map[string]string{}, ToolVersion: map[string]string{}}
	add := func(key, value string) error {
		if value == "" {
			if old := previous[key]; old != "" {
				warning("Could not read %s; keeping the previous value %s", key, old)
				value = old
			} else {
				return fmt.Errorf("could not read %s and no previous value exists", key)
			}
		}
		p.Inputs[key] = value
		return nil
	}

	// 1. Configuration: the Git tree of the configuration paths
	config, err := configInput(o.Dir, o.ConfigPaths)
	if err != nil {
		return nil, err
	}
	if err := add("config", config); err != nil {
		return nil, err
	}
	// 2. Tools: the versions of the tools of the installed layers, chosen by
	// the general rule of layer.Tool.Resolve with the configuration of the
	// feature (Tool.Version) and the overrides of the image (devcontainer.json,
	// customizations.devenv); a pinned line at its end of life and an unknown
	// channel stop the release. The line and the channel are recorded as
	// inputs, so the release notes show them.
	configured := map[string]bool{}
	for _, name := range dockerfile.Layers {
		l, err := layer.Get(name)
		if err != nil {
			return nil, err
		}
		for i := range l.Tools {
			tool := &l.Tools[i]
			key := "tool/" + tool.Name
			choice := tool.Version
			if c, ok := project.Tools[tool.Name]; ok {
				choice = c.apply(choice)
				configured[tool.Name] = true
			}
			choice, err := tool.Effective(choice)
			if err != nil {
				return nil, fmt.Errorf("configuration of %s: %w", tool.Name, err)
			}
			if err := tool.CheckSupport(choice); err != nil {
				if eol := (*layer.EndOfLifeError)(nil); errors.As(err, &eol) {
					return nil, err
				}
				warning("Could not check the support of %s %s: %v", tool.Name, choice.Pin, err)
			}
			for _, v := range []struct{ kind, value string }{{"pin", choice.Pin}, {"channel", choice.Channel}} {
				if v.value != "" {
					if err := add(v.kind+"/"+tool.Name, v.value); err != nil {
						return nil, err
					}
				}
			}
			version, err := tool.Resolve(choice, p.ToolVersion[tool.Follows])
			if err != nil {
				warning("Could not read the version of %s: %v", tool.Name, err)
				version = ""
			}
			if err := add(key, version); err != nil {
				return nil, err
			}
			if choice.Pin != "" && !layer.InLine(p.Inputs[key], choice.Pin) {
				return nil, fmt.Errorf("no version of %s in the pinned line %s (found %s)", tool.Name, choice.Pin, p.Inputs[key])
			}
			p.BuildArgs[tool.Arg] = p.Inputs[key]
			p.ToolVersion[tool.Name] = p.Inputs[key]
		}
	}
	for name := range project.Tools {
		if !configured[name] {
			return nil, fmt.Errorf("devcontainer.json: customizations.devenv.%s: the Dockerfile installs no layer with the tool %s", name, name)
		}
	}
	// 3. Base images: the digest of each FROM image, with the build
	// arguments of the tools (for example debian:${DEBIAN_SERIES})
	if err := dockerfile.Expand(p.BuildArgs); err != nil {
		return nil, fmt.Errorf("%s: %w", project.Dockerfile, err)
	}
	for _, image := range dockerfile.BaseImages {
		if err := add("image/"+image, imageDigest(image)); err != nil {
			return nil, err
		}
	}

	// 4. Features: the newest version of devcontainer-features, when the
	// Dockerfile builds devenv with it (core)
	if contains(dockerfile.Args, FeaturesArg) {
		version, err := newestTag(FeaturesRepository)
		if err != nil {
			warning("Could not read the newest version of devcontainer-features: %v", err)
		}
		if err := add("tool/features", version); err != nil {
			return nil, err
		}
		p.BuildArgs[FeaturesArg] = p.Inputs["tool/features"]
	}
	// The Go that builds devenv (core): the newest release of the Go line in
	// go.mod (go directive, for example 1.27 → 1.27.1). It is not the Go of
	// the feature go. The end of life of the line stops the release.
	if contains(dockerfile.Args, ToolchainArg) {
		line, err := goModLine(filepath.Join(o.Dir, "go.mod"))
		if err != nil {
			return nil, err
		}
		toolchain := layer.Tool{Name: ToolchainName, Arg: ToolchainArg, Source: versions.GoReleases(), Version: layer.Config{Pin: line}}
		if err := toolchain.CheckSupport(toolchain.Version); err != nil {
			if eol := (*layer.EndOfLifeError)(nil); errors.As(err, &eol) {
				eol.Change = "the go line in go.mod"
				return nil, eol
			}
			warning("Could not check the support of Go %s: %v", line, err)
		}
		version, err := toolchain.Resolve(toolchain.Version, "")
		if err != nil {
			warning("Could not read the newest release of Go %s: %v", line, err)
			version = ""
		}
		if err := add("pin/"+ToolchainName, line); err != nil {
			return nil, err
		}
		if err := add("tool/"+ToolchainName, version); err != nil {
			return nil, err
		}
		p.BuildArgs[ToolchainArg] = p.Inputs["tool/"+ToolchainName]
	}

	// 5. Pending Debian updates of the newest image (not for pull requests
	// and tags, which build anyway)
	if o.Event != "pull_request" && !tagRef.MatchString(o.Ref) && !current.Created.IsZero() {
		p.Updates = ownUpdates(o.Image+":latest", dockerfile)
		// 6. Support of the installed layers (for example the Debian
		// release of the layer os): an end of life stops the release
		if err := checkSupport(o.Image+":latest", dockerfile); err != nil {
			return nil, fmt.Errorf("%s: %w", project.Dockerfile, err)
		}
	}

	if err := p.decide(o, current, previous); err != nil {
		return nil, err
	}
	return p, nil
}

var tagRef = regexp.MustCompile(`^refs/tags/v([0-9]+\.[0-9]+\.[0-9]+)$`)

func (p *Plan) decide(o PlanOptions, current imageInfo, previous map[string]string) error {
	p.Build = true
	p.Created = time.Now().UTC().Format(time.RFC3339)
	changes := changedInputs(previous, p.Inputs)
	ageDays := -1
	if !current.Created.IsZero() {
		ageDays = int(time.Since(current.Created).Hours() / 24)
	}

	switch {
	case o.Event == "pull_request":
		p.Version = "pr-" + o.PRNumber
		p.Reason = "Pull request build (not published)"
		return nil
	case tagRef.MatchString(o.Ref):
		p.Version = tagRef.FindStringSubmatch(o.Ref)[1]
		p.Release = true
		p.Reason = "Tag v" + p.Version
	default:
		switch {
		case o.Force:
			p.Reason = "Manual release"
		case len(previous) == 0:
			p.Reason = "First release with recorded inputs"
		case len(changes) > 0 && len(p.Updates) > 0:
			p.Reason = "Changed inputs: " + strings.Join(changes, " ") + "; " + updatesReason(p.Updates)
		case len(changes) > 0:
			p.Reason = "Changed inputs: " + strings.Join(changes, " ")
		case len(p.Updates) > 0:
			p.Reason = updatesReason(p.Updates)
		case ageDays >= o.MaxAgeDays:
			p.Reason = fmt.Sprintf("Refresh: the current image is %d days old (safety net)", ageDays)
		default:
			p.Build = false
			p.Reason = fmt.Sprintf("No input changed and the current image is %d days old; nothing to do", ageDays)
			return nil
		}
		p.Release = true
		last, err := lastVersion(o.Dir, o.Major)
		if err != nil {
			return err
		}
		p.Version = nextVersion(last, o.Major, bumpStep(o.Bump, previous, p.Inputs))
	}
	parts := strings.Split(p.Version, ".")
	p.Tags = []string{p.Version, parts[0] + "." + parts[1], parts[0]}
	if parts[0] == strconv.Itoa(o.Major) {
		p.Tags = append(p.Tags, "latest")
	}
	return nil
}

// bumpStep: patch for every automatic release; minor when the major version of
// Node.js or Deno or the minor version of Go 1.x changes.
func bumpStep(bump string, previous, inputs map[string]string) string {
	if bump != "" && bump != "auto" {
		return bump
	}
	for tool, fields := range map[string]int{"go": 2, "node": 1, "deno": 1} {
		old, new := previous["tool/"+tool], inputs["tool/"+tool]
		if old != "" && new != "" && versionPrefix(old, fields) != versionPrefix(new, fields) {
			return "minor"
		}
	}
	return "patch"
}

func versionPrefix(v string, fields int) string {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) > fields {
		parts = parts[:fields]
	}
	return strings.Join(parts, ".")
}

// nextVersion computes the next version after last ("" = no release yet).
func nextVersion(last string, major int, step string) string {
	if last == "" {
		return fmt.Sprintf("%d.0.0", major)
	}
	var x, y, z int
	fmt.Sscanf(last, "%d.%d.%d", &x, &y, &z)
	switch step {
	case "major":
		return fmt.Sprintf("%d.0.0", x+1)
	case "minor":
		return fmt.Sprintf("%d.%d.0", x, y+1)
	default:
		return fmt.Sprintf("%d.%d.%d", x, y, z+1)
	}
}

// The features library: the core Dockerfile builds devenv with the version
// in the build argument FEATURES_VERSION.
const (
	FeaturesRepository = "https://github.com/majikmate/devcontainer-features"
	FeaturesArg        = "FEATURES_VERSION"
)

// The Go that builds devenv: the core Dockerfile uses the build argument
// DEVENV_GO_VERSION (GOTOOLCHAIN=go<version>); the plan records it as the
// tool devenv-go.
const (
	ToolchainArg  = "DEVENV_GO_VERSION"
	ToolchainName = "devenv-go"
)

// goModLine returns the Go line of the go directive of a go.mod file, for
// example "1.27" for "go 1.27" or "go 1.27.0".
func goModLine(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	m := regexp.MustCompile(`(?m)^go\s+([0-9][0-9.]*)\s*$`).FindStringSubmatch(string(data))
	if m == nil {
		return "", fmt.Errorf("%s has no go directive", path)
	}
	return versions.GoLine(m[1]), nil
}

// newestTag returns the highest version tag of a Git repository, for example
// "v1.0.3".
func newestTag(url string) (string, error) {
	out, err := sys.Output("git", "ls-remote", "--tags", "--refs", url, "refs/tags/v*")
	if err != nil {
		return "", err
	}
	version := highestVersion(out)
	if version == "" {
		return "", fmt.Errorf("no version tag in %s", url)
	}
	return "v" + version, nil
}

// lastVersion returns the highest version tag vMAJOR.x.y of the repository.
func lastVersion(dir string, major int) (string, error) {
	out, err := sys.Output("git", "-C", dir, "ls-remote", "--tags", "--refs", "origin", fmt.Sprintf("refs/tags/v%d.*", major))
	if err != nil {
		return "", err
	}
	return highestVersion(out), nil
}

// highestVersion returns the highest version X.Y.Z of the tags vX.Y.Z in the
// output of "git ls-remote --tags".
func highestVersion(out string) string {
	re := regexp.MustCompile(`refs/tags/v([0-9]+\.[0-9]+\.[0-9]+)$`)
	var best []int
	bestText := ""
	for _, line := range strings.Split(out, "\n") {
		m := re.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		var v [3]int
		fmt.Sscanf(m[1], "%d.%d.%d", &v[0], &v[1], &v[2])
		if best == nil || v[0] > best[0] || (v[0] == best[0] && (v[1] > best[1] || (v[1] == best[1] && v[2] > best[2]))) {
			best = v[:]
			bestText = m[1]
		}
	}
	return bestText
}

// configInput returns the Git object id of the configuration paths (a tree for
// a folder, a blob for a file).
func configInput(dir string, paths []string) (string, error) {
	if len(paths) == 0 {
		paths = defaultConfigPaths
	}
	var trees []string
	for _, path := range paths {
		tree, err := sys.Output("git", "-C", dir, "rev-parse", "HEAD:"+path)
		if err != nil {
			return "", err
		}
		trees = append(trees, path+"="+tree)
	}
	if len(trees) == 1 {
		return strings.TrimPrefix(trees[0], paths[0]+"="), nil
	}
	sum := sha256.Sum256([]byte(strings.Join(trees, "\n")))
	return hex.EncodeToString(sum[:])[:40], nil
}

// Inputs returns the inputs as one line ("key=value key=value ...", sorted).
func (p *Plan) InputsLine() string {
	return formatInputs(p.Inputs)
}

// Fingerprint is the SHA-256 checksum of the inputs line.
func (p *Plan) Fingerprint() string {
	sum := sha256.Sum256([]byte(p.InputsLine()))
	return hex.EncodeToString(sum[:])
}

func formatInputs(inputs map[string]string) string {
	var lines []string
	for k, v := range inputs {
		lines = append(lines, k+"="+v)
	}
	sort.Strings(lines)
	return strings.Join(lines, " ")
}

func parseInputs(line string) map[string]string {
	result := map[string]string{}
	for _, field := range strings.Fields(line) {
		if k, v, ok := strings.Cut(field, "="); ok {
			result[k] = v
		}
	}
	return result
}

func changedInputs(previous, current map[string]string) []string {
	var changes []string
	for k, v := range current {
		if previous[k] != v {
			changes = append(changes, k+"="+v)
		}
	}
	sort.Strings(changes)
	return changes
}

// imageInfo is what the release tool reads from an image in the registry.
type imageInfo struct {
	Labels  map[string]string
	Created time.Time
}

var registryClient = registry.NewClient()

// inspectImage reads the labels and the creation time of an image from its
// registry. A missing image returns an empty result.
func inspectImage(ref string) imageInfo {
	img, err := registryClient.Inspect(ref)
	if err != nil {
		notice("No image %s found (%v).", ref, err)
		return imageInfo{Labels: map[string]string{}}
	}
	return imageInfo{Labels: img.Labels, Created: img.Created}
}

// imageDigest returns the first 16 hex digits of the SHA-256 checksum of the
// raw manifest of an image ("" if it cannot be read). A new image version
// changes its manifest and therefore this value.
func imageDigest(ref string) string {
	data, _, err := registryClient.Manifest(ref)
	if err != nil {
		warning("Could not read the manifest of %s: %v", ref, err)
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}
