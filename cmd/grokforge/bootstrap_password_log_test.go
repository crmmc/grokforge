package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestLogTemporaryBootstrapAdminPassword(t *testing.T) {
	prev := slog.Default()
	defer slog.SetDefault(prev)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)

	logTemporaryBootstrapAdminPassword("secret-pass")

	output := buf.String()
	for _, want := range []string{
		"search_marker=" + tempAdminPasswordLogMarker,
		"msg=" + tempAdminPasswordLogMarker,
		"temp_admin_password=secret-pass",
		"persistence=runtime_only",
		"expires=process_exit",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
