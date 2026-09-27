package release

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadDockerfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	content := `# comment: FROM ignored
FROM golang:1.27-trixie AS devcon
RUN go build ./cmd/devcon
FROM --platform=$BUILDPLATFORM buildpack-deps:trixie-curl
COPY --from=devcon /out/devcon /usr/local/bin/devcon
RUN devcon install os
ARG X
RUN devcon install go \
    node
FROM devcon AS again
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := ReadDockerfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"golang:1.27-trixie", "buildpack-deps:trixie-curl"}; !reflect.DeepEqual(d.BaseImages, want) {
		t.Errorf("base images = %v, want %v", d.BaseImages, want)
	}
	if want := []string{"os", "go", "node"}; !reflect.DeepEqual(d.Layers, want) {
		t.Errorf("layers = %v, want %v", d.Layers, want)
	}
	if want := []string{"X"}; !reflect.DeepEqual(d.Args, want) {
		t.Errorf("args = %v, want %v", d.Args, want)
	}
	// The last FROM line starts from a build stage
	if d.FinalBase != "" {
		t.Errorf("final base = %q, want empty", d.FinalBase)
	}
}

func TestReadDockerfileFinalBase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dockerfile")
	content := "FROM golang:1.27-trixie AS devcon\nFROM ghcr.io/majikmate/devcontainer-core:1\nRUN devcon install go\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := ReadDockerfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.FinalBase != "ghcr.io/majikmate/devcontainer-core:1" {
		t.Errorf("final base = %q", d.FinalBase)
	}
}

func TestReadDockerfileArgDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dockerfile")
	content := `FROM ghcr.io/majikmate/devcontainer-base:2
ARG GO_PIN=1.27
ARG GO_VERSION
ARG PLAYWRIGHT_BROWSERS="chromium firefox webkit" OTHER='a b'
ARG GO_PIN=1.28
RUN devcon install go
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := ReadDockerfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"GO_PIN", "GO_VERSION", "PLAYWRIGHT_BROWSERS", "OTHER"}; !reflect.DeepEqual(d.Args, want) {
		t.Errorf("args = %v, want %v", d.Args, want)
	}
	want := map[string]string{"GO_PIN": "1.28", "PLAYWRIGHT_BROWSERS": "chromium firefox webkit", "OTHER": "a b"}
	if !reflect.DeepEqual(d.ArgDefaults, want) {
		t.Errorf("defaults = %v, want %v", d.ArgDefaults, want)
	}
}
