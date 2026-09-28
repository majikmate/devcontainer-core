// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package release

import (
	"strings"
	"testing"
)

func TestParseSupport(t *testing.T) {
	if err := parseSupport("supported\tos\n"); err != nil {
		t.Errorf("supported: %v", err)
	}
	err := parseSupport("end-of-life\tos\tDebian 12 (bookworm) has reached its end of life (regular security support ended on 2026-06-10).\n")
	if err == nil || !strings.HasPrefix(err.Error(), "layer os: Debian 12 (bookworm) has reached its end of life") {
		t.Errorf("end of life: %v", err)
	}
}
