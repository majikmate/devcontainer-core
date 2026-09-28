// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package versions

import (
	"reflect"
	"testing"

	"github.com/majikmate/devcontainer-core/pkg/layer"
)

func versionsOf(releases []layer.Release) []string {
	var list []string
	for _, r := range releases {
		list = append(list, r.Version)
	}
	return list
}

func TestParseGitHubReleases(t *testing.T) {
	data := `[
	 {"tag_name": "v2.10.0", "prerelease": true},
	 {"tag_name": "v2.9.1"},
	 {"tag_name": "v2.10.1", "draft": true},
	 {"tag_name": "v2.9.10"},
	 {"tag_name": "nightly"}
	]`
	releases, err := parseGitHubReleases([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := versionsOf(releases), []string{"v2.9.10", "v2.9.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("releases = %v, want %v", got, want)
	}
}

func TestParseNPMPackage(t *testing.T) {
	data := `{"dist-tags": {"latest": "3.9.9", "next": "4.0.0-alpha.13"},
	 "versions": {"3.9.9": {}, "3.10.0": {}, "3.9.10": {}, "2.8.8": {}, "4.0.0-alpha.13": {}}}`
	releases, err := parseNPMPackage([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	// Not above latest (3.10.0 and 3.9.10 are not released as latest), no pre-releases
	if got, want := versionsOf(releases), []string{"3.9.9", "2.8.8"}; !reflect.DeepEqual(got, want) {
		t.Errorf("releases = %v, want %v", got, want)
	}
	if _, err := parseNPMPackage([]byte(`{"versions": {}}`)); err == nil {
		t.Error("no dist-tag latest: no error")
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.28.0", "1.27.10", 1},
		{"1.27", "1.27.10", -1},
		{"1.27.0", "1.27", 0},
		{"go1.27.3", "v1.27.3", 0},
		{"v0.9.10", "v0.19.0", -1},
		{"4.0.0-alpha", "3.9.9", 1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
