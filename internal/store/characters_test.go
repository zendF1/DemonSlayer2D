package store_test

import (
	"context"
	"errors"
	"testing"

	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
)

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
