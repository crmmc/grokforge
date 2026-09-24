package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/upstream/console"
	"github.com/crmmc/grokforge/internal/upstream/grok"
	"github.com/crmmc/grokforge/internal/upstream/transport"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restoreLogger swaps the default slog logger for a debug-level buffer and
// restores the previous logger on cleanup. Returns the buffer.
func restoreLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	prev := slog.Default()
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// --- run(): flag and early-exit paths ---

func TestRun_VersionFlag(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"-version"}, &out, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, "grokforge dev (built unknown)\n", out.String())
}

func TestRun_ConfigLoadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("this is not [ valid toml"), 0o644))

	var errOut bytes.Buffer
	err := run([]string{"-config", path}, io.Discard, &errOut)
	require.Error(t, err)
	assert.Contains(t, errOut.String(), "failed to load config:")
}

func TestStartupError(t *testing.T) {
	buf := restoreLogger(t)
	sentinel := errors.New("boom")
	got := startupError("failed to do thing", sentinel)
	assert.Equal(t, sentinel, got)
	assert.Contains(t, buf.String(), "failed to do thing")
	assert.Contains(t, buf.String(), "boom")
}

// --- ensureProxyBrowserUserAgent ---

func TestEnsureProxyBrowserUserAgent(t *testing.T) {
	tests := []struct {
		name         string
		browser      string
		userAgent    string
		wantBrowser  string
		wantUserAgnt string
	}{
		{
			name:         "empty falls back to default profile and UA",
			browser:      "",
			userAgent:    "",
			wantBrowser:  transport.DefaultProfile,
			wantUserAgnt: transport.DefaultUserAgent(),
		},
		{
			name:         "unknown browser resolves to default profile",
			browser:      "netscape4",
			userAgent:    "",
			wantBrowser:  transport.DefaultProfile,
			wantUserAgnt: transport.DefaultUserAgent(),
		},
		{
			name:         "supported browser with empty UA derives paired UA",
			browser:      "chrome131",
			userAgent:    "",
			wantBrowser:  "chrome131",
			wantUserAgnt: transport.ProfileUAMap["chrome131"],
		},
		{
			name:         "explicit values are preserved",
			browser:      "chrome136",
			userAgent:    "Custom/1.0",
			wantBrowser:  "chrome136",
			wantUserAgnt: "Custom/1.0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Proxy: config.ProxyConfig{Browser: tc.browser, UserAgent: tc.userAgent}}
			restoreLogger(t)
			ensureProxyBrowserUserAgent(cfg)
			assert.Equal(t, tc.wantBrowser, cfg.Proxy.Browser)
			assert.Equal(t, tc.wantUserAgnt, cfg.Proxy.UserAgent)
		})
	}
}

func TestEnsureProxyBrowserUserAgent_NilConfig(t *testing.T) {
	assert.NotPanics(t, func() { ensureProxyBrowserUserAgent(nil) })
}

// --- buildXAIOptions / newXAIClient / newImagineClient ---

func TestBuildXAIOptions(t *testing.T) {
	full := &config.Config{
		App: config.AppConfig{DynamicStatsig: true},
		Proxy: config.ProxyConfig{
			Timeout:            5,
			BaseProxyURL:       "http://127.0.0.1:1",
			AssetProxyURL:      "http://127.0.0.1:2",
			SkipProxySSLVerify: true,
			Browser:            "chrome136",
			UserAgent:          "UA/1",
			CFClearance:        "clr",
			CFCookies:          "ck",
		},
	}
	tests := []struct {
		name    string
		cfg     *config.Config
		wantLen int
	}{
		{"minimal config yields only dynamic statsig option", &config.Config{}, 1},
		{"fully populated config yields all options", full, 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := buildXAIOptions(tc.cfg)
			assert.Len(t, opts, tc.wantLen)
		})
	}

	t.Run("options are accepted by xai.NewClient", func(t *testing.T) {
		client, err := xai.NewClient("tok", buildXAIOptions(full)...)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})
}

