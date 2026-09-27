package release

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PruneOptions are the settings of "devcon-release prune".
type PruneOptions struct {
	Org      string   // owner of the packages, for example majikmate
	Packages []string // container packages, for example devcontainer-base
	// Major is the current major version of the image: the versions of
	// lower major lines are outdated (0: keep all major lines).
	Major int
	// DeletePackages deletes the whole packages (for packages that are no
	// longer published).
	DeletePackages bool
	Apply          bool // delete; otherwise only report
	Token          string
}

// packageVersion is one version of a container package on ghcr.io.
type packageVersion struct {
	ID      int64
	Digest  string
	Tags    []string
	Created time.Time
}

// Prune deletes the outdated versions of container packages (or the whole
// packages) and writes a report to the run summary. Outdated versions:
//
//   - untagged versions that no kept image refers to (older builds whose
//     tags moved to a newer build),
//   - the tags buildcache-* of the former release workflow,
//   - the versions of major lines below the current major version.
//
// A version that a kept image refers to (for example the image of one
// architecture in a multi-architecture image) is never deleted.
func Prune(o PruneOptions) error {
	gh := newGitHub(o.Token)
	mode := "report only (nothing is deleted)"
	if o.Apply {
		mode = "delete"
	}
	report := []string{"### Outdated packages", "", "Mode: " + mode, ""}
	var failures []string
	for _, name := range o.Packages {
		path := fmt.Sprintf("orgs/%s/packages/container/%s", o.Org, url.PathEscape(name))
		if o.DeletePackages {
			line, err := prunePackage(gh, path, name, o.Apply)
			report = append(report, line)
			if err != nil {
				failures = append(failures, err.Error())
			}
			continue
		}
		versions, err := listVersions(gh, path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		outdated := outdatedVersions(versions, o.Major, func(digest string) ([]string, error) {
			return indexChildren(fmt.Sprintf("ghcr.io/%s/%s@%s", o.Org, name, digest))
		})
		report = append(report, fmt.Sprintf("**%s**: %d of %d versions outdated", name, len(outdated), len(versions)), "")
		if len(outdated) > 0 {
			report = append(report, "| Version | Tags | Created | Reason | Result |", "| --- | --- | --- | --- | --- |")
		}
		for _, d := range outdated {
			result := "would be deleted"
			if o.Apply {
				result = "deleted"
				if _, err := gh.do(http.MethodDelete, fmt.Sprintf("%s/versions/%d", path, d.version.ID), nil, nil); err != nil {
					result = "failed"
					failures = append(failures, fmt.Sprintf("%s %s: %v", name, shortDigest(d.version.Digest), err))
				}
			}
			report = append(report, fmt.Sprintf("| `%s` | %s | %s | %s | %s |", shortDigest(d.version.Digest),
				strings.Join(d.version.Tags, " "), d.version.Created.Format("2006-01-02"), d.reason, result))
		}
		report = append(report, "")
	}
	// The report goes to the run summary and to the job log
	Summary(strings.Join(report, "\n"))
	if os.Getenv("GITHUB_STEP_SUMMARY") != "" {
		fmt.Println(strings.Join(report, "\n"))
	}
	if len(failures) > 0 {
		return fmt.Errorf("prune: %s", strings.Join(failures, "; "))
	}
	return nil
}

// prunePackage deletes (or reports) a whole package.
func prunePackage(gh *gitHub, path, name string, apply bool) (string, error) {
	var info struct {
		VersionCount int `json:"version_count"`
	}
	status, err := gh.do(http.MethodGet, path, nil, &info)
	if err != nil {
		return fmt.Sprintf("- **%s**: failed (%v)", name, err), err
	}
	if status == http.StatusNotFound {
		return fmt.Sprintf("- **%s**: does not exist", name), nil
	}
	if !apply {
		return fmt.Sprintf("- **%s** (%d versions): would be deleted", name, info.VersionCount), nil
	}
	if _, err := gh.do(http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Sprintf("- **%s**: failed (%v)", name, err), fmt.Errorf("%s: %w", name, err)
	}
	return fmt.Sprintf("- **%s** (%d versions): deleted", name, info.VersionCount), nil
}

// listVersions reads all versions of a container package.
func listVersions(gh *gitHub, path string) ([]packageVersion, error) {
	var result []packageVersion
	for page := 1; ; page++ {
		var list []struct {
			ID        int64     `json:"id"`
			Name      string    `json:"name"`
			CreatedAt time.Time `json:"created_at"`
			Metadata  struct {
				Container struct {
					Tags []string `json:"tags"`
				} `json:"container"`
			} `json:"metadata"`
		}
		status, err := gh.do(http.MethodGet, fmt.Sprintf("%s/versions?per_page=100&page=%d", path, page), nil, &list)
		if err != nil {
			return nil, err
		}
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("package not found")
		}
		for _, v := range list {
			result = append(result, packageVersion{ID: v.ID, Digest: v.Name, Tags: v.Metadata.Container.Tags, Created: v.CreatedAt})
		}
		if len(list) < 100 {
			return result, nil
		}
	}
}

