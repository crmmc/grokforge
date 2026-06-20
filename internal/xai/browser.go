package xai

import "github.com/crmmc/grokforge/internal/upstream/transport"

// ResolveBrowserProfile maps a user-friendly browser name to a supported curl-impersonate profile.
func ResolveBrowserProfile(name string) string {
	if resolved := transport.ResolveProfile(name); resolved != "" {
		return resolved
	}
	return transport.DefaultProfile
}