func TestNewXAIClient(t *testing.T) {
	rt := config.NewRuntime(&config.Config{})

	t.Run("with retry", func(t *testing.T) {
		client, err := newXAIClient(rt, "tok", false)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("noRetry appends max-retry option", func(t *testing.T) {
		client, err := newXAIClient(rt, "tok", true)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("constructor failure propagates", func(t *testing.T) {
		orig := xaiClientConstructor
		xaiClientConstructor = func(token string, opts ...xai.ClientOption) (xai.Client, error) {
			return nil, errors.New("no client")
		}
		t.Cleanup(func() { xaiClientConstructor = orig })

		_, err := newXAIClient(rt, "tok", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no client")
	})
}

func TestNewImagineClient(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
	}{
		{"minimal config", &config.Config{}},
		{"fully populated proxy config", &config.Config{Proxy: config.ProxyConfig{
			BaseProxyURL:       "http://127.0.0.1:1",
			SkipProxySSLVerify: true,
			UserAgent:          "UA/1",
			CFClearance:        "clr",
			CFCookies:          "ck",
		}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gen := newImagineClient(config.NewRuntime(tc.cfg), "tok")
			assert.NotNil(t, gen)
		})
	}
}

// --- runtimeWiring providers ---

func TestRuntimeWiring_ConfigProviders(t *testing.T) {
	cfg := &config.Config{
		App:   config.AppConfig{DynamicStatsig: false, FilterTags: []string{"t1", "t2"}},
		Image: config.ImageConfig{Format: "base64", NSFW: true},
		Proxy: config.ProxyConfig{
			Timeout:            7,
			Browser:            "chrome136",
			UserAgent:          "UA/1",
			BaseProxyURL:       "http://127.0.0.1:1",
			SkipProxySSLVerify: true,
			CFCookies:          "ck",
			CFClearance:        "clr",
		},
		Retry: config.RetryConfig{
			MaxTokens:          3,
			PerTokenRetries:    2,
			RetryBackoffBase:   0.5,
			RetryBackoffFactor: 2,
			RetryBackoffMax:    20,
			RetryBudget:        60,
		},
	}
	w := &runtimeWiring{runtime: config.NewRuntime(cfg)}

	t.Run("appConfig exposes live app section", func(t *testing.T) {
		assert.False(t, w.appConfig().DynamicStatsig)
		w.runtime.Store(&config.Config{App: config.AppConfig{DynamicStatsig: true}})
		assert.True(t, w.appConfig().DynamicStatsig)
		w.runtime.Store(cfg)
	})

	t.Run("imageConfig exposes live image section", func(t *testing.T) {
		ic := w.imageConfig()
		assert.Equal(t, "base64", ic.Format)
		assert.True(t, ic.NSFW)
	})

	t.Run("transportOptions maps proxy fields", func(t *testing.T) {
		opts := w.transportOptions()
		assert.Equal(t, 7*time.Second, opts.RequestTimeout)
		assert.Equal(t, "chrome136", opts.Browser)
		assert.Equal(t, "http://127.0.0.1:1", opts.ProxyURL)
		assert.True(t, opts.SkipProxySSLVerify)
	})

	t.Run("browserProfile and userAgent read runtime", func(t *testing.T) {
		assert.Equal(t, "chrome136", w.browserProfile())
		assert.Equal(t, "UA/1", w.userAgent())
	})

	t.Run("statsigID toggles with dynamic_statsig", func(t *testing.T) {
		assert.Equal(t, upstreamStaticStatsigID, w.statsigID())
		w.runtime.Store(&config.Config{App: config.AppConfig{DynamicStatsig: true}})
		assert.Empty(t, w.statsigID())
		w.runtime.Store(cfg)
	})

	t.Run("grokCookie delegates with cf cookies", func(t *testing.T) {
		assert.Equal(t, grok.BuildCookie("tok", "ck", "clr"), w.grokCookie("tok"))
	})

	t.Run("consoleCookie delegates with cf cookies", func(t *testing.T) {
		assert.Equal(t, console.BuildCookie("tok", "ck", "clr"), w.consoleCookie("tok"))
	})

	t.Run("retryConfig maps retry fields", func(t *testing.T) {
		rc := w.retryConfig()
		require.NotNil(t, rc)
		assert.Equal(t, 3, rc.MaxTokens)
		assert.Equal(t, 2, rc.PerTokenRetries)
		assert.Equal(t, 500*time.Millisecond, rc.BaseDelay)
		assert.Equal(t, 20*time.Second, rc.MaxDelay)
		assert.Equal(t, 0.25, rc.JitterFactor)
		assert.Equal(t, float64(2), rc.BackoffFactor)
		assert.Equal(t, 60*time.Second, rc.RetryBudget)
	})

	t.Run("filterTags returns a defensive copy", func(t *testing.T) {
		tags := w.filterTags()
		require.Equal(t, []string{"t1", "t2"}, tags)
		tags[0] = "mutated"
		assert.Equal(t, []string{"t1", "t2"}, w.filterTags())
	})
}

func TestRuntimeWiring_RegistryProviders(t *testing.T) {
	reg := registry.NewModelRegistry([]modelconfig.ModelSpec{
		{ID: "m1", Type: modelconfig.TypeChat, Enabled: true, UpstreamModel: "um", UpstreamMode: "auto", EnablePro: true},
	}, nil)
	w := &runtimeWiring{runtime: config.NewRuntime(&config.Config{}), reg: reg}

	t.Run("resolveUpstream found", func(t *testing.T) {
		um, mode, ok := w.resolveUpstream("m1")
		assert.True(t, ok)
		assert.Equal(t, "um", um)
		assert.Equal(t, "auto", mode)
	})

	t.Run("resolveUpstream not found", func(t *testing.T) {
		um, mode, ok := w.resolveUpstream("nope")
		assert.False(t, ok)
		assert.Empty(t, um)
		assert.Empty(t, mode)
	})

	t.Run("enablePro found and not found", func(t *testing.T) {
		assert.True(t, w.enablePro("m1"))
		assert.False(t, w.enablePro("nope"))
	})
}

// newTestAPIKeyStore opens a sqlite-backed APIKeyStore in a temp directory
// pre-seeded with one key carrying non-zero usage counters.
func newTestAPIKeyStore(t *testing.T) (*store.APIKeyStore, *store.APIKey) {
	t.Helper()
	cfg := &config.Config{App: config.AppConfig{DBDriver: "sqlite", DBPath: filepath.Join(t.TempDir(), "test.db")}}
	db, err := store.Open(cfg)
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))
	t.Cleanup(func() { _ = store.Close(db) })

	ks := store.NewAPIKeyStore(db)
	ak := &store.APIKey{Name: "wiring-test", Status: "active", DailyUsed: 5, TotalUsed: 7}
	require.NoError(t, ks.Create(context.Background(), ak))
	return ks, ak
}

func TestRuntimeWiring_ClientFactories(t *testing.T) {
	w := &runtimeWiring{runtime: config.NewRuntime(&config.Config{})}

	t.Run("newVideoClient success", func(t *testing.T) {
		client := w.newVideoClient("tok")
		assert.NotNil(t, client)
	})

	t.Run("newImageEditClient success", func(t *testing.T) {
		client := w.newImageEditClient("tok")
		assert.NotNil(t, client)
	})

	t.Run("client factory failure logs and returns nil", func(t *testing.T) {
		buf := restoreLogger(t)
		orig := xaiClientConstructor
		xaiClientConstructor = func(token string, opts ...xai.ClientOption) (xai.Client, error) {
			return nil, errors.New("no client")
		}
		t.Cleanup(func() { xaiClientConstructor = orig })

		assert.Nil(t, w.newVideoClient("tok"))
		assert.Nil(t, w.newImageEditClient("tok"))
		assert.Contains(t, buf.String(), "failed to create xai client")
		assert.Contains(t, buf.String(), "failed to create image edit client")
	})

	t.Run("newImagineGenerator returns client", func(t *testing.T) {
		gen := w.newImagineGenerator("tok")
		assert.NotNil(t, gen)
	})
}

func TestRuntimeWiring_IncAPIKeyUsage(t *testing.T) {
	ks, ak := newTestAPIKeyStore(t)
	w := &runtimeWiring{apiKeys: ks}

	w.incAPIKeyUsage(context.Background(), ak.ID)

	got, err := ks.GetByID(context.Background(), ak.ID)
	require.NoError(t, err)
	assert.Equal(t, 6, got.DailyUsed)
	assert.Equal(t, 8, got.TotalUsed)
}

// --- serveHTTP and the daily reset loop ---

func TestServeHTTP_ListenFailureExits(t *testing.T) {
	// Keep the listener open so ListenAndServe deterministically fails with
	// "address already in use".
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	srv := &http.Server{Addr: ln.Addr().String()}

	exitCodes := make(chan int, 1)
	orig := osExit
	osExit = func(code int) { exitCodes <- code }
	t.Cleanup(func() { osExit = orig })

	done := make(chan struct{})
	go func() {
		defer close(done)
		serveHTTP(srv)
	}()

	select {
	case code := <-exitCodes:
		assert.Equal(t, 1, code)
	case <-time.After(10 * time.Second):
		t.Fatal("serveHTTP did not exit on listen failure")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("serveHTTP did not return after injected exit")
	}
}

func TestResetAPIKeyDailyUsage_Success(t *testing.T) {
	ks, ak := newTestAPIKeyStore(t)
	buf := restoreLogger(t)

	resetAPIKeyDailyUsage(ks)

	got, err := ks.GetByID(context.Background(), ak.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, got.DailyUsed)
	assert.Equal(t, 7, got.TotalUsed)
	assert.Contains(t, buf.String(), "API key daily usage reset complete")
}

func TestResetAPIKeyDailyUsage_Error(t *testing.T) {
	cfg := &config.Config{App: config.AppConfig{DBDriver: "sqlite", DBPath: filepath.Join(t.TempDir(), "test.db")}}
	db, err := store.Open(cfg)
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))
	ks := store.NewAPIKeyStore(db)
	require.NoError(t, store.Close(db))

	buf := restoreLogger(t)
	resetAPIKeyDailyUsage(ks)
	assert.Contains(t, buf.String(), "failed to reset API key daily usage")
}

