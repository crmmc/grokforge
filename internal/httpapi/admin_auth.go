package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/crmmc/grokforge/internal/config"
)

const adminCookieName = "gf_session"
const adminCookieMaxAge = 30 * 24 * 60 * 60 // 30 days
const adminSessionTTL = 30 * 24 * time.Hour

func signAdminSession(appKey string, issuedAt time.Time) string {
	payload := strconv.FormatInt(issuedAt.UTC().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(appKey))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

func verifyAdminSession(appKey, value string) bool {
	if strings.TrimSpace(appKey) == "" || strings.TrimSpace(value) == "" {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return false
	}
	issuedAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	issuedAtTime := time.Unix(issuedAt, 0).UTC()
	now := time.Now().UTC()
	if issuedAtTime.After(now.Add(time.Minute)) {
		return false
	}
	if now.Sub(issuedAtTime) > adminSessionTTL {
		return false
	}
	expected := signAdminSession(appKey, issuedAtTime)
	return subtle.ConstantTimeCompare([]byte(value), []byte(expected)) == 1
}

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

		setAdminCookie(w, r, signAdminSession(appKey, time.Now().UTC()), adminCookieMaxAge)
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleAdminLoginRuntime(runtime *config.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := runtime.Get()
		appKey := ""
		if cfg != nil {
			appKey = cfg.App.AppKey
		}
		handleAdminLogin(appKey).ServeHTTP(w, r)
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
