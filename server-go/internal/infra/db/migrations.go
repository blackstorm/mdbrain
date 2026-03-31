package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	atlasmigrate "ariga.io/atlas/sql/migrate"
	atlassqlite "ariga.io/atlas/sql/sqlite"
	entdialect "entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	entschema "entgo.io/ent/dialect/sql/schema"

	entmigrate "mdbrain.dev/ent/migrate"
	"mdbrain.dev/internal/config"
)

const atlasRevisionTable = "atlas_schema_revisions"

func RunMigrations(ctx context.Context, db *sql.DB, dir string) error {
	executor, err := newMigrationExecutor(ctx, db, dir)
	if err != nil {
		return err
	}
	if err := executor.ExecuteN(ctx, 0); err != nil && !errors.Is(err, atlasmigrate.ErrNoPendingFiles) {
		return err
	}
	return nil
}

func PendingMigrations(ctx context.Context, db *sql.DB, dir string) ([]string, error) {
	executor, err := newMigrationExecutor(ctx, db, dir)
	if err != nil {
		return nil, err
	}

	pending, err := executor.Pending(ctx)
	if errors.Is(err, atlasmigrate.ErrNoPendingFiles) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	out := make([]string, len(pending))
	for i, file := range pending {
		out[i] = file.Name()
	}
	return out, nil
}

func CreateMigration(ctx context.Context, cfg *config.Config, name string) ([]string, error) {
	slug := sanitizeMigrationName(name)
	if slug == "" {
		return nil, errors.New("migration name must contain letters or numbers")
	}

	dir, err := openMigrationDir(cfg.MigrationDir)
	if err != nil {
		return nil, err
	}

	before, err := migrationDirEntries(cfg.MigrationDir)
	if err != nil {
		return nil, err
	}

	devDB, cleanup, err := openTempSQLite(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	migrator, err := entschema.NewMigrate(
		entsql.OpenDB(entdialect.SQLite, devDB),
		entschema.WithDir(dir),
		entschema.WithMigrationMode(entschema.ModeReplay),
		entschema.WithDialect(entdialect.SQLite),
		entschema.WithFormatter(atlasmigrate.DefaultFormatter),
	)
	if err != nil {
		return nil, err
	}

	tables, err := entschema.CopyTables(entmigrate.Tables)
	if err != nil {
		return nil, err
	}
	if err := migrator.NamedDiff(ctx, slug, tables...); err != nil {
		return nil, err
	}

	after, err := migrationDirEntries(cfg.MigrationDir)
	if err != nil {
		return nil, err
	}

	return diffMigrationEntries(before, after), nil
}

func newMigrationExecutor(ctx context.Context, db *sql.DB, dir string) (*atlasmigrate.Executor, error) {
	migrationDir, err := openMigrationDir(dir)
	if err != nil {
		return nil, err
	}

	revisions := &sqliteRevisionStore{db: db}
	if err := ensureLegacyBaseline(ctx, revisions, migrationDir); err != nil {
		return nil, err
	}

	drv, err := atlassqlite.Open(db)
	if err != nil {
		return nil, err
	}
	return atlasmigrate.NewExecutor(drv, migrationDir, revisions)
}

func openMigrationDir(path string) (*atlasmigrate.LocalDir, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}
	return atlasmigrate.NewLocalDir(path)
}

func openTempSQLite(ctx context.Context) (*sql.DB, func(), error) {
	tempDir, err := os.MkdirTemp("", "mdbrain-atlas-dev-*")
	if err != nil {
		return nil, nil, err
	}

	dbPath := filepath.Join(tempDir, "dev.db")
	dsn, err := sqliteDSN(dbPath)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, nil, err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, nil, err
	}
	db.SetMaxOpenConns(sqliteMaxOpenConns)
	db.SetMaxIdleConns(sqliteMaxOpenConns)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		_ = os.RemoveAll(tempDir)
		return nil, nil, err
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.RemoveAll(tempDir)
	}
	return db, cleanup, nil
}

func migrationDirEntries(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func diffMigrationEntries(before, after []string) []string {
	known := make(map[string]struct{}, len(before))
	for _, name := range before {
		known[name] = struct{}{}
	}

	created := make([]string, 0, len(after))
	for _, name := range after {
		if _, ok := known[name]; ok {
			continue
		}
		created = append(created, name)
	}
	return created
}

func sanitizeMigrationName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))

	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func ensureLegacyBaseline(ctx context.Context, revisions *sqliteRevisionStore, dir *atlasmigrate.LocalDir) error {
	current, err := revisions.ReadRevisions(ctx)
	if err != nil {
		return err
	}
	if len(current) > 0 {
		return nil
	}

	applied, err := revisions.hasLegacySchemaMigrations(ctx)
	if err != nil || !applied {
		return err
	}

	files, err := dir.Files()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	first := files[0]
	return revisions.WriteRevision(ctx, &atlasmigrate.Revision{
		Version:     first.Version(),
		Description: first.Desc(),
		Type:        atlasmigrate.RevisionTypeBaseline,
	})
}

type sqliteRevisionStore struct {
	db *sql.DB
}

