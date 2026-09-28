// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

// The layer os: the Debian release of the images (tool debian), the upgrade
// of all Debian packages and the basic tools, and the support check of the
// installed Debian release.

package layers

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/majikmate/devcontainer-core/pkg/debian"
	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// Release choice of the tool debian: the pinned Debian release (series) and
// the release channel (see layer.Config). Pinned to trixie (Debian 13): a new
// Debian release is a decision, not an automatic change; the end of the
// regular security support stops the release. The Debian release list has
// no channels, so the channel is empty. The devcontainer.json of an image
// can override both
// ("customizations": {"devcon": {"debian": {"pin": "...", "channel": "..."}}}).
const (
	debianPin     = "trixie"
	debianChannel = ""
)

// Basic tools of every image. The libraries at the end are needed by VS Code
// Server and by tools that run in the terminal; bubblewrap lets VS Code run
// agent commands in a sandbox.
var osPackages = []string{
	// shells and system tools
	"zsh", "bash-completion", "sudo", "procps", "psmisc", "lsof", "htop", "strace",
	"iproute2", "net-tools", "man-db", "manpages", "bubblewrap",
	// files and text
	"less", "nano", "vim-tiny", "jq", "tree", "ncdu", "rsync",
	"zip", "unzip", "xz-utils", "bzip2",
	// network and version control (netbase: /etc/services and /etc/protocols)
	"ca-certificates", "netbase", "curl", "wget", "gnupg", "openssh-client", "git",
	// locales and time zones (configured by the layer locales)
	"locales", "tzdata",
	// the dates of the Debian releases (support check of this layer)
	"distro-info-data",
	// libraries
	"libc6", "libstdc++6", "libgcc-s1", "zlib1g", "libssl3t64", "libkrb5-3", "libgssapi-krb5-2",
}

func init() {
	layer.Register(&layer.Layer{
		Name:    "os",
		Summary: "upgrades all Debian packages and installs the basic tools",
		// The Debian release: the release plan passes it as DEBIAN_SERIES,
		// and the FROM line of the Dockerfile uses it
		// (FROM debian:${DEBIAN_SERIES}).
		Tools: []layer.Tool{
			{Name: "debian", Arg: "DEBIAN_SERIES", Source: debianSource, Version: layer.Config{Pin: debianPin, Channel: debianChannel}},
		},
		// Only at build time: the release workflow rebuilds the images when
		// Debian publishes security updates (see "devcon os-updates").
		Install: func(e *layer.Env) error {
			if err := debian.AptUpdate(); err != nil {
				return err
			}
			if err := debian.AptUpgrade(); err != nil {
				return err
			}
			packages := append([]string{}, osPackages...)
			if icu, err := newestPackage("libicu", `^libicu[0-9]+$`); err == nil && icu != "" {
				packages = append(packages, icu)
			}
			if err := debian.AptInstall(packages...); err != nil {
				return err
			}
			return debian.AptClean()
		},
		Commands: []layer.Command{
			{Name: "os-updates", Summary: "list the pending Debian package updates, --security: only security updates (root)", Run: osUpdates},
		},
		Check: func() error {
			series, err := debian.InstalledSeries()
			if err != nil {
				return err
			}
			releases, err := debian.InstalledReleases()
			if err != nil {
				return err
			}
			return checkDebianRelease(releases, series, time.Now().UTC())
		},
		Test: func(t *layer.T) {
			series, _ := debian.InstalledSeries()
			release, _ := sys.Output("sh", "-c", ". /etc/os-release && echo $PRETTY_NAME")
			t.Version("debian", series)
			t.Version("debian-release", release)
			t.Command("Debian release is supported (devcon check os)", "devcon", "check", "os")
			for _, cmd := range []string{"zsh", "sudo", "git", "curl", "jq", "less", "nano", "ssh", "zip", "unzip", "rsync", "htop"} {
				t.HasCommand(cmd)
			}
		},
	})
}

// osUpdates prints the packages with a newer version in the package sources,
// in the format of debian.FormatUpdates. It changes nothing in the image. The
// release tool runs it in the newest published image to find security
// updates.
func osUpdates(args []string) error {
	if !sys.IsRoot() {
		return fmt.Errorf("os-updates needs root (sudo devcon os-updates)")
	}
	securityOnly := len(args) > 0 && args[0] == "--security"
	updates, err := debian.PendingUpdates()
	if err != nil {
		return err
	}
	var selected []debian.Update
	for _, u := range updates {
		if u.Security || !securityOnly {
			selected = append(selected, u)
		}
	}
	fmt.Fprint(os.Stdout, debian.FormatUpdates(selected))
	return nil
}

// checkDebianRelease returns an *layer.EndOfLifeError when the regular
// security support of the Debian release series has ended on the day today
// (column eol of the distro-info-data; the later LTS support does not count).
func checkDebianRelease(releases []debian.Release, series string, today time.Time) error {
	r, ok := debian.FindRelease(releases, series)
	if !ok {
		return fmt.Errorf("Debian release %q is not listed in %s", series, debian.DistroInfoFile)
	}
	if r.EOL.IsZero() || today.Before(r.EOL) {
		return nil
	}
	return &layer.EndOfLifeError{
		What:      "Debian " + r.Name(),
		Since:     "regular security support ended on " + r.EOL.Format("2006-01-02"),
		Source:    debian.DistroInfoFile + " (Debian package distro-info-data, https://www.debian.org/releases/)",
		Supported: debian.SupportedReleases(releases, today),
		Change:    debianChange,
	}
}

// debianChange says where the Debian release is configured.
const debianChange = "the pinned Debian release (the constant debianPin at the top of pkg/layers/os.go of devcontainer-core, or customizations.devcon.debian.pin in the devcontainer.json of the image)"

// debianSource lists the released Debian releases, newest first. A release
// line is a release series (for example "trixie" for Debian 13).
var debianSource = &layer.Source{
	Name:   "Debian releases (" + debian.DistroInfoURL + ")",
	Policy: "a line is a Debian release series (for example trixie for Debian 13); it ends with the regular security support (column eol)",
	Releases: func() ([]layer.Release, error) {
		releases, err := debian.PublishedReleases()
		if err != nil {
			return nil, err
		}
		var result []layer.Release
		for _, r := range debian.Released(releases, time.Now().UTC()) {
			result = append(result, layer.Release{Version: r.Series})
		}
		return result, nil
	},
	Support: func(line string) error {
		releases, err := debian.PublishedReleases()
		if err != nil {
			return err
		}
		return checkDebianRelease(releases, line, time.Now().UTC())
	},
}

// newestPackage returns the package with the highest version number in its
// name (for example libicu76), from the packages whose name matches pattern.
func newestPackage(prefix, pattern string) (string, error) {
	out, err := sys.Output("apt-cache", "pkgnames", prefix)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(pattern)
	var names []string
	for _, name := range strings.Fields(out) {
		if re.MatchString(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", nil
	}
	number := regexp.MustCompile(`[0-9]+`)
	sort.Slice(names, func(i, j int) bool {
		return len(number.FindString(names[i])) < len(number.FindString(names[j])) ||
			(len(number.FindString(names[i])) == len(number.FindString(names[j])) && names[i] < names[j])
	})
	return names[len(names)-1], nil
}
