// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package release

import (
	"errors"
	"testing"
	"time"
)

func TestTagReason(t *testing.T) {
	cases := []struct {
		tags  []string
		major int
		want  bool
	}{
		{[]string{"1.1.9", "1.1"}, 2, true},
		{[]string{"1.0.0-amd64"}, 2, true},
		{[]string{"1", "1.1.9"}, 2, true},
		{[]string{"buildcache-amd64"}, 2, true},
		{[]string{"2.0.5", "2.0", "2", "latest"}, 2, false},
		{[]string{"2.0.1-arm64"}, 2, false},
		{[]string{"1.1.9", "latest"}, 2, false}, // latest still points to it
		{[]string{"1.1.9"}, 0, false},           // no major given: keep all lines
		{[]string{"pr-7-amd64"}, 2, false},
		{nil, 2, false}, // untagged: decided by the references
	}
	for _, c := range cases {
		if got := (pruneRules{Major: c.major}).tagReason(c.tags, releaseInfo{}) != ""; got != c.want {
			t.Errorf("tagReason(%v, %d) outdated = %v, want %v", c.tags, c.major, got, c.want)
		}
	}
}

func TestReleaseRules(t *testing.T) {
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	ago := func(days int) time.Time { return now.AddDate(0, 0, -days) }
	versions := []packageVersion{
		{ID: 1, Digest: "sha256:r5", Tags: []string{"2.0.5", "2.0", "2", "latest"}, Created: ago(1)},
		{ID: 2, Digest: "sha256:r5a", Tags: []string{"2.0.5-amd64"}, Created: ago(1)},
		{ID: 3, Digest: "sha256:r4", Tags: []string{"2.0.4"}, Created: ago(20)},
		{ID: 4, Digest: "sha256:r4a", Tags: []string{"2.0.4-amd64"}, Created: ago(20)},
		{ID: 5, Digest: "sha256:r1", Tags: []string{"2.0.1"}, Created: ago(120)},
		{ID: 6, Digest: "sha256:r1a", Tags: []string{"2.0.1-arm64"}, Created: ago(120)},
		{ID: 7, Digest: "sha256:r1x", Created: ago(120)}, // untagged part of 2.0.1
		{ID: 8, Digest: "sha256:r10", Tags: []string{"2.0.10"}, Created: ago(100)},
	}
	children := map[string][]string{"sha256:r5": {"sha256:r5a"}, "sha256:r4": {"sha256:r4a"}, "sha256:r1": {"sha256:r1a", "sha256:r1x"}}
	get := func(d string) ([]string, error) { return children[d], nil }
	ids := func(rules pruneRules) map[int64]bool {
		result := map[int64]bool{}
		for _, o := range outdatedVersions(versions, rules, get) {
			result[o.version.ID] = true
		}
		return result
	}

	// No age limit: all releases of the current major line are kept
	if got := ids(pruneRules{Major: 2, Now: now}); len(got) != 0 {
		t.Errorf("no age limit: %v, want none", got)
	}

	// 90 days: 2.0.1 (120 days) goes with its parts; 2.0.10 (100 days) is
	// the newest release (10 > 5 as a number) and stays
	got := ids(pruneRules{Major: 2, MaxAgeDays: 90, Now: now})
	if want := map[int64]bool{5: true, 6: true, 7: true}; !equalSets(got, want) {
		t.Errorf("90 days: %v, want %v", got, want)
	}

	// All but the newest (2.0.10): the version with the moving tags stays,
	// and so does its part 2.0.5-amd64 (a kept image refers to it)
	got = ids(pruneRules{Major: 2, AllButNewest: true, Now: now})
	if want := map[int64]bool{3: true, 4: true, 5: true, 6: true, 7: true}; !equalSets(got, want) {
		t.Errorf("all but newest: %v, want %v", got, want)
	}
}

func equalSets(a, b map[int64]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func TestOutdatedVersions(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }
	versions := []packageVersion{
		{ID: 1, Digest: "sha256:index2", Tags: []string{"2.0.5", "2", "latest"}, Created: day(20)},
		{ID: 2, Digest: "sha256:amd2", Tags: []string{"2.0.5-amd64"}, Created: day(20)},
		{ID: 3, Digest: "sha256:attest2", Created: day(20)}, // untagged, part of index2
		{ID: 4, Digest: "sha256:old", Created: day(10)},     // untagged, part of nothing
		{ID: 5, Digest: "sha256:index1", Tags: []string{"1.1.9", "1"}, Created: day(1)},
		{ID: 6, Digest: "sha256:attest1", Created: day(1)}, // untagged, part of index1 (deleted)
		{ID: 7, Digest: "sha256:cache", Tags: []string{"buildcache-amd64"}, Created: day(2)},
	}
	children := map[string][]string{"sha256:index2": {"sha256:amd2", "sha256:attest2"}, "sha256:index1": {"sha256:attest1"}}
	get := func(d string) ([]string, error) { return children[d], nil }

	var ids []int64
	for _, o := range outdatedVersions(versions, pruneRules{Major: 2}, get) {
		ids = append(ids, o.version.ID)
	}
	// tagged first (oldest first), then untagged (oldest first)
	want := []int64{5, 7, 6, 4}
	if len(ids) != len(want) {
		t.Fatalf("outdated = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("outdated = %v, want %v", ids, want)
		}
	}

	// References cannot be read: no untagged version is deleted
	fail := func(string) ([]string, error) { return nil, errors.New("registry down") }
	for _, o := range outdatedVersions(versions, pruneRules{Major: 2}, fail) {
		if len(o.version.Tags) == 0 {
			t.Errorf("untagged version %d deleted although the references are unknown", o.version.ID)
		}
	}
}
