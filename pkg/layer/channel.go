package layer

import (
	"fmt"
	"strings"
)

// Channel is a release channel of a tool, for example the Deno channels
// "lts" and "stable" or the Node.js channels "lts" and "current". The
// Dockerfile chooses the channel with the build argument Tool.ChannelArg:
//
//	ARG DENO_CHANNEL=stable
//	RUN devcon install deno
//
// Without the build argument, the first channel of the tool is used. With a
// pinned line (see Pin), the newest release of the channel is used when it is
// inside the line, otherwise the newest release of the line.
type Channel struct {
	Name  string // value of the build argument, for example "lts"
	Label string // shown in the documentation, for example "long-term support"
	// Newest returns the newest release of the channel.
	Newest func() (string, error)
}

// Channel returns the channel with the name, or the default channel (the
// first one) for an empty name. It returns nil for a tool without channels.
func (t *Tool) Channel(name string) (*Channel, error) {
	if len(t.Channels) == 0 {
		if name != "" {
			return nil, fmt.Errorf("tool %s has no release channels", t.Name)
		}
		return nil, nil
	}
	if name == "" {
		return &t.Channels[0], nil
	}
	for i := range t.Channels {
		if t.Channels[i].Name == name {
			return &t.Channels[i], nil
		}
	}
	return nil, fmt.Errorf("%s=%s: the release channels of %s are %s", t.ChannelArg, name, t.Name, strings.Join(t.ChannelNames(), ", "))
}

// ChannelNames returns the names of the channels of the tool.
func (t *Tool) ChannelNames() []string {
	var names []string
	for _, c := range t.Channels {
		names = append(names, c.Name)
	}
	return names
}
