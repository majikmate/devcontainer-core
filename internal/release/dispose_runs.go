package release

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

// DisposeOptions are the settings of "devcon-release dispose-runs".
type DisposeOptions struct {
	Repositories []string // owner/name
	Workflows    []string // workflow names, for example "Dependabot Updates"
	Apply        bool     // delete; otherwise only report
	Token        string
}

// repoWorkflow is one workflow of a repository.
type repoWorkflow struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// matchWorkflows returns the workflows whose name is one of names.
func matchWorkflows(workflows []repoWorkflow, names []string) []repoWorkflow {
	var matched []repoWorkflow
	for _, w := range workflows {
		for _, name := range names {
			if w.Name == name {
				matched = append(matched, w)
				break
			}
		}
	}
	return matched
}

// DisposeRuns deletes (or reports) all completed runs of the named workflows
// in the repositories, for example the runs of the dynamic workflows
// "Dependabot Updates" and "Dev Environment Prebuilds" that are no longer
// used. Runs that are not completed are kept.
func DisposeRuns(o DisposeOptions) error {
	gh := newGitHub(o.Token)
	mode := "report only (nothing is deleted)"
	if o.Apply {
		mode = "delete"
	}
	report := []string{"### Disposed workflow runs", "", "Mode: " + mode, "",
		"Workflows: " + strings.Join(o.Workflows, ", "), "",
		"| Repository | Workflow | Runs | Result |", "| --- | --- | --- | --- |"}
	var failures []string
	for _, repository := range o.Repositories {
		var list struct {
			Workflows []repoWorkflow `json:"workflows"`
		}
		status, err := gh.do(http.MethodGet, "repos/"+repository+"/actions/workflows?per_page=100", nil, &list)
		if err == nil && status == http.StatusNotFound {
			err = fmt.Errorf("not accessible (is the GitHub App installed on the repository?)")
		}
		if err != nil {
			report = append(report, fmt.Sprintf("| %s | — | — | failed: %v |", repository, err))
			failures = append(failures, fmt.Sprintf("%s: %v", repository, err))
			continue
		}
		matched := matchWorkflows(list.Workflows, o.Workflows)
		if len(matched) == 0 {
			report = append(report, fmt.Sprintf("| %s | — | 0 | none of the workflows exists |", repository))
			continue
		}
		for _, w := range matched {
			runs, err := listRuns(gh, fmt.Sprintf("repos/%s/actions/workflows/%d/runs", repository, w.ID), "")
			if err != nil {
				report = append(report, fmt.Sprintf("| %s | %s | — | failed: %v |", repository, w.Name, err))
				failures = append(failures, fmt.Sprintf("%s %s: %v", repository, w.Name, err))
				continue
			}
			var completed []repoRun
			for _, r := range runs {
				if r.Status == "completed" {
					completed = append(completed, r)
				}
			}
			result := "would be deleted"
			if o.Apply {
				failed := 0
				for _, r := range completed {
					if _, err := gh.do(http.MethodDelete, fmt.Sprintf("repos/%s/actions/runs/%d", repository, r.ID), nil, nil); err != nil {
						failed++
						failures = append(failures, fmt.Sprintf("%s run %d: %v", repository, r.ID, err))
					}
				}
				result = "deleted"
				if failed > 0 {
					result = fmt.Sprintf("%d failed", failed)
				}
			}
			if kept := len(runs) - len(completed); kept > 0 {
				result += fmt.Sprintf(" (%d not completed, kept)", kept)
			}
			report = append(report, fmt.Sprintf("| %s | %s | %d | %s |", repository, w.Name, len(completed), result))
		}
	}
	// The report goes to the run summary and to the job log
	Summary(strings.Join(report, "\n"))
	if os.Getenv("GITHUB_STEP_SUMMARY") != "" {
		fmt.Println(strings.Join(report, "\n"))
	}
	if len(failures) > 0 {
		return fmt.Errorf("dispose-runs: %s", strings.Join(failures, "; "))
	}
	return nil
}
