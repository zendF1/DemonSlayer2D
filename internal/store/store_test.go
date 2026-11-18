package store_test

import (
	"context"
	"testing"

	"demonslayer2d/internal/store/storetest"
)

func TestMigrateIsIdempotent(t *testing.T) {
	st := storetest.New(t) // already migrated once
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if err := st.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
