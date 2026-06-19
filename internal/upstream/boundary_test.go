package upstream_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestImportBoundaries(t *testing.T) {
	cases := []struct {
		pkg       string
		forbidden []string
	}{
		{"grok", []string{"internal/flow", "internal/xai", "internal/config", "internal/token", "internal/upstream/console"}},
		{"console", []string{"internal/flow", "internal/xai", "internal/config", "internal/token", "internal/upstream/grok"}},
		{"msgutil", []string{"internal/flow", "internal/xai", "internal/config", "internal/token"}},
		{"mediautil", []string{"internal/flow", "internal/xai", "internal/config", "internal/token"}},
		{"transport", []string{"internal/flow", "internal/xai", "internal/config", "internal/token", "internal/upstream/grok", "internal/upstream/console"}},
	}
	for _, c := range cases {
		out, err := exec.Command("go", "list", "-test", "-deps", "./"+c.pkg+"/...").CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", c.pkg, err, out)
		}
		deps := string(out)
		for _, f := range c.forbidden {
			if strings.Contains(deps, "grokforge/"+f) {
				t.Errorf("upstream/%s imports forbidden %s", c.pkg, f)
			}
		}
	}
}
