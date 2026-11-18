// Package storetest gives each test its own migrated Postgres schema.
package storetest

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"demonslayer2d/internal/store"
)

// New returns a migrated store in a fresh schema, dropped when the test ends.
// The test is skipped when TEST_DATABASE_URL is not set.
func New(t testing.TB) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("t_%d_%d", time.Now().UnixNano(), rand.IntN(1_000_000))
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("storetest: connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close(ctx)
		t.Fatalf("storetest: create schema: %v", err)
	}
	st, err := store.OpenSchema(ctx, url, schema)
	if err != nil {
		t.Fatalf("storetest: open: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("storetest: drop schema: %v", err)
		}
		admin.Close(ctx)
	})
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("storetest: migrate: %v", err)
	}
	return st
}
