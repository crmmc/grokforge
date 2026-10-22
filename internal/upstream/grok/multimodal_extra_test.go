package grok

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMimeToExtension(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		want     string
	}{
		{name: "png", mimeType: "image/png", want: "png"},
		{name: "webp", mimeType: "image/webp", want: "webp"},
		{name: "gif", mimeType: "image/gif", want: "gif"},
		{name: "jpeg", mimeType: "image/jpeg", want: "jpg"},
		{name: "jpg", mimeType: "image/jpg", want: "jpg"},
		{name: "case insensitive", mimeType: " IMAGE/WEBP ", want: "webp"},
		{name: "unknown", mimeType: "image/heic", want: "bin"},
		{name: "empty", mimeType: "", want: "bin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mimeToExtension(tt.mimeType))
		})
	}
}

func TestUploadDataURIAttachment_MetaVariants(t *testing.T) {
	var gotName, gotMime string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload uploadFileRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode upload body: %v", err)
		}
		gotName = payload.FileName
		gotMime = payload.FileMimeType
		_, _ = w.Write([]byte(`{"fileMetadataId":"FID-9","fileUri":"uri-9"}`))
	}))
	defer srv.Close()

	g := New("", &upstream.StdlibDoer{Client: srv.Client()}, Options{
		UploadURL: func() string { return srv.URL },
	})

	tests := []struct {
		name       string
		dataURI    string
		wantExt    string
		wantMime   string
		wantIndex  int
		wantFileID string
	}{
		{name: "mime with boundary", dataURI: "data:image/png;base64,QQ==", wantExt: "png", wantMime: "image/png", wantIndex: 3, wantFileID: "FID-9"},
		{name: "mime without boundary", dataURI: "data:image/webp,QQ==", wantExt: "webp", wantMime: "image/webp", wantIndex: 0, wantFileID: "FID-9"},
		{name: "empty meta falls back", dataURI: "data:,QQ==", wantExt: "bin", wantMime: "application/octet-stream", wantIndex: 7, wantFileID: "FID-9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileID, err := g.uploadDataURIAttachment(context.Background(), "tok", tt.dataURI, tt.wantIndex)
			require.NoError(t, err)
			assert.Equal(t, tt.wantFileID, fileID)
			assert.Equal(t, tt.wantMime, gotMime)
			assert.True(t, strings.HasSuffix(gotName, tt.wantExt), "fileName=%q", gotName)
			assert.Contains(t, gotName, "-"+strconv.Itoa(tt.wantIndex)+".")
		})
	}
}

func TestUploadDataURIAttachment_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	g := New("", &upstream.StdlibDoer{Client: srv.Client()}, Options{
		UploadURL: func() string { return srv.URL },
	})

	t.Run("invalid data uri", func(t *testing.T) {
		_, err := g.uploadDataURIAttachment(context.Background(), "tok", "not-a-data-uri", 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid image data URI")
	})

	t.Run("upload failure", func(t *testing.T) {
		_, err := g.uploadDataURIAttachment(context.Background(), "tok", "data:image/png;base64,QQ==", 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upload attachment")
		assert.Contains(t, err.Error(), "status 500")
	})
}

func TestProcessMultimodalContent_Errors(t *testing.T) {
	g := New("", nil, Options{})

	tests := []struct {
		name    string
		content any
		wantSub string
	}{
		{
			name:    "unknown content part type",
			content: []map[string]any{{"type": "definitely_unknown"}},
			wantSub: "unknown content type",
		},
		{
			name:    "unsupported image scheme",
			content: []upstream.ContentBlock{{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "ftp://example.com/a.png"}}},
			wantSub: "unsupported URL scheme",
		},
		{
			name:    "non-map content part",
			content: []any{"plain string part"},
			wantSub: "content part must be object",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := g.processMultimodalContent(context.Background(), "tok", tt.content)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

func TestProcessMultimodalContent_UploadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := New("", &upstream.StdlibDoer{Client: srv.Client()}, Options{
		UploadURL: func() string { return srv.URL },
	})
	content := []upstream.ContentBlock{
		{Type: "text", Text: "look"},
		{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "data:image/png;base64,QQ=="}},
	}
	_, _, err := g.processMultimodalContent(context.Background(), "tok", content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload attachment")
}

func TestProcessMultimodal_ErrorPropagates(t *testing.T) {
	g := New("", nil, Options{})
	msgs := []upstream.Message{{
		Role:    "user",
		Content: []map[string]any{{"type": "definitely_unknown"}},
	}}
	_, _, err := g.processMultimodal(context.Background(), "tok", msgs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown content type")
}

func TestProcessMultimodal_PreservesSimpleContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"fileMetadataId":"FID-1","fileUri":"uri-1"}`))
	}))
	defer srv.Close()

	g := New("", &upstream.StdlibDoer{Client: srv.Client()}, Options{
		UploadURL: func() string { return srv.URL },
	})
	msgs := []upstream.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: map[string]any{"content": "structured"}},
		{Role: "user", Content: nil},
		{Role: "user", Content: []upstream.ContentBlock{
			{Type: "text", Text: "with image"},
			{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "data:image/gif;base64,QQ=="}},
		}},
	}
	out, atts, err := g.processMultimodal(context.Background(), "tok", msgs)
	require.NoError(t, err)
	assert.Len(t, out, 4)
	assert.Equal(t, []string{"FID-1"}, atts)
}
