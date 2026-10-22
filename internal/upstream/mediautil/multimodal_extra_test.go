package mediautil

import (
	"context"
	"encoding/base64"
	"errors"
	"image"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMultimodalContent_Table(t *testing.T) {
	tests := []struct {
		name       string
		content    any
		wantBlocks []upstream.ContentBlock
		wantErr    string
	}{
		{
			name:       "string becomes single text block",
			content:    "hello",
			wantBlocks: []upstream.ContentBlock{{Type: "text", Text: "hello"}},
		},
		{
			name:       "typed blocks pass through",
			content:    []upstream.ContentBlock{{Type: "text", Text: "typed"}},
			wantBlocks: []upstream.ContentBlock{{Type: "text", Text: "typed"}},
		},
		{
			name: "map slice parses text and image",
			content: []map[string]any{
				{"type": "text", "text": "look"},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AA", "detail": "low"}},
			},
			wantBlocks: []upstream.ContentBlock{
				{Type: "text", Text: "look"},
				{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "data:image/png;base64,AA", Detail: "low"}},
			},
		},
		{
			name: "any slice parses mixed parts",
			content: []any{
				map[string]any{"type": "text", "text": "a"},
				upstream.ContentBlock{Type: "text", Text: "typed"},
			},
			wantBlocks: []upstream.ContentBlock{
				{Type: "text", Text: "a"},
				{Type: "text", Text: "typed"},
			},
		},
		{
			name:       "empty any slice yields no blocks",
			content:    []any{},
			wantBlocks: []upstream.ContentBlock{},
		},
		{
			name:    "unsupported top level type errors",
			content: 12345,
			wantErr: "unsupported content type: int",
		},
		{
			name:    "map slice with unknown part type errors",
			content: []map[string]any{{"type": "hologram"}},
			wantErr: "unknown content type: hologram",
		},
		{
			name:    "any slice with scalar part errors",
			content: []any{42},
			wantErr: "content part must be object, got int",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, err := ParseMultimodalContent(tt.content)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBlocks, blocks)
		})
	}
}

func TestParseContentPart_Table(t *testing.T) {
	tests := []struct {
		name      string
		item      any
		wantBlock upstream.ContentBlock
		wantErr   string
	}{
		{
			name:      "text part",
			item:      map[string]any{"type": "text", "text": "hi"},
			wantBlock: upstream.ContentBlock{Type: "text", Text: "hi"},
		},
		{
			name:      "text part without text key",
			item:      map[string]any{"type": "text"},
			wantBlock: upstream.ContentBlock{Type: "text"},
		},
		{
			name: "image_url part",
			item: map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": "https://example.test/a.png", "detail": "high"},
			},
			wantBlock: upstream.ContentBlock{
				Type:     "image_url",
				ImageURL: &upstream.ImageURLBlock{URL: "https://example.test/a.png", Detail: "high"},
			},
		},
		{
			name:      "image_url without nested url",
			item:      map[string]any{"type": "image_url", "image_url": map[string]any{}},
			wantBlock: upstream.ContentBlock{Type: "image_url", ImageURL: &upstream.ImageURLBlock{}},
		},
		{
			name:    "image_url not an object errors",
			item:    map[string]any{"type": "image_url", "image_url": "nope"},
			wantErr: "image_url must be object",
		},
		{
			name:      "file part maps to placeholder text",
			item:      map[string]any{"type": "file"},
			wantBlock: upstream.ContentBlock{Type: "text", Text: "[file input]"},
		},
		{
			name:      "input_file part maps to placeholder text",
			item:      map[string]any{"type": "input_file"},
			wantBlock: upstream.ContentBlock{Type: "text", Text: "[file input]"},
		},
		{
			name:      "input_audio part maps to placeholder text",
			item:      map[string]any{"type": "input_audio"},
			wantBlock: upstream.ContentBlock{Type: "text", Text: "[audio input]"},
		},
		{
			name:      "audio part maps to placeholder text",
			item:      map[string]any{"type": "audio"},
			wantBlock: upstream.ContentBlock{Type: "text", Text: "[audio input]"},
		},
		{
			name:    "unknown type errors",
			item:    map[string]any{"type": "video"},
			wantErr: "unknown content type: video",
		},
		{
			name:    "missing type errors",
			item:    map[string]any{"text": "orphan"},
			wantErr: "unknown content type: ",
		},
		{
			name:    "non object part errors",
			item:    "just a string",
			wantErr: "content part must be object, got string",
		},
		{
			name:      "typed content block passes through unchanged",
			item:      upstream.ContentBlock{Type: "text", Text: "kept"},
			wantBlock: upstream.ContentBlock{Type: "text", Text: "kept"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := parseContentPart(tt.item)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantBlock, block)
		})
	}
}

