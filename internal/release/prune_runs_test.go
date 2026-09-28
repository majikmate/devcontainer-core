// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package release

import (
	"slices"
	"testing"
	"time"
)

func TestRunsCutoff(t *testing.T) {
	now := time.Date(2026, 9, 27, 19, 30, 0, 0, time.UTC)
	if got := runsCutoff(now, 90).Format("2006-01-02"); got != "2026-06-29" {
		t.Errorf("runsCutoff = %s, want 2026-06-29", got)
	}
}

func TestOutdatedRuns(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC) }
	// Release (workflow 1), Prune (workflow 2) and CI (workflow 3), in any
	// order. GitHub gives every finished run the status "completed", also a
	// failed or cancelled one (its result is in the conclusion), so the
	// completed runs here stand for runs with any result.
	runs := []repoRun{
		{ID: 11, WorkflowID: 1, Workflow: "Release", Status: "completed", Created: day(1)},
		{ID: 12, WorkflowID: 1, Workflow: "Release", Status: "completed", Created: day(20)},
		{ID: 13, WorkflowID: 1, Workflow: "Release", Status: "completed", Created: day(25)},
		{ID: 21, WorkflowID: 2, Workflow: "Prune", Status: "completed", Created: day(2)},
		{ID: 22, WorkflowID: 2, Workflow: "Prune", Status: "in_progress", Created: day(27)},
		{ID: 31, WorkflowID: 3, Workflow: "CI", Status: "in_progress", Created: day(3)},
	}
	ids := func(runs []repoRun) []int64 {
		var out []int64
		for _, r := range runs {
			out = append(out, r.ID)
		}
		slices.Sort(out)
		return out
	}

	// By age: the completed runs before the cutoff; a run in progress stays
	if got := ids(outdatedRuns(runs, day(10), false)); !slices.Equal(got, []int64{11, 21}) {
		t.Errorf("by age: %v, want [11 21]", got)
	}
	// All but the newest: the newest run of each workflow stays (Release 13,
	// Prune 22 in progress, CI 31); all other completed runs are outdated
	if got := ids(outdatedRuns(runs, day(10), true)); !slices.Equal(got, []int64{11, 12, 21}) {
		t.Errorf("all but newest: %v, want [11 12 21]", got)
	}
}

func TestRunsByWorkflow(t *testing.T) {
	names, count := runsByWorkflow([]repoRun{{Workflow: "Release"}, {Workflow: "CI"}, {Workflow: "Release"}})
	if !slices.Equal(names, []string{"CI", "Release"}) || count["Release"] != 2 || count["CI"] != 1 {
		t.Errorf("runsByWorkflow = %v %v", names, count)
	}
}
