//go:build integration

package data

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestIntegrationMigrationsRoundTrip(t *testing.T) {
	_, db := integrationModels(t)
	down, err := filepath.Glob("../../migrations/*.down.sql")
	if err != nil || len(down) == 0 {
		t.Fatalf("down migrations=%v error=%v", down, err)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(down)))
	for _, file := range down {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("rollback %s: %v", file, err)
		}
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()").Scan(&count); err != nil || count != 0 {
		t.Fatalf("remaining tables=%d error=%v", count, err)
	}
	up, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range up {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("reapply %s: %v", file, err)
		}
	}
	if err := db.QueryRow("SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()").Scan(&count); err != nil || count != 4 {
		t.Fatalf("recreated tables=%d error=%v", count, err)
	}
}
