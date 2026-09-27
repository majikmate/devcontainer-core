package layers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/devcontainer"
	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/state"
	"github.com/majikmate/devcontainer-core/pkg/sys"
	"github.com/majikmate/devcontainer-core/pkg/versions"
)

// Paths of Go. The Dockerfile sets the same values with ENV:
//
//	ENV GOROOT=/usr/local/go GOPATH=/go PATH=/go/bin:/usr/local/go/bin:$PATH
const (
	goRoot = "/usr/local/go"
	goPath = "/go"
)

// Go tools that are installed with "go install".
var goTools = []struct{ name, pkg, module, arg string }{
	{"gopls", "golang.org/x/tools/gopls", "golang.org/x/tools/gopls", "GOPLS_VERSION"},
	{"dlv", "github.com/go-delve/delve/cmd/dlv", "github.com/go-delve/delve", "DLV_VERSION"},
	{"staticcheck", "honnef.co/go/tools/cmd/staticcheck", "honnef.co/go/tools", "STATICCHECK_VERSION"},
	{"govulncheck", "golang.org/x/vuln/cmd/govulncheck", "golang.org/x/vuln", "GOVULNCHECK_VERSION"},
}

func init() {
	tools := []layer.Tool{
		{Name: "go", Arg: "GO_VERSION", Newest: versions.GoRelease},
		{Name: "golangci-lint", Arg: "GOLANGCI_LINT_VERSION", Newest: func() (string, error) { return versions.GitHubRelease("golangci/golangci-lint") }},
	}
	for _, tool := range goTools {
		module := tool.module
		tools = append(tools, layer.Tool{Name: tool.name, Arg: tool.arg, Newest: func() (string, error) { return versions.GoModule(module) }})
	}
	layer.Register(&layer.Layer{
		Name:    "go",
		Summary: "Go, gopls, dlv, staticcheck, govulncheck and golangci-lint",
		Needs:   []string{"user"},
		Tools:   tools,
		Metadata: devcontainer.Entry{
			// The Go debugger (dlv) needs these options of the container start
			"capAdd": []string{"SYS_PTRACE"},
			"init":   true,
			"customizations": devcontainer.VSCode([]string{"golang.go"}, map[string]any{
				"[go]": map[string]any{
					"editor.defaultFormatter": "golang.go",
					"editor.insertSpaces":     false,
					"editor.tabSize":          4,
					"editor.codeActionsOnSave": map[string]any{
						"source.organizeImports": "explicit",
					},
				},
				"[go.mod]": map[string]any{"editor.defaultFormatter": "golang.go"},
				"[go.sum]": map[string]any{"editor.formatOnSave": false},
			}),
		},
		Install: installGo,
		Test:    testGo,
	})
}

func installGo(e *layer.Env) error {
	dir, cleanup, err := sys.TempDir()
	if err != nil {
		return err
	}
	defer cleanup()

	// Go from go.dev, with the checksum that go.dev publishes
	version, err := e.Version("go")
	if err != nil {
		return err
	}
	version = "go" + strings.TrimPrefix(version, "go")
	archive := fmt.Sprintf("%s.linux-%s.tar.gz", version, runtime.GOARCH)
	checksum, err := goChecksum(archive)
	if err != nil {
		return err
	}
	sys.Logf("Installing %s", version)
	path := filepath.Join(dir, archive)
	if err := sys.Download("https://go.dev/dl/"+archive, path); err != nil {
		return err
	}
	if err := sys.VerifySHA256(path, checksum); err != nil {
		return err
	}
	if err := os.RemoveAll(goRoot); err != nil {
		return err
	}
	if err := sys.ExtractTarGz(path, filepath.Dir(goRoot)); err != nil {
		return err
	}

	// Go tools, built in a temporary GOPATH so the module cache does not stay in the image
	if err := os.MkdirAll(filepath.Join(goPath, "bin"), 0o755); err != nil {
		return err
	}
	env := []string{
		"PATH=" + goRoot + "/bin:" + os.Getenv("PATH"),
		"GOPATH=" + filepath.Join(dir, "gopath"), "GOCACHE=" + filepath.Join(dir, "gocache"),
		"GOBIN=" + filepath.Join(goPath, "bin"), "GOTOOLCHAIN=local", "CGO_ENABLED=0",
	}
	for _, tool := range goTools {
		v, err := e.Version(tool.name)
		if err != nil {
			return err
		}
		sys.Logf("Installing %s@%s", tool.pkg, v)
		if err := sys.Run(env, goRoot+"/bin/go", "install", tool.pkg+"@"+v); err != nil {
			return err
		}
	}
	_ = sys.Run(nil, "chmod", "-R", "u+w", filepath.Join(dir, "gopath")) // module cache is read-only

	if err := installGolangciLint(e, dir); err != nil {
		return err
	}

	// The group golang owns Go and GOPATH, so the development user can install
	// modules and tools
	if err := sys.Run(nil, "groupadd", "--system", "-f", "golang"); err != nil {
		return err
	}
	if err := sys.Run(nil, "usermod", "-a", "-G", "golang", state.User()); err != nil {
		return err
	}
	u, err := sys.LookupUser(state.User())
	if err != nil {
		return err
	}
	group, err := sys.GroupID("golang")
	if err != nil {
		return err
	}
	for _, root := range []string{goRoot, goPath} {
		if err := sys.ShareWithGroup(root, u.UID, group); err != nil {
			return err
		}
	}
	return nil
}

