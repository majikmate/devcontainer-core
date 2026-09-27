// Package layers contains all layers. Each file registers one layer; see the
// package layer for what a layer declares.
package layers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// lastWord returns the last word of a text (for example the version in
// "git version 2.47.3").
func lastWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// tempDir creates a temporary folder; the returned function removes it.
func tempDir() (string, func(), error) {
	dir, err := os.MkdirTemp("", "devcon-")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

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