func TestProcessContent_Table(t *testing.T) {
	t.Run("empty blocks", func(t *testing.T) {
		result, err := ProcessContent(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, result.Text)
		assert.Empty(t, result.Images)
	})

	t.Run("nil image url block skipped", func(t *testing.T) {
		result, err := ProcessContent(context.Background(), []upstream.ContentBlock{
			{Type: "text", Text: "keep"},
			{Type: "image_url"},
		})
		require.NoError(t, err)
		assert.Equal(t, "keep", result.Text)
		assert.Empty(t, result.Images)
	})

	t.Run("image processing error propagates", func(t *testing.T) {
		_, err := ProcessContent(context.Background(), []upstream.ContentBlock{
			{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "ftp://example.test/x.png"}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "process image")
		assert.Contains(t, err.Error(), "unsupported URL scheme")
	})

	t.Run("data uri images collected", func(t *testing.T) {
		result, err := ProcessContent(context.Background(), []upstream.ContentBlock{
			{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "data:image/png;base64,AAA"}},
			{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "data:image/jpeg;base64,BBB"}},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"data:image/png;base64,AAA", "data:image/jpeg;base64,BBB"}, result.Images)
	})
}

func TestProcessImageURL_Table(t *testing.T) {
	t.Run("data uri passes through", func(t *testing.T) {
		const dataURI = "data:image/jpeg;base64,/9j/4AAQSkZJRg=="
		got, err := processImageURL(context.Background(), dataURI)
		require.NoError(t, err)
		assert.Equal(t, dataURI, got)
	})

	t.Run("loopback http url blocked by ssrf guard", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("request must be blocked before dialing")
		}))
		defer srv.Close()

		_, err := processImageURL(context.Background(), srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked")
	})

	t.Run("unsupported scheme errors", func(t *testing.T) {
		_, err := processImageURL(context.Background(), "ftp://example.test/x.png")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported URL scheme: ftp://example.test/x.png")
	})

	t.Run("validated url handed to downloader", func(t *testing.T) {
		original := downloadAsDataURIFn
		defer func() { downloadAsDataURIFn = original }()

		var (
			gotURL string
			gotIPs []net.IP
			called bool
		)
		downloadAsDataURIFn = func(_ context.Context, rawURL string, ips []net.IP) (string, error) {
			called = true
			gotURL = rawURL
			gotIPs = ips
			return "data:image/png;base64,AAAA", nil
		}

		got, err := processImageURL(context.Background(), "http://198.51.100.7/cat.png")
		require.NoError(t, err)
		assert.Equal(t, "data:image/png;base64,AAAA", got)
		assert.True(t, called, "downloader seam must be invoked")
		assert.Equal(t, "http://198.51.100.7/cat.png", gotURL)
		require.Len(t, gotIPs, 1)
		assert.True(t, gotIPs[0].Equal(net.ParseIP("198.51.100.7")), "resolved IP = %s", gotIPs[0])
	})
}

func TestValidateURLSafety(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantIPs []string
		wantErr string
	}{
		{
			name:    "localhost blocked",
			rawURL:  "http://localhost/x.png",
			wantErr: "blocked internal host: localhost",
		},
		{
			name:    "gcp metadata host blocked",
			rawURL:  "http://metadata.google.internal/computeMetadata/v1/",
			wantErr: "blocked internal host: metadata.google.internal",
		},
		{
			name:    "loopback ip blocked",
			rawURL:  "http://127.0.0.1/x.png",
			wantErr: "blocked private/internal IP: 127.0.0.1",
		},
		{
			name:    "ipv6 loopback blocked",
			rawURL:  "http://[::1]/x.png",
			wantErr: "blocked private/internal IP: ::1",
		},
		{
			name:    "private ipv4 blocked",
			rawURL:  "http://10.1.2.3/x.png",
			wantErr: "blocked private/internal IP: 10.1.2.3",
		},
		{
			name:    "private ipv6 blocked",
			rawURL:  "http://[fd00::1]/x.png",
			wantErr: "blocked private/internal IP: fd00::1",
		},
		{
			name:    "link local ipv4 blocked",
			rawURL:  "http://169.254.169.254/x.png",
			wantErr: "blocked private/internal IP: 169.254.169.254",
		},
		{
			name:    "link local ipv6 blocked",
			rawURL:  "http://[fe80::1]/x.png",
			wantErr: "blocked private/internal IP: fe80::1",
		},
		{
			name:    "link local multicast blocked",
			rawURL:  "http://[ff02::1]/x.png",
			wantErr: "blocked private/internal IP: ff02::1",
		},
		{
			name:    "unresolvable host errors",
			rawURL:  "http:///x.png",
			wantErr: "resolve host",
		},
		{
			name:    "malformed url errors",
			rawURL:  "http://198.51.100.7:notaport/x.png",
			wantErr: "invalid URL",
		},
		{
			name:    "public documentation ip allowed",
			rawURL:  "http://198.51.100.7/x.png",
			wantIPs: []string{"198.51.100.7"},
		},
		{
			name:    "public documentation ipv6 allowed",
			rawURL:  "http://[2001:db8::10]/x.png",
			wantIPs: []string{"2001:db8::10"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, err := validateURLSafety(tt.rawURL)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, ips, len(tt.wantIPs))
			for i, want := range tt.wantIPs {
				assert.True(t, ips[i].Equal(net.ParseIP(want)), "ip[%d] = %s", i, ips[i])
			}
		})
	}
}

