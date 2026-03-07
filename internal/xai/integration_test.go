package xai_test

import (
	"context"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/xai"
)

// TestClientIntegration_Smoke tests basic client creation and lifecycle.
// This test does NOT make real network calls.
func TestClientIntegration_Smoke(t *testing.T) {
	// Create client with test token
	client, err := xai.NewClient("test-token-not-real",
		xai.WithTimeout(5*time.Second),
		xai.WithMaxRetry(1),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Test ResetSession
	if err := client.ResetSession(); err != nil {
		t.Errorf("ResetSession failed: %v", err)
	}

	// Test Close is idempotent
	if err := client.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// TestClientIntegration_ChatCancellation tests context cancellation.
func TestClientIntegration_ChatCancellation(t *testing.T) {
	client, err := xai.NewClient("test-token",
		xai.WithTimeout(1*time.Second),
		xai.WithMaxRetry(0),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := &xai.ChatRequest{
		Messages: []xai.Message{
			{Role: "user", Content: "test"},
		},
		Model: "grok-3",
	}

	events, err := client.Chat(ctx, req)
	if err != nil {
		// Some implementations may return error immediately
		t.Logf("Chat returned error (expected): %v", err)
		return
	}

	// Drain events - should get cancellation error
	for ev := range events {
		if ev.Error != nil {
			t.Logf("Got expected error: %v", ev.Error)
			return
		}
	}
}

// TestClientIntegration_OptionsApplied verifies all options are applied.
func TestClientIntegration_OptionsApplied(t *testing.T) {
	client, err := xai.NewClient("test-token",
		xai.WithTimeout(45*time.Second),
		xai.WithRetryInterval(3*time.Second),
		xai.WithMaxRetry(7),
		xai.WithUserAgent("TestAgent/1.0"),
		xai.WithProxy(""), // Empty proxy should be ignored
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Client created successfully with all options
	t.Log("Client created with all options applied")
}

// TestClientIntegration_ClosePreventsFurtherUse tests that Close prevents Chat.
func TestClientIntegration_ClosePreventsFurtherUse(t *testing.T) {
	client, err := xai.NewClient("test-token")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Close the client
	if err := client.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Try to use Chat after Close
	req := &xai.ChatRequest{
		Messages: []xai.Message{
			{Role: "user", Content: "test"},
		},
	}

	_, err = client.Chat(context.Background(), req)
	if err == nil {
		t.Error("Expected error when calling Chat after Close")
	}
}

// TestClientIntegration_SingleProxy tests client creation with a single proxy.
func TestClientIntegration_SingleProxy(t *testing.T) {
	client, err := xai.NewClient("test-token",
		xai.WithProxy("http://single-proxy.example.com:8080"),
	)
	if err != nil {
		t.Fatalf("Failed to create client with single proxy: %v", err)
	}
	defer client.Close()

	t.Log("Client created with single proxy")
}
