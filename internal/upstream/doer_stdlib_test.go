package upstream

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

func TestStdlibDoer_RoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "v1" {
			t.Errorf("header not forwarded: %q", r.Header.Get("X-Test"))
		}
		w.Header().Set("X-Reply", "ok")
		w.WriteHeader(201)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	req, _ := fhttp.NewRequest("GET", srv.URL, nil)
	req.Header.Set("X-Test", "v1")

	d := &StdlibDoer{Client: srv.Client()}
	resp, err := d.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Errorf("status=%d want 201", resp.StatusCode)
	}
	if resp.Header.Get("X-Reply") != "ok" {
		t.Errorf("reply header missing")
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "hello" {
		t.Errorf("body=%q", b)
	}
}
