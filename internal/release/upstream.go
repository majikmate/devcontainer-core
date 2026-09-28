// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package release

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// UpstreamOptions are the settings of a chain build.
type UpstreamOptions struct {
	Owner        string
	Repositories []string // repositories of the images that this image builds on
	Actor        string   // login of the GitHub App, for example "name[bot]"
	Token        string   // installation token of the GitHub App
	Timeout      time.Duration
}

type workflowRun struct {
	ID         int64     `json:"id"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
	Actor      struct {
		Login string `json:"login"`
	} `json:"triggering_actor"`
}

// Upstream starts the Release workflow of every upstream repository and waits
// for it. The chain build is recursive: the upstream run gets upstream=true, so
// it first runs the Release workflows of its own upstream images. Example:
// classroom-web starts base, base starts core; core finishes first, then base,
// then classroom-web does its own check. Every release.yml therefore declares
// the dispatch inputs upstream, force and bump. A failed run stops the chain.
func Upstream(o UpstreamOptions) error {
	gh := newGitHub(o.Token)
	for _, repo := range o.Repositories {
		full := o.Owner + "/" + repo
		start := time.Now().Add(-time.Minute) // margin for a clock difference
		fmt.Printf("Starting the Release workflow of %s (with its own upstream images)\n", full)
		if _, err := gh.do(http.MethodPost, "repos/"+full+"/actions/workflows/release.yml/dispatches", map[string]any{
			"ref":    "main",
			"inputs": map[string]string{"upstream": "true", "force": "false", "bump": "auto"},
		}, nil); err != nil {
			return err
		}
		run, err := findRun(gh, full, o.Actor, start)
		if err != nil {
			return err
		}
		fmt.Printf("Waiting for %s\n", run.HTMLURL)
		if err := waitForRun(gh, full, run.ID, o.Timeout); err != nil {
			return err
		}
		Summary(fmt.Sprintf("- %s: [release run](%s) finished", full, run.HTMLURL))
	}
	return nil
}

// findRun finds the run that the App started (within 5 minutes).
func findRun(gh *gitHub, repo, actor string, since time.Time) (*workflowRun, error) {
	query := url.Values{"event": {"workflow_dispatch"}, "created": {">=" + since.UTC().Format(time.RFC3339)}}
	for attempt := 0; attempt < 60; attempt++ {
		time.Sleep(5 * time.Second)
		var result struct {
			Runs []workflowRun `json:"workflow_runs"`
		}
		if _, err := gh.do(http.MethodGet, "repos/"+repo+"/actions/workflows/release.yml/runs?"+query.Encode(), nil, &result); err != nil {
			return nil, err
		}
		var mine []workflowRun
		for _, r := range result.Runs {
			if r.Actor.Login == actor {
				mine = append(mine, r)
			}
		}
		if len(mine) > 0 {
			sort.Slice(mine, func(i, j int) bool { return mine[i].CreatedAt.Before(mine[j].CreatedAt) })
			return &mine[0], nil
		}
	}
	return nil, fmt.Errorf("the Release workflow of %s did not start within 5 minutes", repo)
}

func waitForRun(gh *gitHub, repo string, id int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var run workflowRun
		if _, err := gh.do(http.MethodGet, fmt.Sprintf("repos/%s/actions/runs/%d", repo, id), nil, &run); err != nil {
			return err
		}
		if run.Status == "completed" {
			if run.Conclusion != "success" {
				return fmt.Errorf("the Release workflow of %s ended with %q: %s", repo, run.Conclusion, run.HTMLURL)
			}
			return nil
		}
		time.Sleep(30 * time.Second)
	}
	return fmt.Errorf("the Release workflow of %s did not finish within %s", repo, timeout)
}

// KeepAlive enables a scheduled workflow again. GitHub disables scheduled
// workflows in public repositories after 60 days without activity.
func KeepAlive(repository, workflowRef, token string) error {
	// "owner/repo/.github/workflows/release.yml@refs/heads/main" -> "release.yml"
	workflowFile, _, _ := strings.Cut(workflowRef, "@")
	workflowFile = path.Base(workflowFile)
	_, err := newGitHub(token).do(http.MethodPut, "repos/"+repository+"/actions/workflows/"+workflowFile+"/enable", nil, nil)
	if err != nil {
		warning("Could not re-enable workflow %s: %v", workflowFile, err)
	}
	return nil
}
