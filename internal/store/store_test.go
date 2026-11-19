package store_test

import (
	"context"
	"errors"
	"testing"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
)

// ______________________TEST MIGRATIONS_____________________

func TestMigrateIsIdempotent(t *testing.T) {
	st := storetest.New(t) // already migrated once
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if err := st.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// ______________________TEST ACCOUNTS_____________________

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

// ______________________TEST CHARACTERS_____________________

func newChar(acc int64, slot int, name string) store.NewCharacter {
	return store.NewCharacter{AccountID: acc, Slot: slot, Name: name, NameKey: name + "_key", Class: "viem", MapID: "kumotori_1", X: 5.5, Y: 5.5}
}

func TestCharacters(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	other, _ := st.CreateAccount(ctx, "other", "h")

	c2, err := st.CreateCharacter(ctx, newChar(acc, 2, "Zenitsu"))
	if err != nil {
		t.Fatal(err)
	}
	if c2.ID <= 0 || c2.Level != 1 || c2.Exp != 0 || c2.X != 5.5 || c2.Class != "viem" {
		t.Fatalf("created %+v", c2)
	}
	c1, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.CreateCharacter(ctx, newChar(other, 1, "Zenitsu")); !errors.Is(err, store.ErrNameTaken) {
		t.Fatalf("duplicate name err = %v", err)
	}
	if _, err := st.CreateCharacter(ctx, newChar(acc, 1, "Inosuke")); !errors.Is(err, store.ErrSlotOccupied) {
		t.Fatalf("occupied slot err = %v", err)
	}

	list, err := st.ListCharacters(ctx, acc)
	if err != nil || len(list) != 2 || list[0].ID != c1.ID || list[1].ID != c2.ID {
		t.Fatalf("list = %+v, %v (want ordered by slot)", list, err)
	}

	if _, err := st.GetCharacter(ctx, other, c1.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get with wrong account err = %v", err)
	}

	if err := st.SaveCharacterState(ctx, store.CharState{ID: c1.ID, MapID: "kumotori_1", X: 10.25, Y: 7.75, Level: 3, Exp: -5}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCharacter(ctx, acc, c1.ID)
	if err != nil || got.X != 10.25 || got.Y != 7.75 || got.Level != 3 || got.Exp != -5 {
		t.Fatalf("after save: %+v, %v", got, err)
	}

	if err := st.DeleteCharacter(ctx, acc, c2.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCharacter(ctx, acc, c2.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
	if err := st.SaveCharacterState(ctx, store.CharState{ID: c2.ID, MapID: "kumotori_1"}); err != nil {
		t.Fatalf("saving a deleted character must be a no-op, got %v", err)
	}
}

func TestNewCharacterHasFullHPAndNoPoints(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	c, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}
	if c.HP != -1 || c.MP != -1 || c.Points != (combat.Points{}) || c.FreePoints != 0 {
		t.Fatalf("new character %+v", c)
	}
}

func TestSaveCharacterStateRoundTrip(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	c, _ := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))

	want := store.CharState{ID: c.ID, MapID: "kumotori_1", Level: 5, Exp: 40,
		Points: combat.Points{Str: 1, Agi: 2, Vit: 3, Int: 4}, FreePoints: 5, HP: 77, MP: 0}
	if err := st.SaveCharacterState(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCharacter(ctx, acc, c.ID)
	if err != nil || got.Points != want.Points || got.FreePoints != 5 || got.HP != 77 || got.MP != 0 {
		t.Fatalf("after save: %+v, %v", got, err)
	}

	// -1 (full) goes back to NULL and reads as -1 again.
	want.HP, want.MP = -1, -1
	if err := st.SaveCharacterState(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetCharacter(ctx, acc, c.ID)
	if err != nil || got.HP != -1 || got.MP != -1 {
		t.Fatalf("after saving full: %+v, %v", got, err)
	}
}
