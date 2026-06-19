//go:build integration

package console

import (
	"context"
	"os"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/crmmc/grokforge/internal/upstream/transport"
)

func TestConsoleChat_RealUpstream(t *testing.T) {
	token := os.Getenv("CONSOLE_TEST_TOKEN")
	if token == "" {
		t.Skip("CONSOLE_TEST_TOKEN not set")
	}
	doer, err := transport.NewStatelessDoer(transport.Options{Browser: "chrome136"})
	if err != nil {
		t.Fatalf("doer: %v", err)
	}

	c := New(DefaultURL, doer, Options{
		BuildCookieString: func(tok string) string { return "sso=" + tok + "; sso-rw=" + tok },
		HeaderOrder:       func() []string { return nil },
		BrowserProfile:    func() string { return "chrome136" },
		UserAgent: func() string {
			return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
		},
	})
	ch, err := c.Chat(context.Background(), token, &upstream.ChatRequest{
		Messages: []upstream.Message{{Role: "user", Content: "say hello"}},
		Model:    "grok-4.20",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var got string
	for ev := range ch {
		if ev.Error != nil {
			t.Fatalf("stream err: %v", ev.Error)
		}
		got += ev.Content
	}
	if got == "" {
		t.Error("expected non-empty content")
	}
}
