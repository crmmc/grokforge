package xai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadFile(t *testing.T) {
	type setupResult struct {
		doer transport.Doer
		stub *stubUpstream
	}

	tests := []struct {
		name      string
		fileName  string
		mimeType  string
		content   string
		setup     func(t *testing.T) setupResult
		wantID    string
		wantURI   string
		wantErr   string
		checkStub func(t *testing.T, stub *stubUpstream)
	}{
		{
			name:     "missing file name",
			mimeType: "application/pdf",
			content:  "AAAA",
			wantErr:  "fileName is required",
		},
		{
			name:     "missing mime type",
			fileName: "f.pdf",
			content:  "AAAA",
			wantErr:  "fileMimeType is required",
		},
		{
			name:     "missing content",
			fileName: "f.pdf",
			mimeType: "application/pdf",
			content:  " ",
			wantErr:  "content is required",
		},
		{
			name:     "success",
			fileName: "f.pdf",
			mimeType: "application/pdf",
			content:  "AAAA",
			wantID:   "meta-1",
			wantURI:  "https://assets.grok.com/f.pdf",
			setup: func(t *testing.T) setupResult {
				stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, `{"fileMetadataId":"meta-1","fileUri":"https://assets.grok.com/f.pdf"}`)
				})
				return setupResult{doer: stub.doer(), stub: stub}
			},
			checkStub: func(t *testing.T, stub *stubUpstream) {
				calls := stub.calls()
				require.Len(t, calls, 1)
				assert.Equal(t, "/rest/app-chat/upload-file", calls[0].Path)
				assert.Contains(t, string(calls[0].Body), `"fileName":"f.pdf"`)
				assert.Contains(t, string(calls[0].Body), `"content":"AAAA"`)
			},
		},
		{
			name:     "non-200 status",
			fileName: "f.pdf",
			mimeType: "application/pdf",
			content:  "AAAA",
			wantErr:  "status 500",
			setup: func(t *testing.T) setupResult {
				stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = io.WriteString(w, "boom")
				})
				return setupResult{doer: stub.doer(), stub: stub}
			},
		},
		{
			name:     "invalid json response",
			fileName: "f.pdf",
			mimeType: "application/pdf",
			content:  "AAAA",
			wantErr:  "decode response",
			setup: func(t *testing.T) setupResult {
				stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, "not-json")
				})
				return setupResult{doer: stub.doer(), stub: stub}
			},
		},
		{
			name:     "missing file metadata id",
			fileName: "f.pdf",
			mimeType: "application/pdf",
			content:  "AAAA",
			wantErr:  "missing fileMetadataId",
			setup: func(t *testing.T) setupResult {
				stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, `{"fileUri":"u"}`)
				})
				return setupResult{doer: stub.doer(), stub: stub}
			},
		},
		{
			name:     "doer error",
			fileName: "f.pdf",
			mimeType: "application/pdf",
			content:  "AAAA",
			wantErr:  "do request",
			setup: func(t *testing.T) setupResult {
				return setupResult{doer: &staticDoer{err: errors.New("network down")}}
			},
		},
		{
			name:     "response body read error",
			fileName: "f.pdf",
			mimeType: "application/pdf",
			content:  "AAAA",
			wantErr:  "read response",
			setup: func(t *testing.T) setupResult {
				return setupResult{doer: &staticDoer{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       io.NopCloser(&errorReader{err: errors.New("read fail")}),
				}}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doer transport.Doer = &staticDoer{}
			var stub *stubUpstream
			if tt.setup != nil {
				res := tt.setup(t)
				doer = res.doer
				stub = res.stub
			}

			c := newTestClient(doer, nil)
			id, uri, err := c.UploadFile(context.Background(), tt.fileName, tt.mimeType, tt.content)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, id)
				assert.Empty(t, uri)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, id)
			assert.Equal(t, tt.wantURI, uri)
			if tt.checkStub != nil {
				tt.checkStub(t, stub)
			}
		})
	}
}

func TestUploadFileClosedClient(t *testing.T) {
	c := newTestClient(&staticDoer{}, nil)
	require.NoError(t, c.Close())

	_, _, err := c.UploadFile(context.Background(), "f.pdf", "application/pdf", "AAAA")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStreamClosed)
	assert.Contains(t, err.Error(), "upload file: do request")
}
