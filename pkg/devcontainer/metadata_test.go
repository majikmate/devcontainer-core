// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package devcontainer

import (
	"encoding/json"
	"testing"
)

func TestStripJSONC(t *testing.T) {
	in := []byte(`{
  // comment with "quotes"
  "a": "http://example.com/*not a comment*/", /* block
  comment */
  "b": [1, 2,],
  "c": {"d": "x // y",},
}`)
	var got map[string]any
	if err := json.Unmarshal(StripJSONC(in), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, StripJSONC(in))
	}
	if got["a"] != "http://example.com/*not a comment*/" {
		t.Errorf("a = %v", got["a"])
	}
	if c := got["c"].(map[string]any); c["d"] != "x // y" {
		t.Errorf("c.d = %v", c["d"])
	}
	if b := got["b"].([]any); len(b) != 2 {
		t.Errorf("b = %v", b)
	}
}

func TestImageEntry(t *testing.T) {
	entry, err := ImageEntry("devcon/image/x", []byte(`{
  "name": "X",
  "build": {"dockerfile": "Dockerfile"},
  "remoteUser": "dev",
  "customizations": {"vscode": {"extensions": ["a.b"]}, "devcon": {"deno": {"channel": "stable"}}},
}`))
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := entry["customizations"].(map[string]any); c["devcon"] != nil || c["vscode"] == nil {
		t.Errorf("customizations = %v, want vscode without devcon", entry["customizations"])
	}
	if _, ok := entry["build"]; ok {
		t.Error("build must not be in the label")
	}
	if _, ok := entry["name"]; ok {
		t.Error("name must not be in the label")
	}
	if entry["remoteUser"] != "dev" || entry.ID() != "devcon/image/x" {
		t.Errorf("entry = %v", entry)
	}
}

func TestMerge(t *testing.T) {
	base := []Entry{{"id": "devcon/os"}, {"id": "devcon/image/core"}}
	layers := []Entry{{"id": "devcon/os"}, {"id": "devcon/go"}}
	image := Entry{"id": "devcon/image/base"}
	got := Merge(base, layers, image)
	want := []string{"devcon/os", "devcon/image/core", "devcon/go", "devcon/image/base"}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for i, id := range want {
		if got[i].ID() != id {
			t.Errorf("entry %d = %s, want %s", i, got[i].ID(), id)
		}
	}
	// A rebuild of the same image replaces its own entry
	again := Merge(got, nil, Entry{"id": "devcon/image/base", "x": 1})
	if len(again) != 4 || again[3]["x"] != 1 {
		t.Errorf("rebuild: %v", again)
	}
}

func TestParse(t *testing.T) {
	if e, err := Parse(""); err != nil || len(e) != 0 {
		t.Errorf("empty: %v %v", e, err)
	}
	if e, err := Parse(`{"id":"a"}`); err != nil || len(e) != 1 {
		t.Errorf("object: %v %v", e, err)
	}
	if e, err := Parse(`[{"id":"a"},{"id":"b"}]`); err != nil || len(e) != 2 {
		t.Errorf("list: %v %v", e, err)
	}
}
