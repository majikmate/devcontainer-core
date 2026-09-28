// SPDX-License-Identifier: MIT
// © 2026 Hannes Stauss (scalarion@nimblescape.com)
// Licensed under the MIT License. See LICENSE in the repository root for details.

package debian

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// DistroInfoFile lists the Debian releases with their dates. It comes from the
// Debian package distro-info-data (installed by the layer os); Debian updates
// the package in stable releases when a date is set or changes.
const DistroInfoFile = "/usr/share/distro-info/debian.csv"

// Release is one Debian release from DistroInfoFile.
type Release struct {
	Version string    // for example "13"
	Series  string    // for example "trixie"
	Release time.Time // release date (zero: not released)
	EOL     time.Time // end of the regular security support (zero: not set yet)
}

// Name returns the release as "13 (trixie)".
func (r Release) Name() string {
	if r.Version == "" {
		return r.Series
	}
	return r.Version + " (" + r.Series + ")"
}

// Supported reports whether the release is released and has regular security
// support on the day today.
func (r Release) Supported(today time.Time) bool {
	return !r.Release.IsZero() && !r.Release.After(today) && (r.EOL.IsZero() || today.Before(r.EOL))
}

// ParseDistroInfo reads the CSV format of DistroInfoFile:
//
//	version,codename,series,created,release,eol,eol-lts,eol-elts
//
// The column eol is the end of the regular security support by the Debian
// security team; the later LTS dates do not count.
func ParseDistroInfo(data string) ([]Release, error) {
	reader := csv.NewReader(strings.NewReader(data))
	reader.FieldsPerRecord = -1 // rows leave out the dates that are not set yet
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no Debian releases")
	}
	column := map[string]int{}
	for i, name := range records[0] {
		column[strings.TrimSpace(name)] = i
	}
	for _, name := range []string{"version", "series", "release", "eol"} {
		if _, ok := column[name]; !ok {
			return nil, fmt.Errorf("column %s is missing", name)
		}
	}
	field := func(record []string, name string) string {
		if i := column[name]; i < len(record) {
			return strings.TrimSpace(record[i])
		}
		return ""
	}
	var releases []Release
	for _, record := range records[1:] {
		r := Release{Version: field(record, "version"), Series: field(record, "series")}
		for _, d := range []struct {
			name   string
			target *time.Time
		}{{"release", &r.Release}, {"eol", &r.EOL}} {
			if value := field(record, d.name); value != "" {
				t, err := time.Parse("2006-01-02", value)
				if err != nil {
					return nil, fmt.Errorf("release %s, %s: %w", r.Series, d.name, err)
				}
				*d.target = t
			}
		}
		releases = append(releases, r)
	}
	return releases, nil
}

// FindRelease returns the release with the series name (for example "trixie").
func FindRelease(releases []Release, series string) (Release, bool) {
	for _, r := range releases {
		if r.Series == series {
			return r, true
		}
	}
	return Release{}, false
}

// SupportedReleases returns the names of the supported releases on the day today.
func SupportedReleases(releases []Release, today time.Time) []string {
	var names []string
	for _, r := range releases {
		if r.Supported(today) {
			names = append(names, r.Name())
		}
	}
	return names
}

// InstalledSeries returns the series name of the running system, from
// VERSION_CODENAME in /etc/os-release.
func InstalledSeries() (string, error) {
	out, err := sys.Output("sh", "-c", ". /etc/os-release && echo \"$VERSION_CODENAME\"")
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", fmt.Errorf("/etc/os-release has no VERSION_CODENAME")
	}
	return out, nil
}

// DistroInfoURL is the source of DistroInfoFile in the repository of the
// Debian package distro-info-data. The release plan reads it, because it runs
// outside of the image.
const DistroInfoURL = "https://salsa.debian.org/debian/distro-info-data/-/raw/main/debian.csv"

// PublishedReleases reads the Debian releases from DistroInfoURL, or from
// DistroInfoFile of the running system when the URL cannot be read (for
// example on a GitHub runner, which has the package distro-info-data).
func PublishedReleases() ([]Release, error) {
	data, err := sys.Get(DistroInfoURL)
	if err != nil {
		local, localErr := InstalledReleases()
		if localErr != nil {
			return nil, fmt.Errorf("%w; %v", err, localErr)
		}
		return local, nil
	}
	return ParseDistroInfo(string(data))
}

// Released returns the releases that are released on the day today, newest
// first.
func Released(releases []Release, today time.Time) []Release {
	var result []Release
	for _, r := range releases {
		if !r.Release.IsZero() && !r.Release.After(today) {
			result = append(result, r)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Release.After(result[j].Release) })
	return result
}

// InstalledReleases reads DistroInfoFile.
func InstalledReleases() ([]Release, error) {
	data, err := os.ReadFile(DistroInfoFile)
	if err != nil {
		return nil, fmt.Errorf("%w (package distro-info-data)", err)
	}
	return ParseDistroInfo(string(data))
}
