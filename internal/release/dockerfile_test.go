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
FROM golang:1.27-trixie AS devenv
RUN go build ./cmd/devenv
FROM --platform=$BUILDPLATFORM buildpack-deps:trixie-curl
COPY --from=devenv /out/devenv /usr/local/bin/devenv
RUN devenv install os
ARG X
RUN devenv install go \
    node
FROM devenv AS again
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
	content := "FROM golang:1.27-trixie AS devenv\nFROM ghcr.io/majikmate/devcontainer-core:1\nRUN devenv install go\n"
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

// TestExpand: the FROM lines get the versions of the release plan; a
// Dockerfile never decides a version, so a missing value is an error.
func TestExpand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dockerfile")
	content := `ARG DEBIAN_SERIES
FROM debian:${DEBIAN_SERIES} AS devenv
FROM debian:$DEBIAN_SERIES
RUN devenv install os
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := ReadDockerfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Expand(map[string]string{"DEBIAN_SERIES": "trixie"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"debian:trixie"}; !reflect.DeepEqual(d.BaseImages, want) || d.FinalBase != want[0] {
		t.Errorf("base images = %v, final %q", d.BaseImages, d.FinalBase)
	}

	d, _ = ReadDockerfile(path)
	if err := d.Expand(map[string]string{}); err == nil {
		t.Error("no value for DEBIAN_SERIES: no error")
	}
}

func TestReadDockerfileArgs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dockerfile")
	content := `FROM ghcr.io/majikmate/devcontainer-base:2
ARG GO_PIN=1.27
ARG GO_VERSION
ARG PLAYWRIGHT_BROWSERS="chromium firefox webkit" OTHER='a b'
ARG GO_PIN=1.28
RUN devenv install go
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
}
