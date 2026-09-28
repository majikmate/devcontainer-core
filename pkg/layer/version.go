package layer

import (
	"errors"
	"fmt"
	"strings"
)

// The version of a tool is chosen by one general rule for all tools:
//
//  1. The source of the tool lists its releases, newest first (Source).
//  2. The feature decides the release line and the channel (Tool.Version);
//     the devcontainer.json of an image can override them
//     ("customizations": {"devcon": {"<tool>": {"pin": "...", "channel": "..."}}}).
//  3. The newest release in the channel and in the pinned line is installed;
//     a tool that follows another tool (gopls follows go) gets the newest of
//     these releases that works with the version of that tool.
//
// A Dockerfile never decides a version: it only receives the chosen version
// as build argument (Tool.Arg).

// Config is the release choice of a tool: in the feature (Tool.Version) and,
// with the same keys, in the devcontainer.json of an image.
type Config struct {
	Pin     string `json:"pin"`     // release line, for example "1.27", "24" or "trixie"; empty = no pin
	Channel string `json:"channel"` // release channel, for example "lts"; empty = the default channel of the source
}

// Source lists the releases of a tool from the place that the layer installs
// from (for example the npm registry or the GitHub releases).
type Source struct {
	Name string // for the documentation, for example "npm registry (prettier)"
	// Releases returns the releases, newest first, without pre-releases.
	Releases func() ([]Release, error)
	// Channels are the release channels of the source; the first one is the
	// default. Empty: the source has no channels.
	Channels []Channel
	// Policy says what a release line is and when it ends (documentation).
	Policy string
	// Support returns an *EndOfLifeError (with Since, Source and Supported)
	// when a release line has reached its end of life (optional). Other
	// errors mean that the support could not be checked.
	Support func(line string) error
}

// Release is one release of a source.
type Release struct {
	Version  string   // for example "v2.9.3", "3.9.9" or "trixie"
	Channels []string // the channels of the release, for example "lts", "stable"
}

// Channel is a release channel of a source, for example the Deno channels
// "lts" and "stable" or the Node.js channels "lts" and "current".
type Channel struct {
	Name  string // for example "lts"
	Label string // for the documentation, for example "long-term support"
}

// maxFollowChecks limits how many releases are checked against the version
// of the followed tool (each check can download a file).
const maxFollowChecks = 30

// Effective returns the configuration with the default channel of the
// source, and checks that the channel exists.
func (t *Tool) Effective(c Config) (Config, error) {
	if t.Source == nil {
		if c.Pin != "" || c.Channel != "" {
			return c, fmt.Errorf("tool %s has no version source: it cannot be pinned or get a channel", t.Name)
		}
		return c, nil
	}
	channels := t.Source.Channels
	if c.Channel == "" {
		if len(channels) > 0 {
			c.Channel = channels[0].Name
		}
		return c, nil
	}
	for _, ch := range channels {
		if ch.Name == c.Channel {
			return c, nil
		}
	}
	if len(channels) == 0 {
		return c, fmt.Errorf("channel %s: tool %s has no release channels", c.Channel, t.Name)
	}
	var names []string
	for _, ch := range channels {
		names = append(names, ch.Name)
	}
	return c, fmt.Errorf("channel %s: the release channels of %s are %s", c.Channel, t.Name, strings.Join(names, ", "))
}

// CheckSupport returns an *EndOfLifeError when the pinned line of the
// configuration has reached its end of life. Without a pin, or without a
// support rule of the source, the line is always supported.
func (t *Tool) CheckSupport(c Config) error {
	if c.Pin == "" || t.Source == nil || t.Source.Support == nil {
		return nil
	}
	err := t.Source.Support(c.Pin)
	if eol := (*EndOfLifeError)(nil); errors.As(err, &eol) {
		eol.What = fmt.Sprintf("%s %s (pinned line)", t.Name, c.Pin)
		eol.Change = "the pinned line of " + t.Name + " (the pin constant at the top of the file of its layer, or customizations.devcon." + t.Name + ".pin in the devcontainer.json of the image)"
		return eol
	}
	if err != nil {
		return fmt.Errorf("support of %s %s: %w", t.Name, c.Pin, err)
	}
	return nil
}

// Resolve returns the version to install for the configuration c (see
// Effective) and the version of the followed tool ("" when the tool follows
// none): the newest release in the channel and in the pinned line that works
// with the followed version.
func (t *Tool) Resolve(c Config, followed string) (string, error) {
	if t.Source == nil {
		return "", fmt.Errorf("tool %s has no version source", t.Name)
	}
	releases, err := t.Source.Releases()
	if err != nil {
		return "", err
	}
	checks := 0
	for _, r := range releases {
		if c.Channel != "" && !containsString(r.Channels, c.Channel) {
			continue
		}
		if c.Pin != "" && !InLine(r.Version, c.Pin) {
			continue
		}
		if t.Follows != "" && t.Works != nil && followed != "" {
			if checks == maxFollowChecks {
				return "", fmt.Errorf("no release of %s in the newest %d works with %s %s", t.Name, maxFollowChecks, t.Follows, followed)
			}
			checks++
			ok, err := t.Works(r.Version, followed)
			if err != nil {
				return "", err
			}
			if !ok {
				continue
			}
		}
		return r.Version, nil
	}
	what := t.Name
	if c.Channel != "" {
		what += " in the channel " + c.Channel
	}
	if c.Pin != "" {
		what += " in the line " + c.Pin
	}
	return "", fmt.Errorf("no release of %s (source: %s)", what, t.Source.Name)
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
