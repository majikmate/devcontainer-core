package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
)

// Labels in which an image records the inputs of its build.
const (
	LabelInputs      = "devcon.inputs"
	LabelFingerprint = "devcon.fingerprint"
)

// PlanOptions are the settings of a release plan.
type PlanOptions struct {
	Dir         string   // root of the image repository
	Image       string   // for example ghcr.io/majikmate/devcontainer-base
	ConfigPaths []string // paths whose content is an input (default .devcontainer)
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
	Security    []debian.Update   // pending Debian security updates of this image
}

// Project describes the image of a repository (from its devcontainer.json).
type Project struct {
	Dir             string
	DevcontainerDir string
	Dockerfile      string
	Context         string
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
	// 2. Base images: the digest of each FROM image
	for _, image := range dockerfile.BaseImages {
		if err := add("image/"+image, imageDigest(image)); err != nil {
			return nil, err
		}
	}
	// 3. Tools: the newest versions of the tools of the installed layers
	for _, name := range dockerfile.Layers {
		l, err := layer.Get(name)
		if err != nil {
			return nil, err
		}
		for _, tool := range l.Tools {
			version, err := tool.Newest()
			if err != nil {
				warning("Could not read the newest version of %s: %v", tool.Name, err)
				version = ""
			}
			if err := add("tool/"+tool.Name, version); err != nil {
				return nil, err
			}
			p.BuildArgs[tool.Arg] = p.Inputs["tool/"+tool.Name]
			p.ToolVersion[tool.Name] = p.Inputs["tool/"+tool.Name]
		}
	}

	// 4. Features: the newest version of devcontainer-features, when the
	// Dockerfile builds devcon with it (core)
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

	// 5. Debian security updates of the newest image (not for pull requests
	// and tags, which build anyway)
	if o.Event != "pull_request" && !tagRef.MatchString(o.Ref) && !current.Created.IsZero() {
		p.Security = ownSecurityUpdates(o.Image+":latest", dockerfile)
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
		case len(changes) > 0 && len(p.Security) > 0:
			p.Reason = "Changed inputs: " + strings.Join(changes, " ") + "; " + securityReason(p.Security)
		case len(changes) > 0:
			p.Reason = "Changed inputs: " + strings.Join(changes, " ")
		case len(p.Security) > 0:
			p.Reason = securityReason(p.Security)
		case ageDays >= o.MaxAgeDays:
			p.Reason = fmt.Sprintf("Refresh: the current image is %d days old (operating system updates)", ageDays)
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

// The features library: the core Dockerfile builds devcon with the version
// in the build argument FEATURES_VERSION.
const (
	FeaturesRepository = "https://github.com/majikmate/devcontainer-features"
	FeaturesArg        = "FEATURES_VERSION"
)

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

// configInput returns the Git tree id of the configuration paths.
func configInput(dir string, paths []string) (string, error) {
	if len(paths) == 0 {
		paths = []string{".devcontainer"}
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
