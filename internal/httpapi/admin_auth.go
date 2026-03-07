package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

const adminCookieName = "gf_session"
const adminCookieMaxAge = 30 * 24 * 60 * 60 // 30 days

// setAdminCookie writes the httpOnly session cookie.
// Secure flag is set when the request arrives over TLS or behind a TLS-terminating proxy.
func setAdminCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    value,
		Path:     "/admin",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

// handleAdminLogin returns a handler for POST /admin/login.
// Validates the provided key against appKey using constant-time comparison.
func handleAdminLogin(appKey string) http.HandlerFunc {
	type loginRequest struct {
		Key string `json:"key"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if appKey == "" {
			WriteError(w, 403, "forbidden", "app_key_not_configured",
				"Admin API is not configured")
			return
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
			WriteError(w, 400, "invalid_request", "missing_key",
				"Missing or invalid key")
			return
		}

		if subtle.ConstantTimeCompare([]byte(req.Key), []byte(appKey)) != 1 {
			WriteError(w, 401, "authentication_error", "invalid_app_key",
				"Invalid app key")
			return
		}

		setAdminCookie(w, r, req.Key, adminCookieMaxAge)
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleAdminLogout returns a handler for POST /admin/logout.
// Clears the session cookie by setting MaxAge=-1.
func handleAdminLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setAdminCookie(w, r, "", -1)
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
