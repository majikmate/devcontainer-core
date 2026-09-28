// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// SetOutput writes a step output (multi-line values are supported).
func SetOutput(name, value string) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		fmt.Printf("output %s=%s\n", name, value)
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	delimiter := fmt.Sprintf("EOF_%d", time.Now().UnixNano())
	_, err = fmt.Fprintf(f, "%s<<%s\n%s\n%s\n", name, delimiter, value, delimiter)
	return err
}

// Summary adds Markdown to the summary of the workflow run.
func Summary(markdown string) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		fmt.Println(markdown)
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, markdown)
}

// warning shows a warning in the workflow run.
func warning(format string, args ...any) {
	fmt.Printf("::warning::"+format+"\n", args...)
}

// notice shows a notice in the workflow run.
func notice(format string, args ...any) {
	fmt.Printf("::notice::"+format+"\n", args...)
}

// gitHub is a small client of the GitHub REST API.
type gitHub struct {
	token string
	http  *http.Client
}

func newGitHub(token string) *gitHub {
	return &gitHub{token: token, http: &http.Client{Timeout: time.Minute}}
}

// do sends a request; out receives the JSON answer (may be nil). It returns
// the HTTP status code.
func (g *gitHub) do(method, path string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(data)
	}
	url := path
	if !strings.HasPrefix(url, "https://") {
		url = "https://api.github.com/" + strings.TrimPrefix(path, "/")
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return resp.StatusCode, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil && resp.StatusCode < 300 && len(data) > 0 {
		return resp.StatusCode, json.Unmarshal(data, out)
	}
	return resp.StatusCode, nil
}
