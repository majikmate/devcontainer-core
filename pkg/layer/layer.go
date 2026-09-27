// Package layer defines what a layer is. A layer installs one part of an
// image (for example Go or the SSH server). Each layer declares:
//
//   - its build arguments (read from the environment during the build),
//   - its tools with their version sources (for the release tool),
//   - its entry of the devcontainer.metadata label (VS Code extensions and
//     settings, container options, lifecycle commands),
//   - its installation and its test,
//   - optionally a start step (run by "devcon start" when the container
//     starts) and its own devcon commands.
//
// The Debian-bound layers are in devcontainer-core, the distribution
// independent layers in devcontainer-features.
package layer

import (
	"fmt"
	"os"
	"sort"

	"github.com/majikmate/devcontainer-core/pkg/devcontainer"
)

// Arg is a build argument of a layer.
type Arg struct {
	Name    string
	Default string
	Doc     string
}

// Tool is a tool that a layer installs in a version that the release tool
// determines. The release tool passes the version as build argument Arg.
type Tool struct {
	Name   string                 // name in the release notes, for example "go"
	Arg    string                 // build argument, for example "GO_VERSION"
	Newest func() (string, error) // newest version from the source that the layer installs from
}

// Layer is one installable part of an image.
type Layer struct {
	Name    string
	Summary string
	Needs   []string // layers that must be installed before this one
	Args    []Arg
	Tools   []Tool
	// Metadata is the entry of this layer in the devcontainer.metadata label.
	// The id is set automatically ("devcon/<name>").
	Metadata devcontainer.Entry
	Install  func(*Env) error
	Test     func(*T)
	// Start runs when the container starts ("devcon start": the ENTRYPOINT
	// for docker run, postStartCommand for the Dev Containers extension and
	// Codespaces). It runs as root or as the development user; a failure is
	// reported but does not stop the container.
	Start func() error
	// Commands are devcon commands of this layer ("devcon <name> [args]").
	Commands []Command
}

// Command is a devcon command that a layer provides.
type Command struct {
	Name    string
	Summary string
	Run     func(args []string) error
}

// Entry returns the label entry of the layer, or nil if it has none.
func (l *Layer) Entry() devcontainer.Entry {
	if len(l.Metadata) == 0 {
		return nil
	}
	entry := devcontainer.Entry{"id": "devcon/" + l.Name}
	for k, v := range l.Metadata {
		entry[k] = v
	}
	return entry
}

// Env gives an installation access to its build arguments.
type Env struct {
	Layer *Layer
}

// Arg returns the value of a build argument, or its default.
func (e *Env) Arg(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	for _, a := range e.Layer.Args {
		if a.Name == name {
			return a.Default
		}
	}
	for _, t := range e.Layer.Tools {
		if t.Arg == name {
			return ""
		}
	}
	panic(fmt.Sprintf("layer %s: undeclared build argument %s", e.Layer.Name, name))
}

// Version returns the version of a tool from its build argument. Without a
// build argument (for example in a local build) it asks the version source.
func (e *Env) Version(tool string) (string, error) {
	for _, t := range e.Layer.Tools {
		if t.Name != tool {
			continue
		}
		if v := os.Getenv(t.Arg); v != "" && v != "latest" {
			return v, nil
		}
		v, err := t.Newest()
		if err != nil {
			return "", fmt.Errorf("newest version of %s: %w", tool, err)
		}
		return v, nil
	}
	panic(fmt.Sprintf("layer %s: undeclared tool %s", e.Layer.Name, tool))
}

var registry = map[string]*Layer{}

// Register adds a layer. The layers package calls it for every layer.
func Register(l *Layer) {
	if _, ok := registry[l.Name]; ok {
		panic("layer registered twice: " + l.Name)
	}
	registry[l.Name] = l
}

// Get returns a layer by name.
func Get(name string) (*Layer, error) {
	l, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown layer %q (see: devcon layers)", name)
	}
	return l, nil
}

// All returns all layers, sorted by name.
func All() []*Layer {
	var all []*Layer
	for _, l := range registry {
		all = append(all, l)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}
