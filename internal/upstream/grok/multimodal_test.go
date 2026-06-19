package grok

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
)

func TestProcessMultimodal_UploadsImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "sso=tok" {
			t.Errorf("Cookie = %q", r.Header.Get("Cookie"))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"fileMetadataId": "FID-1", "fileUri": "uri-1"})
	}))
	defer srv.Close()

	g := New("", &upstream.StdlibDoer{Client: srv.Client()}, Options{
		UploadURL:         func() string { return srv.URL },
		BuildCookieString: func(tok string) string { return "sso=" + tok },
		HeaderOrder:       func() []string { return nil },
	})
	msgs := []upstream.Message{{
		Role: "user",
		Content: []upstream.ContentBlock{
			{Type: "text", Text: "describe"},
			{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "data:image/png;base64,iVBORw0KGgo="}},
		},
	}}

	out, atts, err := g.processMultimodal(context.Background(), "tok", msgs)
	if err != nil {
		t.Fatalf("processMultimodal: %v", err)
	}
	if len(atts) != 1 || atts[0] != "FID-1" {
		t.Errorf("attachments=%v", atts)
	}
	if len(out) != 1 || out[0].Content != "describe" {
		t.Errorf("messages=%v", out)
	}
}
