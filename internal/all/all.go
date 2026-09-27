// Package all registers all layers of the images: the Debian-bound layers of
// this repository (pkg/layers) and the distribution-independent features of
// devcontainer-features. The program devenv and the release tool import it.
package all

import (
	_ "github.com/majikmate/devcontainer-core/pkg/layers"
	_ "github.com/majikmate/devcontainer-features"
)
