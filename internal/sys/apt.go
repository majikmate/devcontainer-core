package sys

import (
	"os"
	"path/filepath"
)

var aptEnv = []string{"DEBIAN_FRONTEND=noninteractive"}

// AptUpdate reads the package lists.
func AptUpdate() error {
	return Run(aptEnv, "apt-get", "update")
}

// AptUpgrade upgrades all installed packages.
func AptUpgrade() error {
	return Run(aptEnv, "apt-get", "upgrade", "-y")
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
	return Run(aptEnv, "apt-get", args...)
}

// AptClean removes unused packages, the package cache and the package lists,
// so they do not stay in the image layer.
func AptClean() error {
	if err := Run(aptEnv, "apt-get", "autoremove", "-y"); err != nil {
		return err
	}
	if err := Run(aptEnv, "apt-get", "clean"); err != nil {
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
