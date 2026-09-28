// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package release

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// PublishOptions are the settings of a release.
type PublishOptions struct {
	Repository string   // owner/name
	Revision   string   // commit of the release
	Image      string   // for example ghcr.io/owner/name
	Version    string   // for example 2.1.0
	Tags       []string // for example 2.1.0 2.1 2 latest
	Arches     []string // architectures that were built
	Reason     string
	Inputs     string
	OutputsDir string // folder with the test outputs test-output-<arch>.txt
	Token      string
}

// Publish creates the multi-architecture image with all tags and the GitHub release.
func Publish(o PublishOptions) error {
	args := []string{"buildx", "imagetools", "create"}
	for _, tag := range o.Tags {
		args = append(args, "--tag", o.Image+":"+tag)
	}
	for _, arch := range o.Arches {
		args = append(args, o.Image+":"+o.Version+"-"+arch)
	}
	if err := sys.Run(nil, "docker", args...); err != nil {
		return err
	}
	if err := sys.Run(nil, "docker", "buildx", "imagetools", "inspect", o.Image+":"+o.Version); err != nil {
		return err
	}
	return createRelease(o, releaseNotes(o))
}

func releaseNotes(o PublishOptions) string {
	var b strings.Builder
	b.WriteString("## Image\n\n")
	for _, tag := range o.Tags {
		fmt.Fprintf(&b, "- `%s:%s`\n", o.Image, tag)
	}
	var platforms []string
	for _, arch := range o.Arches {
		platforms = append(platforms, "linux/"+arch)
	}
	fmt.Fprintf(&b, "\nPlatforms: %s\n\n## Reason\n\n%s\n\n", strings.Join(platforms, ", "), o.Reason)
	b.WriteString("## Installed versions (linux/amd64)\n\n```\n")
	output, _ := os.ReadFile(filepath.Join(o.OutputsDir, "test-output-amd64.txt"))
	lines := versionLine.FindAllString(string(output), -1)
	if len(lines) == 0 {
		b.WriteString("(no version lines in the test output)\n")
	}
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	b.WriteString("```\n\n## Inputs\n\n```\n")
	b.WriteString(strings.ReplaceAll(o.Inputs, " ", "\n") + "\n```\n")
	return b.String()
}

func createRelease(o PublishOptions, notes string) error {
	gh := newGitHub(o.Token)
	tag := "v" + o.Version
	var existing struct {
		ID int64 `json:"id"`
	}
	status, err := gh.do(http.MethodGet, "repos/"+o.Repository+"/releases/tags/"+tag, nil, &existing)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		_, err = gh.do(http.MethodPatch, fmt.Sprintf("repos/%s/releases/%d", o.Repository, existing.ID), map[string]any{"body": notes}, nil)
		return err
	}
	_, err = gh.do(http.MethodPost, "repos/"+o.Repository+"/releases", map[string]any{
		"tag_name":         tag,
		"target_commitish": o.Revision,
		"name":             tag,
		"body":             notes,
	}, nil)
	return err
}
