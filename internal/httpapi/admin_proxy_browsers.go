package httpapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/crmmc/grokforge/internal/upstream/transport"
)

func handleGetProxyBrowsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles := transport.SupportedProfileList()
		options := make([]ProxyBrowserOption, 0, len(profiles))
		for _, profile := range profiles {
			ua, ok := transport.UserAgentForProfile(profile)
			if !ok {
				continue
			}
			options = append(options, ProxyBrowserOption{
				Browser:   profile,
				Label:     proxyBrowserLabel(profile),
				UserAgent: ua,
			})
		}
		sort.Slice(options, func(i, j int) bool { return options[i].Browser < options[j].Browser })

		WriteJSON(w, http.StatusOK, ProxyBrowsersResponse{
			DefaultBrowser:   transport.DefaultProfile,
			DefaultUserAgent: transport.DefaultUserAgent(),
			Browsers:         options,
		})
	}
}

func proxyBrowserLabel(profile string) string {
	if profile == "" {
		return ""
	}
	for _, family := range []string{"chrome", "firefox", "safari"} {
		if strings.HasPrefix(profile, family) {
			suffix := strings.TrimPrefix(profile, family)
			if suffix == "" {
				return strings.Title(family)
			}
			return strings.Title(family) + " " + suffix
		}
	}
	return profile
}
