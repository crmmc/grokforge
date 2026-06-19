package grok

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
)

func TestGrokChat_FullStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "https://grok.com" {
			t.Errorf("Origin = %q", r.Header.Get("Origin"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["message"] == "" {
			t.Error("empty message in payload")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`{"result":{"response":{"token":"Hi","isThinking":false}}}` + "\n"))
	}))
	defer srv.Close()

	g := New(srv.URL, &upstream.StdlibDoer{Client: srv.Client()}, Options{
		BuildCookieString: func(tok string) string { return "sso=" + tok },
		HeaderOrder:       func() []string { return nil },
		BrowserProfile:    func() string { return "chrome136" },
		UserAgent:         func() string { return "Chrome/136.0.0.0" },
	})
	ch, err := g.Chat(context.Background(), "tok", &upstream.ChatRequest{
		Messages:     []upstream.Message{{Role: "user", Content: "hi"}},
		Model:        "grok-3",
		UpstreamMode: "auto",
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
	if got != "Hi" {
		t.Errorf("content = %q", got)
	}
}

func TestGrokChat_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	g := New(srv.URL, &upstream.StdlibDoer{Client: srv.Client()}, Options{
		BuildCookieString: func(tok string) string { return "" },
		HeaderOrder:       func() []string { return nil },
	})
	_, err := g.Chat(context.Background(), "tok", &upstream.ChatRequest{
		Messages:     []upstream.Message{{Role: "user", Content: "hi"}},
		Model:        "grok-3",
		UpstreamMode: "auto",
	})
	if !errors.Is(err, upstream.ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
}
