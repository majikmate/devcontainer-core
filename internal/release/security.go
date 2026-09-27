package release

import (
	"fmt"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/debian"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// securityUpdates returns the pending Debian security updates of an image: it
// starts the image as root and runs "devcon os-updates --security" (layer
// os). The command only simulates the upgrade; the image is not changed.
func securityUpdates(image string) ([]debian.Update, error) {
	out, err := sys.Output("docker", "run", "--rm", "--pull", "always", "--user", "root",
		"--entrypoint", "devcon", image, "os-updates", "--security")
	if err != nil {
		return nil, err
	}
	var updates []debian.Update
	for _, u := range debian.ReadUpdates(out) {
		if u.Security {
			updates = append(updates, u)
		}
	}
	return updates, nil
}

// ownSecurityUpdates decides which security updates are the job of this
// image. An image that installs the layer os upgrades all packages in its
// build, so all updates count. Any other image cannot upgrade the packages of
// its base image; the updates that the base image has too are the job of the
// base image (it rebuilds, and this image follows through the new base image
// digest). The result is nil when the check is not possible.
func ownSecurityUpdates(image string, dockerfile *Dockerfile) []debian.Update {
	updates, err := securityUpdates(image)
	if err != nil {
		warning("Security check skipped: %v", err)
		return nil
	}
	if contains(dockerfile.Layers, "os") || len(updates) == 0 {
		return updates
	}
	if dockerfile.FinalBase == "" {
		return updates
	}
	baseUpdates, err := securityUpdates(dockerfile.FinalBase)
	if err != nil {
		warning("Security check skipped (base image %s): %v", dockerfile.FinalBase, err)
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

// securityReason describes security updates for the release reason.
func securityReason(updates []debian.Update) string {
	var parts []string
	for _, u := range updates {
		parts = append(parts, u.String())
	}
	return fmt.Sprintf("Debian security updates: %s", strings.Join(parts, ", "))
}
