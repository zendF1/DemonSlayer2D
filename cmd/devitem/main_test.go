package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
)

const dataDir = "../../data"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// ______________________TEST FLAGS_____________________

// Bad flags and codes fail before the database is opened, so no DATABASE_URL
// is needed except for the last case.
func TestRunRefusesBadFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no user", []string{"-xu", "5"}, "usage"},
		{"nothing to give", []string{"-u", "a"}, "usage"},
		{"extra argument", []string{"-u", "a", "-xu", "5", "x"}, "usage"},
		{"negative xu", []string{"-u", "a", "-xu", "-1"}, "-xu"},
		{"zero count", []string{"-u", "a", "-code", "stone_enhance", "-count", "0"}, "-count"},
		{"unknown code", []string{"-u", "a", "-code", "eq10_sword"}, `unknown item code "eq10_sword"`},
		{"quality too high", []string{"-u", "a", "-code", "eq10_armor", "-quality", "5"}, "-quality"},
		{"negative quality", []string{"-u", "a", "-code", "eq10_armor", "-quality", "-1"}, "-quality"},
		{"quality on a stack", []string{"-u", "a", "-code", "stone_enhance", "-quality", "1"}, "-quality"},
		{"count on equipment", []string{"-u", "a", "-code", "eq10_armor", "-count", "2"}, "-count"},
		{"enhance too high", []string{"-u", "a", "-code", "eq10_armor", "-enhance", "16"}, "-enhance"},
		{"negative enhance", []string{"-u", "a", "-code", "eq10_armor", "-enhance", "-1"}, "-enhance"},
		{"enhance on a stack", []string{"-u", "a", "-code", "charm_protect", "-enhance", "1"}, "-enhance"},
		{"holes too high", []string{"-u", "a", "-code", "eq10_armor", "-holes", "4"}, "-holes 4 is outside 0-3"},
		{"negative holes", []string{"-u", "a", "-code", "eq10_armor", "-holes", "-1"}, "-holes -1 is outside 0-3"},
		{"holes on a gem", []string{"-u", "a", "-code", "gem_hp_1", "-holes", "1"}, "-holes 1: gem_hp_1 is not equipment"},
		{"no database", []string{"-u", "a", "-xu", "5"}, "DATABASE_URL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(c.args, env(map[string]string{"DATA_DIR": dataDir}), &out)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want one mentioning %q", err, c.want)
			}
			if strings.Contains(err.Error(), "eq10_boots") {
				t.Fatalf("the error lists other codes: %v", err)
			}
			if out.Len() != 0 {
				t.Fatalf("printed %q before failing", out.String())
			}
		})
	}
}

// ______________________TEST DEV ITEM_____________________

// devDB returns a migrated test schema and a DATABASE_URL that reaches it
// (pgx passes the unknown search_path parameter on to the server).
func devDB(t *testing.T) (*store.Store, string) {
	t.Helper()
	url, schema := storetest.Schema(t)
	ctx := context.Background()
	st, err := store.OpenSchema(ctx, url, schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	return st, url + sep + "search_path=" + schema
}

func TestRunGivesItemsAndXu(t *testing.T) {
	st, url := devDB(t)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	c, err := st.CreateCharacter(ctx, store.NewCharacter{AccountID: acc, Slot: 1, Name: "Tanjiro", NameKey: "tanjiro",
		Class: "viem", MapID: "kumotori_1"})
	if err != nil {
		t.Fatal(err)
	}
	give := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := run(append([]string{"-u", "owner"}, args...), env(map[string]string{"DATA_DIR": dataDir, "DATABASE_URL": url}), &out)
		if strings.Contains(out.String(), url) || strings.Contains(out.String(), "ds:ds") {
			t.Fatalf("printed the database URL: %q", out.String())
		}
		return out.String(), err
	}
	load := func() store.Character {
		t.Helper()
		got, err := st.GetCharacter(ctx, acc, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// A Vàng sword goes into slot 0 of the empty bag with rolled lines, an element and two hidden lines.
	out, err := give("-code", "eq10_weapon_viem", "-quality", "2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "warning:") || !strings.Contains(out, "Tanjiro") || !strings.Contains(out, "bag slot 0") {
		t.Fatalf("output %q", out)
	}
	items := load().Items
	if len(items) != 1 {
		t.Fatalf("items %+v, want the sword", items)
	}
	sword := items[0]
	if sword.Equipped || sword.Slot != 0 || sword.Code != "eq10_weapon_viem" || sword.Count != 1 || sword.Quality != 2 ||
		len(sword.Lines) < 3 || len(sword.Lines) > 4 || sword.Element == "" || len(sword.Hidden) != 2 {
		t.Fatalf("sword %+v", sword)
	}

	// Stones merge into the existing stack.
	if _, err := give("-code", "stone_enhance", "-count", "5"); err != nil {
		t.Fatal(err)
	}
	if out, err = give("-code", "stone_enhance", "-count", "7"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "bag slot 1") {
		t.Fatalf("output %q", out)
	}
	items = load().Items
	stones := store.ItemRow{Slot: 1, Code: "stone_enhance", Count: 12, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}}
	if len(items) != 2 || !reflect.DeepEqual(items[0], sword) || !reflect.DeepEqual(items[1], stones) {
		t.Fatalf("items %+v, want the sword and 12 stones in slot 1", items)
	}

	// -enhance 9 stores a +9 piece; the protection charm is a stack.
	if out, err = give("-code", "eq10_armor", "-enhance", "9"); err != nil || !strings.Contains(out, "+9") ||
		!strings.Contains(out, "bag slot 2") {
		t.Fatalf("enhance: %q, %v", out, err)
	}
	if out, err = give("-code", "charm_protect", "-count", "3"); err != nil || !strings.Contains(out, "bag slot 3") {
		t.Fatalf("charm: %q, %v", out, err)
	}
	items = load().Items
	if len(items) != 4 || items[2].Code != "eq10_armor" || items[2].Enhance != 9 || items[2].Slot != 2 ||
		items[3].Code != "charm_protect" || items[3].Count != 3 || items[3].Enhance != 0 {
		t.Fatalf("items %+v, want a +9 armour in slot 2 and 3 charms in slot 3", items)
	}

	// -holes 2 punches two empty holes (phase 3c spec 7); gems are stacks.
	if out, err = give("-code", "eq20_armor", "-quality", "2", "-holes", "2"); err != nil ||
		!strings.Contains(out, "2 holes") || !strings.Contains(out, "bag slot 4") {
		t.Fatalf("holes: %q, %v", out, err)
	}
	if out, err = give("-code", "gem_hp_3", "-count", "2"); err != nil || !strings.Contains(out, "2 × Ngọc Sinh Lực cấp 3") ||
		!strings.Contains(out, "bag slot 5") {
		t.Fatalf("gem: %q, %v", out, err)
	}
	items = load().Items
	gem := store.ItemRow{Slot: 5, Code: "gem_hp_3", Count: 2, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}}
	if len(items) != 6 || items[4].Code != "eq20_armor" || !reflect.DeepEqual(items[4].Gems, []string{"", ""}) ||
		!reflect.DeepEqual(items[5], gem) {
		t.Fatalf("items %+v, want an armour with two empty holes in slot 4 and 2 gems in slot 5", items)
	}
	if len(items[2].Gems) != 0 {
		t.Fatalf("the armour given without -holes has holes %q", items[2].Gems)
	}

	// -xu alone adds Xu and leaves the items alone.
	if out, err = give("-xu", "500"); err != nil || !strings.Contains(out, "500 Xu") {
		t.Fatalf("xu: %q, %v", out, err)
	}
	if got := load(); got.Xu != 500 || len(got.Items) != 6 {
		t.Fatalf("after -xu: xu %d, items %+v", got.Xu, got.Items)
	}

	// A full bag refuses the item and writes nothing, not even the Xu.
	full := []store.ItemRow{{Equipped: true, Slot: 0, Code: "eq10_weapon_viem", Count: 1}}
	for i := range 60 {
		full = append(full, store.ItemRow{Slot: i, Code: "eq10_armor", Count: 1})
	}
	if err := st.SaveCharacterState(ctx, store.CharState{ID: c.ID, MapID: "kumotori_1", Level: 1, HP: -1, MP: -1,
		Xu: 500, Items: full}); err != nil {
		t.Fatal(err)
	}
	if _, err := give("-code", "eq10_boots", "-xu", "10"); err == nil || !strings.Contains(err.Error(), "bag") {
		t.Fatalf("full bag: %v", err)
	}
	if got := load(); got.Xu != 500 || len(got.Items) != 61 {
		t.Fatalf("after a full bag: xu %d, %d items", got.Xu, len(got.Items))
	}

	// A missing account is an error.
	var out2 bytes.Buffer
	err = run([]string{"-u", "nobody", "-xu", "1"}, env(map[string]string{"DATA_DIR": dataDir, "DATABASE_URL": url}), &out2)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing account: %v, want ErrNotFound", err)
	}
}
