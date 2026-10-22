package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// errInjected is the sentinel error injected into gorm queries by failOnNthQuery.
var errInjected = errors.New("injected query failure")

// closedDB returns a migrated in-memory sqlite database whose underlying
// connection has been closed, so every subsequent query fails.
func closedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

// failOnNthQuery registers a gorm callback that injects errInjected into the
// nth execution of the "query" processor (1-based). gorm routes Count, Find
// and Pluck through it, so this is a deterministic seam for covering
// individual error branches of multi-query store methods.
func failOnNthQuery(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	failOnNthProcessor(t, db, "query", n)
}

// failOnNthRowScan does the same as failOnNthQuery for the "row" processor,
// which gorm uses internally for Scan/Rows calls.
func failOnNthRowScan(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	failOnNthProcessor(t, db, "row", n)
}

// failOnNthProcessor injects errInjected into the nth execution of the given
// gorm processor ("query", "row", "create" or "update"). This makes it
// possible to fail a specific statement of a multi-query method — including
// statements running inside a transaction — without touching production code.
func failOnNthProcessor(t *testing.T, db *gorm.DB, processor string, n int) {
	t.Helper()
	calls := 0
	inject := func(tx *gorm.DB) {
		calls++
		if calls == n {
			tx.AddError(errInjected)
		}
	}
	var err error
	switch processor {
	case "query":
		err = db.Callback().Query().Before("gorm:query").Register("store_extra_fail_on_nth", inject)
	case "row":
		err = db.Callback().Row().Before("gorm:row").Register("store_extra_fail_on_nth", inject)
	case "create":
		err = db.Callback().Create().Before("gorm:create").Register("store_extra_fail_on_nth", inject)
	case "update":
		err = db.Callback().Update().Before("gorm:update").Register("store_extra_fail_on_nth", inject)
	default:
		t.Fatalf("unsupported processor %q", processor)
	}
	require.NoError(t, err)
}

func TestOpenSQLite_FileDB_AppliesPoolLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "grokforge.db")
	db, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = Close(db) })

	require.NoError(t, AutoMigrate(db))
	entry := &ConfigEntry{Key: "probe", Value: "v"}
	require.NoError(t, db.Create(entry).Error)

	var found ConfigEntry
	require.NoError(t, db.First(&found, entry.ID).Error)
	assert.Equal(t, "v", found.Value)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	stats := sqlDB.Stats()
	assert.Equal(t, 1, stats.MaxOpenConnections)
}

func TestOpenSQLite_MkdirAllError(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("file"), 0o644))

	db, err := OpenSQLite(filepath.Join(blocker, "nested", "db.sqlite"))
	assert.Error(t, err)
	assert.Nil(t, db)
}

func TestOpenSQLite_OpenError(t *testing.T) {
	// A directory cannot be opened as the database file.
	dir := filepath.Join(t.TempDir(), "adir")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	db, err := OpenSQLite(dir)
	assert.Error(t, err)
	assert.Nil(t, db)
}

// --- fake postgres server ---

// fakePostgres serves just enough of the PostgreSQL wire protocol for pgx to
// establish a connection and answer the "-- ping" query that gorm.Open issues,
// so OpenPostgres' success path is testable without a real server.
type fakePostgres struct {
	listener net.Listener
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	done     chan struct{}
}

var (
	pgAuthOKAndReady = []byte{
		'R', 0, 0, 0, 8, 0, 0, 0, 0, // AuthenticationOk
		'Z', 0, 0, 0, 5, 'I', // ReadyForQuery (idle)
	}
	pgCommandCompleteAndReady = []byte{
		'C', 0, 0, 0, 13, 'S', 'E', 'L', 'E', 'C', 'T', ' ', '1', 0, // CommandComplete "SELECT 1"
		'Z', 0, 0, 0, 5, 'I', // ReadyForQuery
	}
)

func newFakePostgres(t *testing.T) *fakePostgres {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	fp := &fakePostgres{
		listener: ln,
		conns:    make(map[net.Conn]struct{}),
		done:     make(chan struct{}),
	}
	go fp.serve()
	t.Cleanup(func() {
		fp.close()
		<-fp.done
	})
	return fp
}

func (fp *fakePostgres) port() int {
	return fp.listener.Addr().(*net.TCPAddr).Port
}

