package grok

import (
	"context"
	"io"
	"net/http"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadFile_Validation(t *testing.T) {
	g := New("", &fakeDoer{}, Options{})

	tests := []struct {
		name     string
		fileName string
		mimeType string
		content  string
		wantSub  string
	}{
		{name: "missing fileName", fileName: " ", mimeType: "image/png", content: "QQ==", wantSub: "fileName is required"},
		{name: "missing mimeType", fileName: "a.png", mimeType: "  ", content: "QQ==", wantSub: "fileMimeType is required"},
		{name: "missing content", fileName: "a.png", mimeType: "image/png", content: "", wantSub: "content is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := g.uploadFile(context.Background(), "tok", tt.fileName, tt.mimeType, tt.content)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

func TestUploadFile_Success(t *testing.T) {
	doer := &fakeDoer{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       strBody(`{"fileMetadataId":"FID-1","fileUri":"uri-1"}`),
	}}
	g := New("", doer, Options{})

	fileID, fileURI, err := g.uploadFile(context.Background(), "tok", "image-0.png", "image/png", "QQ==")
	require.NoError(t, err)
	assert.Equal(t, "FID-1", fileID)
	assert.Equal(t, "uri-1", fileURI)
	require.Len(t, doer.reqs, 1)
	assert.Equal(t, "application/json", doer.reqs[0].Header.Get("Content-Type"))
}

func TestUploadFile_Errors(t *testing.T) {
	tests := []struct {
		name      string
		uploadURL string
		resp      *http.Response
		doerErr   error
		wantSub   string
	}{
		{
			name:      "create request with invalid url",
			uploadURL: "http://[",
			wantSub:   "create request",
		},
		{
			name:    "doer failure",
			doerErr: io.ErrClosedPipe,
			wantSub: "do request",
		},
		{
			name: "non-200 status",
			resp: &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{},
				Body:       strBody("denied"),
			},
			wantSub: "status 403",
		},
		{
			name: "body read failure",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(iotest.ErrReader(io.ErrUnexpectedEOF)),
			},
			wantSub: "read response",
		},
		{
			name: "invalid json response",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       strBody("not json"),
			},
			wantSub: "decode response",
		},
		{
			name: "missing fileMetadataId",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       strBody(`{"fileMetadataId":"  "}`),
			},
			wantSub: "missing fileMetadataId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New("", &fakeDoer{resp: tt.resp, err: tt.doerErr}, Options{
				UploadURL: func() string { return tt.uploadURL },
			})
			_, _, err := g.uploadFile(context.Background(), "tok", "a.png", "image/png", "QQ==")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}
