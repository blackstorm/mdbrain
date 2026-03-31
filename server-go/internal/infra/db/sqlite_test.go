package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	atlasmigrate "ariga.io/atlas/sql/migrate"

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

func TestRunMigrationsAndPendingMigrations(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	db := openManagedTestSQLite(t)

	pending, err := PendingMigrations(ctx, db, cfg.MigrationDir)
	if err != nil {
		t.Fatalf("pending migrations before run: %v", err)
	}
	if len(pending) == 0 {
		t.Fatal("expected fresh database to have pending atlas migrations")
	}

	if err := RunMigrations(ctx, db, cfg.MigrationDir); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pending, err = PendingMigrations(ctx, db, cfg.MigrationDir)
	if err != nil {
		t.Fatalf("pending migrations after run: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending migrations after run, got %v", pending)
	}

	revisions, err := (&sqliteRevisionStore{db: db}).ReadRevisions(ctx)
	if err != nil {
		t.Fatalf("read atlas revisions: %v", err)
	}
	if len(revisions) == 0 {
		t.Fatal("expected atlas revisions to be recorded")
	}
}

func TestRunMigrationsBaselinesLegacySchemaMigrations(t *testing.T) {
	ctx := context.Background()
	db := openRawTestSQLite(t)
	migrationDir := filepath.Join(t.TempDir(), "migrations")

	writeAtlasMigrationDir(t, migrationDir, map[string]string{
		"20260331090000_initial.sql": `
CREATE TABLE base (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL
);
`,
		"20260331100000_add_flag.sql": `
ALTER TABLE base ADD COLUMN flag INTEGER NOT NULL DEFAULT 0;
`,
	})

	if _, err := db.ExecContext(ctx, `
CREATE TABLE base (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL
);
`); err != nil {
		t.Fatalf("create legacy base table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
CREATE TABLE schema_migrations (
  id BIGINT UNIQUE NOT NULL,
  applied TIMESTAMP,
  description VARCHAR(1024)
);
`); err != nil {
		t.Fatalf("create legacy schema_migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO schema_migrations (id, applied, description)
VALUES (1, CURRENT_TIMESTAMP, 'initial');
`); err != nil {
		t.Fatalf("seed legacy schema_migrations: %v", err)
	}

	pending, err := PendingMigrations(ctx, db, migrationDir)
	if err != nil {
		t.Fatalf("pending migrations with legacy baseline: %v", err)
	}
	if len(pending) != 1 || pending[0] != "20260331100000_add_flag.sql" {
		t.Fatalf("unexpected pending migrations after legacy baseline: %v", pending)
	}

	if err := RunMigrations(ctx, db, migrationDir); err != nil {
		t.Fatalf("run migrations with legacy baseline: %v", err)
	}

	var hasFlag int
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(base)`)
	if err != nil {
		t.Fatalf("inspect base columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultVal, &pk); err != nil {
			t.Fatalf("scan column info: %v", err)
		}
		if name == "flag" {
			hasFlag++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate base columns: %v", err)
	}
	if hasFlag != 1 {
		t.Fatalf("expected migrated flag column, got %d matches", hasFlag)
	}

	revisions, err := (&sqliteRevisionStore{db: db}).ReadRevisions(ctx)
	if err != nil {
		t.Fatalf("read atlas revisions after baseline: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("unexpected atlas revision count after baseline: %d", len(revisions))
	}
	if revisions[0].Type != atlasmigrate.RevisionTypeBaseline {
		t.Fatalf("expected first atlas revision to be a baseline, got %v", revisions[0].Type)
	}
}

func TestCreateMigrationFromEntSchema(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	cfg.MigrationDir = filepath.Join(t.TempDir(), "migrations")

	created, err := CreateMigration(ctx, cfg, " Initial Schema ")
	if err != nil {
		t.Fatalf("create initial migration: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("unexpected created entries: %v", created)
	}

	var (
		hasSQL bool
		hasSum bool
	)
	for _, name := range created {
		switch {
		case strings.HasSuffix(name, ".sql"):
			hasSQL = true
		case name == atlasmigrate.HashFileName:
			hasSum = true
		}
	}
	if !hasSQL || !hasSum {
		t.Fatalf("expected migration sql file and atlas.sum, got %v", created)
	}

	created, err = CreateMigration(ctx, cfg, "No Changes")
	if err != nil {
		t.Fatalf("create no-op migration: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("expected no files when schema is unchanged, got %v", created)
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

func openRawTestSQLite(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	dsn, err := sqliteDSN(dbPath)
	if err != nil {
		t.Fatalf("build sqlite dsn: %v", err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	db.SetMaxOpenConns(sqliteMaxOpenConns)
	db.SetMaxIdleConns(sqliteMaxOpenConns)
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("ping raw sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func writeAtlasMigrationDir(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create migration dir: %v", err)
	}
	localDir, err := atlasmigrate.NewLocalDir(dir)
	if err != nil {
		t.Fatalf("open atlas migration dir: %v", err)
	}

	for name, content := range files {
		if err := localDir.WriteFile(name, []byte(strings.TrimSpace(content)+"\n")); err != nil {
			t.Fatalf("write migration file %s: %v", name, err)
		}
	}

	sum, err := localDir.Checksum()
	if err != nil {
		t.Fatalf("compute atlas checksum: %v", err)
	}
	if err := atlasmigrate.WriteSumFile(localDir, sum); err != nil {
		t.Fatalf("write atlas.sum: %v", err)
	}
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()

	t.Setenv("DATA_PATH", filepath.Join(t.TempDir(), "data"))
	cfg, err := config.Load(context.Background(), repoRoot())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
