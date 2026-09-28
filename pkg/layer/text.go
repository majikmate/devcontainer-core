// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package layer

import (
	"regexp"
	"strings"
)

// FindVersion returns the first group of pattern in text, for example the
// version in the output of "gh --version" with `gh version ([0-9.]+)`.
func FindVersion(text, pattern string) string {
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// LastWord returns the last word of a text (for example the version in
// "git version 2.47.3").
func LastWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// FirstLine returns the first line of a text.
func FirstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
