package layer

import (
	"errors"
	"strings"
	"testing"
)

// testTools is a layer with a pinnable tool "lang" (line = major version,
// end of life below 3) and a tool "lint" that follows "lang".
func testTools() *Layer {
	return &Layer{
		Name: "test-pin",
		Tools: []Tool{
			{
				Name: "lang", Arg: "LANG_VERSION",
				Newest: func() (string, error) { return "4.1.0", nil },
				Pin: &Pin{
					Arg: "LANG_PIN", Example: "4",
					Newest: func(line string) (string, error) { return line + ".9.9", nil },
					Support: func(line string) error {
						if line < "3" {
							return &EndOfLifeError{Since: "on 2026-01-31", Source: "https://example.com/schedule", Supported: []string{"3", "4"}}
						}
						return nil
					},
				},
			},
			{
				Name: "lint", Arg: "LINT_VERSION",
				Newest:    func() (string, error) { return "v9.0.0", nil },
				Follows:   "lang",
				NewestFor: func(v string) (string, error) { return "v-for-" + v, nil },
			},
		},
	}
}

func TestVersionWithoutPin(t *testing.T) {
	e := &Env{Layer: testTools()}
	if v, err := e.Version("lang"); err != nil || v != "4.1.0" {
		t.Errorf("lang = %q, %v; want 4.1.0", v, err)
	}
	if v, err := e.Version("lint"); err != nil || v != "v-for-4.1.0" {
		t.Errorf("lint = %q, %v; want v-for-4.1.0", v, err)
	}
}

func TestVersionWithPin(t *testing.T) {
	t.Setenv("LANG_PIN", "3")
	e := &Env{Layer: testTools()}
	if got := e.Arg("LANG_PIN"); got != "3" {
		t.Errorf("Arg(LANG_PIN) = %q", got)
	}
	if v, err := e.Version("lang"); err != nil || v != "3.9.9" {
		t.Errorf("lang = %q, %v; want 3.9.9", v, err)
	}
	if v, err := e.Version("lint"); err != nil || v != "v-for-3.9.9" {
		t.Errorf("lint = %q, %v; want v-for-3.9.9", v, err)
	}
}

func TestVersionEndOfLife(t *testing.T) {
	t.Setenv("LANG_PIN", "2")
	e := &Env{Layer: testTools()}
	_, err := e.Version("lang")
	var eol *EndOfLifeError
	if !errors.As(err, &eol) {
		t.Fatalf("err = %v, want an EndOfLifeError", err)
	}
	want := "lang 2 (LANG_PIN=2) has reached its end of life (on 2026-01-31). Source: https://example.com/schedule. Change ARG LANG_PIN in the Dockerfile to a supported version (supported: 3, 4)."
	if err.Error() != want {
		t.Errorf("message:\n got %s\nwant %s", err, want)
	}
	// The tool that follows it fails too
	if _, err := e.Version("lint"); !errors.As(err, &eol) {
		t.Errorf("lint: err = %v, want an EndOfLifeError", err)
	}
}

func TestVersionFromBuildArgument(t *testing.T) {
	t.Setenv("LANG_PIN", "3")
	t.Setenv("LANG_VERSION", "3.2.1")
	e := &Env{Layer: testTools()}
	if v, err := e.Version("lang"); err != nil || v != "3.2.1" {
		t.Errorf("lang = %q, %v; want 3.2.1", v, err)
	}
	t.Setenv("LANG_VERSION", "4.0.0")
	if _, err := e.Version("lang"); err == nil || !strings.Contains(err.Error(), "not in the pinned line") {
		t.Errorf("err = %v, want \"not in the pinned line\"", err)
	}
}

func TestInLine(t *testing.T) {
	cases := []struct {
		version, line string
		want          bool
	}{
		{"1.27.3", "1.27", true},
		{"go1.27.3", "1.27", true},
		{"1.27", "1.27", true},
		{"1.270.1", "1.27", false},
		{"1.28.0", "1.27", false},
		{"v24.9.0", "24", true},
		{"v2.5.1", "2", true},
		{"v20.1.0", "2", false},
	}
	for _, c := range cases {
		if got := InLine(c.version, c.line); got != c.want {
			t.Errorf("InLine(%q, %q) = %v, want %v", c.version, c.line, got, c.want)
		}
	}
}
