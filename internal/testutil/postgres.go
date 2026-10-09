//go:build integration

// Package testutil provides an isolated PostgreSQL schema for integration tests.
package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

// OpenDB applies the real migrations to a unique schema. It never migrates or
// truncates public tables. Set WEBHOOKER_TEST_DSN explicitly to a test database;
// the application DSN is deliberately never used as a fallback.
func OpenDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("WEBHOOKER_TEST_DSN")
	if dsn == "" {
		t.Skip("set WEBHOOKER_TEST_DSN to run PostgreSQL integration tests")
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		var err error
		dsn, err = pq.ParseURL(dsn)
		if err != nil {
			t.Fatalf("parse test DSN: %v", err)
		}
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := admin.PingContext(ctx); err != nil {
		t.Fatalf("connect to test PostgreSQL: %v", err)
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "webhooker_test_" + hex.EncodeToString(nonce[:])
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE"); err != nil {
			t.Errorf("clean up test schema: %v", err)
		}
	})
	// Startup parameters apply the schema to every pooled connection.
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(5)
	t.Cleanup(func() { db.Close() })
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository migrations")
	}
	migrations, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "*.up.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("locate migrations: %v (found %d)", err, len(migrations))
	}
	// filepath.Glob returns names sorted in migration timestamp order.
	for _, migration := range migrations {
		body, err := os.ReadFile(migration)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(migration), err)
		}
	}
	return db
}
