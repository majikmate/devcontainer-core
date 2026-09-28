// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package registry reads images from container registries (Docker Hub,
// ghcr.io and other registries of the OCI distribution API) without other
// tools: the raw manifest of an image, its labels and its creation time.
// Public images need no login; the anonymous token of the registry is used.
package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const manifestTypes = "application/vnd.oci.image.index.v1+json, " +
	"application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

// Ref is a parsed image reference.
type Ref struct {
	Host       string // registry host, for example registry-1.docker.io
	Repository string // for example library/buildpack-deps
	Reference  string // tag or digest
}

// ParseRef parses an image reference such as "buildpack-deps:trixie-curl" or
// "ghcr.io/owner/name:2".
func ParseRef(ref string) (Ref, error) {
	r := Ref{Host: "registry-1.docker.io", Reference: "latest"}
	name := ref
	if i := strings.Index(name, "@"); i >= 0 {
		r.Reference, name = name[i+1:], name[:i]
	} else if i := strings.LastIndex(name, ":"); i >= 0 && !strings.Contains(name[i:], "/") {
		r.Reference, name = name[i+1:], name[:i]
	}
	if first, rest, ok := strings.Cut(name, "/"); ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
		r.Host, name = first, rest
		if r.Host == "docker.io" {
			r.Host = "registry-1.docker.io"
		}
	}
	if r.Host == "registry-1.docker.io" && !strings.Contains(name, "/") {
		name = "library/" + name
	}
	if name == "" {
		return Ref{}, fmt.Errorf("invalid image reference %q", ref)
	}
	r.Repository = name
	return r, nil
}

// Client reads from registries. It caches the tokens per repository.
type Client struct {
	http   *http.Client
	tokens map[string]string
}

// NewClient creates a client.
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 2 * time.Minute}, tokens: map[string]string{}}
}

// Manifest returns the raw manifest of an image (for a multi-architecture
// image: its index) and its media type.
func (c *Client) Manifest(ref string) ([]byte, string, error) {
	r, err := ParseRef(ref)
	if err != nil {
		return nil, "", err
	}
	return c.get(r, "manifests/"+r.Reference, manifestTypes)
}

// Image is what the release tool needs from an image configuration.
type Image struct {
	Labels  map[string]string
	Created time.Time
}

// Inspect returns the labels and the creation time of an image (for a
// multi-architecture image: of its linux/amd64 image).
func (c *Client) Inspect(ref string) (*Image, error) {
	r, err := ParseRef(ref)
	if err != nil {
		return nil, err
	}
	data, mediaType, err := c.get(r, "manifests/"+r.Reference, manifestTypes)
	if err != nil {
		return nil, err
	}
	if strings.Contains(mediaType, "index") || strings.Contains(mediaType, "manifest.list") {
		var index struct {
			Manifests []struct {
				Digest   string `json:"digest"`
				Platform struct {
					OS           string `json:"os"`
					Architecture string `json:"architecture"`
				} `json:"platform"`
			} `json:"manifests"`
		}
		if err := json.Unmarshal(data, &index); err != nil {
			return nil, err
		}
		digest := ""
		for _, m := range index.Manifests {
			if m.Platform.OS == "linux" && m.Platform.Architecture == "amd64" {
				digest = m.Digest
				break
			}
		}
		if digest == "" {
			return nil, fmt.Errorf("%s has no linux/amd64 image", ref)
		}
		if data, _, err = c.get(r, "manifests/"+digest, manifestTypes); err != nil {
			return nil, err
		}
	}
	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	blob, _, err := c.get(r, "blobs/"+manifest.Config.Digest, "*/*")
	if err != nil {
		return nil, err
	}
	var config struct {
		Created time.Time `json:"created"`
		Config  struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(blob, &config); err != nil {
		return nil, err
	}
	if config.Config.Labels == nil {
		config.Config.Labels = map[string]string{}
	}
	return &Image{Labels: config.Config.Labels, Created: config.Created}, nil
}

// ErrNotFound is returned for an image that does not exist.
var ErrNotFound = fmt.Errorf("image not found")

func (c *Client) get(r Ref, path, accept string) ([]byte, string, error) {
	u := "https://" + r.Host + "/v2/" + r.Repository + "/" + path
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Accept", accept)
		if token := c.tokens[r.Host+"/"+r.Repository]; token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, "", err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, "", err
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return data, resp.Header.Get("Content-Type"), nil
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0:
			if err := c.authenticate(r, resp.Header.Get("WWW-Authenticate")); err != nil {
				return nil, "", err
			}
		case resp.StatusCode == http.StatusNotFound:
			return nil, "", ErrNotFound
		default:
			return nil, "", fmt.Errorf("GET %s: %s", u, resp.Status)
		}
	}
	return nil, "", fmt.Errorf("GET %s: not authorized", u)
}

var challengeParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// authenticate gets an anonymous pull token from the token service that the
// registry names in its challenge.
func (c *Client) authenticate(r Ref, challenge string) error {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return fmt.Errorf("%s: unsupported authentication %q", r.Host, challenge)
	}
	params := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(challenge, -1) {
		params[strings.ToLower(m[1])] = m[2]
	}
	query := url.Values{}
	if params["service"] != "" {
		query.Set("service", params["service"])
	}
	query.Set("scope", "repository:"+r.Repository+":pull")
	resp, err := c.http.Get(params["realm"] + "?" + query.Encode())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token of %s: %s", r.Host, resp.Status)
	}
	var token struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return err
	}
	if token.Token == "" {
		token.Token = token.AccessToken
	}
	c.tokens[r.Host+"/"+r.Repository] = token.Token
	return nil
}
