package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSPAHandler_ContentTypes(t *testing.T) {
	fsMap := fstest.MapFS{
		"index.html":  {Data: []byte("<html>home</html>")},
		"style.css":   {Data: []byte("body{}")},
		"app.js":      {Data: []byte("console.log(1)")},
		"data.json":   {Data: []byte("{}")},
		"logo.png":    {Data: []byte("png")},
		"photo.jpg":   {Data: []byte("jpg")},
		"photo.jpeg":  {Data: []byte("jpeg")},
		"icon.svg":    {Data: []byte("<svg/>")},
		"favicon.ico": {Data: []byte("ico")},
		"font.woff2":  {Data: []byte("w2")},
		"font.woff":   {Data: []byte("w")},
		"robots.txt":  {Data: []byte("should not be served from fs")},
		"unknown.xyz": {Data: []byte("mystery")},
	}

	handler := NewSPAHandler(fsMap)

	tests := []struct {
		name     string
		path     string
		wantType string
		wantBody string
	}{
		{name: "html", path: "/index.html", wantType: "text/html; charset=utf-8", wantBody: "<html>home</html>"},
		{name: "css", path: "/style.css", wantType: "text/css; charset=utf-8"},
		{name: "js", path: "/app.js", wantType: "application/javascript"},
		{name: "json", path: "/data.json", wantType: "application/json"},
		{name: "png", path: "/logo.png", wantType: "image/png"},
		{name: "jpg", path: "/photo.jpg", wantType: "image/jpeg"},
		{name: "jpeg", path: "/photo.jpeg", wantType: "image/jpeg"},
		{name: "svg", path: "/icon.svg", wantType: "image/svg+xml"},
		{name: "ico", path: "/favicon.ico", wantType: "image/x-icon"},
		{name: "woff2", path: "/font.woff2", wantType: "font/woff2"},
		{name: "woff", path: "/font.woff", wantType: "font/woff"},
		{name: "unknown extension", path: "/unknown.xyz", wantType: "application/octet-stream"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tc.wantType, rec.Header().Get("Content-Type"))
			if tc.wantBody != "" {
				assert.Contains(t, rec.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestNewSPAHandler_RobotsTxtOverride(t *testing.T) {
	// robots.txt is synthesized and must not be read from the FS even if present.
	fsMap := fstest.MapFS{
		"index.html": {Data: []byte("<html>home</html>")},
		"robots.txt": {Data: []byte("should not be served")},
	}

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	NewSPAHandler(fsMap).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "User-agent: *\nDisallow: /\n", rec.Body.String())
}

func TestNewSPAHandler_HTMLNoCache(t *testing.T) {
	fsMap := fstest.MapFS{
		"page.html": {Data: []byte("<html></html>")},
	}

	req := httptest.NewRequest(http.MethodGet, "/page.html", nil)
	rec := httptest.NewRecorder()
	NewSPAHandler(fsMap).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}

// fakeFileInfo is a minimal non-directory fs.FileInfo.
type fakeFileInfo struct{ name string }

func (fi fakeFileInfo) Name() string       { return fi.name }
func (fi fakeFileInfo) Size() int64        { return 0 }
func (fi fakeFileInfo) Mode() fs.FileMode  { return 0o444 }
func (fi fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fi fakeFileInfo) IsDir() bool        { return false }
func (fi fakeFileInfo) Sys() any           { return nil }

// statFailFile is an fs.File whose Stat always fails.
type statFailFile struct{ err error }

func (f *statFailFile) Stat() (fs.FileInfo, error) { return nil, f.err }
func (f *statFailFile) Read([]byte) (int, error)   { return 0, errors.New("read unsupported") }
func (f *statFailFile) Close() error               { return nil }

// openFailFS implements fs.StatFS: Stat succeeds for failName but Open fails.
type openFailFS struct {
	inner    fs.FS
	failName string
}

func (f *openFailFS) Open(name string) (fs.File, error) {
	if name == f.failName {
		return nil, errors.New("open failed")
	}
	return f.inner.Open(name)
}

func (f *openFailFS) Stat(name string) (fs.FileInfo, error) {
	if name == f.failName {
		return fakeFileInfo{name: f.failName}, nil
	}
	return fs.Stat(f.inner, name)
}

// statFailFS implements fs.StatFS: Stat succeeds for failName but the opened
// file's own Stat fails.
type statFailFS struct {
	inner    fs.FS
	failName string
}

func (f *statFailFS) Open(name string) (fs.File, error) {
	if name == f.failName {
		return &statFailFile{err: errors.New("stat failed")}, nil
	}
	return f.inner.Open(name)
}

func (f *statFailFS) Stat(name string) (fs.FileInfo, error) {
	if name == f.failName {
		return fakeFileInfo{name: f.failName}, nil
	}
	return fs.Stat(f.inner, name)
}

func TestNewSPAHandler_OpenFailureReturnsNotFound(t *testing.T) {
	inner := fstest.MapFS{"index.html": {Data: []byte("<html>home</html>")}}
	handler := NewSPAHandler(&openFailFS{inner: inner, failName: "broken.html"})

	req := httptest.NewRequest(http.MethodGet, "/broken.html", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNewSPAHandler_FileStatFailureReturnsNotFound(t *testing.T) {
	inner := fstest.MapFS{"index.html": {Data: []byte("<html>home</html>")}}
	handler := NewSPAHandler(&statFailFS{inner: inner, failName: "broken.html"})

	req := httptest.NewRequest(http.MethodGet, "/broken.html", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSPAHandler_ServesEmbeddedBundle(t *testing.T) {
	handler := SPAHandler()
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, strings.Contains(rec.Header().Get("Content-Type"), "text/html"))
}

func TestNewSPAHandler_PlainTextFile(t *testing.T) {
	fsMap := fstest.MapFS{
		"notes.txt": {Data: []byte("hello")},
	}

	req := httptest.NewRequest(http.MethodGet, "/notes.txt", nil)
	rec := httptest.NewRecorder()
	NewSPAHandler(fsMap).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "hello", rec.Body.String())
}
