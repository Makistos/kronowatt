package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"kronowatt/backend/migrations"
)

// runMigrate handles `kronowatt-server migrate [up|down|status|...]`.
// DSN comes from KRONOWATT_DB_DSN until internal/config settles on a
// config format (env vars vs. YAML) for the rest of the app.
func runMigrate(args []string) {
	dsn := os.Getenv("KRONOWATT_DB_DSN")
	if dsn == "" {
		log.Fatal("KRONOWATT_DB_DSN must be set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("set dialect: %v", err)
	}
	goose.SetBaseFS(migrations.FS)

	command := "up"
	if len(args) > 0 {
		command = args[0]
	}

	if err := goose.RunContext(context.Background(), command, db, "."); err != nil {
		log.Fatalf("migrate %s: %v", command, err)
	}
}
