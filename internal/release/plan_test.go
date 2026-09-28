// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// TestProjectTools: the overrides of customizations.devcon in the
// devcontainer.json of an image.
func TestProjectTools(t *testing.T) {
	dir := t.TempDir()
	devDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{
  // comment
  "build": {"dockerfile": "Dockerfile"},
  "customizations": {
    "vscode": {"extensions": ["a.b"]},
    "devcon": {"deno": {"channel": "stable"}, "prettier": {"pin": "3"}, "go": {"pin": ""}}
  }
}`
	if err := os.WriteFile(filepath.Join(devDir, "devcontainer.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err := ReadProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	feature := layer.Config{Pin: "2", Channel: "lts"}
	if got := project.Tools["deno"].apply(feature); got != (layer.Config{Pin: "2", Channel: "stable"}) {
		t.Errorf("deno = %+v", got)
	}
	if got := project.Tools["prettier"].apply(layer.Config{}); got != (layer.Config{Pin: "3"}) {
		t.Errorf("prettier = %+v", got)
	}
	// "pin": "" removes the pin of the feature
	if got := project.Tools["go"].apply(layer.Config{Pin: "1.27"}); got != (layer.Config{}) {
		t.Errorf("go = %+v", got)
	}
	// A tool without an override keeps the configuration of the feature
	if got := project.Tools["node"].apply(feature); got != feature {
		t.Errorf("node = %+v", got)
	}
}

func TestNextVersion(t *testing.T) {
	cases := []struct{ last, step, want string }{
		{"", "patch", "2.0.0"},
		{"2.0.3", "patch", "2.0.4"},
		{"2.0.3", "minor", "2.1.0"},
		{"2.4.3", "major", "3.0.0"},
	}
	for _, c := range cases {
		if got := nextVersion(c.last, 2, c.step); got != c.want {
			t.Errorf("nextVersion(%q, %s) = %s, want %s", c.last, c.step, got, c.want)
		}
	}
}

func TestBumpStep(t *testing.T) {
	old := map[string]string{"tool/go": "1.27.1", "tool/node": "v24.21.0", "tool/deno": "v2.9.3"}
	cases := []struct {
		name    string
		changed map[string]string
		want    string
	}{
		{"go patch", map[string]string{"tool/go": "1.27.2"}, "patch"},
		{"go minor", map[string]string{"tool/go": "1.28"}, "minor"},
		{"node major", map[string]string{"tool/node": "v26.0.0"}, "minor"},
		{"node minor", map[string]string{"tool/node": "v24.22.0"}, "patch"},
		{"deno major", map[string]string{"tool/deno": "v3.0.0"}, "minor"},
	}
	for _, c := range cases {
		inputs := map[string]string{}
		for k, v := range old {
			inputs[k] = v
		}
		for k, v := range c.changed {
			inputs[k] = v
		}
		if got := bumpStep("auto", old, inputs); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	if got := bumpStep("major", old, old); got != "major" {
		t.Errorf("explicit bump: got %s", got)
	}
}

func TestInputsRoundTrip(t *testing.T) {
	in := map[string]string{"config": "abc", "tool/go": "1.27.1", "image/x:1": "0123"}
	line := formatInputs(in)
	if line != "config=abc image/x:1=0123 tool/go=1.27.1" {
		t.Errorf("line = %q", line)
	}
	out := parseInputs(line)
	if len(out) != 3 || out["tool/go"] != "1.27.1" {
		t.Errorf("parsed = %v", out)
	}
	if ch := changedInputs(in, map[string]string{"config": "abc", "tool/go": "1.28", "image/x:1": "0123"}); len(ch) != 1 || ch[0] != "tool/go=1.28" {
		t.Errorf("changes = %v", ch)
	}
}

func TestHighestVersion(t *testing.T) {
	out := "a1\trefs/tags/v1.0.9\nb2\trefs/tags/v1.0.10\nc3\trefs/tags/feature_git_1.0.0\nd4\trefs/tags/v1.1.0-rc1\n"
	if got := highestVersion(out); got != "1.0.10" {
		t.Errorf("highestVersion = %q, want 1.0.10", got)
	}
	if got := highestVersion(""); got != "" {
		t.Errorf("highestVersion of no tags = %q", got)
	}
}

// TestConfigInput checks that a change of the README (a file, not a folder)
// changes the configuration input, and that the default paths include it.
func TestConfigInput(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if _, err := sys.Output("git", append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.com"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write(".devcontainer/Dockerfile", "FROM scratch\n")
	write("README.md", "# Image\n")
	git("add", ".")
	git("commit", "-q", "-m", "first")
	first, err := configInput(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	write("README.md", "# Image\n\nMore text.\n")
	git("commit", "-q", "-a", "-m", "README")
	second, err := configInput(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("a README change does not change the configuration input")
	}
	only, err := configInput(dir, []string{".devcontainer"})
	if err != nil {
		t.Fatal(err)
	}
	if only == second {
		t.Error("the default configuration input does not include README.md")
	}
}
