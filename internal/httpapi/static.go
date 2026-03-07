// Package httpapi provides HTTP routing and handlers.
package httpapi

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/crmmc/grokforge/web"
)

// SPAHandler serves static files with SPA fallback for client-side routing.
func SPAHandler() http.Handler {
	sub, err := fs.Sub(web.StaticFS, "out")
	if err != nil {
		// Return a handler that always responds 500 if static assets are missing.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "static assets unavailable", http.StatusInternalServerError)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean path, remove leading slash
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}

		// Serve robots.txt directly (not a static file in the SPA bundle).
		if p == "robots.txt" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("User-agent: *\nDisallow: /\n"))
			return
		}

		// Try to serve the file directly (must be a file, not directory)
		if info, err := fs.Stat(sub, p); err == nil && !info.IsDir() {
			serveFile(w, r, sub, p)
			return
		}

		// Check for directory with index.html (Next.js trailingSlash)
		indexPath := strings.TrimSuffix(p, "/") + "/index.html"
		if _, err := fs.Stat(sub, indexPath); err == nil {
			serveFile(w, r, sub, indexPath)
			return
		}

		// SPA fallback: serve index.html for client-side routing
		serveFile(w, r, sub, "index.html")
	})
}

// serveFile serves a file from the embedded filesystem.
func serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	f, err := fsys.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Detect content type from extension
	contentType := "application/octet-stream"
	switch {
	case strings.HasSuffix(name, ".html"):
		contentType = "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		contentType = "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		contentType = "application/javascript"
	case strings.HasSuffix(name, ".json"):
		contentType = "application/json"
	case strings.HasSuffix(name, ".png"):
		contentType = "image/png"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		contentType = "image/jpeg"
	case strings.HasSuffix(name, ".svg"):
		contentType = "image/svg+xml"
	case strings.HasSuffix(name, ".ico"):
		contentType = "image/x-icon"
	case strings.HasSuffix(name, ".woff2"):
		contentType = "font/woff2"
	case strings.HasSuffix(name, ".woff"):
		contentType = "font/woff"
	case strings.HasSuffix(name, ".txt"):
		contentType = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)

	// Set cache headers: hashed assets get long-term cache, HTML gets no-cache.
	if strings.HasPrefix(name, "_next/static/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if strings.HasSuffix(name, ".html") {
		w.Header().Set("Cache-Control", "no-cache")
	}

	http.ServeContent(w, r, name, stat.ModTime(), f.(io.ReadSeeker))
}
