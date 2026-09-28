// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package notices checks the license notices of a repository:
//
//   - the root LICENSE file is the MIT License with the project attribution,
//   - every authored file that can carry comments begins with the three-line
//     project header in the comment syntax of its language (after a shebang
//     line, Dockerfile parser directives or a Go build constraint),
//   - every authored Markdown document ends with one footer whose link
//     resolves to the root LICENSE file.
//
// The check reads the files that git tracks, so an untracked checkout (for
// example the tooling in .devcon) is not checked. Files that cannot carry
// comments (go.sum, strict JSON, binaries) need no notice: the root LICENSE
// covers them. The release tool runs the check ("devcon-release notices") in
// every image repository and in devcontainer-features.
package notices

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Attribution is the complete attribution line of the project.
const Attribution = "© 2026 Hannes Stauss (scalarion@nimblescape.com)"

// HeaderLines are the three lines of the project header, without comment
// markers.
var HeaderLines = [3]string{
	"SPDX-License-Identifier: MIT",
	Attribution,
	"Licensed under the MIT License. See LICENSE in the repository root for details.",
}

// Footer returns the Markdown footer of a document with a link to the root
// LICENSE file, for example "../LICENSE" for a document in docs/.
func Footer(licenseLink string) string {
	return Attribution + " · [MIT License](" + licenseLink + ")."
}

// footerPattern matches the Markdown footer and captures its link.
var footerPattern = regexp.MustCompile(`^© 2026 Hannes Stauss \(scalarion@nimblescape\.com\) · \[MIT License\]\(([^)]+)\)\.$`)

// The comment marker of the header per file extension and per file name.
var (
	markerByExt  = map[string]string{".go": "//", ".mod": "//", ".yml": "#", ".yaml": "#", ".sh": "#"}
	markerByName = map[string]string{
		"Dockerfile": "#", ".dockerignore": "#", ".gitignore": "#",
		// devcontainer.json is JSON with comments
		"devcontainer.json": "//",
	}
)

// Marker returns the comment marker of the header of a file, or "" when the
// file carries no header (Markdown carries a footer, other files no notice).
func Marker(rel string) string {
	if m, ok := markerByName[path.Base(rel)]; ok {
		return m
	}
	return markerByExt[path.Ext(rel)]
}

// Finding is one missing or wrong notice.
type Finding struct {
	Path    string // relative to the repository root, with slashes
	Line    int
	Message string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Message)
}

// TrackedFiles returns the files that git tracks in the repository at root,
// relative to root and with slashes.
func TrackedFiles(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files in %s: %w", root, err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	return files, nil
}

// Check checks the notices of the files (relative to root, with slashes)
// and the root LICENSE file.
func Check(root string, files []string) ([]Finding, error) {
	var findings []Finding
	license, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	switch {
	case err != nil:
		findings = append(findings, Finding{"LICENSE", 1, "the root LICENSE file is missing"})
	case !strings.HasPrefix(string(license), "MIT License\n") || !strings.Contains(string(license), Attribution):
		findings = append(findings, Finding{"LICENSE", 1, "the root LICENSE file must be the MIT License with the line " + Attribution})
	}
	for _, rel := range files {
		marker := Marker(rel)
		isMarkdown := path.Ext(rel) == ".md"
		if marker == "" && !isMarkdown {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		if isMarkdown {
			findings = append(findings, checkFooter(rel, lines)...)
		} else {
			findings = append(findings, checkHeader(rel, marker, lines)...)
		}
	}
	return findings, nil
}

// dockerDirective matches a Dockerfile parser directive, which must stay on
// the first lines of the file.
var dockerDirective = regexp.MustCompile(`^#\s*(syntax|escape|check)\s*=`)

// checkHeader checks that the header follows the allowed first lines.
func checkHeader(rel, marker string, lines []string) []Finding {
	i := 0
	switch {
	case len(lines) > 0 && strings.HasPrefix(lines[0], "#!"):
		i = 1
	case path.Base(rel) == "Dockerfile":
		for i < len(lines) && dockerDirective.MatchString(lines[i]) {
			i++
		}
	case strings.HasSuffix(rel, ".go") && len(lines) > 1 && strings.HasPrefix(lines[0], "//go:build "):
		i = 2 // the build constraint and the blank line that Go requires
	}
	for n, want := range HeaderLines {
		got := ""
		if i+n < len(lines) {
			got = strings.TrimSpace(lines[i+n])
		}
		if !strings.HasPrefix(got, marker) || strings.TrimSpace(strings.TrimPrefix(got, marker)) != want {
			return []Finding{{rel, i + n + 1, fmt.Sprintf("the license header must start here: %s %s", marker, want)}}
		}
	}
	return nil
}

// checkFooter checks the last non-empty line of a Markdown document.
func checkFooter(rel string, lines []string) []Finding {
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	link := path.Join(strings.Repeat("../", strings.Count(rel, "/")), "LICENSE")
	if last < 0 {
		return []Finding{{rel, 1, "the document must end with the footer: " + Footer(link)}}
	}
	m := footerPattern.FindStringSubmatch(strings.TrimSpace(lines[last]))
	if m == nil {
		return []Finding{{rel, last + 1, "the document must end with the footer: " + Footer(link)}}
	}
	if path.Clean(path.Join(path.Dir(rel), m[1])) != "LICENSE" {
		return []Finding{{rel, last + 1, fmt.Sprintf("the footer link %s does not resolve to the root LICENSE file (use %s)", m[1], link)}}
	}
	return nil
}
