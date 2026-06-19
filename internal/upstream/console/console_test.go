package console

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
)

func TestConsoleChat_FullStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "https://console.x.ai" {
			t.Errorf("Origin = %q", r.Header.Get("Origin"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.output_text.delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":\"Yo\"}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	c := New(srv.URL, &upstream.StdlibDoer{Client: srv.Client()}, Options{
		BuildCookieString: func(tok string) string { return "sso=" + tok },
		HeaderOrder:       func() []string { return nil },
		BrowserProfile:    func() string { return "chrome136" },
		UserAgent:         func() string { return "Chrome/136.0.0.0" },
	})
	ch, err := c.Chat(context.Background(), "tok", &upstream.ChatRequest{
		Messages: []upstream.Message{{Role: "user", Content: "hi"}},
		Model:    "grok-3",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	var got string
	for ev := range ch {
		if ev.Error != nil {
			t.Fatalf("stream error: %v", ev.Error)
		}
		got += ev.Content
	}
	if got != "Yo" {
		t.Errorf("content = %q, want Yo", got)
	}
}

func TestConsoleChat_CreditExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer srv.Close()

	c := New(srv.URL, &upstream.StdlibDoer{Client: srv.Client()}, Options{
		BuildCookieString: func(tok string) string { return "" },
		HeaderOrder:       func() []string { return nil },
	})
	_, err := c.Chat(context.Background(), "tok", &upstream.ChatRequest{
		Messages: []upstream.Message{{Role: "user", Content: "hi"}},
		Model:    "grok-3",
	})
	if !errors.Is(err, upstream.ErrCreditExhausted) {
		t.Errorf("err = %v, want ErrCreditExhausted", err)
	}
}
