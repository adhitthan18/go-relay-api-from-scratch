package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func runMigrations(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    VARCHAR(255) NOT NULL PRIMARY KEY,
			applied_at DATETIME     NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("membuat tabel schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("membaca folder migrations: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var sudah int
		err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", name).Scan(&sudah)
		if err != nil {
			return fmt.Errorf("memeriksa migrasi %s: %w", name, err)
		}
		if sudah > 0 {
			continue
		}

		isi, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("membaca %s: %w", name, err)
		}

		fmt.Println("menjalankan migrasi:", name)
		if _, err := db.ExecContext(ctx, string(isi)); err != nil {
			return fmt.Errorf("migrasi %s gagal: %w", name, err)
		}

		if _, err := db.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
			name, time.Now()); err != nil {
			return fmt.Errorf("mencatat migrasi %s: %w", name, err)
		}
	}

	fmt.Println("migrasi selesai,", len(names), "berkas diperiksa")
	return nil
}