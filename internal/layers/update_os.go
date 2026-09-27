package layers

import (
	"fmt"

	"github.com/majikmate/devcontainer-core/internal/devcontainer"
	"github.com/majikmate/devcontainer-core/internal/layer"
	"github.com/majikmate/devcontainer-core/internal/sys"
)

// The command "devcon update-os" upgrades the Debian packages of a running
// container. This layer runs it when a container is created. The build itself
// always upgrades the packages (layer os).
func init() {
	layer.Register(&layer.Layer{
		Name:    "update-os",
		Summary: "upgrades the Debian packages when a container is created",
		Needs:   []string{"user"},
		Metadata: devcontainer.Entry{
			"onCreateCommand": "sudo devcon update-os",
		},
		Install: func(e *layer.Env) error { return nil },
		Test: func(t *layer.T) {
			t.HasCommand("devcon")
		},
	})
}

// UpdateOS upgrades the Debian packages of the running container. Needs root.
func UpdateOS() error {
	if !sys.IsRoot() {
		return fmt.Errorf("update-os needs root (sudo devcon update-os)")
	}
	fmt.Println("Upgrading the operating system packages ...")
	if err := sys.AptUpdate(); err != nil {
		return err
	}
	if err := sys.AptUpgrade(); err != nil {
		return err
	}
	return sys.AptClean()
}
