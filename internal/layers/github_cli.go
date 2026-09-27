package layers

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
	"github.com/majikmate/devcontainer-core/pkg/versions"
)

func init() {
	layer.Register(&layer.Layer{
		Name:    "github-cli",
		Summary: "GitHub CLI (gh) from the GitHub release archive",
		Tools: []layer.Tool{
			{Name: "gh", Arg: "GITHUB_CLI_VERSION", Newest: func() (string, error) { return versions.GitHubRelease("cli/cli") }},
		},
		Install: installGitHubCLI,
		Test: func(t *layer.T) {
			t.HasCommand("gh")
			t.Version("gh", "v"+findVersion(t.Output("gh version", "gh", "--version"), `gh version ([0-9.]+)`))
		},
	})
}

// installGitHubCLI installs gh and its manual pages from the release archive,
// after checking the archive against the checksum file of the release.
func installGitHubCLI(e *layer.Env) error {
	version, err := e.Version("gh")
	if err != nil {
		return err
	}
	version = strings.TrimPrefix(version, "v")
	name := fmt.Sprintf("gh_%s_linux_%s", version, runtime.GOARCH)
	base := "https://github.com/cli/cli/releases/download/v" + version
	sys.Logf("Installing GitHub CLI v%s", version)

	dir, cleanup, err := sys.TempDir()
	if err != nil {
		return err
	}
	defer cleanup()
	archive := filepath.Join(dir, name+".tar.gz")
	if err := sys.Download(base+"/"+name+".tar.gz", archive); err != nil {
		return err
	}
	sums, err := sys.Get(base + "/gh_" + version + "_checksums.txt")
	if err != nil {
		return err
	}
	if err := sys.VerifySHA256(archive, sys.ChecksumFor(string(sums), name+".tar.gz")); err != nil {
		return err
	}
	if err := sys.ExtractTarGz(archive, dir); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, name, "bin", "gh"))
	if err != nil {
		return err
	}
	if err := sys.WriteFile("/usr/local/bin/gh", string(data), 0o755); err != nil {
		return err
	}
	pages, _ := filepath.Glob(filepath.Join(dir, name, "share", "man", "man1", "*.1"))
	for _, page := range pages {
		data, err := os.ReadFile(page)
		if err != nil {
			return err
		}
		if err := sys.WriteFile(filepath.Join("/usr/local/share/man/man1", filepath.Base(page)), string(data), 0o644); err != nil {
			return err
		}
	}
	return nil
}
