// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package release

import (
	"fmt"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// ModuleOptions are the settings of a Go module release (a version tag with a
// GitHub release, no image), for example devcontainer-features.
type ModuleOptions struct {
	Dir        string // checkout of the repository (with its history)
	Repository string // owner/name
	Revision   string // commit to release
	Major      int
	Bump       string // patch (default), minor or major
	Token      string
}

// ModuleRelease creates the next version tag vX.Y.Z at the revision, as a
// GitHub release whose notes list the commits since the previous version.
// It does nothing when the revision already has a version tag.
func ModuleRelease(o ModuleOptions) error {
	last, err := lastVersion(o.Dir, o.Major)
	if err != nil {
		return err
	}
	if last != "" {
		tagged, err := sys.Output("git", "-C", o.Dir, "rev-list", "-n", "1", "v"+last)
		if err == nil && tagged == o.Revision {
			notice("Revision %s is already released as v%s", o.Revision, last)
			return nil
		}
	}
	step := o.Bump
	if step == "" || step == "auto" {
		step = "patch"
	}
	version := nextVersion(last, o.Major, step)

	logRange := o.Revision
	if last != "" {
		logRange = "v" + last + ".." + o.Revision
	}
	commits, err := sys.Output("git", "-C", o.Dir, "log", "--format=- %s", logRange)
	if err != nil {
		return err
	}
	notes := fmt.Sprintf("## Changes\n\n%s\n\nGo module: `go get github.com/%s@v%s`\n", commits, strings.ToLower(o.Repository), version)
	if err := createRelease(PublishOptions{Repository: o.Repository, Revision: o.Revision, Version: version, Token: o.Token}, notes); err != nil {
		return err
	}
	if err := SetOutput("version", version); err != nil {
		return err
	}
	Summary(fmt.Sprintf("### Released v%s\n\n%s", version, commits))
	fmt.Printf("Released v%s\n", version)
	return nil
}