func TestDownloadAsDataURI(t *testing.T) {
	pngBytes := createTestPNG()
	jpegMagic := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	loopback := []net.IP{net.ParseIP("127.0.0.1")}

	t.Run("success with explicit content type", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		}))
		defer srv.Close()

		host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)
		dataURI, err := downloadAsDataURI(context.Background(), "http://"+net.JoinHostPort(host, port)+"/a.png", loopback)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(dataURI, "data:image/png;base64,"))
		decoded, err := base64Decode(dataURI)
		require.NoError(t, err)
		assert.Equal(t, pngBytes, decoded)
	})

	t.Run("mime sniffed when content type missing", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(pngBytes)
		}))
		defer srv.Close()

		host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)
		dataURI, err := downloadAsDataURI(context.Background(), "http://"+net.JoinHostPort(host, port)+"/a.png", loopback)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(dataURI, "data:image/png;base64,"))
	})

	t.Run("mime sniffed when content type lies", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(jpegMagic)
		}))
		defer srv.Close()

		host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)
		dataURI, err := downloadAsDataURI(context.Background(), "http://"+net.JoinHostPort(host, port)+"/a.jpg", loopback)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(dataURI, "data:image/jpeg;base64,"))
	})

	t.Run("non image payload rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("not an image"))
		}))
		defer srv.Close()

		host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)
		_, err = downloadAsDataURI(context.Background(), "http://"+net.JoinHostPort(host, port)+"/a.png", loopback)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported image mime type: text/plain")
	})

	t.Run("non 200 status rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gone", http.StatusNotFound)
		}))
		defer srv.Close()

		host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)
		_, err = downloadAsDataURI(context.Background(), "http://"+net.JoinHostPort(host, port)+"/a.png", loopback)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download failed: status 404")
	})

	t.Run("connection refused surfaces as download error", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		closedPort := listener.Addr().(*net.TCPAddr).Port
		require.NoError(t, listener.Close())

		_, err = downloadAsDataURI(context.Background(),
			"http://127.0.0.1:"+strconv.Itoa(closedPort)+"/a.png", loopback)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download image")
	})

	t.Run("truncated body surfaces as read error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Length", strconv.Itoa(len(pngBytes)+100))
			_, _ = w.Write(pngBytes[:len(pngBytes)/2])
		}))
		defer srv.Close()

		host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)
		_, err = downloadAsDataURI(context.Background(), "http://"+net.JoinHostPort(host, port)+"/a.png", loopback)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read image")
	})

	t.Run("malformed url rejected before request", func(t *testing.T) {
		_, err := downloadAsDataURI(context.Background(), "http://[::1]:notaport/a.png", loopback)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse URL")
	})

	t.Run("http default port 80 used when missing", func(t *testing.T) {
		_, err := downloadAsDataURI(context.Background(), "http://127.0.0.1/a.png", loopback)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download image")
	})

	t.Run("https default port 443 used when missing", func(t *testing.T) {
		_, err := downloadAsDataURI(context.Background(), "https://127.0.0.1/a.png", loopback)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download image")
	})

	t.Run("empty resolved ip list fails closed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("no IP means no dial")
		}))
		defer srv.Close()

		_, err := downloadAsDataURI(context.Background(), srv.URL, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "all resolved IPs failed")
	})
}

