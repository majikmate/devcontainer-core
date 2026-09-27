package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/devcontainer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// BuildOptions are the settings of an image build for one architecture.
type BuildOptions struct {
	Dir            string            // root of the image repository
	Repository     string            // owner/name
	ServerURL      string            // https://github.com
	Revision       string            // commit
	Image          string            // for example ghcr.io/owner/name
	Arch           string            // amd64 or arm64
	Version        string            // version of the build
	Push           bool              // push the image (releases only)
	Title          string            // OCI title
	Description    string            // OCI description
	Created        string            // OCI creation time
	BuildArgs      map[string]string // tool versions
	ExpectedTools  map[string]string // tool -> expected version
	Inputs         string            // inputs line
	Fingerprint    string            // fingerprint of the inputs
	AbsentCommands []string          // commands that must not exist in the image
	OutputFile     string            // file for the test output
}

// Build builds, labels, tests and (for releases) pushes the image.
func Build(o BuildOptions) error {
	project, err := ReadProject(o.Dir)
	if err != nil {
		return err
	}
	platform := "linux/" + o.Arch
	stage := o.Image + ":build-" + o.Arch
	final := o.Image + ":" + o.Version + "-" + o.Arch

	// 1. Build without cache, so the image contains the newest packages
	args := []string{"buildx", "build", "--load", "--pull", "--no-cache", "--platform", platform,
		"--file", project.Dockerfile, "--tag", stage}
	for _, name := range sortedKeys(o.BuildArgs) {
		args = append(args, "--build-arg", name+"="+o.BuildArgs[name])
	}
	args = append(args, project.Context)
	if err := sys.Run(nil, "docker", args...); err != nil {
		return err
	}

	// 2. The devcontainer.metadata label: entries of the base image, of the
	// installed layers and of this image's devcontainer.json
	label, err := metadataLabel(project, stage, o.Repository)
	if err != nil {
		return err
	}

	// 3. Labels are set in a second step without file changes
	context, err := os.MkdirTemp("", "devenv-label-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(context)
	if err := os.WriteFile(filepath.Join(context, "Dockerfile"), []byte("FROM "+stage+"\n"), 0o644); err != nil {
		return err
	}
	url := o.ServerURL + "/" + o.Repository
	labels := map[string]string{
		devcontainer.Label:                       label,
		"org.opencontainers.image.title":         o.Title,
		"org.opencontainers.image.description":   o.Description,
		"org.opencontainers.image.version":       o.Version,
		"org.opencontainers.image.revision":      o.Revision,
		"org.opencontainers.image.source":        url,
		"org.opencontainers.image.url":           url,
		"org.opencontainers.image.documentation": url + "#readme",
		"org.opencontainers.image.created":       o.Created,
		"org.opencontainers.image.licenses":      "MIT",
		LabelInputs:                              o.Inputs,
		LabelFingerprint:                         o.Fingerprint,
	}
	args = []string{"buildx", "build", "--load", "--platform", platform, "--tag", final}
	for _, name := range sortedKeys(labels) {
		args = append(args, "--label", name+"="+labels[name])
	}
	args = append(args, context)
	if err := sys.Run(nil, "docker", args...); err != nil {
		return err
	}

	// 4. Tests inside the image, as the development user
	testErr := testImage(final, label, o)

	// 5. Push (releases only, and only a tested image)
	if testErr != nil {
		return testErr
	}
	if o.Push {
		return sys.Run(nil, "docker", "push", final)
	}
	return nil
}

// metadataLabel creates the value of the devcontainer.metadata label.
func metadataLabel(project *Project, image, repository string) (string, error) {
	inherited, err := sys.Output("docker", "image", "inspect", "--format", `{{ index .Config.Labels "`+devcontainer.Label+`" }}`, image)
	if err != nil {
		return "", err
	}
	if inherited == "<no value>" {
		inherited = ""
	}
	base, err := devcontainer.Parse(inherited)
	if err != nil {
		return "", fmt.Errorf("label of the base image: %w", err)
	}
	out, err := sys.Output("docker", "run", "--rm", "--entrypoint", "devenv", image, "metadata")
	if err != nil {
		return "", fmt.Errorf("devenv metadata: %w", err)
	}
	var layers []devcontainer.Entry
	if err := json.Unmarshal([]byte(out), &layers); err != nil {
		return "", fmt.Errorf("devenv metadata: %w", err)
	}
	config, err := os.ReadFile(filepath.Join(project.DevcontainerDir, "devcontainer.json"))
	if err != nil {
		return "", err
	}
	name := repository[strings.LastIndex(repository, "/")+1:]
	own, err := devcontainer.ImageEntry("devenv/image/"+name, config)
	if err != nil {
		return "", err
	}
	return devcontainer.Encode(devcontainer.Merge(base, layers, own))
}

// testImage runs the layer tests and the image checks, writes the output to
// o.OutputFile and compares the installed versions with the expected ones.
func testImage(image, label string, o BuildOptions) error {
	entries, err := devcontainer.Parse(label)
	if err != nil {
		return fmt.Errorf("invalid devcontainer.metadata label: %w", err)
	}
	user := ""
	for _, e := range entries {
		if u, ok := e["remoteUser"].(string); ok {
			user = u
		}
	}
	if user == "" {
		return fmt.Errorf("the devcontainer.metadata label has no remoteUser")
	}

	var output bytes.Buffer
	cmd := exec.Command("docker", "run", "--rm", "--user", user, "--entrypoint", "devenv", image, "test")
	cmd.Stdout = &output
	cmd.Stderr = &output
	testErr := cmd.Run()
	for _, absent := range o.AbsentCommands {
		found := exec.Command("docker", "run", "--rm", "--entrypoint", "which", image, absent).Run() == nil
		if found {
			fmt.Fprintf(&output, "  FAIL  command %s must not exist in this image\n", absent)
			testErr = fmt.Errorf("command %s exists in the image", absent)
		} else {
			fmt.Fprintf(&output, "  ok    command %s does not exist\n", absent)
		}
	}
	fmt.Print(output.String())
	if o.OutputFile != "" {
		if err := os.WriteFile(o.OutputFile, output.Bytes(), 0o644); err != nil {
			return err
		}
	}
	Summary(fmt.Sprintf("### Test output (%s)\n\n```\n%s```\n", o.Arch, output.String()))
	if testErr != nil {
		return fmt.Errorf("tests of %s failed: %w", image, testErr)
	}

	installed := InstalledVersions(output.String())
	for _, tool := range sortedKeys(o.ExpectedTools) {
		want := strings.TrimPrefix(o.ExpectedTools[tool], "v")
		got := strings.TrimPrefix(installed[tool], "v")
		if got != "" && got != want {
			warning("%s: installed %s, expected %s", tool, got, want)
		}
	}
	return nil
}

var versionLine = regexp.MustCompile(`(?m)^version: ([A-Za-z0-9._-]+)=(.*)$`)

// InstalledVersions reads the "version: tool=version" lines of a test output.
func InstalledVersions(output string) map[string]string {
	result := map[string]string{}
	for _, m := range versionLine.FindAllStringSubmatch(output, -1) {
		result[m[1]] = strings.TrimSpace(m[2])
	}
	return result
}

func sortedKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
