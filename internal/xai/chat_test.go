package xai

import "testing"

func TestIsCFChallenge(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        bool
	}{
		{"html content type", "text/html; charset=UTF-8", "<html>", true},
		{"cloudflare in body", "application/json", `{"error":"cloudflare block"}`, true},
		{"cf- prefix in body", "text/plain", `<div class="cf-browser-verification">`, true},
		{"challenge-platform", "text/plain", `<div id="challenge-platform">`, true},
		{"just a moment", "text/plain", `<title>Just a moment...</title>`, true},
		{"json 403 body", "application/json", `{"error":"forbidden"}`, false},
		{"empty body", "", "", false},
		{"short text", "text/plain", "Forbidden", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCFChallenge(tt.contentType, tt.body); got != tt.want {
				t.Errorf("isCFChallenge(%q, %q) = %v, want %v",
					tt.contentType, tt.body, got, tt.want)
			}
		})
	}
}
