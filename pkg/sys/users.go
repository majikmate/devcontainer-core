// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sys

import (
	"fmt"
	"os/user"
	"strconv"
)

// User is an account of the image.
type User struct {
	Name   string
	UID    int
	GID    int
	Home   string
	Groups []uint32
}

// LookupUser reads an account from /etc/passwd and /etc/group. With
// CGO_ENABLED=0 the Go standard library reads these files directly.
func LookupUser(name string) (*User, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return nil, fmt.Errorf("user %s: %w", name, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	result := &User{Name: u.Username, UID: uid, GID: gid, Home: u.HomeDir}
	ids, err := u.GroupIds()
	if err == nil {
		for _, id := range ids {
			n, _ := strconv.Atoi(id)
			result.Groups = append(result.Groups, uint32(n))
		}
	}
	return result, nil
}
