package xai

import (
	"strings"

	"github.com/bogdanfinn/tls-client/profiles"
)

// ResolveBrowserProfile maps a user-friendly browser name to a tls-client ClientProfile.
// Accepts formats like "chrome133", "chrome_133", "Chrome_133", "firefox135", etc.
// Falls back to Chrome_146 if no match is found.
func ResolveBrowserProfile(name string) profiles.ClientProfile {
	if name == "" {
		return profiles.Chrome_146
	}

	// Normalize: lowercase, ensure underscore between name and version
	key := strings.ToLower(strings.TrimSpace(name))

	// Try direct lookup first
	if p, ok := profiles.MappedTLSClients[key]; ok {
		return p
	}

	// Insert underscore between letters and digits: "chrome133" → "chrome_133"
	normalized := insertUnderscore(key)
	if p, ok := profiles.MappedTLSClients[normalized]; ok {
		return p
	}

	return profiles.Chrome_146
}

// insertUnderscore inserts an underscore between trailing letters and leading digits.
// e.g. "chrome133" → "chrome_133", "firefox135" → "firefox_135"
func insertUnderscore(s string) string {
	for i := 1; i < len(s); i++ {
		if isLetter(s[i-1]) && isDigit(s[i]) {
			return s[:i] + "_" + s[i:]
		}
	}
	return s
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }
