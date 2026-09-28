// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sys

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var client = &http.Client{Timeout: 10 * time.Minute}

// Get downloads a URL into memory (for small files such as version lists).
func Get(url string) ([]byte, error) {
	return GetWithToken(url, "")
}

// GetWithToken is Get with a bearer token (sent only if not empty).
func GetWithToken(url, token string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		data, err := get(url, token)
		if err == nil {
			return data, nil
		}
		lastErr = err
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	return nil, lastErr
}

func get(url, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// Download downloads a URL into a file.
func Download(url, path string) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		err := download(url, path)
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	return lastErr
}

func download(url, path string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// SHA256 returns the SHA-256 checksum of a file in hexadecimal.
func SHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifySHA256 returns an error if the checksum of a file differs from want.
func VerifySHA256(path, want string) error {
	if want == "" {
		return fmt.Errorf("no checksum known for %s", path)
	}
	got, err := SHA256(path)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", path, want, got)
	}
	Logf("Checksum verified: %s", path)
	return nil
}
