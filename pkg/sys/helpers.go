// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sys

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// TempDir creates a temporary folder; the returned function removes it.
func TempDir() (string, func(), error) {
	dir, err := os.MkdirTemp("", "devcon-")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

// ChecksumFor finds the SHA-256 checksum of a file in a list of
// "<sha256>  <file>" lines (the checksum files of GitHub releases).
func ChecksumFor(list, file string) string {
	scanner := bufio.NewScanner(strings.NewReader(list))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == file {
			return fields[0]
		}
	}
	return ""
}

// GroupID returns the id of a group.
func GroupID(name string) (int, error) {
	out, err := Output("getent", "group", name)
	if err != nil {
		return 0, err
	}
	parts := strings.Split(out, ":")
	if len(parts) < 3 {
		return 0, fmt.Errorf("group %s: unexpected entry %q", name, out)
	}
	var id int
	_, err = fmt.Sscanf(parts[2], "%d", &id)
	return id, err
}
