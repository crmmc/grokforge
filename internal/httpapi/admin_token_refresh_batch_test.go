package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBatchRefresher struct {
	refreshedIDs []uint
	failIDs      map[uint]error
}

func (m *mockBatchRefresher) RefreshToken(_ context.Context, id uint) (*store.Token, error) {
	m.refreshedIDs = append(m.refreshedIDs, id)
	if err, ok := m.failIDs[id]; ok {
		return nil, err
	}
	return &store.Token{ID: id}, nil
}

func TestHandleBatchRefresh_WithIDs(t *testing.T) {
	ts := newMockTokenStore()
	ts.tokens[1] = &store.Token{ID: 1, Status: store.TokenStatusActive}
	ts.tokens[2] = &store.Token{ID: 2, Status: store.TokenStatusActive}
	ts.tokens[3] = &store.Token{ID: 3, Status: store.TokenStatusActive}

	refresher := &mockBatchRefresher{failIDs: map[uint]error{2: errors.New("quota sync failed")}}
	handler := handleBatchRefresh(ts, refresher)

	body := `{"ids":[1,2,3]}`
	req := httptest.NewRequest(http.MethodPost, "/tokens/batch/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

	events := parseSSEEvents(t, w.Body.String())
	require.Len(t, events, 4) // 3 progress + 1 complete

	// Verify progress events
	assert.Equal(t, "progress", events[0].Type)
	assert.Equal(t, uint(1), events[0].TokenID)
	assert.Equal(t, "success", events[0].Status)

	assert.Equal(t, "progress", events[1].Type)
	assert.Equal(t, uint(2), events[1].TokenID)
	assert.Equal(t, "error", events[1].Status)
	assert.Equal(t, tokenRefreshFailedMessage, events[1].Error)
	assert.NotContains(t, w.Body.String(), "quota sync failed")

	assert.Equal(t, "progress", events[2].Type)
	assert.Equal(t, uint(3), events[2].TokenID)
	assert.Equal(t, "success", events[2].Status)

	// Verify complete event
	assert.Equal(t, "complete", events[3].Type)
	assert.Equal(t, 2, events[3].Success)
	assert.Equal(t, 1, events[3].Failed)
	assert.Equal(t, 3, events[3].Total)

	// Verify only requested IDs were refreshed
	assert.Equal(t, []uint{1, 2, 3}, refresher.refreshedIDs)
}

func TestHandleBatchRefresh_EmptyBody_RefreshesAllActive(t *testing.T) {
	ts := newMockTokenStore()
	ts.tokens[1] = &store.Token{ID: 1, Status: store.TokenStatusActive}
	ts.tokens[2] = &store.Token{ID: 2, Status: "disabled"}

	refresher := &mockBatchRefresher{failIDs: map[uint]error{}}
	handler := handleBatchRefresh(ts, refresher)

	req := httptest.NewRequest(http.MethodPost, "/tokens/batch/refresh", http.NoBody)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Only active token should be refreshed
	assert.Equal(t, []uint{1}, refresher.refreshedIDs)
}

func TestHandleBatchRefresh_ChunkedBody(t *testing.T) {
	ts := newMockTokenStore()
	ts.tokens[5] = &store.Token{ID: 5, Status: store.TokenStatusActive}
	ts.tokens[10] = &store.Token{ID: 10, Status: store.TokenStatusActive}

	refresher := &mockBatchRefresher{failIDs: map[uint]error{}}
	handler := handleBatchRefresh(ts, refresher)

	// Simulate chunked transfer: ContentLength = -1
	body := bytes.NewReader([]byte(`{"ids":[5]}`))
	req := httptest.NewRequest(http.MethodPost, "/tokens/batch/refresh", body)
	req.ContentLength = -1 // simulate chunked encoding
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Must only refresh ID 5, not all active tokens
	assert.Equal(t, []uint{5}, refresher.refreshedIDs)
}

func TestHandleBatchRefresh_NilRefresher(t *testing.T) {
	ts := newMockTokenStore()
	handler := handleBatchRefresh(ts, nil)

	req := httptest.NewRequest(http.MethodPost, "/tokens/batch/refresh", http.NoBody)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

func TestHandleBatchRefresh_NoActiveTokens(t *testing.T) {
	ts := newMockTokenStore()
	ts.tokens[1] = &store.Token{ID: 1, Status: "disabled"}

	refresher := &mockBatchRefresher{failIDs: map[uint]error{}}
	handler := handleBatchRefresh(ts, refresher)

	req := httptest.NewRequest(http.MethodPost, "/tokens/batch/refresh", http.NoBody)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func parseSSEEvents(t *testing.T, raw string) []BatchRefreshEvent {
	t.Helper()
	var events []BatchRefreshEvent
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		var evt BatchRefreshEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			t.Fatalf("failed to parse SSE event: %v, data: %s", err, data)
		}
		events = append(events, evt)
	}
	return events
}
