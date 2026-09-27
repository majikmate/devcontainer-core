// Command devcon-release is the release tool of the Dev Container images. The
// shared workflow .github/workflows/devcontainer-image.yml runs it with
// "go run" (CGO_ENABLED=0):
//
//	devcon-release plan        collect the inputs, decide, compute the version
//	devcon-release build       build, label, test and push one architecture
//	devcon-release publish     create the multi-architecture image and the release
//	devcon-release upstream    run the Release workflows of the upstream images (chain build)
//	devcon-release keep-alive  keep the scheduled workflow enabled
//	devcon-release inspect     show the digest, labels and creation time of an image
//	devcon-release module-release  create the next version tag of a Go module (no image)
//	devcon-release prune       delete outdated versions of container packages (or whole packages)
//
// Defaults come from the environment of GitHub Actions (GITHUB_REPOSITORY,
// GITHUB_SHA, GITHUB_TOKEN, ...).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/majikmate/devcontainer-core/internal/registry"
	"github.com/majikmate/devcontainer-core/internal/release"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: devcon-release plan|build|publish|upstream|keep-alive|inspect|module-release|prune [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "plan":
		err = plan(os.Args[2:])
	case "build":
		err = build(os.Args[2:])
	case "publish":
		err = publish(os.Args[2:])
	case "upstream":
		err = upstream(os.Args[2:])
	case "keep-alive":
		err = keepAlive(os.Args[2:])
	case "inspect":
		err = inspect(os.Args[2:])
	case "module-release":
		err = moduleRelease(os.Args[2:])
	case "prune":
		err = prune(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Printf("::error::%v\n", err)
		os.Exit(1)
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func defaultImage() string {
	return "ghcr.io/" + strings.ToLower(os.Getenv("GITHUB_REPOSITORY"))
}

func plan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	dir := fs.String("dir", env("GITHUB_WORKSPACE", "."), "root of the image repository")
	image := fs.String("image", defaultImage(), "image name")
	configPaths := fs.String("config-paths", ".devcontainer", "paths whose content is an input, separated by spaces")
	major := fs.Int("major", 1, "major version")
	maxAge := fs.Int("max-age-days", 7, "rebuild when the newest image is older")
	force := fs.Bool("force", false, "release without a change")
	bump := fs.String("bump", "auto", "version step: auto, patch, minor, major")
	fs.Parse(args)

	p, err := release.MakePlan(release.PlanOptions{
		Dir: *dir, Image: *image, ConfigPaths: strings.Fields(*configPaths),
		Major: *major, MaxAgeDays: *maxAge, Force: *force, Bump: *bump,
		Event: os.Getenv("GITHUB_EVENT_NAME"), Ref: os.Getenv("GITHUB_REF"), PRNumber: os.Getenv("PR_NUMBER"),
	})
	if err != nil {
		return err
	}
	buildArgs, _ := json.Marshal(p.BuildArgs)
	tools, _ := json.Marshal(p.ToolVersion)
	outputs := map[string]string{
		"build": strconv.FormatBool(p.Build), "release": strconv.FormatBool(p.Release),
		"version": p.Version, "tags": strings.Join(p.Tags, " "), "reason": p.Reason,
		"created": p.Created, "inputs": p.InputsLine(), "fingerprint": p.Fingerprint(),
		"build-args": string(buildArgs), "tools": string(tools),
	}
	for name, value := range outputs {
		if err := release.SetOutput(name, value); err != nil {
			return err
		}
	}
	var lines []string
	for _, field := range strings.Fields(p.InputsLine()) {
		lines = append(lines, "- `"+field+"`")
	}
	release.Summary(fmt.Sprintf("### Decision\n\n- Build: %t, release: %t, version: %s\n- Tags: %s\n- Reason: %s\n\n### Inputs\n\n%s\n",
		p.Build, p.Release, p.Version, strings.Join(p.Tags, " "), p.Reason, strings.Join(lines, "\n")))
	fmt.Println(p.Reason)
	return nil
}

func build(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	o := release.BuildOptions{}
	fs.StringVar(&o.Dir, "dir", env("GITHUB_WORKSPACE", "."), "root of the image repository")
	fs.StringVar(&o.Repository, "repository", os.Getenv("GITHUB_REPOSITORY"), "owner/name")
	fs.StringVar(&o.ServerURL, "server-url", env("GITHUB_SERVER_URL", "https://github.com"), "GitHub server")
	fs.StringVar(&o.Revision, "revision", os.Getenv("GITHUB_SHA"), "commit")
	fs.StringVar(&o.Image, "image", defaultImage(), "image name")
	fs.StringVar(&o.Arch, "arch", "amd64", "architecture")
	fs.StringVar(&o.Version, "version", "", "version")
	fs.BoolVar(&o.Push, "push", false, "push the image")
	fs.StringVar(&o.Title, "title", "", "OCI title")
	fs.StringVar(&o.Description, "description", "", "OCI description")
	fs.StringVar(&o.Created, "created", "", "creation time")
	fs.StringVar(&o.Inputs, "inputs", "", "inputs line")
	fs.StringVar(&o.Fingerprint, "fingerprint", "", "fingerprint")
	fs.StringVar(&o.OutputFile, "output", "", "file for the test output")
	buildArgs := fs.String("build-args", "{}", "build arguments as JSON object")
	tools := fs.String("tools", "{}", "expected tool versions as JSON object")
	absent := fs.String("absent-commands", "", "commands that must not exist, separated by spaces")
	fs.Parse(args)
	if err := json.Unmarshal([]byte(*buildArgs), &o.BuildArgs); err != nil {
		return fmt.Errorf("--build-args: %w", err)
	}
	if err := json.Unmarshal([]byte(*tools), &o.ExpectedTools); err != nil {
		return fmt.Errorf("--tools: %w", err)
	}
	o.AbsentCommands = strings.Fields(*absent)
	return release.Build(o)
}

func publish(args []string) error {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	o := release.PublishOptions{}
	fs.StringVar(&o.Repository, "repository", os.Getenv("GITHUB_REPOSITORY"), "owner/name")
	fs.StringVar(&o.Revision, "revision", os.Getenv("GITHUB_SHA"), "commit")
	fs.StringVar(&o.Image, "image", defaultImage(), "image name")
	fs.StringVar(&o.Version, "version", "", "version")
	fs.StringVar(&o.Reason, "reason", "", "reason of the release")
	fs.StringVar(&o.Inputs, "inputs", "", "inputs line")
	fs.StringVar(&o.OutputsDir, "outputs", ".", "folder with the test outputs")
	tags := fs.String("tags", "", "tags, separated by spaces")
	arches := fs.String("arches", "amd64 arm64", "architectures, separated by spaces")
	fs.Parse(args)
	o.Tags, o.Arches = strings.Fields(*tags), strings.Fields(*arches)
	o.Token = os.Getenv("GITHUB_TOKEN")
	if abs, err := filepath.Abs(o.OutputsDir); err == nil {
		o.OutputsDir = abs
	}
	return release.Publish(o)
}

func upstream(args []string) error {
	fs := flag.NewFlagSet("upstream", flag.ExitOnError)
	o := release.UpstreamOptions{}
	fs.StringVar(&o.Owner, "owner", os.Getenv("GITHUB_REPOSITORY_OWNER"), "owner of the repositories")
	fs.StringVar(&o.Actor, "actor", "", "login of the GitHub App (name[bot])")
	repos := fs.String("repositories", "", "upstream repositories, separated by spaces or commas")
	timeout := fs.Duration("timeout", 2*time.Hour, "maximum waiting time per repository")
	fs.Parse(args)
	o.Repositories = strings.Fields(strings.ReplaceAll(*repos, ",", " "))
	o.Token = os.Getenv("UPSTREAM_TOKEN")
	o.Timeout = *timeout
	if o.Token == "" {
		return fmt.Errorf("UPSTREAM_TOKEN is empty (token of the GitHub App)")
	}
	return release.Upstream(o)
}

func keepAlive(args []string) error {
	fs := flag.NewFlagSet("keep-alive", flag.ExitOnError)
	workflow := fs.String("workflow", os.Getenv("GITHUB_WORKFLOW_REF"), "workflow reference (GITHUB_WORKFLOW_REF) or file name")
	fs.Parse(args)
	return release.KeepAlive(os.Getenv("GITHUB_REPOSITORY"), *workflow, os.Getenv("GITHUB_TOKEN"))
}

func moduleRelease(args []string) error {
	fs := flag.NewFlagSet("module-release", flag.ExitOnError)
	o := release.ModuleOptions{}
	fs.StringVar(&o.Dir, "dir", env("GITHUB_WORKSPACE", "."), "checkout of the repository, with history")
	fs.StringVar(&o.Repository, "repository", os.Getenv("GITHUB_REPOSITORY"), "owner/name")
	fs.StringVar(&o.Revision, "revision", os.Getenv("GITHUB_SHA"), "commit to release")
	fs.IntVar(&o.Major, "major", 1, "major version")
	fs.StringVar(&o.Bump, "bump", "patch", "version step: patch, minor, major")
	fs.Parse(args)
	o.Token = os.Getenv("GITHUB_TOKEN")
	return release.ModuleRelease(o)
}

func prune(args []string) error {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	o := release.PruneOptions{}
	fs.StringVar(&o.Org, "org", os.Getenv("GITHUB_REPOSITORY_OWNER"), "owner of the packages")
	packages := fs.String("packages", "", "container packages, separated by spaces (default: the package of the repository)")
	fs.IntVar(&o.Major, "major", 0, "current major version: versions of lower major lines are outdated (0: keep all)")
	fs.BoolVar(&o.DeletePackages, "delete-packages", false, "delete the whole packages")
	mode := fs.String("mode", "report", "report (list only) or apply (delete)")
	fs.Parse(args)
	switch *mode {
	case "report":
	case "apply":
		o.Apply = true
	default:
		return fmt.Errorf("--mode must be report or apply, not %q", *mode)
	}
	o.Packages = strings.Fields(*packages)
	if len(o.Packages) == 0 {
		_, name, _ := strings.Cut(strings.ToLower(os.Getenv("GITHUB_REPOSITORY")), "/")
		o.Packages = []string{name}
	}
	if o.Org == "" || o.Packages[0] == "" {
		return fmt.Errorf("prune: --org and --packages are needed outside of GitHub Actions")
	}
	o.Token = os.Getenv("GITHUB_TOKEN")
	return release.Prune(o)
}

func inspect(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: devcon-release inspect <image>")
	}
	client := registry.NewClient()
	manifest, mediaType, err := client.Manifest(args[0])
	if err != nil {
		return err
	}
	sum := sha256.Sum256(manifest)
	fmt.Printf("manifest: %s (sha256:%s)\n", mediaType, hex.EncodeToString(sum[:]))
	img, err := client.Inspect(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("created: %s\n", img.Created.Format(time.RFC3339))
	for _, name := range sortedLabelNames(img.Labels) {
		fmt.Printf("label %s=%s\n", name, img.Labels[name])
	}
	return nil
}

func sortedLabelNames(labels map[string]string) []string {
	var names []string
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
