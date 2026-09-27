package layers

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/debian"
	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
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
	// network and version control
	"ca-certificates", "curl", "wget", "gnupg", "openssh-client", "git",
	// locales and time zones (configured by the layer locales)
	"locales", "tzdata",
	// libraries
	"libc6", "libstdc++6", "libgcc-s1", "zlib1g", "libssl3t64", "libkrb5-3", "libgssapi-krb5-2",
}

func init() {
	layer.Register(&layer.Layer{
		Name:    "os",
		Summary: "upgrades all Debian packages and installs the basic tools",
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
		Test: func(t *layer.T) {
			release, _ := sys.Output("sh", "-c", ". /etc/os-release && echo $PRETTY_NAME")
			t.Version("debian", release)
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
