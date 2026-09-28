// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package versions

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// The general version sources: a tool that installs from one of these places
// declares it as its layer.Source, for example
//
//	{Name: "prettier", Arg: "PRETTIER_VERSION", Source: versions.NPMPackage("prettier")}

// GitHubRepository lists the releases of a GitHub repository (tags vX.Y.Z or
// X.Y.Z, without drafts and pre-releases). It uses GITHUB_TOKEN if set
// (higher rate limit).
func GitHubRepository(repo string) *layer.Source {
	return &layer.Source{
		Name: "GitHub releases (https://github.com/" + repo + "/releases)",
		Releases: func() ([]layer.Release, error) {
			data, err := sys.GetWithToken("https://api.github.com/repos/"+repo+"/releases?per_page=100", os.Getenv("GITHUB_TOKEN"))
			if err != nil {
				return nil, err
			}
			return parseGitHubReleases(data)
		},
	}
}

func parseGitHubReleases(data []byte) ([]layer.Release, error) {
	var releases []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, err
	}
	var tags []string
	for _, r := range releases {
		if !r.Draft && !r.Prerelease {
			tags = append(tags, r.TagName)
		}
	}
	return newestFirst(tags), nil
}

// NPMPackage lists the releases of an npm package up to its dist-tag latest
// (without pre-releases and without releases of other dist-tags such as next).
func NPMPackage(pkg string) *layer.Source {
	return &layer.Source{
		Name: "npm registry (https://www.npmjs.com/package/" + pkg + ")",
		Releases: func() ([]layer.Release, error) {
			data, err := sys.Get("https://registry.npmjs.org/" + pkg)
			if err != nil {
				return nil, err
			}
			return parseNPMPackage(data)
		},
	}
}

func parseNPMPackage(data []byte) ([]layer.Release, error) {
	var doc struct {
		DistTags map[string]string          `json:"dist-tags"`
		Versions map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	latest := doc.DistTags["latest"]
	if latest == "" {
		return nil, fmt.Errorf("the npm package has no dist-tag latest")
	}
	var versions []string
	for v := range doc.Versions {
		if Compare(v, latest) <= 0 {
			versions = append(versions, v)
		}
	}
	return newestFirst(versions), nil
}

// GoModuleProxy lists the releases of a Go module from the Go module proxy
// (without pre-releases).
func GoModuleProxy(module string) *layer.Source {
	return &layer.Source{
		Name: "Go module proxy (" + module + ")",
		Releases: func() ([]layer.Release, error) {
			data, err := sys.Get("https://proxy.golang.org/" + strings.ToLower(module) + "/@v/list")
			if err != nil {
				return nil, err
			}
			return newestFirst(strings.Fields(string(data))), nil
		},
	}
}

var releaseVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// newestFirst returns the release versions (vX.Y.Z or X.Y.Z) of the list as
// releases, highest version first; other versions (pre-releases) are left
// out.
func newestFirst(list []string) []layer.Release {
	var versions []string
	for _, v := range list {
		if releaseVersion.MatchString(v) {
			versions = append(versions, v)
		}
	}
	sort.SliceStable(versions, func(i, j int) bool { return Compare(versions[i], versions[j]) > 0 })
	releases := make([]layer.Release, len(versions))
	for i, v := range versions {
		releases[i] = layer.Release{Version: v}
	}
	return releases
}

// Compare compares two versions X.Y.Z number by number (a prefix "v" or "go"
// does not count): -1 if a < b, 0 if equal, +1 if a > b. Missing numbers are
// 0; a part that is not a number (a pre-release suffix) counts as lower.
func Compare(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(strings.TrimPrefix(a, "go"), "v"), ".")
	pb := strings.Split(strings.TrimPrefix(strings.TrimPrefix(b, "go"), "v"), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		x, y := number(pa, i), number(pb, i)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

func number(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return -1
	}
	return n
}
