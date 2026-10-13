// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ImagesDir is the folder of the image files: the release build of every
// image in the chain (for example core, base, classroom-web) adds one file
// <name>.json, so an image contains the files of all its base images.
const ImagesDir = Dir + "/images"

// Image describes one released image of the chain.
type Image struct {
	Name     string `json:"name"`     // for example ghcr.io/majikmate/devcontainer-base
	Version  string `json:"version"`  // for example 2.0.28
	Revision string `json:"revision"` // commit of the image repository
	Created  string `json:"created"`  // creation time (RFC 3339)
	Source   string `json:"source"`   // URL of the image repository
}

// ImageFile returns the file name of an image in ImagesDir, for example
// devcontainer-base.json for ghcr.io/majikmate/devcontainer-base.
func ImageFile(name string) string {
	return name[strings.LastIndex(name, "/")+1:] + ".json"
}

// Images reads the image files of dir (normally ImagesDir), newest first:
// the image itself, then its base images. A missing folder gives no images
// (images built before the image files existed).
func Images(dir string) ([]Image, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var images []Image
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var img Image
		if err := json.Unmarshal(data, &img); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		images = append(images, img)
	}
	// An image is always built after its base image; RFC 3339 times in UTC
	// sort as text
	sort.SliceStable(images, func(i, j int) bool { return images[i].Created > images[j].Created })
	return images, nil
}
