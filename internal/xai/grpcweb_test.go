package xai

import (
	"bytes"
	"strings"
	"testing"
)

func successTrailer() []byte {
	trailer := []byte("grpc-status:0\r\ngrpc-message:\r\n")
	buf := make([]byte, 5+len(trailer))
	buf[0] = 0x80
	buf[1] = 0
	buf[2] = 0
	buf[3] = 0
	buf[4] = byte(len(trailer))
	copy(buf[5:], trailer)
	return buf
}

func errorTrailer(code int, msg string) []byte {
	trailer := []byte("grpc-status:" + strings.Repeat("0", 0) + string(rune('0'+code)) + "\r\ngrpc-message:" + msg + "\r\n")
	buf := make([]byte, 5+len(trailer))
	buf[0] = 0x80
	buf[1] = 0
	buf[2] = 0
	buf[3] = 0
	buf[4] = byte(len(trailer))
	copy(buf[5:], trailer)
	return buf
}

// --- grpcwebEncode tests ---

func TestGrpcwebEncode(t *testing.T) {
	data := []byte{0x10, 0x01}
	encoded := grpcwebEncode(data)

	if encoded[0] != 0x00 {
		t.Errorf("expected flag 0x00, got 0x%02x", encoded[0])
	}
	if encoded[1] != 0 || encoded[2] != 0 || encoded[3] != 0 || encoded[4] != 2 {
		t.Errorf("unexpected length bytes: %v", encoded[1:5])
	}
	if !bytes.Equal(encoded[5:], data) {
		t.Errorf("payload mismatch: got %v, want %v", encoded[5:], data)
	}
}

// --- grpcwebParseTrailers tests ---

func TestGrpcwebParseTrailers_Success(t *testing.T) {
	body := successTrailer()
	code, msg, err := grpcwebParseTrailers(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("expected code 0, got %d", code)
	}
	if msg != "" {
		t.Errorf("expected empty message, got %q", msg)
	}
}

func TestGrpcwebParseTrailers_Error(t *testing.T) {
	body := errorTrailer(7, "forbidden")
	code, msg, err := grpcwebParseTrailers(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 7 {
		t.Errorf("expected code 7, got %d", code)
	}
	if msg != "forbidden" {
		t.Errorf("expected message 'forbidden', got %q", msg)
	}
}

func TestGrpcwebParseTrailers_NoTrailer(t *testing.T) {
	data := grpcwebEncode([]byte{0x10, 0x01})
	_, _, err := grpcwebParseTrailers(data)
	if err == nil {
		t.Error("expected error for missing trailer")
	}
}

// --- buildNsfwPayload tests ---

func TestBuildNsfwPayload_Enabled(t *testing.T) {
	payload := buildNsfwPayload(true)
	if payload[0] != 0x00 {
		t.Errorf("expected flag 0x00, got 0x%02x", payload[0])
	}
	if !bytes.Contains(payload, []byte("always_show_nsfw_content")) {
		t.Error("payload should contain feature name")
	}
	protoStart := 5
	if payload[protoStart+3] != 0x01 {
		t.Errorf("expected enable byte 0x01, got 0x%02x", payload[protoStart+3])
	}
}

func TestBuildNsfwPayload_Disabled(t *testing.T) {
	payload := buildNsfwPayload(false)
	protoStart := 5
	if payload[protoStart+3] != 0x00 {
		t.Errorf("expected enable byte 0x00, got 0x%02x", payload[protoStart+3])
	}
}

// --- truncate tests ---

func TestTruncate(t *testing.T) {
	if got := truncate([]byte("short"), 10); got != "short" {
		t.Errorf("expected 'short', got %q", got)
	}
	if got := truncate([]byte("hello world"), 5); got != "hello..." {
		t.Errorf("expected 'hello...', got %q", got)
	}
}
