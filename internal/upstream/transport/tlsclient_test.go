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
	client.CloseIdleConnections()
}

func TestDynamicStatelessDoer_ClientForRebuildsOnlyWhenOptionsChange(t *testing.T) {
	opts := Options{Browser: "chrome_146"}
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

	first.CloseIdleConnections()
	third.CloseIdleConnections()
}

func TestResolveBrowserProfile_MatchesCompactAndUnderscoreNames(t *testing.T) {
	compact := resolveBrowserProfile("chrome136")
	underscore := resolveBrowserProfile("chrome_136")
	if compact.GetClientHelloStr() != underscore.GetClientHelloStr() {
		t.Fatalf("chrome136=%q chrome_136=%q", compact.GetClientHelloStr(), underscore.GetClientHelloStr())
	}

	knownCompact := resolveBrowserProfile("chrome146")
	knownUnderscore := resolveBrowserProfile("chrome_146")
	if knownCompact.GetClientHelloStr() != knownUnderscore.GetClientHelloStr() {
		t.Fatalf("chrome146=%q chrome_146=%q", knownCompact.GetClientHelloStr(), knownUnderscore.GetClientHelloStr())
	}
	if knownUnderscore.GetClientHelloStr() == "" {
		t.Fatal("resolved profile should expose a client hello identifier")
	}
}

func TestTransportDoesNotDependOnInternalXAI(t *testing.T) {
	source, err := os.ReadFile("tlsclient.go")
	if err != nil {
		t.Fatalf("read tlsclient.go: %v", err)
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
