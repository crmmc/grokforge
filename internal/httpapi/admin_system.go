package httpapi

import (
	"net/http"
	"time"
)

// SystemStatusResponse matches the frontend SystemStatus type.
type SystemStatusResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Uptime  float64           `json:"uptime"`
	Tokens  StatusTokenCount  `json:"tokens"`
	APIKeys StatusAPIKeyCount `json:"api_keys"`
}

// StatusTokenCount holds token counts for system status.
type StatusTokenCount struct {
	Total  int `json:"total"`
	Active int `json:"active"`
}

// StatusAPIKeyCount holds API key counts for system status.
type StatusAPIKeyCount struct {
	Total  int `json:"total"`
	Active int `json:"active"`
}

// handleSystemStatus returns a handler that reports system status.
func handleSystemStatus(ts TokenStoreInterface, aks APIKeyStoreInterface, startTime time.Time, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := SystemStatusResponse{
			Status:  "healthy",
			Version: version,
			Uptime:  time.Since(startTime).Seconds(),
			Tokens:  StatusTokenCount{},
			APIKeys: StatusAPIKeyCount{},
		}

		// Get token counts
		if ts != nil {
			tokens, err := ts.ListTokens(r.Context())
			if err == nil {
				resp.Tokens.Total = len(tokens)
				for _, t := range tokens {
					if t.Status == "active" {
						resp.Tokens.Active++
					}
				}
			}
		}

		// Get API key counts
		if aks != nil {
			total, active, _, _, _, err := aks.CountByStatus(r.Context())
			if err == nil {
				resp.APIKeys.Total = total
				resp.APIKeys.Active = active
			}
		}

		WriteJSON(w, http.StatusOK, resp)
	}
}
