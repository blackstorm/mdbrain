package db

import (
	"context"
	"database/sql"

	entdialect "entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	entschema "entgo.io/ent/dialect/sql/schema"

	entmigrate "mdbrain.dev/ent/migrate"
)

func EnsureSchema(ctx context.Context, db *sql.DB) error {
	tables, err := entschema.CopyTables(entmigrate.Tables)
	if err != nil {
		return err
	}
	driver := entsql.OpenDB(entdialect.SQLite, db)
	return entmigrate.Create(ctx, entmigrate.NewSchema(driver), tables)
}
