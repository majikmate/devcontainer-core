// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package versions

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/majikmate/devcontainer-core/pkg/layer"
)

// Excerpt of https://go.dev/dl/?mode=json&include=all
const goReleases = `[
 {"version": "go1.29rc1", "stable": false},
 {"version": "go1.28.2", "stable": true},
 {"version": "go1.28.0", "stable": true},
 {"version": "go1.27.10", "stable": true},
 {"version": "go1.27.9", "stable": true},
 {"version": "go1.27.0", "stable": true},
 {"version": "go1.26.12", "stable": true}
]`

func TestGoReleases(t *testing.T) {
	releases, err := ParseGoReleases([]byte(goReleases))
	if err != nil {
		t.Fatal(err)
	}
	source := &layer.Source{Name: "go", Releases: func() ([]layer.Release, error) { return newestFirst(releases), nil }}
	tool := layer.Tool{Name: "go", Source: source}
	for line, want := range map[string]string{"1.27": "1.27.10", "1.28": "1.28.2", "": "1.28.2"} {
		if v, err := tool.Resolve(layer.Config{Pin: line}, ""); err != nil || v != want {
			t.Errorf("line %q: %q, %v; want %s", line, v, err, want)
		}
	}
	if _, err := tool.Resolve(layer.Config{Pin: "1.29"}, ""); err == nil {
		t.Error("1.29 (only a release candidate): no error")
	}
}

func TestGoSupport(t *testing.T) {
	releases, _ := ParseGoReleases([]byte(goReleases))
	for _, line := range []string{"1.27", "1.28"} {
		if err := GoSupport(releases, line); err != nil {
			t.Errorf("support of %s: %v", line, err)
		}
	}
	if err := GoSupport(releases, "1.30"); err == nil || errors.As(err, new(*layer.EndOfLifeError)) {
		t.Errorf("1.30 (not released): err = %v", err)
	}
	if err := GoSupport(releases, "1"); err == nil {
		t.Error("line 1: no error")
	}
	err := GoSupport(releases, "1.26")
	var eol *layer.EndOfLifeError
	if !errors.As(err, &eol) {
		t.Fatalf("1.26: err = %v, want an EndOfLifeError", err)
	}
	if !strings.Contains(eol.Since, "Go 1.28.0 was released") || !reflect.DeepEqual(eol.Supported, []string{"1.27", "1.28"}) {
		t.Errorf("1.26: %+v", eol)
	}
}

func TestGoLine(t *testing.T) {
	for version, want := range map[string]string{"1.27.1": "1.27", "1.27": "1.27", "go1.28.0": "1.28", "1": "1"} {
		if got := GoLine(version); got != want {
			t.Errorf("GoLine(%q) = %q, want %q", version, got, want)
		}
	}
}
