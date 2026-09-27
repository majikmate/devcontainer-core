package layers

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/majikmate/devcontainer-core/internal/devcontainer"
	"github.com/majikmate/devcontainer-core/internal/layer"
	"github.com/majikmate/devcontainer-core/internal/sys"
	"github.com/majikmate/devcontainer-core/internal/versions"
)

const (
	prettierPrefix  = "/usr/local"
	prettierModules = prettierPrefix + "/lib/node_modules"
	prettierConfig  = "/.prettierrc.json"
)

func init() {
	layer.Register(&layer.Layer{
		Name:    "prettier",
		Summary: "Prettier with the Tailwind CSS plugin and a global configuration",
		Needs:   []string{"node"},
		Tools: []layer.Tool{
			{Name: "prettier", Arg: "PRETTIER_VERSION", Newest: func() (string, error) { return versions.NPM("prettier") }},
			{Name: "prettier-plugin-tailwindcss", Arg: "PRETTIER_PLUGIN_TAILWINDCSS_VERSION", Newest: func() (string, error) { return versions.NPM("prettier-plugin-tailwindcss") }},
		},
		Metadata: devcontainer.Entry{
			"customizations": devcontainer.VSCode([]string{"esbenp.prettier-vscode"}, map[string]any{
				// The editor uses the same Prettier as the terminal
				"prettier.prettierPath": prettierModules + "/prettier",
			}),
		},
		Install: installPrettier,
		Test: func(t *layer.T) {
			dir, cleanup, err := tempDir()
			if err == nil {
				defer cleanup()
				file := filepath.Join(dir, "index.html")
				_ = os.WriteFile(file, []byte(`<div class="text-center p-4 flex card"></div>`+"\n"), 0o644)
				out := t.Output("format with Prettier", "prettier", file)
				t.Check("Tailwind CSS classes sorted", out == `<div class="card flex p-4 text-center"></div>`)
			}
			t.Version("prettier", t.Output("prettier version", "prettier", "--version"))
			t.Version("prettier-plugin-tailwindcss", packageVersion(prettierModules+"/prettier-plugin-tailwindcss"))
		},
	})
}

func installPrettier(e *layer.Env) error {
	prettier, err := e.Version("prettier")
	if err != nil {
		return err
	}
	plugin, err := e.Version("prettier-plugin-tailwindcss")
	if err != nil {
		return err
	}
	// In /usr/local, not in the nvm folder, so Prettier does not depend on the
	// Node.js version that is selected with nvm.
	if err := sys.Run(nil, "npm", "install", "--global", "--prefix", prettierPrefix,
		"--no-fund", "--no-audit", "--no-update-notifier",
		"prettier@"+prettier, "prettier-plugin-tailwindcss@"+plugin); err != nil {
		return err
	}
	_ = sys.Run(nil, "npm", "cache", "clean", "--force")

	// Global fallback configuration. Prettier searches for a configuration
	// from the folder of the formatted file up to "/", so every project
	// without its own configuration uses this file. It has no style options
	// (standard Prettier style), only the plugin that sorts Tailwind CSS classes.
	entry, err := sys.Output("node", "-p", "require.resolve('prettier-plugin-tailwindcss', { paths: ['"+prettierModules+"'] })")
	if err != nil {
		return err
	}
	config, _ := json.MarshalIndent(map[string]any{"plugins": []string{entry}}, "", "  ")
	return sys.WriteFile(prettierConfig, string(config)+"\n", 0o644)
}

// packageVersion reads the version of an installed npm package.
func packageVersion(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(data, &pkg)
	return pkg.Version
}
