package layers

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/majikmate/devcontainer-core/internal/devcontainer"
	"github.com/majikmate/devcontainer-core/internal/layer"
	"github.com/majikmate/devcontainer-core/internal/shellrc"
	"github.com/majikmate/devcontainer-core/internal/state"
	"github.com/majikmate/devcontainer-core/internal/sys"
)

func init() {
	layer.Register(&layer.Layer{
		Name:    "user",
		Summary: "creates the development user with zsh and sudo",
		Needs:   []string{"os"},
		Args: []layer.Arg{
			{Name: "USERNAME", Default: "dev", Doc: "name of the development user"},
			{Name: "USER_UID", Default: "1000", Doc: "user id"},
			{Name: "USER_GID", Default: "1000", Doc: "group id"},
		},
		Metadata: devcontainer.Entry{
			"remoteUser": "dev",
			"customizations": devcontainer.VSCode(nil, map[string]any{
				"terminal.integrated.defaultProfile.linux": "zsh",
			}),
		},
		Install: installUser,
		Test: func(t *layer.T) {
			name := state.User()
			current, _ := user.Current()
			t.Check("tests run as "+name, current != nil && current.Username == name)
			t.Check("user id 1000", os.Getuid() == 1000)
			t.Check("group id 1000", os.Getgid() == 1000)
			shell, _ := sys.Output("getent", "passwd", name)
			t.Check("default shell zsh", filepath.Base(lastField(shell)) == "zsh")
			t.Command("sudo without password", "sudo", "-n", "true")
			home, _ := os.UserHomeDir()
			t.Check("~/.zshrc exists (no zsh setup wizard)", sys.Exists(filepath.Join(home, ".zshrc")))
			t.Command("~/.local/bin in PATH of zsh", "zsh", "-ic", `[[ ":$PATH:" == *":$HOME/.local/bin:"* ]]`)
		},
	})
}

func installUser(e *layer.Env) error {
	name, uid, gid := e.Arg("USERNAME"), e.Arg("USER_UID"), e.Arg("USER_GID")
	if err := sys.Run(nil, "groupadd", "--gid", gid, name); err != nil {
		return err
	}
	if err := sys.Run(nil, "useradd", "--uid", uid, "--gid", name, "--shell", "/usr/bin/zsh", "--create-home", name); err != nil {
		return err
	}
	if err := state.SetUser(name); err != nil {
		return err
	}
	if err := sys.WriteFile("/etc/sudoers.d/"+name, fmt.Sprintf("%s ALL=(root) NOPASSWD:ALL\n", name), 0o440); err != nil {
		return err
	}
	u, err := sys.LookupUser(name)
	if err != nil {
		return err
	}
	// An empty ~/.zshrc prevents the zsh setup wizard at the first start
	for _, home := range []string{u.Home, "/root"} {
		rc := filepath.Join(home, ".zshrc")
		if !sys.Exists(rc) {
			if err := sys.WriteFile(rc, "# Personal zsh settings\n", 0o644); err != nil {
				return err
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(u.Home, ".local", "bin"), 0o755); err != nil {
		return err
	}
	if err := sys.ChownTree(u.Home, u.UID, u.GID); err != nil {
		return err
	}
	// Programs that a user installs for himself are in ~/.local/bin
	return shellrc.Shared("path", `case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) PATH="$HOME/.local/bin:$PATH" ;;
esac`)
}

func lastField(passwdLine string) string {
	for i := len(passwdLine) - 1; i >= 0; i-- {
		if passwdLine[i] == ':' {
			return passwdLine[i+1:]
		}
	}
	return passwdLine
}
