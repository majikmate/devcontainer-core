package layers

import (
	"github.com/majikmate/devcontainer-core/pkg/debian"
	"github.com/majikmate/devcontainer-core/pkg/layer"
)

// Compilers and tools that native modules need when they are built from
// source, for example npm modules with node-gyp.
var buildToolsPackages = []string{"make", "gcc", "g++", "python3-minimal"}

func init() {
	layer.Register(&layer.Layer{
		Name:    "build-tools",
		Summary: "make, gcc, g++ and python3 for native modules (for example node-gyp)",
		Needs:   []string{"os"},
		Install: func(e *layer.Env) error {
			if err := debian.AptInstall(buildToolsPackages...); err != nil {
				return err
			}
			return debian.AptClean()
		},
		Test: func(t *layer.T) {
			for _, cmd := range []string{"make", "gcc", "g++", "python3"} {
				t.HasCommand(cmd)
			}
			t.Version("gcc", layer.LastWord(layer.FirstLine(t.Output("gcc version", "gcc", "--version"))))
		},
	})
}
