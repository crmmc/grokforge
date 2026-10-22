package cfrefresh

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSolveCFChallenge_ConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	result, err := SolveCFChallenge(url, 5, "")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "POST")
	assert.Contains(t, err.Error(), url)
}

func TestSolveCFChallenge_ReadResponseError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// Read the request headers first so the response is never
				// seen as unsolicited by the HTTP transport.
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
				}
				// Promise 1000 bytes but close after a few, so the client's
				// io.ReadAll on the body fails with an unexpected EOF.
				_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1000\r\nConnection: close\r\n\r\nshort"))
			}(conn)
		}
	}()

	url := "http://" + ln.Addr().String()
	result, err := SolveCFChallenge(url, 5, "")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "read response")
}

func TestSolveCFChallenge_InvalidJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("this is not json"))
	}))
	defer srv.Close()

	result, err := SolveCFChallenge(srv.URL, 5, "")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "decode response")
}

func TestSolveCFChallenge_ErrorPreviewTruncatedTo300Chars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, strings.Repeat("x", 500), http.StatusBadGateway)
	}))
	defer srv.Close()

	result, err := SolveCFChallenge(srv.URL, 5, "")
	require.Error(t, err)
	assert.Nil(t, result)
	// "HTTP 502: " prefix plus at most 300 characters of body preview.
	assert.Equal(t, len("HTTP 502: ")+300, len(err.Error()))
	assert.True(t, strings.HasPrefix(err.Error(), "HTTP 502: "))
}

func TestSolveCFChallenge_TrailingSlashesInURL(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		resp := makeOKResponse(
			[]flaresolverrCookie{{Name: "cf_clearance", Value: "trimmed"}},
			"Mozilla/5.0 Chrome/136.0.0.0 Safari/537.36",
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	result, err := SolveCFChallenge(srv.URL+"///", 5, "")
	require.NoError(t, err)
	assert.Equal(t, "/v1", seenPath)
	assert.Equal(t, "trimmed", result.CFClearance)
}
