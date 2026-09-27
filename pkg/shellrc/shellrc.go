// Package shellrc manages the shell configuration of the image. Layers add
// files to two folders; bash and zsh load them in interactive shells:
//
//	/etc/devenv/shellrc.d/*.sh   bash and zsh (for example aliases)
//	/etc/devenv/zshrc.d/*.zsh    zsh only (for example the prompt)
//
// The system start files of bash and zsh get one loader block each. Layers
// never edit these start files themselves.
package shellrc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	sharedDir = "/etc/devenv/shellrc.d"
	zshDir    = "/etc/devenv/zshrc.d"

	markerBegin = "# >>> devenv >>>"
	markerEnd   = "# <<< devenv <<<"
)

const bashLoader = markerBegin + `
for devenv_rc in /etc/devenv/shellrc.d/*.sh; do [ -r "$devenv_rc" ] && . "$devenv_rc"; done
unset devenv_rc
` + markerEnd

const zshLoader = markerBegin + `
for devenv_rc in /etc/devenv/shellrc.d/*.sh(N) /etc/devenv/zshrc.d/*.zsh(N); do . "$devenv_rc"; done
unset devenv_rc
` + markerEnd

// Shared adds a configuration file for bash and zsh.
func Shared(name, content string) error {
	return add(filepath.Join(sharedDir, name+".sh"), content)
}

// Zsh adds a configuration file for zsh only.
func Zsh(name, content string) error {
	return add(filepath.Join(zshDir, name+".zsh"), content)
}

func add(path, content string) error {
	if err := ensureLoaders(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	header := fmt.Sprintf("# Written by devenv (layer file %s). Changes are lost at the next image build.\n", filepath.Base(path))
	return os.WriteFile(path, []byte(header+strings.TrimSpace(content)+"\n"), 0o644)
}

// ensureLoaders adds the loader blocks to /etc/bash.bashrc and /etc/zsh/zshrc once.
func ensureLoaders() error {
	if err := ensureBlock("/etc/bash.bashrc", bashLoader); err != nil {
		return err
	}
	return ensureBlock("/etc/zsh/zshrc", zshLoader)
}

func ensureBlock(path, block string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(data), markerBegin) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("\n" + block + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
