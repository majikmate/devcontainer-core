// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Tests of the notice check with example files, and the check of this
// repository itself.

package notices

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const goHeader = "// SPDX-License-Identifier: MIT\n// © 2026 Hannes Stauss (scalarion@nimblescape.com)\n// Licensed under the MIT License. See LICENSE in the repository root for details.\n"
const hashHeader = "# SPDX-License-Identifier: MIT\n# © 2026 Hannes Stauss (scalarion@nimblescape.com)\n# Licensed under the MIT License. See LICENSE in the repository root for details.\n"

func TestCheck(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"LICENSE":                         "MIT License\n\n" + Attribution + "\n\nPermission is hereby granted ...\n",
		"ok.go":                           goHeader + "\npackage x\n",
		"build.go":                        "//go:build linux\n\n" + goHeader + "\npackage x\n",
		"go.mod":                          goHeader + "\nmodule x\n",
		"go.sum":                          "x v1 h1:abc\n",
		".github/workflows/ci.yml":        hashHeader + "\nname: CI\n",
		".devcontainer/Dockerfile":        "# check=skip=InvalidDefaultArgInFrom\n" + hashHeader + "\nFROM debian\n",
		".devcontainer/devcontainer.json": goHeader + "{}\n",
		"script.sh":                       "#!/bin/sh\n" + hashHeader + "echo\n",
		"README.md":                       "# X\n\n---\n\n" + Footer("LICENSE") + "\n",
		"docs/guide.md":                   "# G\n\n" + Footer("../LICENSE") + "\n\n",
		"missing.go":                      "package x\n",
		"late.yml":                        "name: x\n" + hashHeader,
		"docs/wrong-link.md":              "# W\n\n" + Footer("LICENSE") + "\n",
		"no-footer.md":                    "# N\n",
		"data.json":                       "{}\n",
	}
	var names []string
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	findings, err := Check(root, names)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range findings {
		got[f.Path] = true
	}
	want := []string{"missing.go", "late.yml", "docs/wrong-link.md", "no-footer.md"}
	for _, name := range want {
		if !got[name] {
			t.Errorf("no finding for %s", name)
		}
	}
	if len(findings) != len(want) {
		t.Errorf("findings = %v, want one for each of %v", findings, want)
	}
}

func TestLicense(t *testing.T) {
	root := t.TempDir()
	if f, _ := Check(root, nil); len(f) != 1 || !strings.Contains(f[0].Message, "missing") {
		t.Errorf("no LICENSE: %v", f)
	}
	_ = os.WriteFile(filepath.Join(root, "LICENSE"), []byte("MIT License\n\nCopyright (c) 2024 someone\n"), 0o644)
	if f, _ := Check(root, nil); len(f) != 1 {
		t.Errorf("LICENSE without the attribution: %v", f)
	}
}

// TestRepository checks the notices of this repository (skipped outside a
// git checkout).
func TestRepository(t *testing.T) {
	root := filepath.Join("..", "..")
	if err := exec.Command("git", "-C", root, "rev-parse", "--git-dir").Run(); err != nil {
		t.Skip("not a git checkout")
	}
	files, err := TrackedFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := Check(root, files)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		t.Error(f)
	}
}
