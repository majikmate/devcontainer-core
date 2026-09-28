// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package layers

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/majikmate/devcontainer-core/pkg/debian"
	"github.com/majikmate/devcontainer-core/pkg/layer"
)

func TestCheckDebianRelease(t *testing.T) {
	releases, err := debian.ParseDistroInfo(`version,codename,series,created,release,eol,eol-lts,eol-elts
12,Bookworm,bookworm,2021-08-14,2023-06-10,2026-06-10,2028-06-30,2033-06-30
13,Trixie,trixie,2023-06-10,2025-08-09
`)
	if err != nil {
		t.Fatal(err)
	}
	day := func(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }

	if err := checkDebianRelease(releases, "bookworm", day("2026-06-09")); err != nil {
		t.Errorf("bookworm before its eol: %v", err)
	}
	if err := checkDebianRelease(releases, "trixie", day("2030-01-01")); err != nil {
		t.Errorf("trixie without an eol date: %v", err)
	}
	if err := checkDebianRelease(releases, "sid", day("2026-01-01")); err == nil {
		t.Error("unknown release: no error")
	}

	err = checkDebianRelease(releases, "bookworm", day("2026-06-10"))
	var eol *layer.EndOfLifeError
	if !errors.As(err, &eol) {
		t.Fatalf("bookworm at its eol: err = %v, want an EndOfLifeError", err)
	}
	for _, part := range []string{"Debian 12 (bookworm) has reached its end of life", "ended on 2026-06-10", "supported: 13 (trixie)", "customizations.devcon.debian.pin"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("message %q does not contain %q", err, part)
		}
	}
}
