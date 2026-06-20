package transport

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNewStatelessDoer_ConstructsDefault(t *testing.T) {
	client, err := NewStatelessDoer(Options{})
	if err != nil {
		t.Fatalf("NewStatelessDoer: %v", err)
	}
	if client == nil {
		t.Fatal("NewStatelessDoer returned nil client")
	}
}

func TestDynamicStatelessDoer_ClientForRebuildsOnlyWhenOptionsChange(t *testing.T) {
	opts := Options{Browser: DefaultProfile}
	d := NewDynamicStatelessDoer(func() Options { return opts })

	first, err := d.clientFor(d.provide())
	if err != nil {
		t.Fatalf("first clientFor: %v", err)
	}
	second, err := d.clientFor(d.provide())
	if err != nil {
		t.Fatalf("second clientFor: %v", err)
	}
	if first != second {
		t.Fatal("same options should reuse client")
	}

	opts.RequestTimeout = 2 * time.Second
	third, err := d.clientFor(d.provide())
	if err != nil {
		t.Fatalf("third clientFor: %v", err)
	}
	if third == first {
		t.Fatal("changed options should rebuild client")
	}
}

func TestResolveProfile_MatchesCompactAndUnderscoreNames(t *testing.T) {
	if got := ResolveProfile("chrome136"); got != "chrome136" {
		t.Fatalf("ResolveProfile(chrome136)=%q", got)
	}
	if got := ResolveProfile("chrome_136"); got != "chrome136" {
		t.Fatalf("ResolveProfile(chrome_136)=%q", got)
	}
	if got := ResolveProfile("edge120"); got != "" {
		t.Fatalf("ResolveProfile(edge120)=%q", got)
	}
	if ua := DefaultUserAgent(); ua == "" {
		t.Fatal("DefaultUserAgent is empty")
	}
}

func TestTransportDoesNotDependOnInternalXAI(t *testing.T) {
	source, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatalf("read client.go: %v", err)
	}
	if strings.Contains(string(source), "internal/xai") {
		t.Fatal("transport source must not import internal/xai")
	}

	cmd := exec.Command("go", "list", "-deps", ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps .: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "github.com/crmmc/grokforge/internal/xai") {
		t.Fatal("transport deps must not include internal/xai")
	}
}
