package xai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockWSServer creates a test WebSocket server for imagine protocol testing.
func mockWSServer(t *testing.T, handler func(*websocket.Conn)) *httptest.Server {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()
		handler(conn)
	}))
	return server
}

func TestImagineClient_Generate_Success(t *testing.T) {
	requestID := "test-req-123"
	finalImageData := "base64-image-data-here"

	server := mockWSServer(t, func(conn *websocket.Conn) {
		// Read the request message
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)

		var req ImagineRequest
		require.NoError(t, json.Unmarshal(msg, &req))
		assert.Equal(t, "conversation.item.create", req.Type)
		assert.NotEmpty(t, req.Item.Content)

		// Send preview event
		previewResp := ImagineResponse{
			Type: "image.preview",
			Item: ImagineResponseItem{
				RequestID: requestID,
				ImageData: "preview-data",
			},
		}
		conn.WriteJSON(previewResp)

		// Send medium event
		mediumResp := ImagineResponse{
			Type: "image.medium",
			Item: ImagineResponseItem{
				RequestID: requestID,
				ImageData: "medium-data",
			},
		}
		conn.WriteJSON(mediumResp)

		// Send final event
		finalResp := ImagineResponse{
			Type: "image.final",
			Item: ImagineResponseItem{
				RequestID: requestID,
				ImageData: finalImageData,
			},
		}
		conn.WriteJSON(finalResp)

		// Send done event
		doneResp := ImagineResponse{
			Type: "done",
		}
		conn.WriteJSON(doneResp)
	})
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	client := &ImagineClient{
		wsURL: wsURL,
		token: "test-token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh, err := client.Generate(ctx, "a cat", "1:1", false)
	require.NoError(t, err)

	var events []ImageEvent
	for event := range eventCh {
		events = append(events, event)
	}

	// Should have preview, medium, final events
	require.GreaterOrEqual(t, len(events), 1)

	// Find final event
	var finalEvent *ImageEvent
	for i := range events {
		if events[i].Type == ImageEventFinal {
			finalEvent = &events[i]
			break
		}
	}
	require.NotNil(t, finalEvent, "should have final event")
	assert.Equal(t, finalImageData, finalEvent.ImageData)
}

func TestImagineClient_Generate_Blocked(t *testing.T) {
	server := mockWSServer(t, func(conn *websocket.Conn) {
		// Read request
		_, _, err := conn.ReadMessage()
		require.NoError(t, err)

		// Send medium but no final (simulates blocked content)
		mediumResp := ImagineResponse{
			Type: "image.medium",
			Item: ImagineResponseItem{
				RequestID: "req-1",
				ImageData: "medium-data",
			},
		}
		conn.WriteJSON(mediumResp)

		// Wait for client to timeout (don't send final)
		time.Sleep(200 * time.Millisecond)
	})
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	client := &ImagineClient{
		wsURL:              wsURL,
		token:              "test-token",
		blockedGraceMillis: 100, // Short grace period for test
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, err := client.Generate(ctx, "blocked content", "1:1", false)
	require.NoError(t, err)

	var blockedEvent *ImageEvent
	for event := range eventCh {
		if event.Type == ImageEventBlocked {
			blockedEvent = &event
			break
		}
	}

	require.NotNil(t, blockedEvent, "should detect blocked content")
}

func TestImagineClient_Generate_ContextCancel(t *testing.T) {
	server := mockWSServer(t, func(conn *websocket.Conn) {
		// Read request
		_, _, _ = conn.ReadMessage()
		// Just wait - client will cancel
		time.Sleep(5 * time.Second)
	})
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	client := &ImagineClient{
		wsURL: wsURL,
		token: "test-token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	eventCh, err := client.Generate(ctx, "test", "1:1", false)
	require.NoError(t, err)

	// Drain channel - should close when context cancels
	var lastEvent ImageEvent
	for event := range eventCh {
		lastEvent = event
	}

	// Should have error event from context cancellation
	assert.True(t, lastEvent.Error != nil || lastEvent.Type == ImageEventError,
		"should have error on context cancel")
}

func TestImagineClient_Generate_LegacyImagePayload(t *testing.T) {
	finalID := "f130879e-dbf7-49fc-8095-816eb37f6b22"
	previewBlob := strings.Repeat("p", 120)
	mediumBlob := strings.Repeat("m", legacyMediumMinBytes+1)
	finalBlob := strings.Repeat("f", legacyFinalMinBytes+1)

	server := mockWSServer(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		require.NoError(t, err)

		conn.WriteJSON(map[string]any{
			"type": "image",
			"url":  "https://grok.com/images/" + finalID + ".png",
			"blob": previewBlob,
		})
		conn.WriteJSON(map[string]any{
			"type": "image",
			"url":  "https://grok.com/images/" + finalID + ".png",
			"blob": mediumBlob,
		})
		conn.WriteJSON(map[string]any{
			"type": "image",
			"url":  "https://grok.com/images/" + finalID + ".png",
			"blob": "data:image/png;base64," + finalBlob,
		})
	})
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := &ImagineClient{
		wsURL: wsURL,
		token: "test-token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh, err := client.Generate(ctx, "legacy payload", "1:1", false)
	require.NoError(t, err)

	hasFinal := false
	for event := range eventCh {
		if event.Type == ImageEventFinal {
			hasFinal = true
			assert.Equal(t, finalBlob, event.ImageData)
		}
	}
	assert.True(t, hasFinal, "should classify legacy payload final image")
}

func TestImagineClient_Generate_CloseAfterFinalShouldSucceed(t *testing.T) {
	finalBlob := strings.Repeat("f", legacyFinalMinBytes+1)

	server := mockWSServer(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		require.NoError(t, err)

		conn.WriteJSON(map[string]any{
			"type": "image",
			"url":  "https://grok.com/images/9d560d18-c143-46ef-a4cb-7b7537c6bbeb.png",
			"blob": finalBlob,
		})
		_ = conn.UnderlyingConn().Close()
	})
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := &ImagineClient{
		wsURL: wsURL,
		token: "test-token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh, err := client.Generate(ctx, "close after final", "1:1", false)
	require.NoError(t, err)

	var (
		hasFinal bool
		hasError bool
	)
	for event := range eventCh {
		if event.Type == ImageEventFinal {
			hasFinal = true
		}
		if event.Type == ImageEventError {
			hasError = true
		}
	}

	assert.True(t, hasFinal, "final event should be emitted before close")
	assert.False(t, hasError, "abrupt close after final should not fail generation")
}

func TestParseAspectRatio(t *testing.T) {
	tests := []struct {
		size     string
		expected string
	}{
		{"1024x1024", "1:1"},
		{"1792x1024", "3:2"},
		{"1024x1792", "2:3"},
		{"1280x720", "16:9"},
		{"720x1280", "9:16"},
		{"unknown", "2:3"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			result := ParseAspectRatio(tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}
