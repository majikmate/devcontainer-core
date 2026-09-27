package debian

import "testing"

const simulated = `NOTE: This is only a simulation!
Reading package lists...
Building dependency tree...
Calculating upgrade...
The following packages will be upgraded:
  libssl3t64 tzdata
Inst libssl3t64 [3.5.1-1] (3.5.1-1+deb13u1 Debian-Security:13/stable-security [amd64])
Inst tzdata [2025b-4] (2025b-4+deb13u1 Debian:13.1/stable, Debian:13.1/stable [all])
Inst libnew1 (1.0-1 Debian:13.1/stable [arm64])
Conf libssl3t64 (3.5.1-1+deb13u1 Debian-Security:13/stable-security [amd64])
`

func TestParseUpgrade(t *testing.T) {
	got := ParseUpgrade(simulated)
	want := []Update{
		{Package: "libnew1", Old: "", New: "1.0-1", Source: "Debian:13.1/stable"},
		{Package: "libssl3t64", Old: "3.5.1-1", New: "3.5.1-1+deb13u1", Source: "Debian-Security:13/stable-security", Security: true},
		{Package: "tzdata", Old: "2025b-4", New: "2025b-4+deb13u1", Source: "Debian:13.1/stable, Debian:13.1/stable"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d updates, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("update %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFormatAndReadUpdates(t *testing.T) {
	updates := ParseUpgrade(simulated)
	back := ReadUpdates("other output\n" + FormatUpdates(updates))
	if len(back) != len(updates) {
		t.Fatalf("got %d updates back, want %d", len(back), len(updates))
	}
	for i := range updates {
		if back[i] != updates[i] {
			t.Errorf("update %d: got %+v, want %+v", i, back[i], updates[i])
		}
	}
}
