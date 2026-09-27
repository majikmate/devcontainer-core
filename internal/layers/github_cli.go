package layers

import (
	"os"
	"runtime"
	"strings"

	"github.com/majikmate/devcontainer-core/internal/layer"
	"github.com/majikmate/devcontainer-core/internal/sys"
	"github.com/majikmate/devcontainer-core/internal/versions"
)

const githubCLIKeyring = "/etc/apt/keyrings/githubcli-archive-keyring.gpg"

func init() {
	layer.Register(&layer.Layer{
		Name:    "github-cli",
		Summary: "GitHub CLI (gh) from its signed Debian package source",
		Needs:   []string{"os"},
		Tools: []layer.Tool{
			{Name: "gh", Arg: "GITHUB_CLI_VERSION", Newest: func() (string, error) { return versions.GitHubRelease("cli/cli") }},
		},
		Install: func(e *layer.Env) error {
			version, err := e.Version("gh")
			if err != nil {
				return err
			}
			if err := os.MkdirAll("/etc/apt/keyrings", 0o755); err != nil {
				return err
			}
			if err := sys.Download("https://cli.github.com/packages/githubcli-archive-keyring.gpg", githubCLIKeyring); err != nil {
				return err
			}
			if err := os.Chmod(githubCLIKeyring, 0o644); err != nil {
				return err
			}
			source := "deb [arch=" + runtime.GOARCH + " signed-by=" + githubCLIKeyring + "] https://cli.github.com/packages stable main\n"
			if err := sys.WriteFile("/etc/apt/sources.list.d/github-cli.list", source, 0o644); err != nil {
				return err
			}
			// apt checks the signature of the package lists and the package checksums
			if err := sys.AptUpdate(); err != nil {
				return err
			}
			if err := sys.AptInstall("gh=" + strings.TrimPrefix(version, "v")); err != nil {
				return err
			}
			return sys.AptClean()
		},
		Test: func(t *layer.T) {
			t.HasCommand("gh")
			t.Version("gh", "v"+findVersion(t.Output("gh version", "gh", "--version"), `gh version ([0-9.]+)`))
		},
	})
}
