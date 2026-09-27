// Package layer defines what a layer is. A layer installs one part of an
// image (for example Go or the SSH server). Each layer declares:
//
//   - its build arguments (read from the environment during the build),
//   - its tools with their version sources and the release choice of the
//     feature: pinned line and channel (see Tool, Source and Config),
//   - its entry of the devcontainer.metadata label (VS Code extensions and
//     settings, container options, lifecycle commands),
//   - its installation and its test,
//   - optionally a start step (run by "devenv start" when the container
//     starts), its own devenv commands and a support check (Check).
//
// The Debian-bound layers are in devcontainer-core, the distribution
// independent layers in devcontainer-features.
package layer

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/devcontainer"
)

// Arg is a build argument of a layer.
type Arg struct {
	Name    string
	Default string
	Doc     string
}

// Tool is a tool that a layer installs in a version that the release tool
// determines (see Resolve). The release tool passes the version as build
// argument Arg.
type Tool struct {
	Name string // name in the release notes and in customizations.devenv, for example "go"
	Arg  string // build argument, for example "GO_VERSION"
	// Source lists the releases of the tool.
	Source *Source
	// Version is the release choice of the feature: pinned line and channel.
	// Empty: the newest release of the default channel.
	Version Config
	// Follows names an earlier tool of the same layer whose version decides
	// the version of this tool, for example gopls follows go. Works reports
	// whether a release works with the version of that tool.
	Follows string
	Works   func(release, followed string) (bool, error)

	// Deprecated: the version rule before Source, used only for a tool
	// without Source (a features library that is not updated yet).
	Newest    func() (string, error)
	NewestFor func(version string) (string, error)
	Pin       *Pin
}

// Layer is one installable part of an image.
type Layer struct {
	Name    string
	Summary string
	Needs   []string // layers that must be installed before this one
	Args    []Arg
	Tools   []Tool
	// Metadata is the entry of this layer in the devcontainer.metadata label.
	// The id is set automatically ("devenv/<name>").
	Metadata devcontainer.Entry
	Install  func(*Env) error
	Test     func(*T)
	// Start runs when the container starts ("devenv start": the ENTRYPOINT
	// for docker run, postStartCommand for the Dev Containers extension and
	// Codespaces). It runs as root or as the development user; a failure is
	// reported but does not stop the container.
	Start func() error
	// Commands are devenv commands of this layer ("devenv <name> [args]").
	Commands []Command
	// Check verifies that what the layer installed is still supported, for
	// example the Debian release (layer os). It returns an *EndOfLifeError
	// at the end of life. "devenv install" runs it after the installation;
	// the release tool runs it in the newest image ("devenv check").
	Check func() error
	// Package is the Go package that registered the layer (set by Register).
	Package string
}

// Command is a devenv command that a layer provides.
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
	entry := devcontainer.Entry{"id": "devenv/" + l.Name}
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

// Version returns the version of a tool from its build argument (the release
// workflow passes it). Without a build argument (for example in a local
// build) it resolves the version with the configuration of the feature
// (Tool.Version; the overrides of an image are known only to the release
// plan). A pinned line at its end of life is an error.
func (e *Env) Version(tool string) (string, error) {
	for i := range e.Layer.Tools {
		t := &e.Layer.Tools[i]
		if t.Name != tool {
			continue
		}
		// The release plan resolved the version (with the configuration of
		// the feature and of the image), so the build takes it as it is.
		if v := os.Getenv(t.Arg); v != "" && v != "latest" {
			return v, nil
		}
		c, err := t.Effective(t.Version)
		if err != nil {
			return "", err
		}
		if err := t.CheckSupport(c); err != nil {
			return "", err
		}
		followed := ""
		if t.Follows != "" {
			v, err := e.Version(t.Follows)
			if err != nil {
				return "", err
			}
			followed = v
		}
		v, err := t.Resolve(c, followed)
		if err != nil {
			return "", fmt.Errorf("newest version of %s: %w", tool, err)
		}
		return v, nil
	}
	panic(fmt.Sprintf("layer %s: undeclared tool %s", e.Layer.Name, tool))
}

var registry = map[string]*Layer{}

// Register adds a layer. The layers packages call it for every layer. It
// records the Go package of the caller as the source of the layer.
func Register(l *Layer) {
	if _, ok := registry[l.Name]; ok {
		panic("layer registered twice: " + l.Name)
	}
	if pc, _, _, ok := runtime.Caller(1); ok && l.Package == "" {
		l.Package = packageOf(runtime.FuncForPC(pc).Name())
	}
	registry[l.Name] = l
}

// packageOf returns the package path of a function name, for example
// "github.com/majikmate/devcontainer-features" for
// "github.com/majikmate/devcontainer-features.init.0".
func packageOf(function string) string {
	slash := strings.LastIndex(function, "/")
	if dot := strings.Index(function[slash+1:], "."); dot >= 0 {
		return function[:slash+1+dot]
	}
	return function
}

// Get returns a layer by name.
func Get(name string) (*Layer, error) {
	l, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown layer %q (see: devenv layers)", name)
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
