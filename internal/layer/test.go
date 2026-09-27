package layer

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// T collects the results of a layer test. Tests run as the development user
// inside the built image.
type T struct {
	Failures int
	Versions map[string]string
}

// NewT creates an empty test result.
func NewT() *T {
	return &T{Versions: map[string]string{}}
}

// Check records a check.
func (t *T) Check(description string, ok bool) {
	if ok {
		fmt.Printf("  ok    %s\n", description)
		return
	}
	fmt.Printf("  FAIL  %s\n", description)
	t.Failures++
}

// Command checks that a program runs without error.
func (t *T) Command(description, name string, args ...string) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	t.Check(description, err == nil)
	if err != nil {
		printError(err, stderr.String())
	}
}

// HasCommand checks that a program is in PATH.
func (t *T) HasCommand(name string) {
	_, err := exec.LookPath(name)
	t.Check("command "+name, err == nil)
}

// Output runs a program and returns its standard output (empty on error, which
// is recorded). Messages on standard error, such as "Check file:///..." of
// "deno run --check", are not part of the result.
func (t *T) Output(description, name string, args ...string) string {
	return t.OutputIn(description, "", name, args...)
}

// OutputIn is Output with the working directory dir (for example a Go module
// folder: "go run ." works only inside the module).
func (t *T) OutputIn(description, dir, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Check(description, false)
		printError(err, stderr.String())
		return ""
	}
	return strings.TrimSpace(string(out))
}

// printError shows why a command failed: the error and the first lines of its
// standard error.
func printError(err error, stderr string) {
	fmt.Printf("        error: %v\n", err)
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) > 10 {
		lines = append(lines[:10], "...")
	}
	for _, line := range lines {
		if line != "" {
			fmt.Printf("        %s\n", line)
		}
	}
}

// Version records the installed version of a tool. The release tool compares
// it with the expected version and lists it in the release notes.
func (t *T) Version(tool, version string) {
	version = strings.TrimSpace(version)
	t.Versions[tool] = version
	fmt.Printf("version: %s=%s\n", tool, version)
}
