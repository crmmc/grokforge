package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleBatchRefresh_InvalidJSON(t *testing.T) {
	ts := newMockTokenStore()
	refresher := &mockBatchRefresher{}

	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"foo":1}`},
		{name: "malformed json", body: `{"ids":`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleBatchRefresh(ts, refresher)(w, httptest.NewRequest(http.MethodPost, "/admin/tokens/batch/refresh", strings.NewReader(tc.body)))
			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "invalid_json")
		})
	}
}

func TestHandleBatchRefresh_ListTokenIDsError(t *testing.T) {
	ts := &errTokenStore{mockTokenStore: newMockTokenStore(), listIDsErr: errors.New("db")}
	refresher := &mockBatchRefresher{}

	w := httptest.NewRecorder()
	handleBatchRefresh(ts, refresher)(w, httptest.NewRequest(http.MethodPost, "/admin/tokens/batch/refresh", http.NoBody))

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "list_failed")
}

func TestHandleBatchRefresh_ClientDisconnected(t *testing.T) {
	ts := newMockTokenStore()
	ts.tokens[1] = &store.Token{ID: 1, Status: store.TokenStatusActive}
	refresher := &mockBatchRefresher{failIDs: map[uint]error{}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch/refresh", http.NoBody).WithContext(ctx)
	w := httptest.NewRecorder()

	handleBatchRefresh(ts, refresher)(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, refresher.refreshedIDs, "no refresh should run after cancellation")
	assert.NotContains(t, w.Body.String(), "[DONE]", "loop returns before writing events")
}

func TestStreamBatchRefresh_CompletesWithDone(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := NewSSEWriter(rec)
	refresher := &mockBatchRefresher{failIDs: map[uint]error{2: errors.New("boom")}}

	streamBatchRefresh(context.Background(), writer, refresher, []uint{1, 2})

	events := parseSSEEvents(t, rec.Body.String())
	require.Len(t, events, 3)
	assert.Equal(t, "complete", events[2].Type)
	assert.Equal(t, 1, events[2].Success)
	assert.Equal(t, 1, events[2].Failed)
}