func TestAPIKeyDailyResetLoop_ResetsAtMidnightAndStops(t *testing.T) {
	ks, ak := newTestAPIKeyStore(t)

	var callCount int
	secondIteration := make(chan struct{})
	var once sync.Once
	now := func() time.Time {
		callCount++
		if callCount == 1 {
			// 10 ms before the next UTC midnight: first timer fires quickly.
			return time.Date(2026, 9, 25, 23, 59, 59, 990_000_000, time.UTC)
		}
		// Second iteration: far from midnight so the timer blocks until ctx is done.
		once.Do(func() { close(secondIteration) })
		return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		apiKeyDailyResetLoop(ctx, ks, now)
	}()

	select {
	case <-secondIteration:
	case <-time.After(10 * time.Second):
		t.Fatal("loop did not reach second iteration")
	}

	got, err := ks.GetByID(context.Background(), ak.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, got.DailyUsed, "daily usage should be reset at midnight")

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("loop did not exit after context cancellation")
	}
}

// --- nsfwEnablerImpl ---

func TestNsfwEnablerImpl_EnableNsfwUpstream(t *testing.T) {
	tests := []struct {
		name    string
		timeout int
	}{
		{name: "uses configured proxy timeout", timeout: 1},
		{name: "falls back to 90s when timeout unset", timeout: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The dead loopback proxy guarantees the upstream call fails
			// without any external network traffic.
			cfg := &config.Config{Proxy: config.ProxyConfig{BaseProxyURL: "http://127.0.0.1:1", Timeout: tc.timeout}}
			s := &nsfwEnablerImpl{runtime: config.NewRuntime(cfg)}

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // already canceled: the upstream call must fail fast

			done := make(chan error, 1)
			go func() { done <- s.EnableNsfwUpstream(ctx, "tok") }()

			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(30 * time.Second):
				t.Fatal("EnableNsfwUpstream did not return")
			}
		})
	}
}

