// Command devenv installs, tests and starts the layers of a Dev Container image.
//
// Build (in a Dockerfile, as root):
//
//	devenv install <layer>...   install layers (settings from build arguments)
//	devenv layers [--markdown]  list all layers (--markdown: documentation)
//
// Release workflow (in the built image, as the development user):
//
//	devenv test [<layer>...]    test the installed layers (default: all)
//	devenv metadata             print the label entries of the installed layers
//	devenv check [<layer>...]   check the support of the installed layers (end of life)
//
// Container:
//
//	devenv start [<command>...] run the start steps of the installed layers, then <command>
//
// Layers add their own commands, for example "devenv ssh-keys" (layer sshd) or
// "devenv os-updates" (layer os); "devenv help" lists them.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	_ "github.com/majikmate/devcontainer-core/internal/all" // registers the layers and features
	"github.com/majikmate/devcontainer-core/pkg/devcontainer"
	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/state"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "install":
		err = install(args)
	case "test":
		err = test(args)
	case "metadata":
		err = metadata()
	case "check":
		err = check(args)
	case "layers":
		if len(args) > 0 && args[0] == "--markdown" {
			markdownLayers()
		} else {
			listLayers()
		}
	case "start":
		start()
		if len(args) > 0 {
			err = execCommand(args)
		}
	case "help", "-h", "--help":
		usage()
	default:
		err = layerCommand(cmd, args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "devenv:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: devenv <command> [arguments]

  install <layer>...    install layers (build, root)
  layers [--markdown]   list all layers (--markdown: documentation of all layers)
  test [<layer>...]     test the installed layers
  metadata              print the devcontainer.metadata entries of the installed layers
  check [<layer>...]    check the support of the installed layers (for example the Debian release)
  start [<command>...]  run the start steps of the installed layers, then run <command>

Commands of the layers:`)
	for _, l := range layer.All() {
		for _, c := range l.Commands {
			fmt.Fprintf(os.Stderr, "  %-21s %s (layer %s)\n", c.Name, c.Summary, l.Name)
		}
	}
}

func install(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("install: name at least one layer")
	}
	for _, name := range names {
		l, err := layer.Get(name)
		if err != nil {
			return err
		}
		for _, need := range l.Needs {
			if !state.IsInstalled(need) {
				return fmt.Errorf("layer %s needs layer %s; install it first", name, need)
			}
		}
		fmt.Printf("\n=== Layer %s: %s ===\n", l.Name, l.Summary)
		if err := l.Install(&layer.Env{Layer: l}); err != nil {
			return fmt.Errorf("layer %s: %w", name, err)
		}
		if l.Check != nil {
			if err := l.Check(); err != nil {
				return fmt.Errorf("layer %s: %w", name, err)
			}
		}
		if err := state.MarkInstalled(name); err != nil {
			return err
		}
		fmt.Printf("=== Layer %s installed ===\n", l.Name)
	}
	return nil
}

func test(names []string) error {
	if len(names) == 0 {
		names = state.Installed()
	}
	if len(names) == 0 {
		return fmt.Errorf("no installed layers")
	}
	var failed []string
	for _, name := range names {
		l, err := layer.Get(name)
		if err != nil {
			return err
		}
		fmt.Printf("--- Test of layer %s\n", name)
		t := layer.NewT()
		l.Test(t)
		if t.Failures > 0 {
			failed = append(failed, name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed layers: %s", strings.Join(failed, ", "))
	}
	fmt.Println("All layer tests passed.")
	return nil
}

// check runs the support checks of layers (default: the installed layers)
// and prints one line per layer with a check, for the release tool:
//
//	supported<TAB><layer>
//	end-of-life<TAB><layer><TAB><message>
//
// An end of life is a result, not an error; an error means that the support
// could not be checked.
func check(names []string) error {
	if len(names) == 0 {
		names = state.Installed()
	}
	for _, name := range names {
		l, err := layer.Get(name)
		if err != nil {
			return err
		}
		if l.Check == nil {
			continue
		}
		err = l.Check()
		if eol := (*layer.EndOfLifeError)(nil); errors.As(err, &eol) {
			fmt.Printf("end-of-life\t%s\t%s\n", name, eol.Error())
			continue
		}
		if err != nil {
			return fmt.Errorf("layer %s: %w", name, err)
		}
		fmt.Printf("supported\t%s\n", name)
	}
	return nil
}

// startEntry is the label entry that runs the start steps. The framework adds
// it once, for all layers with a start step: the Dev Containers extension and
// Codespaces do not use the ENTRYPOINT of the image.
var startEntry = devcontainer.Entry{"id": "devenv/start", "postStartCommand": "devenv start"}

func installedLayers() []*layer.Layer {
	var result []*layer.Layer
	for _, name := range state.Installed() {
		if l, err := layer.Get(name); err == nil {
			result = append(result, l)
		}
	}
	return result
}

func metadata() error {
	entries := []devcontainer.Entry{}
	hasStart := false
	for _, l := range installedLayers() {
		if e := l.Entry(); e != nil {
			entries = append(entries, e)
		}
		hasStart = hasStart || l.Start != nil
	}
	if hasStart {
		entries = append(entries, startEntry)
	}
	out := json.NewEncoder(os.Stdout)
	out.SetIndent("", "  ")
	return out.Encode(entries)
}

// start runs the start steps of the installed layers, in installation order.
// A failure is reported and does not stop the container.
func start() {
	for _, l := range installedLayers() {
		if l.Start == nil {
			continue
		}
		if err := l.Start(); err != nil {
			fmt.Printf("devenv start: layer %s: %v\n", l.Name, err)
		}
	}
}

// layerCommand runs a command of a layer. Commands of installed layers come
// first; a command of a layer that is not installed is an error.
func layerCommand(name string, args []string) error {
	for _, l := range installedLayers() {
		for _, c := range l.Commands {
			if c.Name == name {
				return c.Run(args)
			}
		}
	}
	for _, l := range layer.All() {
		for _, c := range l.Commands {
			if c.Name == name {
				return fmt.Errorf("command %q belongs to layer %s, which is not installed", name, l.Name)
			}
		}
	}
	usage()
	return fmt.Errorf("unknown command %q", name)
}

func listLayers() {
	installed := map[string]bool{}
	for _, n := range state.Installed() {
		installed[n] = true
	}
	for _, l := range layer.All() {
		mark := " "
		if installed[l.Name] {
			mark = "*"
		}
		fmt.Printf("%s %-16s %s\n", mark, l.Name, l.Summary)
	}
	fmt.Println("\n* installed in this image")
}

// execCommand replaces devenv with the command of the container (for example
// "sleep infinity"), so the command runs as the main process.
func execCommand(args []string) error {
	path, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, args, os.Environ())
}

// repositoryLink returns a Markdown link to the GitHub repository of a Go
// package, for example [devcontainer-features](https://github.com/...).
func repositoryLink(pkg string) string {
	parts := strings.Split(pkg, "/")
	if len(parts) < 3 {
		return pkg
	}
	return fmt.Sprintf("[%s](https://%s)", parts[2], strings.Join(parts[:3], "/"))
}

// markdownLayers prints the documentation of all layers (docs/layers.md is
// generated with it), so the documentation always matches the code.
func markdownLayers() {
	fmt.Println("# Layers")
	fmt.Println()
	fmt.Println("Generated from the code with `go run ./cmd/devenv layers --markdown > docs/layers.md`.")
	fmt.Println("Do not edit this file by hand.")
	fmt.Println()
	fmt.Println("The Debian-bound layers are in devcontainer-core (`pkg/layers`), the")
	fmt.Println("distribution-independent features in devcontainer-features. This list shows")
	fmt.Println("the features version of `go.mod`; the images use the newest features version.")
	fmt.Println()
	fmt.Println("| Layer | Content | Needs | Repository |")
	fmt.Println("| ----- | ------- | ----- | ---------- |")
	for _, l := range layer.All() {
		fmt.Printf("| [`%s`](#%s) | %s | %s | %s |\n", l.Name, l.Name, l.Summary, strings.Join(l.Needs, ", "), repositoryLink(l.Package))
	}
	for _, l := range layer.All() {
		fmt.Printf("\n## %s\n\n%s.\n\n", l.Name, strings.ToUpper(l.Summary[:1])+l.Summary[1:])
		fmt.Printf("Dockerfile: `RUN devenv install %s`\n\n", l.Name)
		fmt.Printf("Source: %s (`%s`)\n\n", repositoryLink(l.Package), l.Package)
		if len(l.Needs) > 0 {
			fmt.Printf("Needs the layers: %s.\n\n", strings.Join(l.Needs, ", "))
		}
		if len(l.Args) > 0 {
			fmt.Println("Build arguments:")
			fmt.Println()
			for _, a := range l.Args {
				fmt.Printf("- `%s` (default `%s`): %s\n", a.Name, a.Default, a.Doc)
			}
			fmt.Println()
		}
		if len(l.Tools) > 0 {
			fmt.Println("Tools with versions (the release workflow chooses the version with the configuration of the feature and the overrides in `customizations.devenv` of the devcontainer.json, and passes it as build argument):")
			fmt.Println()
			for _, t := range l.Tools {
				fmt.Printf("- %s: `%s`", t.Name, t.Arg)
				if s := t.Source; s != nil {
					fmt.Printf("; source: %s", s.Name)
					pin := "none (the newest release)"
					if t.Version.Pin != "" {
						pin = "`" + t.Version.Pin + "`"
					}
					fmt.Printf("; pinned line: %s", pin)
					if len(s.Channels) > 0 {
						var channels []string
						for _, c := range s.Channels {
							channels = append(channels, fmt.Sprintf("`%s` (%s)", c.Name, c.Label))
						}
						channel := t.Version.Channel
						if channel == "" {
							channel = s.Channels[0].Name
						}
						fmt.Printf("; channel: `%s` (channels: %s)", channel, strings.Join(channels, ", "))
					}
					if s.Policy != "" {
						fmt.Printf("; %s", s.Policy)
					}
				}
				if t.Follows != "" {
					fmt.Printf("; follows %s: the newest version that works with the installed %s", t.Follows, t.Follows)
				}
				fmt.Println()
			}
			fmt.Println()
			for _, t := range l.Tools {
				if t.Source != nil && t.Source.Support != nil {
					fmt.Println("With a pin, the layer installs the newest release inside the pinned line. When the line reaches its end of life, the build fails and names the supported lines.")
					fmt.Println()
					break
				}
			}
		}
		if l.Check != nil {
			fmt.Println("Has a support check: the installation and the nightly release check fail when the installed release has reached its end of life (`devenv check`).")
			fmt.Println()
		}
		if l.Start != nil {
			fmt.Println("Has a start step: `devenv start` runs it when the container starts.")
			fmt.Println()
		}
		if len(l.Commands) > 0 {
			fmt.Println("Commands:")
			fmt.Println()
			for _, c := range l.Commands {
				fmt.Printf("- `devenv %s`: %s\n", c.Name, c.Summary)
			}
			fmt.Println()
		}
		if e := l.Entry(); e != nil {
			data, _ := json.MarshalIndent(e, "", "  ")
			fmt.Println("Entry in the image label `devcontainer.metadata`:")
			fmt.Println()
			fmt.Println("```json")
			fmt.Println(string(data))
			fmt.Println("```")
		}
	}
	fmt.Println()
	fmt.Println("## Start step (framework)")
	fmt.Println()
	fmt.Println("When at least one installed layer has a start step, `devenv metadata` adds this entry once:")
	fmt.Println()
	data, _ := json.MarshalIndent(startEntry, "", "  ")
	fmt.Println("```json")
	fmt.Println(string(data))
	fmt.Println("```")
}
