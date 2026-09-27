// Package devcontainer builds the image label "devcontainer.metadata".
//
// The label is a JSON list of configuration entries
// (https://containers.dev/implementors/spec/#image-metadata). VS Code, the
// Dev Containers extension and Codespaces read it from the image and merge the
// entries in order. Our images add entries of two kinds:
//
//   - one entry per installed layer (id "devenv/<layer>"), declared as data in
//     the layer: its VS Code extensions and settings, container options and
//     lifecycle commands;
//   - one entry per image (id "devenv/image/<repository>") with the settings of
//     its .devcontainer/devcontainer.json.
package devcontainer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Label is the name of the image label.
const Label = "devcontainer.metadata"

// Entry is one configuration entry of the label. Only the properties that our
// layers use are typed; the entry of an image keeps all its allowed
// properties (see ImageEntry).
type Entry map[string]any

// ID returns the id of the entry.
func (e Entry) ID() string {
	id, _ := e["id"].(string)
	return id
}

// VSCode creates the "customizations.vscode" part of an entry.
func VSCode(extensions []string, settings map[string]any) map[string]any {
	vscode := map[string]any{}
	if len(extensions) > 0 {
		vscode["extensions"] = extensions
	}
	if len(settings) > 0 {
		vscode["settings"] = settings
	}
	return map[string]any{"vscode": vscode}
}

// allowedProperties are the devcontainer.json properties that belong in the
// image label (container options, lifecycle commands, tool customizations).
// Build properties such as "build", "image" or "name" do not.
var allowedProperties = map[string]bool{
	"customizations": true, "remoteUser": true, "containerUser": true,
	"remoteEnv": true, "containerEnv": true, "updateRemoteUserUID": true,
	"userEnvProbe": true, "overrideCommand": true, "shutdownAction": true,
	"init": true, "privileged": true, "capAdd": true, "securityOpt": true,
	"mounts": true, "forwardPorts": true, "portsAttributes": true,
	"otherPortsAttributes": true, "hostRequirements": true, "waitFor": true,
	"onCreateCommand": true, "updateContentCommand": true,
	"postCreateCommand": true, "postStartCommand": true, "postAttachCommand": true,
}

// ImageEntry creates the entry of an image from the content of its
// devcontainer.json (JSON with comments). Properties that do not belong in the
// label are left out.
func ImageEntry(id string, devcontainerJSON []byte) (Entry, error) {
	var config map[string]any
	if err := json.Unmarshal(StripJSONC(devcontainerJSON), &config); err != nil {
		return nil, fmt.Errorf("devcontainer.json: %w", err)
	}
	entry := Entry{"id": id}
	for key, value := range config {
		if allowedProperties[key] {
			entry[key] = value
		}
	}
	// customizations.devenv is the release configuration of the image (read
	// by the release plan); the label records the result as inputs instead.
	if c, ok := entry["customizations"].(map[string]any); ok {
		delete(c, "devenv")
		if len(c) == 0 {
			delete(entry, "customizations")
		}
	}
	return entry, nil
}

// Parse reads the value of a label. An empty value is an empty list; a single
// object (allowed by the specification) becomes a list with one entry.
func Parse(value string) ([]Entry, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "{") {
		var single Entry
		if err := json.Unmarshal([]byte(value), &single); err != nil {
			return nil, err
		}
		return []Entry{single}, nil
	}
	var entries []Entry
	if err := json.Unmarshal([]byte(value), &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// Merge returns the label entries of a new image: the entries of the base
// image, then the layer entries that the base image does not have yet (by id),
// then the entry of the image itself (it replaces an older entry with the same id).
func Merge(base, layers []Entry, image Entry) []Entry {
	seen := map[string]bool{}
	var result []Entry
	for _, e := range base {
		if image != nil && e.ID() == image.ID() {
			continue
		}
		result = append(result, e)
		seen[e.ID()] = true
	}
	for _, e := range layers {
		if id := e.ID(); id == "" || !seen[id] {
			result = append(result, e)
			seen[id] = true
		}
	}
	if image != nil {
		result = append(result, image)
	}
	return result
}

// Encode returns the label value.
func Encode(entries []Entry) (string, error) {
	if entries == nil {
		entries = []Entry{}
	}
	data, err := json.Marshal(entries)
	return string(data), err
}
