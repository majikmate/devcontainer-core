package release

import (
	"fmt"
	"strings"
	"testing"

	"github.com/majikmate/devcontainer-core/pkg/debian"
)

func TestSubtractUpdates(t *testing.T) {
	updates := []debian.Update{
		{Package: "gcc-14", Old: "14.2.0-19", New: "14.2.0-19+deb13u1"},
		{Package: "libssl3t64", Old: "3.5.1-1", New: "3.5.1-1+deb13u1", Security: true},
	}
	base := []debian.Update{
		{Package: "libssl3t64", Old: "3.5.1-1", New: "3.5.1-1+deb13u1", Security: true},
	}
	got := subtractUpdates(updates, base)
	if len(got) != 1 || got[0].Package != "gcc-14" {
		t.Fatalf("got %+v, want only gcc-14", got)
	}
}

func TestUpdatesReason(t *testing.T) {
	updates := []debian.Update{
		{Package: "libssl3t64", Old: "3.5.1-1", New: "3.5.1-1+deb13u1", Security: true},
		{Package: "tzdata", Old: "2025b-4", New: "2025b-4+deb13u1"},
	}
	want := "Debian updates: 2 package(s); security: libssl3t64 3.5.1-1 → 3.5.1-1+deb13u1; other: tzdata 2025b-4 → 2025b-4+deb13u1"
	if got := updatesReason(updates); got != want {
		t.Errorf("reason = %q, want %q", got, want)
	}

	var many []debian.Update
	for i := 0; i < maxListed+3; i++ {
		many = append(many, debian.Update{Package: fmt.Sprintf("pkg%02d", i), Old: "1", New: "2"})
	}
	if got := updatesReason(many); !strings.HasSuffix(got, "and 3 more") {
		t.Errorf("reason = %q, want the suffix \"and 3 more\"", got)
	}
}
