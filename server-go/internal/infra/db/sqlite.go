package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mdbrain.dev/internal/config"

	_ "modernc.org/sqlite"
)

func OpenSQLite(ctx context.Context, cfg *config.Config) (*sql.DB, error) {
	if err := os.MkdirAll(cfg.DataPath, 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	for _, stmt := range []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA foreign_keys = ON;",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

func RunMigrationFile(ctx context.Context, db *sql.DB, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	stmts := splitMigrationStatements(string(content))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, stmt := range stmts {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func RunMigrations(ctx context.Context, db *sql.DB, dir string) error {
	if err := ensureMigrationTable(ctx, db); err != nil {
		return err
	}

	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}
	format, applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	for _, path := range files {
		name := filepath.Base(path)
		id, err := migrationNumber(name)
		if err != nil {
			return err
		}
		if _, ok := applied[id]; ok {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		content, err := os.ReadFile(path)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		for _, stmt := range splitMigrationStatements(string(content)) {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("run migration %s: %w", name, err)
			}
		}
		if err := recordAppliedMigration(ctx, tx, format, id, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func PendingMigrations(ctx context.Context, db *sql.DB, dir string) ([]string, error) {
	if err := ensureMigrationTable(ctx, db); err != nil {
		return nil, err
	}
	files, err := migrationFiles(dir)
	if err != nil {
		return nil, err
	}
	_, applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(files))
	for _, path := range files {
		name := filepath.Base(path)
		id, err := migrationNumber(name)
		if err != nil {
			return nil, err
		}
		if _, ok := applied[id]; !ok {
			out = append(out, name)
		}
	}
	return out, nil
}

func CreateMigrationFiles(dir, name string) (string, string, error) {
	slug := sanitizeMigrationName(name)
	if slug == "" {
		return "", "", errors.New("migration name must contain letters or numbers")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}

	next, err := nextMigrationNumber(dir)
	if err != nil {
		return "", "", err
	}

	upPath := filepath.Join(dir, fmt.Sprintf("%03d-%s.up.sql", next, slug))
	downPath := filepath.Join(dir, fmt.Sprintf("%03d-%s.down.sql", next, slug))

	upContent := "-- Migration up\n"
	downContent := "-- Migration down\n"
	if err := os.WriteFile(upPath, []byte(upContent), 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(downPath, []byte(downContent), 0o644); err != nil {
		_ = os.Remove(upPath)
		return "", "", err
	}
	return upPath, downPath, nil
}

var separatorPattern = regexp.MustCompile(`(?m)^--;;\s*$`)

type migrationTableFormat int

const (
	migrationTableFormatName migrationTableFormat = iota
	migrationTableFormatMigratus
)

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func appliedMigrations(ctx context.Context, db *sql.DB) (migrationTableFormat, map[int]struct{}, error) {
	format, err := migrationTableFormatFor(ctx, db)
	if err != nil {
		return 0, nil, err
	}

	var rows *sql.Rows
	switch format {
	case migrationTableFormatName:
		rows, err = db.QueryContext(ctx, `SELECT name FROM schema_migrations`)
	case migrationTableFormatMigratus:
		rows, err = db.QueryContext(ctx, `SELECT id FROM schema_migrations`)
	default:
		err = fmt.Errorf("unknown migration table format: %d", format)
	}
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	out := map[int]struct{}{}
	for rows.Next() {
		switch format {
		case migrationTableFormatName:
			var name string
			if err := rows.Scan(&name); err != nil {
				return 0, nil, err
			}
			id, err := migrationNumber(name)
			if err != nil {
				return 0, nil, err
			}
			out[id] = struct{}{}
		case migrationTableFormatMigratus:
			var id int
			if err := rows.Scan(&id); err != nil {
				return 0, nil, err
			}
			out[id] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}
	return format, out, nil
}

func migrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func nextMigrationNumber(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	maxNum := 0
	for _, entry := range entries {
		name := entry.Name()
		if len(name) < 3 {
			continue
		}
		n, err := strconv.Atoi(name[:3])
		if err != nil {
			continue
		}
		if n > maxNum {
			maxNum = n
		}
	}
	return maxNum + 1, nil
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

func migrationTableFormatFor(ctx context.Context, db *sql.DB) (migrationTableFormat, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(schema_migrations)`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	hasName := false
	hasID := false
	for rows.Next() {
		var (
			cid        int
			columnName string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return 0, err
		}
		switch columnName {
		case "name":
			hasName = true
		case "id":
			hasID = true
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	switch {
	case hasName:
		return migrationTableFormatName, nil
	case hasID:
		return migrationTableFormatMigratus, nil
	default:
		return 0, errors.New("schema_migrations has unsupported columns")
	}
}

func recordAppliedMigration(ctx context.Context, tx *sql.Tx, format migrationTableFormat, id int, name string) error {
	switch format {
	case migrationTableFormatName:
		_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?)`, name)
		return err
	case migrationTableFormatMigratus:
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO schema_migrations (id, applied, description) VALUES (?, CURRENT_TIMESTAMP, ?)`,
			id,
			migrationDescription(name),
		)
		return err
	default:
		return fmt.Errorf("unknown migration table format: %d", format)
	}
}

func migrationNumber(name string) (int, error) {
	if len(name) < 3 {
		return 0, fmt.Errorf("invalid migration filename %q", name)
	}
	n, err := strconv.Atoi(name[:3])
	if err != nil {
		return 0, fmt.Errorf("invalid migration filename %q: %w", name, err)
	}
	return n, nil
}

func migrationDescription(name string) string {
	name = strings.TrimSuffix(name, ".up.sql")
	if dash := strings.IndexByte(name, '-'); dash >= 0 && dash+1 < len(name) {
		return name[dash+1:]
	}
	return name
}

func splitMigrationStatements(content string) []string {
	parts := separatorPattern.Split(content, -1)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt != "" {
			out = append(out, stmt)
		}
	}
	return out
}

func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return errors.Join(db.Close())
}