// --- run() end-to-end ---

// e2eConfig writes a minimal config file into dir and returns its path.
// extra is appended verbatim to the [app] section.
func e2eConfig(t *testing.T, dir string, extra string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	content := "[app]\n" +
		"host = \"127.0.0.1\"\n" +
		"port = 0\n" +
		"log_level = \"error\"\n" +
		"log_file_path = \"\"\n" +
		"db_path = \"" + filepath.Join(dir, "grokforge.db") + "\"\n" +
		extra
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// useFakeSignal makes signalNotify deliver SIGTERM as soon as run registers
// its quit channel, so the graceful shutdown path is exercised deterministically.
func useFakeSignal(t *testing.T) {
	t.Helper()
	orig := signalNotify
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) {
		c <- syscall.SIGTERM
	}
	t.Cleanup(func() { signalNotify = orig })
}

// runAsync executes run in a goroutine guarded by a watchdog.
func runAsync(t *testing.T, argv []string, stdout, stderr io.Writer) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- run(argv, stdout, stderr) }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Minute):
		t.Fatal("run did not return before watchdog timeout")
		return nil
	}
}

// seedDBOverrides pre-populates the config KV table used by run's override step.
func seedDBOverrides(t *testing.T, dbPath string, kvs map[string]string) {
	t.Helper()
	cfg := &config.Config{App: config.AppConfig{DBDriver: "sqlite", DBPath: dbPath}}
	db, err := store.Open(cfg)
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))
	require.NoError(t, store.NewConfigStore(db).SetMany(kvs))
	require.NoError(t, store.Close(db))
}

