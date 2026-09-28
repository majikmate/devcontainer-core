// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package layers

import (
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/debian"
	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

func init() {
	layer.Register(&layer.Layer{
		Name:    "playwright-deps",
		Summary: "system libraries of the Playwright browsers (the projects install the browsers)",
		Needs:   []string{"node"},
		Args: []layer.Arg{
			{Name: "PLAYWRIGHT_BROWSERS", Default: "chromium firefox webkit", Doc: "browsers, separated by spaces"},
		},
		Install: func(e *layer.Env) error {
			if err := debian.AptUpdate(); err != nil {
				return err
			}
			// Playwright's own list of Debian packages per browser
			args := append([]string{"--yes", "playwright@latest", "install-deps"}, strings.Fields(e.Arg("PLAYWRIGHT_BROWSERS"))...)
			if err := sys.Run(nil, "npx", args...); err != nil {
				return err
			}
			_ = sys.Run(nil, "npm", "cache", "clean", "--force")
			return debian.AptClean()
		},
		Test: func(t *layer.T) {
			libraries := t.Output("library list", "/sbin/ldconfig", "-p")
			for _, lib := range []string{"libnss3.so", "libatk-1.0.so.0", "libgbm.so.1", "libgtk-3.so.0"} {
				t.Check("library "+lib, strings.Contains(libraries, lib))
			}
		},
	})
}
