package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
