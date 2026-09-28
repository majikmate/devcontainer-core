package layer

import (
	"errors"
	"strings"
	"testing"
)

// testSource has the channels "lts" (default) and "stable", the releases
// 3.0.1 (stable), 2.9.7 (stable), 2.9.3 (lts, stable), 1.5.0 (lts, stable),
// and the end of life below line 2.
func testSource() *Source {
	return &Source{
		Name:     "test",
		Channels: []Channel{{Name: "lts", Label: "long-term support"}, {Name: "stable", Label: "all releases"}},
		Releases: func() ([]Release, error) {
			return []Release{
				{Version: "v3.0.1", Channels: []string{"stable"}},
				{Version: "v2.9.7", Channels: []string{"stable"}},
				{Version: "v2.9.3", Channels: []string{"lts", "stable"}},
				{Version: "v1.5.0", Channels: []string{"lts", "stable"}},
			}, nil
		},
		Support: func(line string) error {
			if line < "2" {
				return &EndOfLifeError{Since: "on 2026-01-31", Source: "https://example.com/schedule", Supported: []string{"2", "3"}}
			}
			return nil
		},
	}
}

func TestResolve(t *testing.T) {
	tool := Tool{Name: "rt", Arg: "RT_VERSION", Source: testSource()}
	cases := []struct {
		config Config
		want   string
	}{
		{Config{}, "v2.9.3"},                                // default channel: lts
		{Config{Channel: "stable"}, "v3.0.1"},               // the newest release
		{Config{Pin: "2", Channel: "stable"}, "v2.9.7"},     // the newest release of line 2
		{Config{Pin: "2"}, "v2.9.3"},                        // lts inside line 2
		{Config{Pin: "2.9", Channel: "stable"}, "v2.9.7"},   // a minor line
		{Config{Pin: "1", Channel: "lts"}, "v1.5.0"},        // an older line
		{Config{Pin: "2.9.3", Channel: "stable"}, "v2.9.3"}, // an exact version
	}
	for _, c := range cases {
		effective, err := tool.Effective(c.config)
		if err != nil {
			t.Fatal(err)
		}
		if v, err := tool.Resolve(effective, ""); err != nil || v != c.want {
			t.Errorf("%+v: %q, %v; want %s", c.config, v, err, c.want)
		}
	}
	// No lts release in line 3
	if _, err := tool.Resolve(Config{Pin: "3", Channel: "lts"}, ""); err == nil || !strings.Contains(err.Error(), "no release of rt in the channel lts in the line 3") {
		t.Errorf("line 3, lts: err = %v", err)
	}
}

func TestEffective(t *testing.T) {
	tool := Tool{Name: "rt", Source: testSource()}
	if c, err := tool.Effective(Config{Pin: "2"}); err != nil || c.Channel != "lts" {
		t.Errorf("default channel: %+v, %v", c, err)
	}
	want := "channel nightly: the release channels of rt are lts, stable"
	if _, err := tool.Effective(Config{Channel: "nightly"}); err == nil || err.Error() != want {
		t.Errorf("unknown channel: err = %v, want %q", err, want)
	}
	plain := Tool{Name: "plain", Source: &Source{Name: "plain"}}
	if c, err := plain.Effective(Config{}); err != nil || c.Channel != "" {
		t.Errorf("no channels: %+v, %v", c, err)
	}
	if _, err := plain.Effective(Config{Channel: "lts"}); err == nil {
		t.Error("no channels, channel lts: no error")
	}
}

func TestCheckSupport(t *testing.T) {
	tool := Tool{Name: "rt", Source: testSource()}
	if err := tool.CheckSupport(Config{Pin: "2"}); err != nil {
		t.Errorf("line 2: %v", err)
	}
	if err := tool.CheckSupport(Config{}); err != nil {
		t.Errorf("no pin: %v", err)
	}
	err := tool.CheckSupport(Config{Pin: "1"})
	var eol *EndOfLifeError
	if !errors.As(err, &eol) {
		t.Fatalf("line 1: err = %v, want an EndOfLifeError", err)
	}
	want := "rt 1 (pinned line) has reached its end of life (on 2026-01-31). Source: https://example.com/schedule. Change the pinned line of rt (the pin constant at the top of the file of its layer, or customizations.devcon.rt.pin in the devcontainer.json of the image) to a supported version (supported: 2, 3)."
	if err.Error() != want {
		t.Errorf("message:\n got %s\nwant %s", err, want)
	}
}

// testLayer has the tool "rt" and the tool "lint" that follows it: a lint
// release works with rt when its major version is at most the one of rt.
func testLayer(config Config) *Layer {
	lintReleases := []Release{{Version: "4.0.3"}, {Version: "3.1.2"}, {Version: "2.0.2"}}
	return &Layer{
		Name: "test",
		Tools: []Tool{
			{Name: "rt", Arg: "RT_VERSION", Source: testSource(), Version: config},
			{
				Name: "lint", Arg: "LINT_VERSION", Follows: "rt",
				Source: &Source{Name: "lint", Releases: func() ([]Release, error) { return lintReleases, nil }},
				Works: func(release, followed string) (bool, error) {
					return release[:1] <= strings.TrimPrefix(followed, "v")[:1], nil
				},
			},
		},
	}
}

func TestVersion(t *testing.T) {
	e := &Env{Layer: testLayer(Config{Pin: "2", Channel: "stable"})}
	if v, err := e.Version("rt"); err != nil || v != "v2.9.7" {
		t.Errorf("rt = %q, %v; want v2.9.7", v, err)
	}
	if v, err := e.Version("lint"); err != nil || v != "2.0.2" {
		t.Errorf("lint = %q, %v; want 2.0.2 (works with rt 2)", v, err)
	}
	// The pinned line at its end of life stops the tool and its follower
	e = &Env{Layer: testLayer(Config{Pin: "1"})}
	var eol *EndOfLifeError
	if _, err := e.Version("lint"); !errors.As(err, &eol) {
		t.Errorf("lint with rt 1: err = %v, want an EndOfLifeError", err)
	}
	// The build argument of the release plan wins (the image may override
	// the configuration of the feature)
	t.Setenv("RT_VERSION", "v3.0.1")
	e = &Env{Layer: testLayer(Config{Pin: "2"})}
	if v, err := e.Version("rt"); err != nil || v != "v3.0.1" {
		t.Errorf("rt with build argument = %q, %v; want v3.0.1", v, err)
	}
}

func TestNoSource(t *testing.T) {
	tool := Tool{Name: "none"}
	if _, err := tool.Resolve(Config{}, ""); err == nil {
		t.Error("tool without a source: no error")
	}
	if _, err := tool.Effective(Config{Pin: "1"}); err == nil {
		t.Error("tool without a source, with a pin: no error")
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
		{"trixie", "trixie", true},
	}
	for _, c := range cases {
		if got := InLine(c.version, c.line); got != c.want {
			t.Errorf("InLine(%q, %q) = %v, want %v", c.version, c.line, got, c.want)
		}
	}
}
