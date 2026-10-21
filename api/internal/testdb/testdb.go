// Package testdb gives tests a fresh Postgres database with every migration
// applied (local docker compose Postgres). Tests skip when it is not running.
package testdb

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RepoRoot is the repository root (from this file's location).
func RepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func baseURL(t testing.TB) string {
	if u := os.Getenv("DATABASE_URL"); u != "" {
		return u
	}
	f, err := os.Open(filepath.Join(RepoRoot(), ".env"))
	if err != nil {
		t.Skip("DATABASE_URL not set")
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "DATABASE_URL="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	t.Skip("DATABASE_URL not set")
	return ""
}

// New drops and recreates database `name`, applies db/migrations, and returns a pool.
func New(t testing.TB, name string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	base := baseURL(t)
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Skip("local Postgres not reachable (docker compose up -d db)")
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name // never the dev database
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var current string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&current); err != nil || current != name {
		t.Fatalf("connected to %q, want %q", current, name)
	}
	files, _ := filepath.Glob(filepath.Join(RepoRoot(), "db", "migrations", "*.sql"))
	sort.Strings(files)
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		up := strings.SplitN(strings.SplitN(string(raw), "-- +goose Up", 2)[1], "-- +goose Down", 2)[0]
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("%s: %v", filepath.Base(f), err)
		}
	}
	return pool
}
