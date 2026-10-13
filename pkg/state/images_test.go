// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImagesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"devcontainer-core.json":          `{"name":"ghcr.io/o/devcontainer-core","version":"1.0.30","created":"2026-10-08T22:32:17Z"}`,
		"devcontainer-classroom-web.json": `{"name":"ghcr.io/o/devcontainer-classroom-web","version":"2.0.28","created":"2026-10-11T20:50:00Z"}`,
		"devcontainer-base.json":          `{"name":"ghcr.io/o/devcontainer-base","version":"2.0.27","created":"2026-10-10T23:29:25Z"}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	images, err := Images(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, img := range images {
		got = append(got, img.Version)
	}
	if len(got) != 3 || got[0] != "2.0.28" || got[1] != "2.0.27" || got[2] != "1.0.30" {
		t.Fatalf("order: got %v, want [2.0.28 2.0.27 1.0.30]", got)
	}
}

func TestImagesMissingFolder(t *testing.T) {
	images, err := Images(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(images) != 0 {
		t.Fatalf("got %v, %v; want no images and no error", images, err)
	}
}

func TestImageFile(t *testing.T) {
	if got := ImageFile("ghcr.io/majikmate/devcontainer-base"); got != "devcontainer-base.json" {
		t.Fatalf("got %q", got)
	}
}
