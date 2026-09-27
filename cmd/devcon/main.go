// Command devcon installs, tests and starts the layers of a Dev Container image.
//
// Build (in a Dockerfile, as root):
//
//	devcon install <layer>...   install layers (settings from build arguments)
//	devcon layers [--markdown]  list all layers (--markdown: documentation)
//
// Release workflow (in the built image, as the development user):
//
//	devcon test [<layer>...]    test the installed layers (default: all)
//	devcon metadata             print the label entries of the installed layers
//
// Container (at start and at runtime):
//
//	devcon start [<command>...] start the SSH server, load the SSH keys, then run <command>
//	devcon ssh-keys             load the SSH keys of the owner's GitHub account
//	devcon sshd-start           start the SSH server (root)
//	devcon update-os            upgrade the Debian packages (root)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/majikmate/devcontainer-core/internal/devcontainer"
	"github.com/majikmate/devcontainer-core/internal/layer"
	"github.com/majikmate/devcontainer-core/internal/layers"
	"github.com/majikmate/devcontainer-core/internal/state"
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
	case "layers":
		if len(args) > 0 && args[0] == "--markdown" {
			markdownLayers()
		} else {
			listLayers()
		}
	case "start":
		layers.Start()
		if len(args) > 0 {
			err = execCommand(args)
		}
	case "ssh-keys":
		err = layers.LoadSSHKeys()
	case "sshd-start":
		err = layers.SSHDStart()
	case "update-os":
		err = layers.UpdateOS()
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "devcon: unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "devcon:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: devcon <command> [arguments]

  install <layer>...    install layers (build, root)
  layers [--markdown]   list all layers (--markdown: documentation of all layers)
  test [<layer>...]     test the installed layers
  metadata              print the devcontainer.metadata entries of the installed layers
  start [<command>...]  start the SSH server and load the SSH keys, then run <command>
  ssh-keys              load the SSH keys of the owner's GitHub account
  sshd-start            start the SSH server (root)
  update-os             upgrade the Debian packages (root)`)
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

func metadata() error {
	var entries []devcontainer.Entry
	for _, name := range state.Installed() {
		l, err := layer.Get(name)
		if err != nil {
			return err
		}
		if e := l.Entry(); e != nil {
			entries = append(entries, e)
		}
	}
	if entries == nil {
		entries = []devcontainer.Entry{}
	}
	out := json.NewEncoder(os.Stdout)
	out.SetIndent("", "  ")
	return out.Encode(entries)
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

// execCommand replaces devcon with the command of the container (for example
// "sleep infinity"), so the command runs as the main process.
func execCommand(args []string) error {
	path, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, args, os.Environ())
}

// markdownLayers prints the documentation of all layers (docs/layers.md is
// generated with it), so the documentation always matches the code.
func markdownLayers() {
	fmt.Println("# Layers")
	fmt.Println()
	fmt.Println("Generated from the code with `go run ./cmd/devcon layers --markdown > docs/layers.md`.")
	fmt.Println("Do not edit this file by hand.")
	fmt.Println()
	fmt.Println("| Layer | Content | Needs |")
	fmt.Println("| ----- | ------- | ----- |")
	for _, l := range layer.All() {
		fmt.Printf("| [`%s`](#%s) | %s | %s |\n", l.Name, l.Name, l.Summary, strings.Join(l.Needs, ", "))
	}
	for _, l := range layer.All() {
		fmt.Printf("\n## %s\n\n%s.\n\n", l.Name, strings.ToUpper(l.Summary[:1])+l.Summary[1:])
		fmt.Printf("Dockerfile: `RUN devcon install %s`\n\n", l.Name)
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
			fmt.Println("Tools with versions (the release workflow passes the newest version as build argument; without it the layer installs the newest version):")
			fmt.Println()
			for _, t := range l.Tools {
				fmt.Printf("- %s: `%s`\n", t.Name, t.Arg)
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
}
