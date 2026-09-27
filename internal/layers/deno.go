package layers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/devcontainer"
	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
	"github.com/majikmate/devcontainer-core/pkg/versions"
)

func init() {
	layer.Register(&layer.Layer{
		Name:    "deno",
		Summary: "Deno, the JavaScript/TypeScript runtime and language server",
		Needs:   []string{"user"},
		Tools: []layer.Tool{
			{Name: "deno", Arg: "DENO_VERSION", Newest: versions.DenoLTS},
		},
		Metadata: devcontainer.Entry{
			"customizations": devcontainer.VSCode([]string{"denoland.vscode-deno"}, map[string]any{
				"deno.enable":            true,
				"deno.codeLens.test":     true,
				"deno.codeLens.testArgs": []string{"--allow-all", "--check=all"},
				"deno.testing.args":      []string{"--allow-all", "--check=all"},
			}),
		},
		Install: installDeno,
		Test: func(t *layer.T) {
			dir, cleanup, err := sys.TempDir()
			if err == nil {
				defer cleanup()
				file := filepath.Join(dir, "check.ts")
				_ = os.WriteFile(file, []byte("const n: number = 1 + 1;\nconsole.log(n);\n"), 0o644)
				t.Check("run TypeScript with type check", t.Output("deno run", "deno", "run", "--check", file) == "2")
			}
			t.Version("deno", "v"+layer.FindVersion(t.Output("deno version", "deno", "--version"), `deno ([0-9.]+)`))
		},
	})
}

func installDeno(e *layer.Env) error {
	version, err := e.Version("deno")
	if err != nil {
		return err
	}
	version = "v" + strings.TrimPrefix(version, "v")
	target := map[string]string{"amd64": "x86_64-unknown-linux-gnu", "arm64": "aarch64-unknown-linux-gnu"}[runtime.GOARCH]
	if target == "" {
		return fmt.Errorf("deno: unsupported architecture %s", runtime.GOARCH)
	}
	dir, cleanup, err := sys.TempDir()
	if err != nil {
		return err
	}
	defer cleanup()
	name := "deno-" + target + ".zip"
	url := "https://dl.deno.land/release/" + version + "/" + name
	sys.Logf("Installing Deno %s", version)
	archive := filepath.Join(dir, name)
	if err := sys.Download(url, archive); err != nil {
		return err
	}
	sum, err := sys.Get(url + ".sha256sum")
	if err != nil {
		return err
	}
	if err := sys.VerifySHA256(archive, strings.ToLower(regexp.MustCompile(`[0-9a-fA-F]{64}`).FindString(string(sum)))); err != nil {
		return err
	}
	if err := sys.ExtractZip(archive, dir); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, "deno"))
	if err != nil {
		return err
	}
	return sys.WriteFile("/usr/local/bin/deno", string(data), 0o755)
}
