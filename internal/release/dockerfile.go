// Package release is the release tool of the Dev Container images: it plans a
// release (inputs, decision, version), builds and tests an image, and
// publishes it. The shared workflow runs it with "go run".
package release

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Dockerfile is what the release tool needs to know about a Dockerfile.
type Dockerfile struct {
	// BaseImages are the external images in FROM lines (not build stages).
	BaseImages []string
	// Layers are the layers of "devcon install" lines, in order.
	Layers []string
	// FinalBase is the external image of the last FROM line ("" when the
	// final stage starts from a build stage or from scratch).
	FinalBase string
}

// ReadDockerfile reads the base images and the installed layers.
func ReadDockerfile(path string) (*Dockerfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result := &Dockerfile{}
	stages := map[string]bool{}
	var logical strings.Builder
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, "\\") {
			logical.WriteString(strings.TrimSuffix(line, "\\") + " ")
			continue
		}
		logical.WriteString(line)
		if err := result.parse(logical.String(), stages); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		logical.Reset()
	}
	return result, scanner.Err()
}

func (d *Dockerfile) parse(line string, stages map[string]bool) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	switch strings.ToUpper(fields[0]) {
	case "FROM":
		args := fields[1:]
		for len(args) > 0 && strings.HasPrefix(args[0], "--") {
			args = args[1:]
		}
		if len(args) == 0 {
			return fmt.Errorf("FROM without image")
		}
		image := args[0]
		if strings.Contains(image, "$") {
			return fmt.Errorf("FROM with a variable is not supported: %s", image)
		}
		external := !stages[image] && image != "scratch"
		if external && !contains(d.BaseImages, image) {
			d.BaseImages = append(d.BaseImages, image)
		}
		// The last FROM line wins: the image that the final stage starts from
		d.FinalBase = ""
		if external {
			d.FinalBase = image
		}
		if len(args) >= 3 && strings.EqualFold(args[1], "AS") {
			stages[args[2]] = true
		}
	case "RUN":
		args := fields[1:]
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "devcon" && args[i+1] == "install" {
				for _, name := range args[i+2:] {
					if name == "&&" || name == ";" {
						break
					}
					d.Layers = append(d.Layers, name)
				}
			}
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