func base64Decode(dataURI string) ([]byte, error) {
	const marker = ";base64,"
	idx := strings.Index(dataURI, marker)
	if idx < 0 {
		return nil, errors.New("not a base64 data URI")
	}
	return base64.StdEncoding.DecodeString(dataURI[idx+len(marker):])
}

func TestProcessImageData_Table(t *testing.T) {
	pngBytes := createTestPNG()
	tests := []struct {
		name        string
		data        []byte
		contentType string
		wantMIME    string
		wantErr     string
	}{
		{
			name:        "content type wins",
			data:        pngBytes,
			contentType: "image/png",
			wantMIME:    "image/png",
		},
		{
			name:     "sniffed png",
			data:     pngBytes,
			wantMIME: "image/png",
		},
		{
			name:        "sniffed jpeg from magic bytes",
			data:        []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10},
			contentType: "application/octet-stream",
			wantMIME:    "image/jpeg",
		},
		{
			name:        "non image rejected",
			data:        []byte("plain text payload"),
			contentType: "text/plain; charset=utf-8",
			wantErr:     "unsupported image mime type: text/plain",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processImageData(tt.data, tt.contentType)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "data:"+tt.wantMIME+";base64,", got[:len("data:"+tt.wantMIME+";base64,")])
			decoded, err := base64Decode(got)
			require.NoError(t, err)
			assert.Equal(t, tt.data, decoded)
		})
	}
}

func TestDetectImageMIME(t *testing.T) {
	pngBytes := createTestPNG()
	tests := []struct {
		name        string
		contentType string
		data        []byte
		want        string
	}{
		{"image header kept verbatim", "image/png", pngBytes, "image/png"},
		{"header charset stripped", " image/png ; charset=binary ", pngBytes, "image/png"},
		{"non canonical header casing falls back to sniffing", "Image/PNG", pngBytes, "image/png"},
		{"non image header falls back to sniffing", "application/octet-stream", pngBytes, "image/png"},
		{"empty header sniffs jpeg magic", "", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00}, "image/jpeg"},
		{"unknown bytes sniff as text", "", []byte("hello there"), "text/plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, detectImageMIME(tt.contentType, tt.data))
		})
	}
}

func TestResizeImageMaxDim_Table(t *testing.T) {
	// Small dimensions keep the CatmullRom kernel fast under -race while
	// exercising the same aspect-ratio branches as large images.
	t.Run("landscape scaled to width", func(t *testing.T) {
		resized := resizeImageMaxDim(image.NewRGBA(image.Rect(0, 0, 100, 60)), 64)
		assert.Equal(t, 64, resized.Bounds().Dx())
		assert.Equal(t, 60*64/100, resized.Bounds().Dy())
	})

	t.Run("portrait scaled to height", func(t *testing.T) {
		resized := resizeImageMaxDim(image.NewRGBA(image.Rect(0, 0, 60, 100)), 64)
		assert.Equal(t, 60*64/100, resized.Bounds().Dx())
		assert.Equal(t, 64, resized.Bounds().Dy())
	})

	t.Run("square scaled", func(t *testing.T) {
		resized := resizeImageMaxDim(image.NewRGBA(image.Rect(0, 0, 80, 80)), 64)
		assert.Equal(t, 64, resized.Bounds().Dx())
		assert.Equal(t, 64, resized.Bounds().Dy())
	})

	t.Run("within limits returned as is", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 20, 10))
		assert.Same(t, img, resizeImageMaxDim(img, 64))
	})
}

func TestPinnedDialer(t *testing.T) {
	t.Run("connects to pinned ip and port", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()

		_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)

		conn, err := pinnedDialer([]net.IP{net.ParseIP("127.0.0.1")}, port)(context.Background(), "tcp", "ignored.example.test:80")
		require.NoError(t, err)
		require.NotNil(t, conn)
		_ = conn.Close()
	})

	t.Run("all ips failing reports error", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		closedPort := listener.Addr().(*net.TCPAddr).Port
		require.NoError(t, listener.Close())

		_, err = pinnedDialer([]net.IP{net.ParseIP("127.0.0.1")}, strconv.Itoa(closedPort))(context.Background(), "tcp", "ignored.example.test:80")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "all resolved IPs failed")
	})

	t.Run("empty ip list fails closed", func(t *testing.T) {
		_, err := pinnedDialer(nil, "80")(context.Background(), "tcp", "ignored.example.test:80")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "all resolved IPs failed")
	})
}
