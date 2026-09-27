package layers

import (
	"github.com/majikmate/devcontainer-core/internal/layer"
	"github.com/majikmate/devcontainer-core/internal/sys"
)

// System-wide git settings for simple workflows: pull rebases local commits,
// and local changes are stashed during a rebase.
var gitSettings = [][2]string{
	{"pull.rebase", "true"},
	{"rebase.autoStash", "true"},
}

func init() {
	layer.Register(&layer.Layer{
		Name:    "git",
		Summary: "system-wide git settings (rebase on pull, auto stash)",
		Needs:   []string{"os"},
		Install: func(e *layer.Env) error {
			for _, s := range gitSettings {
				if err := sys.Run(nil, "git", "config", "--system", s[0], s[1]); err != nil {
					return err
				}
			}
			return nil
		},
		Test: func(t *layer.T) {
			for _, s := range gitSettings {
				value, _ := sys.Output("git", "config", "--system", s[0])
				t.Check(s[0]+" = "+s[1], value == s[1])
			}
			v, _ := sys.Output("git", "--version")
			t.Version("git", lastWord(v))
		},
	})
}
