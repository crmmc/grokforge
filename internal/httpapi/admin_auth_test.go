package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleAdminLogin_Success(t *testing.T) {
	handler := handleAdminLogin("my-secret-key")

	body := `{"key":"my-secret-key"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify cookie is set
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	assert.Equal(t, "gf_session", c.Name)
	assert.NotEqual(t, "my-secret-key", c.Value)
	assert.True(t, verifyAdminSession("my-secret-key", c.Value))
	assert.Equal(t, "/admin", c.Path)
	assert.True(t, c.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, 30*24*60*60, c.MaxAge)
}

func TestHandleAdminLogin_WrongKey(t *testing.T) {
	handler := handleAdminLogin("my-secret-key")

	body := `{"key":"wrong-key"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
}

func TestHandleAdminLogin_EmptyKey(t *testing.T) {
	handler := handleAdminLogin("my-secret-key")

	body := `{"key":""}`
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleAdminLogin_AppKeyNotConfigured(t *testing.T) {
	handler := handleAdminLogin("")

	body := `{"key":"anything"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleAdminLogin_SecureFlagOnHTTPS(t *testing.T) {
	handler := handleAdminLogin("my-secret-key")

	body := `{"key":"my-secret-key"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].Secure)
}

func TestHandleAdminLogout(t *testing.T) {
	handler := handleAdminLogout()

	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	assert.Equal(t, "gf_session", c.Name)
	assert.Equal(t, -1, c.MaxAge)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["status"])
}

func TestVerifyAdminSession_Expired(t *testing.T) {
	value := signAdminSession("my-secret-key", time.Now().Add(-adminSessionTTL-time.Hour))
	assert.False(t, verifyAdminSession("my-secret-key", value))
}

func TestVerifyAdminSession_FutureTimestampRejected(t *testing.T) {
	value := signAdminSession("my-secret-key", time.Now().Add(2*time.Minute))
	assert.False(t, verifyAdminSession("my-secret-key", value))
}

func TestLogout_RevokesSession(t *testing.T) {
	appKey := "test-key"

	// Login to get a valid session
	session := signAdminSession(appKey, time.Now().UTC())
	assert.True(t, verifyAdminSession(appKey, session), "session should be valid before logout")

	// Logout with the session cookie
	handler := handleAdminLogout()
	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: session})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Session should now be revoked
	assert.False(t, verifyAdminSession(appKey, session), "session should be rejected after logout")

	// Cleanup
	revokedSessions.Delete(session)
}

func TestLogout_NoCookie_NoRevocation(t *testing.T) {
	handler := handleAdminLogout()
	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	// No panic, no error — graceful no-op for revocation
}

func TestRevokeSession_InvalidFormat(t *testing.T) {
	// Should not panic on invalid session values
	revokeSession("")
	revokeSession("invalid")
	revokeSession("not.a.valid.session")
}

func TestIsSessionRevoked_ExpiredEntry(t *testing.T) {
	// Manually insert an already-expired revocation entry
	expiredSession := signAdminSession("key", time.Now().Add(-adminSessionTTL-time.Hour))
	revokedSessions.Store(expiredSession, time.Now().Add(-time.Hour))

	// Still in map (cleanup hasn't run), but isSessionRevoked just checks presence
	assert.True(t, isSessionRevoked(expiredSession))

	// Cleanup
	revokedSessions.Delete(expiredSession)
}
