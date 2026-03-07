package cfrefresh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func makeOKResponse(cookies []flaresolverrCookie, userAgent string) flaresolverrResponse {
	return flaresolverrResponse{
		Status: "ok",
		Solution: flaresolverrSolution{
			UserAgent: userAgent,
			Cookies:   cookies,
		},
	}
}

func TestSolveCFChallenge_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req flaresolverrRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Cmd != "request.get" {
			t.Errorf("expected cmd request.get, got %s", req.Cmd)
		}
		if req.URL != grokURL {
			t.Errorf("expected url %s, got %s", grokURL, req.URL)
		}

		resp := makeOKResponse(
			[]flaresolverrCookie{
				{Name: "cf_clearance", Value: "abc123"},
				{Name: "__cf_bm", Value: "xyz"},
			},
			"Mozilla/5.0 (X11; Linux x86_64) Chrome/133.0.0.0 Safari/537.36",
		)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	result, err := SolveCFChallenge(srv.URL, 60, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CFClearance != "abc123" {
		t.Errorf("expected cf_clearance abc123, got %s", result.CFClearance)
	}
	if result.Browser != "chrome133" {
		t.Errorf("expected browser chrome133, got %s", result.Browser)
	}
	if result.Cookies != "cf_clearance=abc123; __cf_bm=xyz" {
		t.Errorf("unexpected cookies: %s", result.Cookies)
	}
	if result.UserAgent == "" {
		t.Error("expected non-empty user agent")
	}
}

func TestSolveCFChallenge_WithProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req flaresolverrRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Proxy == nil || req.Proxy.URL != "http://proxy:8080" {
			t.Errorf("expected proxy url http://proxy:8080, got %+v", req.Proxy)
		}
		resp := makeOKResponse(
			[]flaresolverrCookie{{Name: "cf_clearance", Value: "ok"}},
			"Chrome/130",
		)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	result, err := SolveCFChallenge(srv.URL, 60, "http://proxy:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CFClearance != "ok" {
		t.Errorf("expected cf_clearance ok, got %s", result.CFClearance)
	}
}

func TestSolveCFChallenge_StatusNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(flaresolverrResponse{
			Status:  "error",
			Message: "something went wrong",
		})
	}))
	defer srv.Close()

	_, err := SolveCFChallenge(srv.URL, 60, "")
	if err == nil {
		t.Fatal("expected error for non-ok status")
	}
}

func TestSolveCFChallenge_EmptyCookies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(flaresolverrResponse{
			Status:   "ok",
			Solution: flaresolverrSolution{Cookies: []flaresolverrCookie{}},
		})
	}))
	defer srv.Close()

	_, err := SolveCFChallenge(srv.URL, 60, "")
	if err == nil {
		t.Fatal("expected error for empty cookies")
	}
}

func TestSolveCFChallenge_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := SolveCFChallenge(srv.URL, 60, "")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestExtractBrowserProfile(t *testing.T) {
	tests := []struct {
		ua   string
		want string
	}{
		{"Mozilla/5.0 Chrome/133.0.0.0 Safari/537.36", "chrome133"},
		{"Mozilla/5.0 Chrome/120.0.6099.234 Safari/537.36", "chrome120"},
		{"Mozilla/5.0 (Firefox)", "chrome120"},
		{"", "chrome120"},
	}
	for _, tt := range tests {
		got := extractBrowserProfile(tt.ua)
		if got != tt.want {
			t.Errorf("extractBrowserProfile(%q) = %q, want %q", tt.ua, got, tt.want)
		}
	}
}

func TestExtractAllCookies(t *testing.T) {
	cookies := []flaresolverrCookie{
		{Name: "a", Value: "1"},
		{Name: "b", Value: "2"},
	}
	got := extractAllCookies(cookies)
	want := "a=1; b=2"
	if got != want {
		t.Errorf("extractAllCookies = %q, want %q", got, want)
	}
}

func TestExtractCookieValue(t *testing.T) {
	cookies := []flaresolverrCookie{
		{Name: "cf_clearance", Value: "abc"},
		{Name: "other", Value: "xyz"},
	}
	if v := extractCookieValue(cookies, "cf_clearance"); v != "abc" {
		t.Errorf("expected abc, got %s", v)
	}
	if v := extractCookieValue(cookies, "missing"); v != "" {
		t.Errorf("expected empty, got %s", v)
	}
}
