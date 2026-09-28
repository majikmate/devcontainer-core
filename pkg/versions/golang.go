// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package versions

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// GoPolicy is the release policy of Go: Go supports the two newest major
// releases.
const GoPolicy = "https://go.dev/doc/devel/release#policy"

const goReleasesURL = "https://go.dev/dl/?mode=json&include=all"

// GoReleases lists the stable Go releases from go.dev, newest first, without
// the prefix "go" (for example "1.27.1"). A release line is a Go major release
// 1.N (for example 1.27); it ends when Go 1.(N+2) is released.
func GoReleases() *layer.Source {
	return &layer.Source{
		Name:   "Go releases (" + goReleasesURL + ")",
		Policy: "a line is a Go major release 1.N; it ends when Go 1.(N+2) is released (Go supports the two newest major releases, " + GoPolicy + ")",
		Releases: func() ([]layer.Release, error) {
			releases, err := readGoReleases()
			if err != nil {
				return nil, err
			}
			return newestFirst(releases), nil
		},
		Support: func(line string) error {
			releases, err := readGoReleases()
			if err != nil {
				return err
			}
			return GoSupport(releases, line)
		},
	}
}

func readGoReleases() ([]string, error) {
	data, err := sys.Get(goReleasesURL)
	if err != nil {
		return nil, err
	}
	return ParseGoReleases(data)
}

// ParseGoReleases returns the stable releases of the go.dev release list,
// without the prefix "go".
func ParseGoReleases(data []byte) ([]string, error) {
	var list []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	var releases []string
	for _, r := range list {
		if r.Stable {
			releases = append(releases, strings.TrimPrefix(r.Version, "go"))
		}
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no stable Go release on go.dev")
	}
	return releases, nil
}

var goLine = regexp.MustCompile(`^1\.([0-9]+)$`)

// goMinor returns N of a Go line "1.N" (-1 when the line is not valid).
func goMinor(line string) int {
	m := goLine.FindStringSubmatch(line)
	if m == nil {
		return -1
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// GoLine returns the line 1.N of a Go version, for example "1.27" for
// "1.27.1", "1.27.0" or "1.27".
func GoLine(version string) string {
	parts := strings.SplitN(strings.TrimPrefix(version, "go"), ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

// GoSupport returns an *layer.EndOfLifeError when Go 1.(N+2) is released:
// Go supports the two newest major releases.
func GoSupport(releases []string, line string) error {
	n := goMinor(line)
	if n < 0 {
		return fmt.Errorf("Go line %s: a Go line has the form 1.N, for example 1.27", line)
	}
	newest := -1
	for _, r := range releases {
		if m := goMinor(GoLine(r)); m > newest {
			newest = m
		}
	}
	switch {
	case n > newest:
		return fmt.Errorf("Go %s is not released yet (newest: 1.%d)", line, newest)
	case n >= newest-1:
		return nil
	}
	return &layer.EndOfLifeError{
		Since:     fmt.Sprintf("Go 1.%d.0 was released; Go supports the two newest major releases", n+2),
		Source:    GoPolicy,
		Supported: []string{fmt.Sprintf("1.%d", newest-1), fmt.Sprintf("1.%d", newest)},
	}
}
