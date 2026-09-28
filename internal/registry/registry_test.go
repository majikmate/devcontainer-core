// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package registry

import "testing"

func TestParseRef(t *testing.T) {
	cases := []struct{ in, host, repo, ref string }{
		{"buildpack-deps:trixie-curl", "registry-1.docker.io", "library/buildpack-deps", "trixie-curl"},
		{"golang:1.27-trixie", "registry-1.docker.io", "library/golang", "1.27-trixie"},
		{"ghcr.io/owner/devcontainer-core:1", "ghcr.io", "owner/devcontainer-core", "1"},
		{"ghcr.io/owner/name", "ghcr.io", "owner/name", "latest"},
		{"docker.io/user/image:tag", "registry-1.docker.io", "user/image", "tag"},
		{"localhost:5000/x:1", "localhost:5000", "x", "1"},
		{"ghcr.io/o/n@sha256:abc", "ghcr.io", "o/n", "sha256:abc"},
	}
	for _, c := range cases {
		r, err := ParseRef(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if r.Host != c.host || r.Repository != c.repo || r.Reference != c.ref {
			t.Errorf("%s: got %+v", c.in, r)
		}
	}
}
