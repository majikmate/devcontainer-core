package release

import (
	"errors"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// checkSupport runs the support checks of the installed layers in the newest
// image ("devcon check", for example the Debian release of the layer os). It
// returns an error when a layer has reached its end of life. When the check is
// not possible (for example an older image without "devcon check"), it only
// warns: the build runs the same checks after the installation.
func checkSupport(image string, dockerfile *Dockerfile) error {
	var names []string
	for _, name := range dockerfile.Layers {
		if l, err := layer.Get(name); err == nil && l.Check != nil {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"run", "--rm", "--pull", "always", "--entrypoint", "devcon", image, "check"}, names...)
	out, err := sys.Output("docker", args...)
	if err != nil {
		warning("Support check skipped: %v", err)
		return nil
	}
	return parseSupport(out)
}

// parseSupport reads the output of "devcon check" and returns the end-of-life
// messages as one error (nil when all layers are supported).
func parseSupport(out string) error {
	var messages []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 3)
		if len(fields) == 3 && fields[0] == "end-of-life" {
			messages = append(messages, "layer "+fields[1]+": "+fields[2])
		}
	}
	if len(messages) == 0 {
		return nil
	}
	return errors.New(strings.Join(messages, "; "))
}
