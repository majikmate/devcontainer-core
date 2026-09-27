package layer

import (
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
	err := exec.Command(name, args...).Run()
	t.Check(description, err == nil)
}

// HasCommand checks that a program is in PATH.
func (t *T) HasCommand(name string) {
	_, err := exec.LookPath(name)
	t.Check("command "+name, err == nil)
}

// Output runs a program and returns its output (empty on error, which is recorded).
func (t *T) Output(description, name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Check(description, false)
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Version records the installed version of a tool. The release tool compares
// it with the expected version and lists it in the release notes.
func (t *T) Version(tool, version string) {
	version = strings.TrimSpace(version)
	t.Versions[tool] = version
	fmt.Printf("version: %s=%s\n", tool, version)
}