// indexChildren returns the digests that an image index refers to (nil for
// a single image).
func indexChildren(ref string) ([]string, error) {
	data, mediaType, err := registryClient.Manifest(ref)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(mediaType, "index") && !strings.Contains(mediaType, "manifest.list") {
		return nil, nil
	}
	var index struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	var digests []string
	for _, m := range index.Manifests {
		digests = append(digests, m.Digest)
	}
	return digests, nil
}

// outdatedVersion is a version to delete and why.
type outdatedVersion struct {
	version packageVersion
	reason  string
}

// outdatedVersions decides which versions are outdated. children returns the
// digests that a version refers to (the images of a multi-architecture
// image); a version that a kept version refers to is kept. When the
// references of a kept version cannot be read, no untagged version is
// deleted.
func outdatedVersions(versions []packageVersion, major int, children func(digest string) ([]string, error)) []outdatedVersion {
	reasons := map[string]string{}
	for _, v := range versions {
		if r := tagReason(v.Tags, major); r != "" {
			reasons[v.Digest] = r
		}
	}
	// The references of the kept tagged versions
	referenced := map[string]bool{}
	complete := true
	for _, v := range versions {
		if len(v.Tags) > 0 && reasons[v.Digest] == "" {
			digests, err := children(v.Digest)
			if err != nil {
				warning("Cannot read %s (%v); untagged versions are kept", shortDigest(v.Digest), err)
				complete = false
			}
			for _, d := range digests {
				referenced[d] = true
			}
		}
	}
	var result []outdatedVersion
	for _, v := range versions {
		reason := reasons[v.Digest]
		if len(v.Tags) == 0 && complete {
			reason = "untagged, no kept image refers to it"
		}
		if reason == "" || referenced[v.Digest] {
			continue
		}
		result = append(result, outdatedVersion{version: v, reason: reason})
	}
	// Tagged versions first (images before their parts), then oldest first
	sort.SliceStable(result, func(i, j int) bool {
		ti, tj := len(result[i].version.Tags) > 0, len(result[j].version.Tags) > 0
		if ti != tj {
			return ti
		}
		return result[i].version.Created.Before(result[j].version.Created)
	})
	return result
}

var versionTag = regexp.MustCompile(`^([0-9]+)(\.[0-9]+)*(-[a-z0-9]+)?$`)

// tagReason returns why a tagged version is outdated ("" = keep): all its
// tags are build cache tags or belong to a major line below major.
func tagReason(tags []string, major int) string {
	if len(tags) == 0 {
		return ""
	}
	cache, old := true, true
	for _, tag := range tags {
		if !strings.HasPrefix(tag, "buildcache-") {
			cache = false
		}
		m := versionTag.FindStringSubmatch(tag)
		n := -1
		if m != nil {
			n, _ = strconv.Atoi(m[1])
		}
		if major <= 0 || n < 0 || n >= major {
			old = false
		}
	}
	switch {
	case cache:
		return "build cache of the former release workflow"
	case old:
		return fmt.Sprintf("major version below %d", major)
	}
	return ""
}

func shortDigest(digest string) string {
	digest = strings.TrimPrefix(digest, "sha256:")
	if len(digest) > 12 {
		digest = digest[:12]
	}
	return digest
}
