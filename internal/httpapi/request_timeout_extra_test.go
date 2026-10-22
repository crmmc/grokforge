package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestTimeoutRuntimeMiddleware(t *testing.T) {
	cfg := &config.Config{
		App:   config.AppConfig{RequestTimeout: 90},
		Proxy: config.ProxyConfig{Timeout: 180},
	}
	runtime := config.NewRuntime(cfg)

	handler := requestTimeoutRuntimeMiddleware(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, ok := r.Context().Deadline()
		require.True(t, ok, "expected context deadline")
		_, _ = w.Write([]byte(strconv.FormatInt(int64(time.Until(deadline)/time.Second), 10)))
	}))

	t.Run("chat route uses proxy timeout", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		got, err := strconv.Atoi(w.Body.String())
		require.NoError(t, err)
		assert.LessOrEqual(t, got, 180)
		assert.GreaterOrEqual(t, got, 178)
	})

	t.Run("hot reload changes deadline", func(t *testing.T) {
		updated := &config.Config{Proxy: config.ProxyConfig{Timeout: 300}}
		runtime.Store(updated)

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		got, err := strconv.Atoi(w.Body.String())
		require.NoError(t, err)
		assert.LessOrEqual(t, got, 300)
		assert.GreaterOrEqual(t, got, 298)
	})
}
