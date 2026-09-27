// Package debian contains the Debian-specific helpers of the framework: the
// package manager apt and the list of pending package updates. The images are
// based on Debian (see the core image), so layers that install system
// packages use this package; distribution-independent layers do not.
package debian

import (
	"os"
	"path/filepath"

	"github.com/majikmate/devcontainer-core/pkg/sys"
)

var aptEnv = []string{"DEBIAN_FRONTEND=noninteractive"}

// AptUpdate reads the package lists.
func AptUpdate() error {
	return sys.Run(aptEnv, "apt-get", "update")
}

// AptUpgrade upgrades all installed packages.
func AptUpgrade() error {
	return sys.Run(aptEnv, "apt-get", "upgrade", "-y")
}

// AptInstall installs packages without recommended packages. It reads the
// package lists first if they are missing.
func AptInstall(packages ...string) error {
	if lists, _ := filepath.Glob("/var/lib/apt/lists/*_Packages"); len(lists) == 0 {
		if err := AptUpdate(); err != nil {
			return err
		}
	}
	args := append([]string{"install", "-y", "--no-install-recommends"}, packages...)
	return sys.Run(aptEnv, "apt-get", args...)
}

// AptClean removes unused packages, the package cache and the package lists,
// so they do not stay in the image layer.
func AptClean() error {
	if err := sys.Run(aptEnv, "apt-get", "autoremove", "-y"); err != nil {
		return err
	}
	if err := sys.Run(aptEnv, "apt-get", "clean"); err != nil {
		return err
	}
	entries, _ := filepath.Glob("/var/lib/apt/lists/*")
	for _, e := range entries {
		if filepath.Base(e) == "partial" || filepath.Base(e) == "lock" {
			continue
		}
		if err := os.RemoveAll(e); err != nil {
			return err
		}
	}
	return nil
}
