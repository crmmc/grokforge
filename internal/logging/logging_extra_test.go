package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupFileLog configures the global logger to write into a temp file and
// returns the file path. Logs also go to stdout (MultiWriter), which is
// harmless in tests.
func setupFileLog(t *testing.T, level string, jsonFormat bool, mutate func(*FileConfig)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.log")
	fc := &FileConfig{Path: path, MaxSizeMB: 1, MaxBackups: 1}
	if mutate != nil {
		mutate(fc)
	}
	Setup(level, jsonFormat, fc)
	return path
}

func readLogFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func logAtAllLevels() {
	Debug("dbg-msg", "k", "v")
	Info("info-msg", "k", "v")
	Warn("warn-msg", "k", "v")
	Error("error-msg", "k", "v")
}

func TestSetup_TextHandler_WritesToRotatingFile(t *testing.T) {
	path := setupFileLog(t, "debug", false, nil)
	logAtAllLevels()

	content := readLogFile(t, path)
	assert.Contains(t, content, "level=DEBUG")
	assert.Contains(t, content, "msg=dbg-msg")
	assert.Contains(t, content, "msg=info-msg")
	assert.Contains(t, content, "msg=warn-msg")
	assert.Contains(t, content, "msg=error-msg")
	assert.Contains(t, content, "k=v")
	assert.NotContains(t, content, `"level"`)
}

func TestSetup_JSONHandler_FiltersDebugAtInfoLevel(t *testing.T) {
	path := setupFileLog(t, "info", true, nil)
	logAtAllLevels()

	content := readLogFile(t, path)
	assert.Contains(t, content, `"msg":"info-msg"`)
	assert.Contains(t, content, `"msg":"warn-msg"`)
	assert.Contains(t, content, `"msg":"error-msg"`)
	assert.NotContains(t, content, `"msg":"dbg-msg"`)

	var entry map[string]any
	line := strings.Split(strings.TrimSpace(content), "\n")[0]
	require.NoError(t, json.Unmarshal([]byte(line), &entry))
	assert.Equal(t, "INFO", entry["level"])
}

func TestSetup_LevelParsing(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		visible map[string]bool // msg prefix -> expected presence
	}{
		{"debug shows everything", "debug", map[string]bool{"dbg": true, "info": true, "warn": true, "error": true}},
		{"empty defaults to info", "", map[string]bool{"dbg": false, "info": true, "warn": true, "error": true}},
		{"unknown defaults to info", "nonsense", map[string]bool{"dbg": false, "info": true, "warn": true, "error": true}},
		{"warn hides debug and info", "warn", map[string]bool{"dbg": false, "info": false, "warn": true, "error": true}},
		{"warning alias case-insensitive", "WARNING", map[string]bool{"dbg": false, "info": false, "warn": true, "error": true}},
		{"error only shows errors", "error", map[string]bool{"dbg": false, "info": false, "warn": false, "error": true}},
		{"mixed case debug", "Debug", map[string]bool{"dbg": true, "info": true, "warn": true, "error": true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := setupFileLog(t, tc.level, false, nil)
			logAtAllLevels()
			content := readLogFile(t, path)

			for prefix, want := range tc.visible {
				if want {
					assert.Contains(t, content, "msg="+prefix+"-msg")
				} else {
					assert.NotContains(t, content, "msg="+prefix+"-msg")
				}
			}
		})
	}
}

func TestSetup_DefaultsRotationSettingsWhenInvalid(t *testing.T) {
	// MaxSizeMB/MaxBackups <= 0 fall back to internal defaults; the call must
	// succeed and the file must receive logs.
	path := setupFileLog(t, "info", false, func(fc *FileConfig) {
		fc.MaxSizeMB = 0
		fc.MaxBackups = -3
	})
	Info("after-defaults")
	assert.Contains(t, readLogFile(t, path), "msg=after-defaults")

	path = setupFileLog(t, "info", false, func(fc *FileConfig) {
		fc.MaxSizeMB = -1
		fc.MaxBackups = 0
	})
	Error("after-negative-defaults")
	assert.Contains(t, readLogFile(t, path), "msg=after-negative-defaults")
}

func TestSetup_NilFileConfig_LogsToStdoutOnly(t *testing.T) {
	require.NotPanics(t, func() {
		Setup("info", true, nil)
	})
	Info("nil-fileconfig") // goes to stdout; nothing to assert beyond no-panic
}

func TestSetup_EmptyFileConfigPath_LogsToStdoutOnly(t *testing.T) {
	require.NotPanics(t, func() {
		Setup("info", false, &FileConfig{})
	})
	Info("empty-path")
}

func TestSetup_CreatesMissingLogDirectory(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "nested", "deeper", "app.log")
	Setup("info", false, &FileConfig{Path: path, MaxSizeMB: 1, MaxBackups: 1})
	Info("dir-creation")

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Contains(t, readLogFile(t, path), "msg=dir-creation")
}

func TestSetup_WarnsButContinuesWhenMkdirFails(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a dir"), 0o644))

	path := filepath.Join(blocker, "inner", "app.log")
	require.NotPanics(t, func() {
		Setup("info", false, &FileConfig{Path: path, MaxSizeMB: 1, MaxBackups: 1})
	})
	// No file could be created under the blocker path.
	_, statErr := os.Stat(path)
	require.Error(t, statErr)
}

func TestSetup_RelativePathWithoutDirectory_SkipsMkdir(t *testing.T) {
	// filepath.Dir returns "." for a bare filename; the MkdirAll branch must be
	// skipped and no file should be created until the first log write.
	name := "grokforge-extra-test-must-not-exist.log"
	t.Cleanup(func() {
		_ = os.Remove(name)
	})

	require.NotPanics(t, func() {
		Setup("info", false, &FileConfig{Path: name, MaxSizeMB: 1, MaxBackups: 1})
	})
	_, err := os.Stat(name)
	assert.Error(t, err, "lumberjack opens lazily; file must not exist without a log write")
}
