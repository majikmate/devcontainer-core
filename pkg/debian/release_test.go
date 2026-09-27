package debian

import (
	"reflect"
	"testing"
	"time"
)

// distroInfo is an excerpt of /usr/share/distro-info/debian.csv.
const distroInfo = `version,codename,series,created,release,eol,eol-lts,eol-elts
11,Bullseye,bullseye,2019-07-06,2021-08-14,2024-08-14,2026-08-31,2031-06-30
12,Bookworm,bookworm,2021-08-14,2023-06-10,2026-06-10,2028-06-30,2033-06-30
13,Trixie,trixie,2023-06-10,2025-08-09
14,Forky,forky,2025-08-09
,Sid,sid,1993-08-16
`

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestParseDistroInfo(t *testing.T) {
	releases, err := ParseDistroInfo(distroInfo)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 5 {
		t.Fatalf("got %d releases, want 5", len(releases))
	}
	bookworm, ok := FindRelease(releases, "bookworm")
	if !ok || bookworm.Name() != "12 (bookworm)" || !bookworm.EOL.Equal(day("2026-06-10")) {
		t.Errorf("bookworm = %+v", bookworm)
	}
	trixie, _ := FindRelease(releases, "trixie")
	if !trixie.EOL.IsZero() {
		t.Errorf("trixie has no eol date yet, got %v", trixie.EOL)
	}
}

func TestSupportedReleases(t *testing.T) {
	releases, _ := ParseDistroInfo(distroInfo)
	// The LTS dates do not count: bullseye is not supported in 2025
	if got, want := SupportedReleases(releases, day("2025-09-01")), []string{"12 (bookworm)", "13 (trixie)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("2025-09-01: %v, want %v", got, want)
	}
	if got, want := SupportedReleases(releases, day("2026-06-10")), []string{"13 (trixie)"}; !reflect.DeepEqual(got, want) {
		t.Errorf("2026-06-10: %v, want %v", got, want)
	}
}

func TestParseDistroInfoErrors(t *testing.T) {
	if _, err := ParseDistroInfo("version,codename\n12,Bookworm\n"); err == nil {
		t.Error("missing columns: no error")
	}
	if _, err := ParseDistroInfo("version,codename,series,created,release,eol\n12,Bookworm,bookworm,x,2023-06-10,soon\n"); err == nil {
		t.Error("bad date: no error")
	}
}
