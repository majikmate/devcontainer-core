package release

import "testing"

func TestNextVersion(t *testing.T) {
	cases := []struct{ last, step, want string }{
		{"", "patch", "2.0.0"},
		{"2.0.3", "patch", "2.0.4"},
		{"2.0.3", "minor", "2.1.0"},
		{"2.4.3", "major", "3.0.0"},
	}
	for _, c := range cases {
		if got := nextVersion(c.last, 2, c.step); got != c.want {
			t.Errorf("nextVersion(%q, %s) = %s, want %s", c.last, c.step, got, c.want)
		}
	}
}

func TestBumpStep(t *testing.T) {
	old := map[string]string{"tool/go": "1.27.1", "tool/node": "v24.21.0", "tool/deno": "v2.9.3"}
	cases := []struct {
		name    string
		changed map[string]string
		want    string
	}{
		{"go patch", map[string]string{"tool/go": "1.27.2"}, "patch"},
		{"go minor", map[string]string{"tool/go": "1.28"}, "minor"},
		{"node major", map[string]string{"tool/node": "v26.0.0"}, "minor"},
		{"node minor", map[string]string{"tool/node": "v24.22.0"}, "patch"},
		{"deno major", map[string]string{"tool/deno": "v3.0.0"}, "minor"},
	}
	for _, c := range cases {
		inputs := map[string]string{}
		for k, v := range old {
			inputs[k] = v
		}
		for k, v := range c.changed {
			inputs[k] = v
		}
		if got := bumpStep("auto", old, inputs); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	if got := bumpStep("major", old, old); got != "major" {
		t.Errorf("explicit bump: got %s", got)
	}
}

func TestInputsRoundTrip(t *testing.T) {
	in := map[string]string{"config": "abc", "tool/go": "1.27.1", "image/x:1": "0123"}
	line := formatInputs(in)
	if line != "config=abc image/x:1=0123 tool/go=1.27.1" {
		t.Errorf("line = %q", line)
	}
	out := parseInputs(line)
	if len(out) != 3 || out["tool/go"] != "1.27.1" {
		t.Errorf("parsed = %v", out)
	}
	if ch := changedInputs(in, map[string]string{"config": "abc", "tool/go": "1.28", "image/x:1": "0123"}); len(ch) != 1 || ch[0] != "tool/go=1.28" {
		t.Errorf("changes = %v", ch)
	}
}