func (fp *fakePostgres) close() {
	_ = fp.listener.Close()
	fp.mu.Lock()
	for conn := range fp.conns {
		_ = conn.Close()
	}
	fp.mu.Unlock()
}

func (fp *fakePostgres) track(conn net.Conn) {
	fp.mu.Lock()
	fp.conns[conn] = struct{}{}
	fp.mu.Unlock()
}

func (fp *fakePostgres) untrack(conn net.Conn) {
	fp.mu.Lock()
	delete(fp.conns, conn)
	fp.mu.Unlock()
}

func (fp *fakePostgres) serve() {
	defer close(fp.done)
	for {
		conn, err := fp.listener.Accept()
		if err != nil {
			return
		}
		fp.serveConn(conn)
	}
}

func (fp *fakePostgres) serveConn(conn net.Conn) {
	fp.track(conn)
	defer fp.untrack(conn)
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	// StartupMessage: int32 length (including itself) + protocol + parameters.
	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return
	}
	startupLen := int64(binary.BigEndian.Uint32(head[:]))
	if startupLen < 8 || startupLen > 8192 {
		return
	}
	if _, err := io.CopyN(io.Discard, conn, startupLen-4); err != nil {
		return
	}
	if _, err := conn.Write(pgAuthOKAndReady); err != nil {
		return
	}

	// Answer simple queries (gorm.Open sends "-- ping") until the client goes away.
	var hdr [5]byte
	for {
		if _, err := io.ReadFull(conn, hdr[:]); err != nil {
			return
		}
		bodyLen := int64(binary.BigEndian.Uint32(hdr[1:]))
		if bodyLen < 4 || bodyLen > 1<<24 {
			return
		}
		if _, err := io.CopyN(io.Discard, conn, bodyLen-4); err != nil {
			return
		}
		switch hdr[0] {
		case 'X': // Terminate
			return
		case 'Q': // Simple query
			if _, err := conn.Write(pgCommandCompleteAndReady); err != nil {
				return
			}
		}
	}
}

func fakePostgresDSN(port int) string {
	return fmt.Sprintf("host=127.0.0.1 port=%d user=test password=test dbname=test sslmode=disable", port)
}

func TestOpenPostgres_ConnectsToFakeServer(t *testing.T) {
	fp := newFakePostgres(t)

	db, err := OpenPostgres(fakePostgresDSN(fp.port()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = Close(db) })

	sqlDB, err := db.DB()
	require.NoError(t, err)
	assert.Equal(t, 25, sqlDB.Stats().MaxOpenConnections)
}

func TestOpenPostgres_ConnectError(t *testing.T) {
	// Nothing listens on port 1; gorm.Open pings on open and fails fast.
	db, err := OpenPostgres("host=127.0.0.1 port=1 user=test password=test dbname=test sslmode=disable")
	assert.Error(t, err)
	assert.Nil(t, db)
}

func TestOpen_SQLiteBranches(t *testing.T) {
	tests := []struct {
		name   string
		driver string
	}{
		{name: "explicit sqlite", driver: "sqlite"},
		{name: "unknown driver falls back to sqlite", driver: "oracle"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.App.DBDriver = tc.driver
			cfg.App.DBPath = filepath.Join(t.TempDir(), "db.sqlite")

			db, err := Open(cfg)
			require.NoError(t, err)
			require.NoError(t, AutoMigrate(db))
			assert.NoError(t, Close(db))
		})
	}
}

func TestOpen_PostgresBranch(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "connects"},
		{name: "connection refused fails", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.App.DBDriver = "postgres"
			if tc.wantErr {
				cfg.App.DBDSN = "host=127.0.0.1 port=1 user=test password=test dbname=test sslmode=disable"
			} else {
				cfg.App.DBDSN = fakePostgresDSN(newFakePostgres(t).port())
			}

			db, err := Open(cfg)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, db)
				return
			}
			require.NoError(t, err)
			assert.NoError(t, Close(db))
		})
	}
}

func TestClose_ErrorOnInvalidDB(t *testing.T) {
	// A gorm.DB with a config but no connection pool cannot expose a *sql.DB.
	assert.Error(t, Close(&gorm.DB{Config: &gorm.Config{}}))
}
