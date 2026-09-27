package debian

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// Update is a package with a newer version in the package sources.
type Update struct {
	Package  string // for example libssl3t64
	Old      string // installed version ("" for a new package)
	New      string // version of the upgrade
	Source   string // for example "Debian-Security:13/stable-security"
	Security bool   // the new version comes from the security archive
}

func (u Update) String() string {
	old := u.Old
	if old == "" {
		old = "new"
	}
	return fmt.Sprintf("%s %s → %s", u.Package, old, u.New)
}

// PendingUpdates reads the package lists and returns the packages that an
// upgrade would change. It only simulates the upgrade; nothing is installed.
// Needs root (apt-get update).
func PendingUpdates() ([]Update, error) {
	if err := sys.Run(aptEnv, "apt-get", "update", "-qq"); err != nil {
		return nil, err
	}
	out, err := sys.Output("apt-get", "-s", "-o", "Debug::NoLocking=1", "dist-upgrade")
	if err != nil {
		return nil, err
	}
	return ParseUpgrade(out), nil
}

// "Inst libssl3t64 [3.5.1-1] (3.5.1-1+deb13u1 Debian-Security:13/stable-security [amd64])"
var instLine = regexp.MustCompile(`^Inst (\S+) (?:\[(\S+)\] )?\((\S+) (.*?)(?: \[[^\]]*\])?\)`)

// ParseUpgrade reads the output of "apt-get -s dist-upgrade".
func ParseUpgrade(output string) []Update {
	var updates []Update
	for _, line := range strings.Split(output, "\n") {
		m := instLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		updates = append(updates, Update{
			Package:  m[1],
			Old:      m[2],
			New:      m[3],
			Source:   m[4],
			Security: strings.Contains(strings.ToLower(m[4]), "security"),
		})
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].Package < updates[j].Package })
	return updates
}

// FormatUpdates writes updates one per line, fields separated by tabs:
// package, old version, new version, "security" or "-", source.
func FormatUpdates(updates []Update) string {
	var b strings.Builder
	for _, u := range updates {
		kind := "-"
		if u.Security {
			kind = "security"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", u.Package, u.Old, u.New, kind, u.Source)
	}
	return b.String()
}

// ReadUpdates reads the output of FormatUpdates. Lines in another format are
// ignored.
func ReadUpdates(text string) []Update {
	var updates []Update
	for _, line := range strings.Split(text, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			continue
		}
		updates = append(updates, Update{Package: f[0], Old: f[1], New: f[2], Security: f[3] == "security", Source: f[4]})
	}
	return updates
}
