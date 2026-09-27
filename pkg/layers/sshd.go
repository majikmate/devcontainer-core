package layers

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/majikmate/devcontainer-core/pkg/debian"
	"github.com/majikmate/devcontainer-core/pkg/devcontainer"
	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/state"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// SSHPort is the port of the SSH server.
const SSHPort = "2222"

const sshdConfig = `# SSH server of the Dev Container images (devcon layer sshd).
# This file comes first in the Include order, and the first value of a
# setting wins, so these settings cannot be weakened by later files.
Port ` + SSHPort + `
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitEmptyPasswords no
PubkeyAuthentication yes
AuthorizedKeysFile .ssh/authorized_keys
UsePAM yes
`

func init() {
	layer.Register(&layer.Layer{
		Name:    "sshd",
		Summary: "SSH server on port 2222 (keys only, no root) with the keys of the owner's GitHub account",
		Needs:   []string{"user"},
		Metadata: devcontainer.Entry{
			// VS Code copies ~/.gitconfig (with github.user) when it connects
			"postAttachCommand": "devcon ssh-keys",
		},
		Start: startSSH,
		Commands: []layer.Command{
			{Name: "ssh-keys", Summary: "load the SSH keys of the owner's GitHub account", Run: func([]string) error { return LoadSSHKeys() }},
			{Name: "sshd-start", Summary: "start the SSH server (root)", Run: func([]string) error { return SSHDStart() }},
		},
		Install: func(e *layer.Env) error {
			if err := debian.AptInstall("openssh-server"); err != nil {
				return err
			}
			if err := sys.WriteFile("/etc/ssh/sshd_config.d/00-devcon.conf", sshdConfig, 0o644); err != nil {
				return err
			}
			// No host keys in the image: each container creates its own
			keys, _ := filepath.Glob("/etc/ssh/ssh_host_*")
			for _, k := range keys {
				if err := os.Remove(k); err != nil {
					return err
				}
			}
			return debian.AptClean()
		},
		Test: testSSHD,
	})
}

func testSSHD(t *layer.T) {
	keys, _ := filepath.Glob("/etc/ssh/ssh_host_*")
	t.Check("no host keys in the image", len(keys) == 0)
	t.Command("server starts", "sudo", "-n", "devcon", "sshd-start")
	config := t.Output("effective configuration", "sudo", "-n", "/usr/sbin/sshd", "-T")
	for _, line := range []string{"port " + SSHPort, "permitrootlogin no", "passwordauthentication no", "kbdinteractiveauthentication no"} {
		t.Check(line, containsLine(config, line))
	}

	// Key login end to end, with a temporary key pair
	dir, cleanup, err := sys.TempDir()
	if err != nil {
		t.Check("temporary folder", false)
		return
	}
	defer cleanup()
	home, _ := os.UserHomeDir()
	authorized := filepath.Join(home, ".ssh", "authorized_keys")
	previous, readErr := os.ReadFile(authorized)
	t.Command("create a test key", "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", filepath.Join(dir, "id"))
	public, _ := os.ReadFile(filepath.Join(dir, "id.pub"))
	_ = os.MkdirAll(filepath.Dir(authorized), 0o700)
	_ = os.WriteFile(authorized, public, 0o600)
	t.Command("login with a key", "ssh", "-p", SSHPort, "-i", filepath.Join(dir, "id"),
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		state.User()+"@127.0.0.1", "true")
	if readErr == nil {
		_ = os.WriteFile(authorized, previous, 0o600)
	} else {
		_ = os.Remove(authorized)
	}

	cmd := exec.Command("devcon", "ssh-keys")
	cmd.Env = append(filterEnv(os.Environ(), "GITHUB_USER", "HOME"), "HOME=/nonexistent")
	out, _ := cmd.CombinedOutput()
	t.Check("no GitHub user: keys unchanged", strings.Contains(string(out), "no GitHub user"))
	cmd = exec.Command("devcon", "ssh-keys")
	cmd.Env = append(filterEnv(os.Environ(), "GITHUB_USER"), "GITHUB_USER=x;y")
	out, _ = cmd.CombinedOutput()
	t.Check("invalid GitHub user name rejected", strings.Contains(string(out), "invalid GitHub user name"))

	v, _ := sys.Output("dpkg-query", "-W", "-f", "${Version}", "openssh-server")
	t.Version("openssh", v)
}

