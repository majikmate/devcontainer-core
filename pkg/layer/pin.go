package layer

import (
	"errors"
	"fmt"
	"strings"
)

// Pin makes a tool pinnable to a release line, for example Go 1.27 or
// Node.js 24. The Dockerfile sets the line with the build argument Arg:
//
//	ARG GO_PIN=1.27
//	RUN devcon install go
//
// With a pin, the layer installs the newest release inside the line. Without
// a pin, it installs the newest release. When the line reaches its end of
// life, the build fails with an *EndOfLifeError; there is no warning before.
type Pin struct {
	Arg     string // build argument, for example "GO_PIN"
	Example string // example value, for example "1.27"
	Policy  string // what a line is and when it ends (documentation)
	// Newest returns the newest release inside a line.
	Newest func(line string) (string, error)
	// Support returns an *EndOfLifeError (with Since, Source and Supported)
	// when the line has reached its end of life, and nil while it is
	// supported. Other errors mean that the support could not be checked.
	Support func(line string) error
}

// EndOfLifeError reports a release line (a pinned tool version or the Debian
// release) that has reached its end of life. It stops the build.
type EndOfLifeError struct {
	What      string   // the release line, for example "go 1.27 (GO_PIN=1.27)"
	Since     string   // when or why the support ended, for example "on 2028-04-30"
	Source    string   // where this information comes from (URL)
	Supported []string // the supported lines, for example "1.28", "1.29"
	Change    string   // what to change, for example "ARG GO_PIN in the Dockerfile"
}

func (e *EndOfLifeError) Error() string {
	msg := fmt.Sprintf("%s has reached its end of life (%s). Source: %s.", e.What, e.Since, e.Source)
	if e.Change != "" {
		msg += " Change " + e.Change + " to a supported version"
		if len(e.Supported) > 0 {
			msg += " (supported: " + strings.Join(e.Supported, ", ") + ")"
		}
		msg += "."
	}
	return msg
}

// CheckPin returns an *EndOfLifeError when the pinned line of the tool has
// reached its end of life. An empty line (no pin) is always supported.
func (t *Tool) CheckPin(line string) error {
	if line == "" {
		return nil
	}
	if t.Pin == nil {
		return fmt.Errorf("tool %s cannot be pinned", t.Name)
	}
	if t.Pin.Support == nil {
		return nil
	}
	err := t.Pin.Support(line)
	if eol := (*EndOfLifeError)(nil); errors.As(err, &eol) {
		eol.What = fmt.Sprintf("%s %s (%s=%s)", t.Name, line, t.Pin.Arg, line)
		eol.Change = "ARG " + t.Pin.Arg + " in the Dockerfile"
		return eol
	}
	if err != nil {
		return fmt.Errorf("support of %s %s: %w", t.Name, line, err)
	}
	return nil
}

// NewestVersion returns the version to install:
//
//   - with release channels: the newest release of the channel (empty = the
//     default channel); with a pinned line, only when it is inside the line,
//     otherwise the newest release inside the line,
//   - the newest release inside the pinned line,
//   - the newest release that works with the version of the tool it follows,
//   - or the newest release.
func (t *Tool) NewestVersion(line, channel, followed string) (string, error) {
	if line != "" && t.Pin == nil {
		return "", fmt.Errorf("tool %s cannot be pinned", t.Name)
	}
	c, err := t.Channel(channel)
	if err != nil {
		return "", err
	}
	switch {
	case c != nil:
		v, err := c.Newest()
		if line == "" || (err == nil && InLine(v, line)) {
			return v, err
		}
		return t.Pin.Newest(line)
	case line != "":
		return t.Pin.Newest(line)
	case t.Follows != "" && t.NewestFor != nil && followed != "":
		return t.NewestFor(followed)
	default:
		return t.Newest()
	}
}

// InLine reports whether a version belongs to a release line, for example
// "1.27.3" and "go1.27.3" to "1.27", or "v24.9.0" to "24".
func InLine(version, line string) bool {
	version = strings.TrimPrefix(strings.TrimPrefix(version, "go"), "v")
	line = strings.TrimPrefix(strings.TrimPrefix(line, "go"), "v")
	return version == line || strings.HasPrefix(version, line+".")
}
