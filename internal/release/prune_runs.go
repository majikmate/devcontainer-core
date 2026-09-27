package release

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"
)

// repoRun is one workflow run of a repository.
type repoRun struct {
	ID         int64
	WorkflowID int64
	Workflow   string
	Status     string
	Created    time.Time
}

// runsCutoff is the day before which a workflow run is outdated.
func runsCutoff(now time.Time, maxAgeDays int) time.Time {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -maxAgeDays)
}

// listRuns reads the workflow runs of the repository, newest first. query
// filters the list (for example created=<2026-01-01); GitHub returns at most
// 1000 runs for a filtered list, and the next prune deletes the rest.
func listRuns(gh *gitHub, repository, query string) ([]repoRun, error) {
	var runs []repoRun
	for page := 1; page <= 100; page++ {
		var list struct {
			WorkflowRuns []struct {
				ID         int64     `json:"id"`
				WorkflowID int64     `json:"workflow_id"`
				Name       string    `json:"name"`
				Status     string    `json:"status"`
				CreatedAt  time.Time `json:"created_at"`
			} `json:"workflow_runs"`
		}
		path := fmt.Sprintf("repos/%s/actions/runs?per_page=100&page=%d", repository, page)
		if query != "" {
			path += "&" + query
		}
		if _, err := gh.do(http.MethodGet, path, nil, &list); err != nil {
			return nil, err
		}
		for _, r := range list.WorkflowRuns {
			runs = append(runs, repoRun{ID: r.ID, WorkflowID: r.WorkflowID, Workflow: r.Name, Status: r.Status, Created: r.CreatedAt})
		}
		if len(list.WorkflowRuns) < 100 {
			break
		}
	}
	return runs, nil
}

// outdatedRuns selects the workflow runs to delete. A run that is not
// completed is always kept. With allButNewest, every completed run except the
// newest run of each workflow is outdated; otherwise every completed run
// created before the cutoff day.
func outdatedRuns(runs []repoRun, cutoff time.Time, allButNewest bool) []repoRun {
	sorted := append([]repoRun(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Created.After(sorted[j].Created) })
	newest := map[int64]bool{}
	var outdated []repoRun
	for _, r := range sorted {
		first := !newest[r.WorkflowID]
		newest[r.WorkflowID] = true
		if r.Status != "completed" {
			continue
		}
		if allButNewest {
			if !first {
				outdated = append(outdated, r)
			}
		} else if r.Created.Before(cutoff) {
			outdated = append(outdated, r)
		}
	}
	return outdated
}

// runsByWorkflow counts the runs per workflow name, sorted by name.
func runsByWorkflow(runs []repoRun) ([]string, map[string]int) {
	count := map[string]int{}
	for _, r := range runs {
		count[r.Workflow]++
	}
	names := make([]string, 0, len(count))
	for name := range count {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, count
}

// pruneRuns deletes (or reports) the outdated workflow runs of the
// repository: the completed runs older than maxAgeDays, or with allButNewest
// all completed runs except the newest run of each workflow. It returns the
// report lines and the failures.
func pruneRuns(gh *gitHub, repository string, maxAgeDays int, allButNewest bool, now time.Time, apply bool) ([]string, []string) {
	cutoff := runsCutoff(now, maxAgeDays)
	rule := fmt.Sprintf("completed runs older than %d days (created before %s)", maxAgeDays, cutoff.Format("2006-01-02"))
	query := "created=" + url.QueryEscape("<"+cutoff.Format("2006-01-02"))
	if allButNewest {
		rule = "all completed runs except the newest run of each workflow"
		query = ""
	}
	runs, err := listRuns(gh, repository, query)
	if err != nil {
		return []string{fmt.Sprintf("**Workflow runs of %s**: failed (%v)", repository, err), ""},
			[]string{fmt.Sprintf("workflow runs: %v", err)}
	}
	outdated := outdatedRuns(runs, cutoff, allButNewest)
	report := []string{fmt.Sprintf("**Workflow runs of %s**: %d outdated (%s)", repository, len(outdated), rule), ""}
	if len(outdated) == 0 {
		return report, nil
	}
	failed := map[string]int{}
	var failures []string
	if apply {
		for _, r := range outdated {
			if _, err := gh.do(http.MethodDelete, fmt.Sprintf("repos/%s/actions/runs/%d", repository, r.ID), nil, nil); err != nil {
				failed[r.Workflow]++
				failures = append(failures, fmt.Sprintf("workflow run %d: %v", r.ID, err))
			}
		}
	}
	report = append(report, "| Workflow | Runs | Result |", "| --- | --- | --- |")
	names, count := runsByWorkflow(outdated)
	for _, name := range names {
		result := "would be deleted"
		if apply {
			result = "deleted"
			if failed[name] > 0 {
				result = fmt.Sprintf("%d failed", failed[name])
			}
		}
		report = append(report, fmt.Sprintf("| %s | %d | %s |", name, count[name], result))
	}
	return append(report, ""), failures
}