// SSHDStart starts the SSH server of the container and creates its host keys
// at the first start. Needs root.
func SSHDStart() error {
	if !sys.IsRoot() {
		return fmt.Errorf("sshd-start needs root (sudo devcon sshd-start)")
	}
	if err := sys.Run(nil, "ssh-keygen", "-A"); err != nil {
		return err
	}
	if err := os.MkdirAll("/run/sshd", 0o755); err != nil {
		return err
	}
	if exec.Command("pgrep", "-x", "sshd").Run() == nil {
		fmt.Println("SSH server is already running (port " + SSHPort + ").")
		return nil
	}
	if err := sys.Run(nil, "/usr/sbin/sshd"); err != nil {
		return err
	}
	fmt.Println("SSH server started (port " + SSHPort + ", key login only).")
	return nil
}

var githubUserName = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)

// LoadSSHKeys loads the public SSH keys of the container owner's GitHub account
// into ~/.ssh/authorized_keys of the development user. The GitHub user name
// comes from (first match wins): GITHUB_USER, the Codespaces environment file,
// the git setting github.user. The file is replaced, so a key that the owner
// deletes on GitHub stops working. Without a GitHub user, or when GitHub cannot
// be reached, the file stays unchanged. Runs as the user or as root.
func LoadSSHKeys() error {
	u, err := sys.LookupUser(state.User())
	if err != nil {
		return err
	}
	name := githubUser(u)
	if name == "" {
		fmt.Println("devcon ssh-keys: no GitHub user known (GITHUB_USER or git config github.user); keys unchanged.")
		return nil
	}
	if !githubUserName.MatchString(name) {
		fmt.Printf("devcon ssh-keys: invalid GitHub user name %q; keys unchanged.\n", name)
		return nil
	}
	data, err := getWithTimeout("https://github.com/"+name+".keys", 10*time.Second)
	keys := strings.TrimSpace(string(data))
	if err != nil || keys == "" {
		fmt.Printf("devcon ssh-keys: no keys received for GitHub user %s; keys unchanged.\n", name)
		return nil
	}
	dir := filepath.Join(u.Home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".authorized_keys.new")
	if err := os.WriteFile(tmp, []byte(keys+"\n"), 0o600); err != nil {
		return err
	}
	if sys.IsRoot() {
		for _, p := range []string{dir, tmp} {
			if err := os.Chown(p, u.UID, u.GID); err != nil {
				return err
			}
		}
	}
	if err := os.Rename(tmp, filepath.Join(dir, "authorized_keys")); err != nil {
		return err
	}
	fmt.Printf("devcon ssh-keys: %d key(s) of GitHub user %s loaded.\n", len(strings.Split(keys, "\n")), name)
	return nil
}

func githubUser(u *sys.User) string {
	if name := strings.TrimSpace(os.Getenv("GITHUB_USER")); name != "" {
		return name
	}
	if f, err := os.Open("/workspaces/.codespaces/shared/.env"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if v, ok := strings.CutPrefix(scanner.Text(), "GITHUB_USER="); ok {
				return strings.Trim(strings.TrimSpace(v), `"`)
			}
		}
	}
	cmd := exec.Command("git", "config", "--file", filepath.Join(u.Home, ".gitconfig"), "--get", "github.user")
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// startSSH is the start step of the layer: it starts the SSH server and loads
// the SSH keys. docker run: the ENTRYPOINT runs it as root. Dev Containers
// extension and Codespaces: postStartCommand runs it as the development user.
func startSSH() error {
	var err error
	if sys.IsRoot() {
		err = SSHDStart()
	} else {
		err = sys.Run(nil, "sudo", "-n", "devcon", "sshd-start")
	}
	if err != nil {
		err = fmt.Errorf("the SSH server did not start: %w", err)
	}
	if keyErr := LoadSSHKeys(); keyErr != nil && err == nil {
		err = fmt.Errorf("the SSH keys were not loaded: %w", keyErr)
	}
	return err
}

func containsLine(text, line string) bool {
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}

func filterEnv(env []string, names ...string) []string {
	var result []string
	for _, e := range env {
		keep := true
		for _, n := range names {
			if strings.HasPrefix(e, n+"=") {
				keep = false
			}
		}
		if keep {
			result = append(result, e)
		}
	}
	return result
}
