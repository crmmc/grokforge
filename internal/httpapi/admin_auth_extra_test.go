package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyAdminSession_Extra(t *testing.T) {
	valid := signAdminSession("secret-key", time.Now().UTC())

	tests := []struct {
		name   string
		appKey string
		value  string
		want   bool
	}{
		{name: "empty app key rejected", appKey: "", value: valid, want: false},
		{name: "whitespace app key rejected", appKey: "   ", value: valid, want: false},
		{name: "empty value rejected", appKey: "secret-key", value: "", want: false},
		{name: "value without dot rejected", appKey: "secret-key", value: "nodot", want: false},
		{name: "non numeric issuedAt rejected", appKey: "secret-key", value: "abc.def", want: false},
		{name: "wrong signature rejected", appKey: "secret-key", value: fmt.Sprintf("%d.badc0ffee", time.Now().UTC().Unix()), want: false},
		{name: "valid session accepted", appKey: "secret-key", value: valid, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, verifyAdminSession(tc.appKey, tc.value))
		})
	}
}

func TestRevokeSession_NonNumericIssuedAt(t *testing.T) {
	revokeSession("abc.def")

	if _, ok := revokedSessions.Load("abc.def"); ok {
		t.Fatal("non-numeric session value must not be stored")
	}
}

func TestPurgeExpiredRevokedSessions(t *testing.T) {
	past := time.Now().Add(-2 * adminSessionTTL)
	future := time.Now().Add(adminSessionTTL)

	revokedSessions.Store("expired.session", past)
	revokedSessions.Store("active.session", future)

	purgeExpiredRevokedSessions(time.Now())

	_, expiredStillThere := revokedSessions.Load("expired.session")
	assert.False(t, expiredStillThere, "expired entry must be purged")

	_, activeStillThere := revokedSessions.Load("active.session")
	assert.True(t, activeStillThere, "valid entry must be kept")
}

func TestPurgeRevokedLoop_ProcessesTicksUntilClosed(t *testing.T) {
	revokedSessions.Store("loop.expired", time.Now().Add(-2*adminSessionTTL))

	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		purgeRevokedLoop(tick)
		close(done)
	}()

	tick <- time.Now()
	close(tick)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("purgeRevokedLoop did not exit after channel close")
	}

	_, stillThere := revokedSessions.Load("loop.expired")
	assert.False(t, stillThere, "tick must purge expired entries")
}

func TestHandleAdminLogin_WhitespaceOnlyKey(t *testing.T) {
	w := httptest.NewRecorder()
	body := strings.NewReader(`{"key":"  "}`)
	handleAdminLogin("secret")(w, httptest.NewRequest(http.MethodPost, "/admin/login", body))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleAdminLoginRuntime_NilConfig(t *testing.T) {
	runtime := config.NewRuntime(nil)
	w := httptest.NewRecorder()
	handleAdminLoginRuntime(runtime)(w, httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(`{"key":"x"}`)))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandleAdminLogout_ClearsCookieAndRevokes(t *testing.T) {
	session := signAdminSession("secret-key", time.Now().UTC())
	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: session})

	w := httptest.NewRecorder()
	handleAdminLogout()(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, isSessionRevoked(session), "session must be revoked server-side")

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, adminCookieName, cookies[0].Name)
	assert.Equal(t, -1, cookies[0].MaxAge)
}

func TestSignAdminSession_Deterministic(t *testing.T) {
	at := time.Unix(1750000000, 0)
	a := signAdminSession("k", at)
	b := signAdminSession("k", at)
	assert.Equal(t, a, b)
	assert.Contains(t, a, "1750000000.")
}

func TestSetAdminCookie_SecureBehindProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/login", nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	w := httptest.NewRecorder()
	setAdminCookie(w, req, "sess", 60)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].Secure)
	assert.Equal(t, "/admin", cookies[0].Path)
}

func TestVerifyAdminSession_RevokedSessionRejected(t *testing.T) {
	session := signAdminSession("secret-key", time.Now().UTC().Add(30*time.Second))
	require.True(t, verifyAdminSession("secret-key", session))

	revokeSession(session)
	assert.False(t, verifyAdminSession("secret-key", session), "revoked session must be rejected")
}
