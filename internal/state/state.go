// Package state stores what devcon has installed in an image: the list of
// installed layers and the name of the development user.
package state

import (
	"os"
	"path/filepath"
	"strings"
)

// Dir is the folder of the devcon files in the image.
const Dir = "/usr/local/share/devcon"

var (
	layersFile = filepath.Join(Dir, "layers")
	userFile   = filepath.Join(Dir, "user")
)

// Installed returns the installed layers in installation order.
func Installed() []string {
	data, err := os.ReadFile(layersFile)
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}

// IsInstalled reports whether a layer is installed.
func IsInstalled(name string) bool {
	for _, n := range Installed() {
		if n == name {
			return true
		}
	}
	return false
}

// MarkInstalled adds a layer to the list of installed layers.
func MarkInstalled(name string) error {
	if IsInstalled(name) {
		return nil
	}
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(layersFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(name + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// User returns the name of the development user (default "dev").
func User() string {
	data, err := os.ReadFile(userFile)
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return "dev"
	}
	return strings.TrimSpace(string(data))
}

// SetUser stores the name of the development user.
func SetUser(name string) error {
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(userFile, []byte(name+"\n"), 0o644)
}
