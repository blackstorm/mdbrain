package db

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"mdbrain.dev/internal/config"

	_ "modernc.org/sqlite"
)

const (
	sqliteBusyTimeoutMillis = 5000
	sqliteMaxOpenConns      = 1
)

func OpenSQLite(ctx context.Context, cfg *config.Config) (*sql.DB, error) {
	if err := os.MkdirAll(cfg.DataPath, 0o755); err != nil {
		return nil, err
	}

	dsn, err := sqliteDSN(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(sqliteMaxOpenConns)
	db.SetMaxIdleConns(sqliteMaxOpenConns)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func sqliteDSN(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	u := &url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(absPath),
	}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout("+strconv.Itoa(sqliteBusyTimeoutMillis)+")")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return errors.Join(db.Close())
}