// goChecksum reads the SHA-256 checksum of a Go archive from go.dev.
func goChecksum(archive string) (string, error) {
	data, err := sys.Get("https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		return "", err
	}
	var releases []struct {
		Files []struct {
			Filename string `json:"filename"`
			SHA256   string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return "", err
	}
	for _, r := range releases {
		for _, f := range r.Files {
			if f.Filename == archive {
				return f.SHA256, nil
			}
		}
	}
	return "", fmt.Errorf("%s is not listed on go.dev", archive)
}

// installGolangciLint installs the release archive, the recommended installation.
func installGolangciLint(e *layer.Env, dir string) error {
	version, err := e.Version("golangci-lint")
	if err != nil {
		return err
	}
	version = strings.TrimPrefix(version, "v")
	name := fmt.Sprintf("golangci-lint-%s-linux-%s", version, runtime.GOARCH)
	base := "https://github.com/golangci/golangci-lint/releases/download/v" + version
	sys.Logf("Installing golangci-lint v%s", version)
	archive := filepath.Join(dir, name+".tar.gz")
	if err := sys.Download(base+"/"+name+".tar.gz", archive); err != nil {
		return err
	}
	sums, err := sys.Get(base + "/golangci-lint-" + version + "-checksums.txt")
	if err != nil {
		return err
	}
	if err := sys.VerifySHA256(archive, sys.ChecksumFor(string(sums), name+".tar.gz")); err != nil {
		return err
	}
	if err := sys.ExtractTarGz(archive, dir); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, name, "golangci-lint"))
	if err != nil {
		return err
	}
	return sys.WriteFile(filepath.Join(goPath, "bin", "golangci-lint"), string(data), 0o755)
}

func testGo(t *layer.T) {
	for _, cmd := range []string{"go", "gofmt", "gopls", "dlv", "staticcheck", "govulncheck", "golangci-lint"} {
		t.HasCommand(cmd)
	}
	gopath := t.Output("go env GOPATH", "go", "env", "GOPATH")
	t.Check("GOPATH is "+goPath, gopath == goPath)
	t.Command("user can write to "+goPath+"/bin", "test", "-w", goPath+"/bin")

	dir, cleanup, err := sys.TempDir()
	if err == nil {
		defer cleanup()
		_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/hello\n\ngo 1.21\n"), 0o644)
		// Inside the module folder: from another folder, "go run <dir>" fails
		// because the folder is outside the main module
		out := t.OutputIn("build and run a program", dir, "go", "run", ".")
		t.Check("program prints hello", out == "hello")
	}

	t.Version("go", strings.TrimPrefix(t.Output("go version", "go", "env", "GOVERSION"), "go"))
	t.Version("gopls", layer.FindVersion(t.Output("gopls version", "gopls", "version"), `gopls (v[0-9.]+)`))
	t.Version("dlv", "v"+layer.FindVersion(t.Output("dlv version", "dlv", "version"), `Version: ([0-9.]+)`))
	t.Version("staticcheck", "v"+layer.FindVersion(t.Output("staticcheck version", "staticcheck", "-version"), `\(([0-9.]+)\)`))
	t.Version("govulncheck", layer.FindVersion(t.Output("govulncheck version", "govulncheck", "-version"), `govulncheck@(v[0-9.]+)`))
	t.Version("golangci-lint", "v"+layer.FindVersion(t.Output("golangci-lint version", "golangci-lint", "--version"), `version ([0-9.]+)`))
}
