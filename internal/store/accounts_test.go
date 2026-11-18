package store_test

import (
	"context"
	"errors"
	"testing"

	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
)

func TestAccounts(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()

	id, err := st.CreateAccount(ctx, "tanjiro", "hash1")
	if err != nil || id <= 0 {
		t.Fatalf("create: id=%d err=%v", id, err)
	}
	if _, err := st.CreateAccount(ctx, "tanjiro", "hash2"); !errors.Is(err, store.ErrUsernameTaken) {
		t.Fatalf("duplicate username err = %v", err)
	}
	a, err := st.GetAccountByUsername(ctx, "tanjiro")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != id || a.PasswordHash != "hash1" || a.Status != "active" {
		t.Fatalf("got %+v", a)
	}
	if _, err := st.GetAccountByUsername(ctx, "nezuko"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing account err = %v", err)
	}
	if err := st.TouchLogin(ctx, id); err != nil {
		t.Fatal(err)
	}
}