func TestRun_StartupGracefulShutdownAndExternalCatalog(t *testing.T) {
	useFakeSignal(t)
	dir := t.TempDir()

	// External model catalog: a copy of the embedded one, loaded from the
	// config directory instead of the embedded FS.
	modelsData, err := fs.ReadFile(modelconfig.EmbeddedFS, "models.toml")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "models.toml"), modelsData, 0o644))

	// app_key omitted: EnsureAdminAppKey generates a bootstrap key.
	path := e2eConfig(t, dir, "models_file = \"models.toml\"\n")

	require.NoError(t, runAsync(t, []string{"-config", path}, io.Discard, io.Discard))

	_, statErr := os.Stat(filepath.Join(dir, "grokforge.db"))
	assert.NoError(t, statErr, "sqlite database should exist after startup")
}

func TestRun_AppliesDBOverridesWithDefaultAppKey(t *testing.T) {
	useFakeSignal(t)
	dir := t.TempDir()
	seedDBOverrides(t, filepath.Join(dir, "grokforge.db"), map[string]string{
		"app.dynamic_statsig": "false",
	})
	// app_key = "grokforge" exercises the default-key warning branch; the
	// model catalog comes from the embedded FS (no models_file set).
	path := e2eConfig(t, dir, "app_key = \"grokforge\"\n")

	require.NoError(t, runAsync(t, []string{"-config", path}, io.Discard, io.Discard))
}

func TestRun_RejectsInvalidDBOverride(t *testing.T) {
	useFakeSignal(t)
	dir := t.TempDir()
	seedDBOverrides(t, filepath.Join(dir, "grokforge.db"), map[string]string{
		"app.dynamic_statsig": "not-a-bool",
	})
	path := e2eConfig(t, dir, "")

	require.Error(t, runAsync(t, []string{"-config", path}, io.Discard, io.Discard))
}

func TestRun_FailsOnUnreachablePostgres(t *testing.T) {
	dir := t.TempDir()
	path := e2eConfig(t, dir, "db_driver = \"postgres\"\n"+
		"db_dsn = \"postgres://grokforge:grokforge@127.0.0.1:1/grokforge?sslmode=disable\"\n")

	require.Error(t, runAsync(t, []string{"-config", path}, io.Discard, io.Discard))
}

func TestRun_FailsOnBrokenModelCatalog(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.toml"), []byte("not [ valid toml"), 0o644))
	path := e2eConfig(t, dir, "models_file = \"broken.toml\"\n")

	require.Error(t, runAsync(t, []string{"-config", path}, io.Discard, io.Discard))
}

func TestRun_FailsWhenMigrationCollides(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "grokforge.db")

	// A view named like a migrated table makes AutoMigrate fail deterministically.
	cfg := &config.Config{App: config.AppConfig{DBDriver: "sqlite", DBPath: dbPath}}
	db, err := store.Open(cfg)
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE VIEW api_keys AS SELECT 1 AS id").Error)
	require.NoError(t, store.Close(db))

	path := e2eConfig(t, dir, "")
	require.Error(t, runAsync(t, []string{"-config", path}, io.Discard, io.Discard))
}

func TestRun_FailsWhenBootstrapEntropyErrors(t *testing.T) {
	useFakeSignal(t)
	dir := t.TempDir()
	path := e2eConfig(t, dir, "")

	orig := adminKeyEntropy
	adminKeyEntropy = failingEntropyReader{}
	t.Cleanup(func() { adminKeyEntropy = orig })

	require.Error(t, runAsync(t, []string{"-config", path}, io.Discard, io.Discard))
}

type failingEntropyReader struct{}

func (failingEntropyReader) Read([]byte) (int, error) {
	return 0, errors.New("no entropy")
}
