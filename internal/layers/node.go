package layers

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/majikmate/devcontainer-core/internal/layer"
	"github.com/majikmate/devcontainer-core/internal/shellrc"
	"github.com/majikmate/devcontainer-core/internal/state"
	"github.com/majikmate/devcontainer-core/internal/sys"
	"github.com/majikmate/devcontainer-core/internal/versions"
)

// The nvm folder. The Dockerfile sets:
//
//	ENV NVM_DIR=/usr/local/share/nvm NVM_SYMLINK_CURRENT=true PATH=/usr/local/share/nvm/current/bin:$PATH
const nvmDir = "/usr/local/share/nvm"

// The nvm files that a shell loads (nvm is a shell function).
var nvmFiles = []string{"nvm.sh", "nvm-exec", "bash_completion"}

func init() {
	layer.Register(&layer.Layer{
		Name:    "node",
		Summary: "nvm, the newest Node.js LTS release and npm",
		Needs:   []string{"user"},
		Tools: []layer.Tool{
			{Name: "nvm", Arg: "NVM_VERSION", Newest: func() (string, error) { return versions.GitHubRelease("nvm-sh/nvm") }},
			{Name: "node", Arg: "NODE_VERSION", Newest: versions.NodeLTS},
		},
		Install: installNode,
		Test: func(t *layer.T) {
			for _, cmd := range []string{"node", "npm", "npx", "make", "g++", "python3"} {
				t.HasCommand(cmd)
			}
			t.Check("default Node.js is an LTS release", t.Output("node LTS", "node", "-p", "process.release.lts ? 'yes' : 'no'") == "yes")
			t.Command("user can write to the nvm folder", "test", "-w", filepath.Join(nvmDir, "versions"))
			nvm := t.Output("nvm loads", "bash", "-c", `. "$NVM_DIR/nvm.sh" && nvm --version`)
			t.Version("node", t.Output("node version", "node", "--version"))
			t.Version("npm", t.Output("npm version", "npm", "--version"))
			t.Version("nvm", "v"+nvm)
		},
	})
}

func installNode(e *layer.Env) error {
	// Build tools for native npm modules (node-gyp)
	if err := sys.AptInstall("make", "gcc", "g++", "python3-minimal"); err != nil {
		return err
	}
	nvmVersion, err := e.Version("nvm")
	if err != nil {
		return err
	}
	nodeVersion, err := e.Version("node")
	if err != nil {
		return err
	}
	nvmVersion = "v" + strings.TrimPrefix(nvmVersion, "v")
	sys.Logf("Installing nvm %s, Node.js %s", nvmVersion, nodeVersion)

	// The group nvm owns the nvm folder, so the development user can install
	// Node.js versions and global npm packages.
	user := state.User()
	if err := sys.Run(nil, "groupadd", "--system", "-f", "nvm"); err != nil {
		return err
	}
	if err := sys.Run(nil, "usermod", "-a", "-G", "nvm", user); err != nil {
		return err
	}
	if err := os.MkdirAll(nvmDir, 0o755); err != nil {
		return err
	}
	for _, file := range nvmFiles {
		url := "https://raw.githubusercontent.com/nvm-sh/nvm/" + nvmVersion + "/" + file
		if err := sys.Download(url, filepath.Join(nvmDir, file)); err != nil {
			return err
		}
	}
	u, err := sys.LookupUser(user)
	if err != nil {
		return err
	}
	group, err := groupID("nvm")
	if err != nil {
		return err
	}
	if err := sys.ShareWithGroup(nvmDir, u.UID, group); err != nil {
		return err
	}

	// nvm installs Node.js (it checks the SHA-256 checksum of the download).
	// It runs as the development user; umask 0002 keeps the files writable
	// for the group nvm.
	script := `set -e
umask 0002
. "$NVM_DIR/nvm.sh"
nvm install "$1"
nvm alias default "$1"
nvm use default
nvm cache clear
npm cache clean --force`
	env := []string{"NVM_DIR=" + nvmDir, "NVM_SYMLINK_CURRENT=true"}
	if err := sys.RunAs(user, env, "bash", "-c", script, "nvm-install", nodeVersion); err != nil {
		return err
	}
	if err := sys.ShareWithGroup(nvmDir, u.UID, group); err != nil {
		return err
	}

	// nvm is a shell function: interactive shells load it
	if err := shellrc.Shared("nvm", `export NVM_DIR="`+nvmDir+`"
[ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh"
[ -n "$BASH_VERSION" ] && [ -s "$NVM_DIR/bash_completion" ] && . "$NVM_DIR/bash_completion"`); err != nil {
		return err
	}
	return sys.AptClean()
}
