package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodySizeLimitRuntimeMiddleware(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.BodyLimit = 1024
	cfg.App.ChatBodyLimit = 2048
	runtime := config.NewRuntime(cfg)

	t.Run("within general limit", func(t *testing.T) {
		handler := bodySizeLimitRuntimeMiddleware(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodPost, "/v1/models", bytes.NewReader(make([]byte, 512)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("over general limit rejected", func(t *testing.T) {
		handler := bodySizeLimitRuntimeMiddleware(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodPost, "/v1/models", bytes.NewReader(make([]byte, 4096)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})

	t.Run("chat route uses chat limit", func(t *testing.T) {
		handler := bodySizeLimitRuntimeMiddleware(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(make([]byte, 1500)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("hot reload applies to new requests", func(t *testing.T) {
		updated := config.DefaultConfig()
		updated.App.BodyLimit = 128
		runtime.Store(updated)

		handler := bodySizeLimitRuntimeMiddleware(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodPost, "/v1/models", bytes.NewReader(make([]byte, 512)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})
}
