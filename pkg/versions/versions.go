// Package versions finds the newest versions of the tools, from the same
// sources that the layers install from.
package versions

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// GoRelease returns the newest stable Go release, for example "1.27.1".
func GoRelease() (string, error) {
	data, err := sys.Get("https://go.dev/dl/?mode=json")
	if err != nil {
		return "", err
	}
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return "", err
	}
	for _, r := range releases {
		if r.Stable {
			return strings.TrimPrefix(r.Version, "go"), nil
		}
	}
	return "", fmt.Errorf("no stable Go release found")
}

// GoModule returns the newest version of a Go module from the Go module proxy,
// for example "v0.20.0".
func GoModule(module string) (string, error) {
	data, err := sys.Get("https://proxy.golang.org/" + strings.ToLower(module) + "/@latest")
	if err != nil {
		return "", err
	}
	var info struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", err
	}
	return info.Version, nil
}

// NodeLTS returns the newest Node.js LTS release, for example "v24.21.0".
func NodeLTS() (string, error) {
	data, err := sys.Get("https://nodejs.org/dist/index.json")
	if err != nil {
		return "", err
	}
	var releases []struct {
		Version string `json:"version"`
		LTS     any    `json:"lts"`
	}
	if err := json.Unmarshal(data, &releases); err != nil {
		return "", err
	}
	for _, r := range releases {
		if lts, ok := r.LTS.(string); ok && lts != "" {
			return r.Version, nil
		}
	}
	return "", fmt.Errorf("no Node.js LTS release found")
}

// DenoLTS returns the newest Deno LTS release, or the newest release if Deno
// publishes no LTS release, for example "v2.9.3".
func DenoLTS() (string, error) {
	if data, err := sys.Get("https://dl.deno.land/release-lts-latest.txt"); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			return v, nil
		}
	}
	data, err := sys.Get("https://dl.deno.land/release-latest.txt")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// NPM returns the newest version of an npm package, for example "3.9.9".
func NPM(pkg string) (string, error) {
	data, err := sys.Get("https://registry.npmjs.org/" + pkg + "/latest")
	if err != nil {
		return "", err
	}
	var info struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", err
	}
	return info.Version, nil
}

// GitHubRelease returns the tag of the newest release of a GitHub repository,
// for example "v0.40.8". It uses GITHUB_TOKEN if set (higher rate limit).
func GitHubRelease(repo string) (string, error) {
	data, err := sys.GetWithToken("https://api.github.com/repos/"+repo+"/releases/latest", os.Getenv("GITHUB_TOKEN"))
	if err != nil {
		return "", err
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("no release of %s found", repo)
	}
	return release.TagName, nil
}
