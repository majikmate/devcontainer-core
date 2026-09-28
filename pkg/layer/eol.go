// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package layer

import (
	"fmt"
	"strings"
)

// EndOfLifeError reports a release line (a pinned tool version or the Debian
// release) that has reached its end of life. It stops the build.
type EndOfLifeError struct {
	What      string   // the release line, for example "go 1.27 (pinned line)"
	Since     string   // when or why the support ended, for example "on 2028-04-30"
	Source    string   // where this information comes from (URL)
	Supported []string // the supported lines, for example "1.28", "1.29"
	Change    string   // what to change, for example "the pinned line of go (...)"
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

// InLine reports whether a version belongs to a release line, for example
// "1.27.3" and "go1.27.3" to "1.27", or "v24.9.0" to "24".
func InLine(version, line string) bool {
	version = strings.TrimPrefix(strings.TrimPrefix(version, "go"), "v")
	line = strings.TrimPrefix(strings.TrimPrefix(line, "go"), "v")
	return version == line || strings.HasPrefix(version, line+".")
}
