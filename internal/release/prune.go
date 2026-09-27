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
	// MaxAgeDays: the releases of the current major line that are older
	// are outdated; the newest release is always kept (0: keep all).
	MaxAgeDays int
	// AllButNewest: every release of the current major line except the
	// newest is outdated (a one-time clean-up).
	AllButNewest bool
	// DeletePackages deletes the whole packages (for packages that are no
	// longer published).
	DeletePackages bool
	// Repository (owner/name) whose completed workflow runs older than
	// RunsMaxAgeDays are deleted (0: keep all runs); with AllButNewest, all
	// completed runs except the newest run of each workflow.
	Repository     string
	RunsMaxAgeDays int
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
//   - the versions of major lines below the current major version,
//   - the releases of the current major line older than MaxAgeDays (or,
//     with AllButNewest, all but the newest release).
//
// A version that a kept image refers to (for example the image of one
// architecture in a multi-architecture image) is never deleted, and neither
// is the newest release or a version with a moving tag (2, 2.0, latest).
//
// With RunsMaxAgeDays, Prune also deletes the completed workflow runs of the
// repository that are older, or with AllButNewest all completed runs except
// the newest run of each workflow (see pruneRuns).
func Prune(o PruneOptions) error {
	gh := newGitHub(o.Token)
	mode := "report only (nothing is deleted)"
	if o.Apply {
		mode = "delete"
	}
	rules := pruneRules{Major: o.Major, MaxAgeDays: o.MaxAgeDays, AllButNewest: o.AllButNewest, Now: time.Now()}
	report := []string{"### Outdated packages", "", "Mode: " + mode, "", "Rules: " + rules.String(), ""}
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
		outdated := outdatedVersions(versions, rules, func(digest string) ([]string, error) {
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
	if o.RunsMaxAgeDays > 0 && o.Repository != "" {
		lines, runFailures := pruneRuns(gh, o.Repository, o.RunsMaxAgeDays, o.AllButNewest, rules.Now, o.Apply)
		report = append(report, lines...)
		failures = append(failures, runFailures...)
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
func outdatedVersions(versions []packageVersion, rules pruneRules, children func(digest string) ([]string, error)) []outdatedVersion {
	releases := currentReleases(versions, rules.Major)
	reasons := map[string]string{}
	for _, v := range versions {
		if r := rules.tagReason(v.Tags, releases); r != "" {
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

// pruneRules decide which tagged versions are outdated.
type pruneRules struct {
	Major        int // current major version (0: keep all major lines)
	MaxAgeDays   int // releases of the current major line older than this (0: keep all)
	AllButNewest bool
	Now          time.Time
}

func (r pruneRules) String() string {
	var rules []string
	if r.Major > 0 {
		rules = append(rules, fmt.Sprintf("major versions below %d", r.Major))
	}
	rules = append(rules, "build cache tags", "untagged versions that no kept image uses")
	switch {
	case r.Major <= 0:
	case r.AllButNewest:
		rules = append(rules, fmt.Sprintf("all releases %d.x.y except the newest", r.Major))
	case r.MaxAgeDays > 0:
		rules = append(rules, fmt.Sprintf("releases %d.x.y older than %d days (the newest is kept)", r.Major, r.MaxAgeDays))
	}
	return strings.Join(rules, ", ")
}

var (
	versionTag = regexp.MustCompile(`^([0-9]+)(\.[0-9]+)*(-[a-z0-9]+)?$`)
	// releaseTag is the tag of one release: X.Y.Z, or X.Y.Z-<arch> for the
	// image of one architecture
	releaseTag = regexp.MustCompile(`^(([0-9]+)\.[0-9]+\.[0-9]+)(-[a-z0-9]+)?$`)
)

// releaseInfo describes the releases X.Y.Z of the current major line: when
// each was created (its newest version) and which one is the newest.
type releaseInfo struct {
	created map[string]time.Time
	newest  string
}

// currentReleases collects the releases of the major line major.
func currentReleases(versions []packageVersion, major int) releaseInfo {
	info := releaseInfo{created: map[string]time.Time{}}
	if major <= 0 {
		return info
	}
	for _, v := range versions {
		for _, tag := range v.Tags {
			m := releaseTag.FindStringSubmatch(tag)
			if m == nil || m[2] != strconv.Itoa(major) {
				continue
			}
			if v.Created.After(info.created[m[1]]) {
				info.created[m[1]] = v.Created
			}
			if info.newest == "" || compareRelease(m[1], info.newest) > 0 {
				info.newest = m[1]
			}
		}
	}
	return info
}

// compareRelease compares two versions X.Y.Z by their numbers: -1, 0 or 1.
func compareRelease(a, b string) int {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(x) && i < len(y); i++ {
		p, _ := strconv.Atoi(x[i])
		q, _ := strconv.Atoi(y[i])
		if p != q {
			if p < q {
				return -1
			}
			return 1
		}
	}
	return len(x) - len(y)
}

// tagReason returns why a tagged version is outdated ("" = keep):
//
//   - all its tags are build cache tags,
//   - all its tags belong to a major line below the current one,
//   - all its tags belong to one release of the current major line that is
//     not the newest and is older than MaxAgeDays (or AllButNewest).
//
// A version with a moving tag (2, 2.0, latest) is always kept.
func (r pruneRules) tagReason(tags []string, releases releaseInfo) string {
	if len(tags) == 0 {
		return ""
	}
	cache, old := true, true
	release := ""
	for _, tag := range tags {
		if !strings.HasPrefix(tag, "buildcache-") {
			cache = false
		}
		m := versionTag.FindStringSubmatch(tag)
		n := -1
		if m != nil {
			n, _ = strconv.Atoi(m[1])
		}
		if r.Major <= 0 || n < 0 || n >= r.Major {
			old = false
		}
		// every tag must name the same release of the current major line
		if rm := releaseTag.FindStringSubmatch(tag); rm != nil && rm[2] == strconv.Itoa(r.Major) && (release == "" || release == rm[1]) {
			release = rm[1]
		} else {
			release = "-"
		}
	}
	switch {
	case cache:
		return "build cache of the former release workflow"
	case old:
		return fmt.Sprintf("major version below %d", r.Major)
	case r.Major <= 0 || release == "-" || release == "" || release == releases.newest:
		return ""
	case r.AllButNewest:
		return fmt.Sprintf("release %s, not the newest release (%s)", release, releases.newest)
	case r.MaxAgeDays > 0 && releases.created[release].Before(r.Now.AddDate(0, 0, -r.MaxAgeDays)):
		return fmt.Sprintf("release %s, older than %d days", release, r.MaxAgeDays)
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
