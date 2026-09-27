package release

import (
	"testing"

	"github.com/majikmate/devcontainer-core/pkg/debian"
)

func TestSubtractUpdates(t *testing.T) {
	updates := []debian.Update{
		{Package: "gcc-14", Old: "14.2.0-19", New: "14.2.0-19+deb13u1", Security: true},
		{Package: "libssl3t64", Old: "3.5.1-1", New: "3.5.1-1+deb13u1", Security: true},
	}
	base := []debian.Update{
		{Package: "libssl3t64", Old: "3.5.1-1", New: "3.5.1-1+deb13u1", Security: true},
	}
	got := subtractUpdates(updates, base)
	if len(got) != 1 || got[0].Package != "gcc-14" {
		t.Fatalf("got %+v, want only gcc-14", got)
	}
	if reason := securityReason(got); reason != "Debian security updates: gcc-14 14.2.0-19 → 14.2.0-19+deb13u1" {
		t.Errorf("reason = %q", reason)
	}
}
