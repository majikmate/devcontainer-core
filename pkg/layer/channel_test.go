package layer

import (
	"strings"
	"testing"
)

// channelTool has the channels "lts" (default, 2.9.3) and "stable" (3.0.1),
// and a pin whose line 2 has the newest release 2.9.7.
func channelTool() *Layer {
	return &Layer{
		Name: "test-channel",
		Tools: []Tool{{
			Name: "rt", Arg: "RT_VERSION", ChannelArg: "RT_CHANNEL",
			Channels: []Channel{
				{Name: "lts", Label: "long-term support", Newest: func() (string, error) { return "v2.9.3", nil }},
				{Name: "stable", Label: "stable", Newest: func() (string, error) { return "v3.0.1", nil }},
			},
			Pin: &Pin{Arg: "RT_PIN", Example: "2", Newest: func(line string) (string, error) { return "v" + line + ".9.7", nil }},
		}},
	}
}

func TestChannel(t *testing.T) {
	cases := []struct {
		channel, pin, want string
	}{
		{"", "", "v2.9.3"},        // default channel: the first one
		{"lts", "", "v2.9.3"},     // the exact release of the channel
		{"stable", "", "v3.0.1"},  // the other channel
		{"lts", "2", "v2.9.3"},    // the release of the channel is inside the line
		{"stable", "2", "v2.9.7"}, // outside the line: the newest release of the line
	}
	for _, c := range cases {
		t.Setenv("RT_CHANNEL", c.channel)
		t.Setenv("RT_PIN", c.pin)
		e := &Env{Layer: channelTool()}
		if v, err := e.Version("rt"); err != nil || v != c.want {
			t.Errorf("channel %q, pin %q: %q, %v; want %s", c.channel, c.pin, v, err, c.want)
		}
	}

	t.Setenv("RT_PIN", "")
	t.Setenv("RT_CHANNEL", "nightly")
	e := &Env{Layer: channelTool()}
	if got := e.Arg("RT_CHANNEL"); got != "nightly" {
		t.Errorf("Arg(RT_CHANNEL) = %q", got)
	}
	want := "RT_CHANNEL=nightly: the release channels of rt are lts, stable"
	if _, err := e.Version("rt"); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("unknown channel: err = %v, want %q", err, want)
	}

	// A tool without channels accepts no channel
	plain := Tool{Name: "plain", Newest: func() (string, error) { return "1.0.0", nil }}
	if c, err := plain.Channel(""); c != nil || err != nil {
		t.Errorf("no channels: %v, %v", c, err)
	}
	if _, err := plain.Channel("lts"); err == nil {
		t.Error("no channels, channel lts: no error")
	}
}
