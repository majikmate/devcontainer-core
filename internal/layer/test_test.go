package layer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutputIgnoresStandardError(t *testing.T) {
	lt := NewT()
	// Like "deno run --check": a message on standard error, the result on standard output
	out := lt.Output("output", "sh", "-c", "echo 'Check file:///check.ts' >&2; echo 2")
	if out != "2" || lt.Failures != 0 {
		t.Fatalf("got %q with %d failures, want \"2\" without failures", out, lt.Failures)
	}
}

func TestOutputInUsesDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	lt := NewT()
	if out := lt.OutputIn("ls", dir, "ls"); out != "marker" {
		t.Fatalf("got %q, want \"marker\"", out)
	}
}

func TestOutputRecordsFailure(t *testing.T) {
	lt := NewT()
	if out := lt.Output("fails", "sh", "-c", "echo partial; exit 3"); out != "" || lt.Failures != 1 {
		t.Fatalf("got %q with %d failures, want empty output and 1 failure", out, lt.Failures)
	}
}
