package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"mdbrain.dev/internal/app"
	"mdbrain.dev/internal/config"
	dbinfra "mdbrain.dev/internal/infra/db"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: mdbrain-migrate <migrate|pending|create NAME>")
	}

	projectRoot, err := app.DetectProjectRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return err
	}

	switch args[0] {
	case "migrate":
		return withDB(ctx, cfg, func(db *sql.DB) error {
			return dbinfra.RunMigrations(ctx, db, cfg.MigrationDir())
		})
	case "pending":
		return withDB(ctx, cfg, func(db *sql.DB) error {
			pending, err := dbinfra.PendingMigrations(ctx, db, cfg.MigrationDir())
			if err != nil {
				return err
			}
			if len(pending) == 0 {
				fmt.Println("No pending migrations.")
				return nil
			}
			for _, name := range pending {
				fmt.Println(name)
			}
			return nil
		})
	case "create":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return errors.New("usage: mdbrain-migrate create NAME")
		}
		files, err := dbinfra.CreateMigration(ctx, cfg, args[1])
		if err != nil {
			return err
		}
		if len(files) == 0 {
			fmt.Println("No schema changes.")
			return nil
		}
		for _, name := range files {
			fmt.Println(name)
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func withDB(ctx context.Context, cfg *config.Config, fn func(db *sql.DB) error) error {
	db, err := dbinfra.OpenSQLite(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = dbinfra.Close(db) }()
	return fn(db)
}
