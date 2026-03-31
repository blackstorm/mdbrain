package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestRunMigrationsAndPendingMigrations(t *testing.T) {
	ctx := context.Background()
	db := openTestSQLite(t)
	migrationDir := filepath.Join(t.TempDir(), "migrations")

	writeMigrationFile(t, filepath.Join(migrationDir, "001-create-notes.up.sql"), `
CREATE TABLE IF NOT EXISTS notes (
  id TEXT PRIMARY KEY
);
--;;
CREATE INDEX IF NOT EXISTS idx_notes_id ON notes(id);
`)
	writeMigrationFile(t, filepath.Join(migrationDir, "002-create-assets.up.sql"), `
CREATE TABLE IF NOT EXISTS assets (
  id TEXT PRIMARY KEY
);
`)

	pending, err := PendingMigrations(ctx, db, migrationDir)
	if err != nil {
		t.Fatalf("pending migrations before run: %v", err)
	}
	wantPending := []string{
		"001-create-notes.up.sql",
		"002-create-assets.up.sql",
	}
	if !reflect.DeepEqual(pending, wantPending) {
		t.Fatalf("unexpected pending migrations before run: got %v want %v", pending, wantPending)
	}

	if err := RunMigrations(ctx, db, migrationDir); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pending, err = PendingMigrations(ctx, db, migrationDir)
	if err != nil {
		t.Fatalf("pending migrations after run: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending migrations after run, got %v", pending)
	}

	var appliedCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&appliedCount); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if appliedCount != 2 {
		t.Fatalf("unexpected applied migration count: %d", appliedCount)
	}

	if err := RunMigrations(ctx, db, migrationDir); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&appliedCount); err != nil {
		t.Fatalf("count applied migrations after rerun: %v", err)
	}
	if appliedCount != 2 {
		t.Fatalf("unexpected applied migration count after rerun: %d", appliedCount)
	}
}

func TestRunMigrationsSupportsMigratusSchemaTable(t *testing.T) {
	ctx := context.Background()
	db := openTestSQLite(t)
	migrationDir := filepath.Join(t.TempDir(), "migrations")

	writeMigrationFile(t, filepath.Join(migrationDir, "001-create-notes.up.sql"), `
CREATE TABLE IF NOT EXISTS notes (
  id TEXT PRIMARY KEY
);
`)
	writeMigrationFile(t, filepath.Join(migrationDir, "002-create-assets.up.sql"), `
CREATE TABLE IF NOT EXISTS assets (
  id TEXT PRIMARY KEY
);
`)

	if _, err := db.ExecContext(ctx, `
CREATE TABLE schema_migrations (
  id BIGINT UNIQUE NOT NULL,
  applied TIMESTAMP,
  description VARCHAR(1024)
);
`); err != nil {
		t.Fatalf("create migratus schema_migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO schema_migrations (id, applied, description)
VALUES (1, CURRENT_TIMESTAMP, 'create-notes');
`); err != nil {
		t.Fatalf("seed migratus schema_migrations: %v", err)
	}

	pending, err := PendingMigrations(ctx, db, migrationDir)
	if err != nil {
		t.Fatalf("pending migrations with migratus table: %v", err)
	}
	wantPending := []string{"002-create-assets.up.sql"}
	if !reflect.DeepEqual(pending, wantPending) {
		t.Fatalf("unexpected pending migrations with migratus table: got %v want %v", pending, wantPending)
	}

	if err := RunMigrations(ctx, db, migrationDir); err != nil {
		t.Fatalf("run migrations with migratus table: %v", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT id, description FROM schema_migrations ORDER BY id`)
	if err != nil {
		t.Fatalf("query migratus schema_migrations: %v", err)
	}
	defer rows.Close()

	var got [][2]string
	for rows.Next() {
		var id int
		var description string
		if err := rows.Scan(&id, &description); err != nil {
			t.Fatalf("scan migratus schema_migrations row: %v", err)
		}
		got = append(got, [2]string{strconv.Itoa(id), description})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migratus schema_migrations rows: %v", err)
	}

	want := [][2]string{
		{"1", "create-notes"},
		{"2", "create-assets"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected migratus schema_migrations rows: got %v want %v", got, want)
	}
}

func TestCreateMigrationFiles(t *testing.T) {
	migrationDir := t.TempDir()

	upPath, downPath, err := CreateMigrationFiles(migrationDir, " Add User Profiles ")
	if err != nil {
		t.Fatalf("create first migration pair: %v", err)
	}
	if filepath.Base(upPath) != "001-add-user-profiles.up.sql" {
		t.Fatalf("unexpected first up migration name: %s", filepath.Base(upPath))
	}
	if filepath.Base(downPath) != "001-add-user-profiles.down.sql" {
		t.Fatalf("unexpected first down migration name: %s", filepath.Base(downPath))
	}

	upPath, downPath, err = CreateMigrationFiles(migrationDir, "Create__Index")
	if err != nil {
		t.Fatalf("create second migration pair: %v", err)
	}
	if filepath.Base(upPath) != "002-create-index.up.sql" {
		t.Fatalf("unexpected second up migration name: %s", filepath.Base(upPath))
	}
	if filepath.Base(downPath) != "002-create-index.down.sql" {
		t.Fatalf("unexpected second down migration name: %s", filepath.Base(downPath))
	}
}

func openTestSQLite(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func writeMigrationFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create migration dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write migration file %s: %v", path, err)
	}
}
