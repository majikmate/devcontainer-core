package release

import (
	"fmt"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/debian"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// pendingUpdates returns the pending Debian package updates of an image: it
// starts the image as root and runs "devenv os-updates" (layer os). The
// command only simulates the upgrade; the image is not changed. Debian stable
// receives only fixes (security archive, stable-updates, point releases), so
// every pending update counts.
func pendingUpdates(image string) ([]debian.Update, error) {
	out, err := sys.Output("docker", "run", "--rm", "--pull", "always", "--user", "root",
		"--entrypoint", "devenv", image, "os-updates")
	if err != nil {
		return nil, err
	}
	return debian.ReadUpdates(out), nil
}

// ownUpdates decides which Debian updates are the job of this image. An image
// that installs the layer os upgrades all packages in its build, so all
// updates count. Any other image cannot upgrade the packages of its base
// image; the updates that the base image has too are the job of the base
// image (it rebuilds, and this image follows through the new base image
// digest). The result is nil when the check is not possible.
func ownUpdates(image string, dockerfile *Dockerfile) []debian.Update {
	updates, err := pendingUpdates(image)
	if err != nil {
		warning("Debian update check skipped: %v", err)
		return nil
	}
	if contains(dockerfile.Layers, "os") || len(updates) == 0 || dockerfile.FinalBase == "" {
		return updates
	}
	baseUpdates, err := pendingUpdates(dockerfile.FinalBase)
	if err != nil {
		warning("Debian update check skipped (base image %s): %v", dockerfile.FinalBase, err)
		return nil
	}
	return subtractUpdates(updates, baseUpdates)
}

// subtractUpdates returns the updates that are not in base (same package and
// same new version).
func subtractUpdates(updates, base []debian.Update) []debian.Update {
	inBase := map[string]bool{}
	for _, u := range base {
		inBase[u.Package+"="+u.New] = true
	}
	var result []debian.Update
	for _, u := range updates {
		if !inBase[u.Package+"="+u.New] {
			result = append(result, u)
		}
	}
	return result
}

// maxListed limits the packages named in the release reason (a point release
// can update hundreds of packages); security updates are always named.
const maxListed = 10

// updatesReason describes Debian updates for the release reason: the number of
// packages, all security updates by name, and the first other packages.
func updatesReason(updates []debian.Update) string {
	var security, other []string
	for _, u := range updates {
		if u.Security {
			security = append(security, u.String())
		} else {
			other = append(other, u.String())
		}
	}
	reason := fmt.Sprintf("Debian updates: %d package(s)", len(updates))
	if len(security) > 0 {
		reason += "; security: " + strings.Join(security, ", ")
	}
	if len(other) > 0 {
		listed := other
		if len(listed) > maxListed {
			listed = listed[:maxListed]
		}
		reason += "; other: " + strings.Join(listed, ", ")
		if len(other) > maxListed {
			reason += fmt.Sprintf(" and %d more", len(other)-maxListed)
		}
	}
	return reason
}
