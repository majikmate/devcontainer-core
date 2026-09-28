// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package layers contains the Debian-bound layers of the images: os, user,
// locales, sshd, build-tools and playwright-deps. The distribution-independent
// layers are in devcontainer-features. Each file registers one layer; see the
// package layer for what a layer declares.
package layers

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// getWithTimeout downloads a small file with a short timeout and no retries
// (for steps that must not delay the start of a container).
func getWithTimeout(url string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
