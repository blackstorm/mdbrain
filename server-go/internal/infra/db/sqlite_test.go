package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"mdbrain.dev/internal/config"
)

func TestOpenSQLiteSerializesWritersAndConfiguresBusyTimeout(t *testing.T) {
	ctx := context.Background()
	db := openManagedTestSQLite(t)

	if _, err := db.ExecContext(ctx, `CREATE TABLE writer_lock (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create writer_lock: %v", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve connection: %v", err)
	}
	defer conn.Close()

	var busyTimeout int
	if err := conn.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout pragma: %v", err)
	}
	if busyTimeout != sqliteBusyTimeoutMillis {
		t.Fatalf("unexpected busy_timeout: got %d want %d", busyTimeout, sqliteBusyTimeoutMillis)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO writer_lock (id) VALUES (1)`); err != nil {
		t.Fatalf("insert row in locked tx: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := db.ExecContext(ctx, `INSERT INTO writer_lock (id) VALUES (2)`)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("concurrent writer returned before lock release: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close reserved connection: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("concurrent writer failed after lock release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent writer timed out after lock release")
	}
}

func TestOpenSQLiteEnablesForeignKeysOnEveryNewConnection(t *testing.T) {
	ctx := context.Background()
	db := openManagedTestSQLite(t)
	db.SetMaxIdleConns(0)

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE parent (
			id INTEGER PRIMARY KEY
		)
	`); err != nil {
		t.Fatalf("create parent table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE child (
			id INTEGER PRIMARY KEY,
			parent_id INTEGER NOT NULL REFERENCES parent(id) ON DELETE CASCADE
		)
	`); err != nil {
		t.Fatalf("create child table: %v", err)
	}

	conn1, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open first connection: %v", err)
	}
	var foreignKeys int
	if err := conn1.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys pragma on first connection: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("unexpected foreign_keys pragma on first connection: %d", foreignKeys)
	}
	if _, err := conn1.ExecContext(ctx, `INSERT INTO parent (id) VALUES (1)`); err != nil {
		t.Fatalf("insert parent: %v", err)
	}
	if _, err := conn1.ExecContext(ctx, `INSERT INTO child (id, parent_id) VALUES (10, 1)`); err != nil {
		t.Fatalf("insert child: %v", err)
	}
	if err := conn1.Close(); err != nil {
		t.Fatalf("close first connection: %v", err)
	}

	conn2, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open second connection: %v", err)
	}
	if err := conn2.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys pragma on second connection: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("unexpected foreign_keys pragma on second connection: %d", foreignKeys)
	}
	if _, err := conn2.ExecContext(ctx, `DELETE FROM parent WHERE id = 1`); err != nil {
		t.Fatalf("delete parent on second connection: %v", err)
	}
	if err := conn2.Close(); err != nil {
		t.Fatalf("close second connection: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM child`).Scan(&count); err != nil {
		t.Fatalf("count child rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected cascade delete to remove child row, got %d rows", count)
	}
}

func TestOpenSQLiteEnsuresApplicationSchema(t *testing.T) {
	ctx := context.Background()
	db := openManagedTestSQLite(t)

	for _, table := range []string{
		"tenants",
		"users",
		"vaults",
		"notes",
		"assets",
		"note_links",
		"note_asset_refs",
	} {
		var count int
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM sqlite_master
			WHERE type = 'table' AND name = ?
		`, table).Scan(&count); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s to exist once, got %d", table, count)
		}
	}
}

func TestEnsureSchemaIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openManagedTestSQLite(t)

	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES ('tenant-1', 'Test Org')`); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}

	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("ensure schema first rerun: %v", err)
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("ensure schema second rerun: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants`).Scan(&count); err != nil {
		t.Fatalf("count tenants after ensure schema reruns: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected tenant row to survive ensure schema reruns, got %d", count)
	}
}

func openManagedTestSQLite(t *testing.T) *sql.DB {
	t.Helper()

	cfg := testConfig(t)
	db, err := OpenSQLite(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()

	t.Setenv("DATA_PATH", filepath.Join(t.TempDir(), "data"))
	cfg, err := config.Load(repoRoot())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
