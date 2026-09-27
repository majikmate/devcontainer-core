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
		if got := tagReason(c.tags, c.major) != ""; got != c.want {
			t.Errorf("tagReason(%v, %d) outdated = %v, want %v", c.tags, c.major, got, c.want)
		}
	}
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
	for _, o := range outdatedVersions(versions, 2, get) {
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
	for _, o := range outdatedVersions(versions, 2, fail) {
		if len(o.version.Tags) == 0 {
			t.Errorf("untagged version %d deleted although the references are unknown", o.version.ID)
		}
	}
}
