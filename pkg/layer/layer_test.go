package layer

import "testing"

func TestPackageOf(t *testing.T) {
	cases := map[string]string{
		"github.com/majikmate/devcontainer-features.init.0":            "github.com/majikmate/devcontainer-features",
		"github.com/majikmate/devcontainer-core/pkg/layers.init.3":     "github.com/majikmate/devcontainer-core/pkg/layers",
		"github.com/majikmate/devcontainer-core/pkg/layer.TestPackage": "github.com/majikmate/devcontainer-core/pkg/layer",
	}
	for function, want := range cases {
		if got := packageOf(function); got != want {
			t.Errorf("packageOf(%q) = %q, want %q", function, got, want)
		}
	}
}

func TestRegisterRecordsPackage(t *testing.T) {
	l := &Layer{Name: "test-register-package"}
	Register(l)
	defer delete(registry, l.Name)
	if l.Package != "github.com/majikmate/devcontainer-core/pkg/layer" {
		t.Errorf("Package = %q", l.Package)
	}
}
