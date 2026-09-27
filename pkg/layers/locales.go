package layers

import (
	"fmt"
	"os"
	"strings"

	"github.com/majikmate/devcontainer-core/pkg/layer"
	"github.com/majikmate/devcontainer-core/pkg/sys"
)

// The locale variables. The Dockerfile sets them with ENV, so every process
// uses them (terminals, VS Code Server, docker run). This layer generates the
// locales that they name and sets the time zone from TZ.
var localeVariables = []string{"LANG", "LANGUAGE", "LC_TIME", "LC_NUMERIC", "LC_MONETARY", "LC_MEASUREMENT"}

func init() {
	layer.Register(&layer.Layer{
		Name:    "locales",
		Summary: "generates the locales of LANG and LC_* and sets the time zone TZ",
		Needs:   []string{"os"},
		Args: []layer.Arg{
			{Name: "LANG", Default: "en_US.UTF-8", Doc: "language of messages (ENV in the Dockerfile)"},
			{Name: "TZ", Default: "Europe/Vienna", Doc: "time zone (ENV in the Dockerfile)"},
		},
		Install: func(e *layer.Env) error {
			locales := map[string]bool{e.Arg("LANG"): true}
			for _, name := range localeVariables[2:] {
				if v := os.Getenv(name); v != "" {
					locales[v] = true
				}
			}
			gen, err := os.ReadFile("/etc/locale.gen")
			if err != nil {
				return err
			}
			var enabled []string
			for locale := range locales {
				enabled = append(enabled, locale+" UTF-8")
			}
			content := string(gen) + "\n# Enabled by devcon (layer locales)\n" + strings.Join(enabled, "\n") + "\n"
			if err := sys.WriteFile("/etc/locale.gen", content, 0o644); err != nil {
				return err
			}
			if err := sys.Run(nil, "locale-gen"); err != nil {
				return err
			}
			// /etc/default/locale: for programs that read it (for example PAM in SSH sessions)
			var def strings.Builder
			for _, name := range localeVariables {
				if v := os.Getenv(name); v != "" {
					fmt.Fprintf(&def, "%s=%s\n", name, v)
				}
			}
			if err := sys.WriteFile("/etc/default/locale", def.String(), 0o644); err != nil {
				return err
			}
			tz := e.Arg("TZ")
			if !sys.Exists("/usr/share/zoneinfo/" + tz) {
				return fmt.Errorf("unknown time zone %s", tz)
			}
			_ = os.Remove("/etc/localtime")
			if err := os.Symlink("/usr/share/zoneinfo/"+tz, "/etc/localtime"); err != nil {
				return err
			}
			return sys.WriteFile("/etc/timezone", tz+"\n", 0o644)
		},
		Test: func(t *layer.T) {
			available := t.Output("locale -a", "locale", "-a")
			for _, name := range localeVariables {
				if v := os.Getenv(name); v != "" && strings.Contains(v, ".") {
					normalized := strings.ToLower(strings.Replace(v, "UTF-8", "utf8", 1))
					t.Check(name+"="+v+" generated", strings.Contains(strings.ToLower(available), normalized))
				}
			}
			tz := os.Getenv("TZ")
			target, _ := os.Readlink("/etc/localtime")
			t.Check("time zone "+tz, tz != "" && target == "/usr/share/zoneinfo/"+tz)
		},
	})
}