func (s *sqliteRevisionStore) Ident() *atlasmigrate.TableIdent {
	return &atlasmigrate.TableIdent{Name: atlasRevisionTable}
}

func (s *sqliteRevisionStore) ReadRevisions(ctx context.Context) ([]*atlasmigrate.Revision, error) {
	exists, err := tableExists(ctx, s.db, atlasRevisionTable)
	if err != nil || !exists {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			version,
			description,
			type,
			applied,
			total,
			executed_at_unix_nano,
			execution_time_nanos,
			error,
			error_stmt,
			hash,
			partial_hashes_json,
			operator_version
		FROM atlas_schema_revisions
		ORDER BY version
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var revisions []*atlasmigrate.Revision
	for rows.Next() {
		revision, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return revisions, nil
}

func (s *sqliteRevisionStore) ReadRevision(ctx context.Context, version string) (*atlasmigrate.Revision, error) {
	exists, err := tableExists(ctx, s.db, atlasRevisionTable)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, atlasmigrate.ErrRevisionNotExist
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT
			version,
			description,
			type,
			applied,
			total,
			executed_at_unix_nano,
			execution_time_nanos,
			error,
			error_stmt,
			hash,
			partial_hashes_json,
			operator_version
		FROM atlas_schema_revisions
		WHERE version = ?
	`, version)

	revision, err := scanRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, atlasmigrate.ErrRevisionNotExist
	}
	return revision, err
}

func (s *sqliteRevisionStore) WriteRevision(ctx context.Context, revision *atlasmigrate.Revision) error {
	if err := s.ensureTable(ctx); err != nil {
		return err
	}

	partialHashes, err := json.Marshal(revision.PartialHashes)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO atlas_schema_revisions (
			version,
			description,
			type,
			applied,
			total,
			executed_at_unix_nano,
			execution_time_nanos,
			error,
			error_stmt,
			hash,
			partial_hashes_json,
			operator_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(version) DO UPDATE SET
			description = excluded.description,
			type = excluded.type,
			applied = excluded.applied,
			total = excluded.total,
			executed_at_unix_nano = excluded.executed_at_unix_nano,
			execution_time_nanos = excluded.execution_time_nanos,
			error = excluded.error,
			error_stmt = excluded.error_stmt,
			hash = excluded.hash,
			partial_hashes_json = excluded.partial_hashes_json,
			operator_version = excluded.operator_version
	`,
		revision.Version,
		revision.Description,
		int(revision.Type),
		revision.Applied,
		revision.Total,
		revision.ExecutedAt.UnixNano(),
		revision.ExecutionTime.Nanoseconds(),
		revision.Error,
		revision.ErrorStmt,
		revision.Hash,
		string(partialHashes),
		revision.OperatorVersion,
	)
	return err
}

func (s *sqliteRevisionStore) DeleteRevision(ctx context.Context, version string) error {
	exists, err := tableExists(ctx, s.db, atlasRevisionTable)
	if err != nil || !exists {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM atlas_schema_revisions WHERE version = ?`, version)
	return err
}

func (s *sqliteRevisionStore) hasLegacySchemaMigrations(ctx context.Context) (bool, error) {
	exists, err := tableExists(ctx, s.db, "schema_migrations")
	if err != nil || !exists {
		return false, err
	}

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *sqliteRevisionStore) ensureTable(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS atlas_schema_revisions (
			version TEXT PRIMARY KEY,
			description TEXT NOT NULL,
			type INTEGER NOT NULL,
			applied INTEGER NOT NULL,
			total INTEGER NOT NULL,
			executed_at_unix_nano INTEGER NOT NULL,
			execution_time_nanos INTEGER NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			error_stmt TEXT NOT NULL DEFAULT '',
			hash TEXT NOT NULL DEFAULT '',
			partial_hashes_json TEXT NOT NULL DEFAULT '[]',
			operator_version TEXT NOT NULL DEFAULT ''
		)
	`)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRevision(scanner rowScanner) (*atlasmigrate.Revision, error) {
	var (
		revision           atlasmigrate.Revision
		revisionType       int
		executedAtUnix     int64
		executionTimeNanos int64
		partialHashesJSON  string
	)

	if err := scanner.Scan(
		&revision.Version,
		&revision.Description,
		&revisionType,
		&revision.Applied,
		&revision.Total,
		&executedAtUnix,
		&executionTimeNanos,
		&revision.Error,
		&revision.ErrorStmt,
		&revision.Hash,
		&partialHashesJSON,
		&revision.OperatorVersion,
	); err != nil {
		return nil, err
	}

	revision.Type = atlasmigrate.RevisionType(revisionType)
	if executedAtUnix != 0 {
		revision.ExecutedAt = time.Unix(0, executedAtUnix).UTC()
	}
	revision.ExecutionTime = time.Duration(executionTimeNanos)
	if partialHashesJSON != "" {
		if err := json.Unmarshal([]byte(partialHashesJSON), &revision.PartialHashes); err != nil {
			return nil, fmt.Errorf("decode partial hashes: %w", err)
		}
	}
	return &revision, nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = ?
	`, name).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
