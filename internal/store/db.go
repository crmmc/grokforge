// Package store provides database initialization and models.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenSQLite opens a SQLite database with WAL mode enabled.
func OpenSQLite(path string) (*gorm.DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// DSN with WAL mode and busy timeout
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_journal_size_limit=67108864", path)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}

	// SQLite should use single connection to avoid locking issues
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// OpenPostgres opens a PostgreSQL database.
func OpenPostgres(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// Open opens a database based on configuration.
func Open(cfg *config.Config) (*gorm.DB, error) {
	switch cfg.App.DBDriver {
	case "postgres":
		return OpenPostgres(cfg.App.DBDSN)
	case "sqlite":
		fallthrough
	default:
		return OpenSQLite(cfg.App.DBPath)
	}
}

// Close closes the database connection.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
