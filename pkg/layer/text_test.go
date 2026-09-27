package layer

import "testing"

func TestTextHelpers(t *testing.T) {
	if got := FindVersion("gh version 2.99.0 (2026-09-01)\nhttps://…", `gh version ([0-9.]+)`); got != "2.99.0" {
		t.Errorf("FindVersion = %q", got)
	}
	if got := FindVersion("no match", `v([0-9]+)`); got != "" {
		t.Errorf("FindVersion without match = %q", got)
	}
	if got := LastWord("git version 2.47.3\n"); got != "2.47.3" {
		t.Errorf("LastWord = %q", got)
	}
	if got := FirstLine("gcc (Debian 14.2.0-19) 14.2.0\nCopyright"); got != "gcc (Debian 14.2.0-19) 14.2.0" {
		t.Errorf("FirstLine = %q", got)
	}
}
