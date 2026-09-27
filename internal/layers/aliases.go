package layers

import (
	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/shellrc"
)

// Shell aliases of all users (bash and zsh).
const aliases = `alias ls='ls -h --color=auto'
alias ll='ls -lah --color=auto'
alias grep='grep --color=auto'
alias vs='code -r .'`

func init() {
	layer.Register(&layer.Layer{
		Name:    "aliases",
		Summary: "shell aliases: ls, ll, grep, vs",
		Needs:   []string{"user"},
		Install: func(e *layer.Env) error {
			return shellrc.Shared("aliases", aliases)
		},
		Test: func(t *layer.T) {
			for _, name := range []string{"ls", "ll", "grep", "vs"} {
				t.Command("zsh alias "+name, "zsh", "-ic", "alias "+name)
				t.Command("bash alias "+name, "bash", "-ic", "alias "+name)
			}
		},
	})
}
