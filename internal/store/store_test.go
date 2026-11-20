package store_test

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
	"demonslayer2d/migrations"
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

// ______________________TEST SKILLS STORE_____________________

// schemaConfig is a connection config for url that works in schema.
func schemaConfig(t *testing.T, url, schema string) *pgx.ConnConfig {
	t.Helper()
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeParams["search_path"] = schema
	return cfg
}

// openMigrated opens a store on schema and migrates it to the latest version.
func openMigrated(t *testing.T, url, schema string) *store.Store {
	t.Helper()
	st, err := store.OpenSchema(context.Background(), url, schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st
}

// The migration gives existing characters the skill points of the levels
// they already have.
func TestMigrationBackfillsSkillPoints(t *testing.T) {
	url, schema := storetest.Schema(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*schemaConfig(t, url, schema))
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	var acc, char1, char5 int64
	if err := db.QueryRowContext(ctx, `INSERT INTO accounts (username, password_hash) VALUES ('old', 'h') RETURNING id`).Scan(&acc); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO characters (account_id, slot, name, name_key, class, level, map_id, pos_x, pos_y)
		VALUES ($1, $2, $3, $3, 'nham', $4, 'kumotori_1', 5.5, 5.5) RETURNING id`
	if err := db.QueryRowContext(ctx, insert, acc, 1, "five", 5).Scan(&char5); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, insert, acc, 2, "one", 1).Scan(&char1); err != nil {
		t.Fatal(err)
	}

	st := openMigrated(t, url, schema)
	for id, want := range map[int64]int{char5: 4, char1: 0} {
		c, err := st.GetCharacter(ctx, acc, id)
		if err != nil || c.SkillPoints != want || len(c.Skills) != 0 {
			t.Fatalf("character %d: %+v, %v, want %d skill points and no skills", id, c, err, want)
		}
	}

	// Down to version 2 removes both again, and Up comes back cleanly.
	if _, err := p.DownTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	var cols, tables int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM information_schema.columns WHERE table_schema = $1 AND table_name = 'characters' AND column_name = 'skill_points'),
		(SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'character_skills')`,
		schema).Scan(&cols, &tables); err != nil || cols != 0 || tables != 0 {
		t.Fatalf("after down: %d skill_points columns, %d character_skills tables, %v", cols, tables, err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSaveAndLoadSkills(t *testing.T) {
	url, schema := storetest.Schema(t)
	st := openMigrated(t, url, schema)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	c, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}
	if c.SkillPoints != 0 || len(c.Skills) != 0 {
		t.Fatalf("new character %+v, want no skill points or skills", c)
	}
	other, _ := st.CreateCharacter(ctx, newChar(acc, 2, "Zenitsu"))
	if err := st.SaveCharacterState(ctx, store.CharState{ID: other.ID, MapID: "kumotori_1", Level: 1, HP: -1, MP: -1,
		Skills: map[string]int{"viem_1": 1}}); err != nil {
		t.Fatal(err)
	}

	state := store.CharState{ID: c.ID, MapID: "kumotori_1", Level: 30, HP: -1, MP: -1, SkillPoints: 3,
		Skills: map[string]int{"viem_1": 10, "viem_2": 4, "viem_3": 1}}
	if err := st.SaveCharacterState(ctx, state); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCharacter(ctx, acc, c.ID)
	if err != nil || got.SkillPoints != 3 || !maps.Equal(got.Skills, state.Skills) {
		t.Fatalf("after save: %+v, %v", got, err)
	}

	// Saving replaces the rows: a dropped skill is gone, a changed one updated.
	state.SkillPoints, state.Skills = 1, map[string]int{"viem_2": 5}
	if err := st.SaveCharacterState(ctx, state); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListCharacters(ctx, acc)
	if err != nil || len(list) != 2 {
		t.Fatalf("list %+v, %v", list, err)
	}
	if list[0].SkillPoints != 1 || !maps.Equal(list[0].Skills, state.Skills) {
		t.Fatalf("listed %+v, want viem_2 at 5 and 1 point", list[0])
	}
	if !maps.Equal(list[1].Skills, map[string]int{"viem_1": 1}) {
		t.Fatalf("other character %+v, want its own skills untouched", list[1])
	}

	// An empty map deletes every row.
	state.Skills = nil
	if err := st.SaveCharacterState(ctx, state); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetCharacter(ctx, acc, c.ID); err != nil || len(got.Skills) != 0 {
		t.Fatalf("after saving no skills: %+v, %v", got, err)
	}

	// Deleting a character takes its skills with it, and saving it after is still a no-op.
	if err := st.DeleteCharacter(ctx, acc, other.ID); err != nil {
		t.Fatal(err)
	}
	raw, err := pgx.ConnectConfig(ctx, schemaConfig(t, url, schema))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close(ctx)
	var n int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM character_skills WHERE character_id = $1`, other.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d skill rows left after delete, %v", n, err)
	}
	if err := st.SaveCharacterState(ctx, store.CharState{ID: other.ID, MapID: "kumotori_1", Level: 1,
		Skills: map[string]int{"viem_1": 2}}); err != nil {
		t.Fatalf("saving a deleted character with skills must be a no-op, got %v", err)
	}
}

// ______________________TEST ITEMS STORE_____________________

// The items migration gives existing characters 0 Xu and no items, and its
// Down removes the column and the table again.
func TestMigrationAddsXuAndItems(t *testing.T) {
	url, schema := storetest.Schema(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*schemaConfig(t, url, schema))
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	var acc, char int64
	if err := db.QueryRowContext(ctx, `INSERT INTO accounts (username, password_hash) VALUES ('old', 'h') RETURNING id`).Scan(&acc); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO characters (account_id, slot, name, name_key, class, level, map_id, pos_x, pos_y)
		VALUES ($1, 1, 'old', 'old', 'viem', 7, 'kumotori_1', 5.5, 5.5) RETURNING id`, acc).Scan(&char); err != nil {
		t.Fatal(err)
	}

	st := openMigrated(t, url, schema)
	c, err := st.GetCharacter(ctx, acc, char)
	if err != nil || c.Xu != 0 || c.Items == nil || len(c.Items) != 0 {
		t.Fatalf("migrated character %+v, %v, want 0 Xu and an empty, non-nil item list", c, err)
	}

	if _, err := p.DownTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	var cols, tables int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM information_schema.columns WHERE table_schema = $1 AND table_name = 'characters' AND column_name = 'xu'),
		(SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'character_items')`,
		schema).Scan(&cols, &tables); err != nil || cols != 0 || tables != 0 {
		t.Fatalf("after down: %d xu columns, %d character_items tables, %v", cols, tables, err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
}

// testItems has a worn Vàng weapon with an element, lines, hidden lines, a
// hidden enhancement pity and three holes, the middle one empty; two Xanh
// pieces (one line each, so a decoder reusing the previous row's slice would
// show; one with its own pity and one empty hole) and a stone stack in the
// bag.
func testItems() []store.ItemRow {
	return []store.ItemRow{
		{Equipped: false, Slot: 0, Code: "eq10_boots", Count: 1, Quality: 1,
			Lines: []store.ItemLine{{Stat: "hp", Value: 40}}, Hidden: []store.ItemLine{}},
		{Equipped: false, Slot: 2, Code: "eq10_bracers", Count: 1, Quality: 1, Enhance: 4,
			Lines: []store.ItemLine{{Stat: "phys_defense", Value: 2}}, Hidden: []store.ItemLine{},
			Pity: []int{0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, Gems: []string{""}},
		{Equipped: false, Slot: 7, Code: "stone_enhance", Count: 250, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}},
		{Equipped: true, Slot: 0, Code: "eq10_weapon_viem", Count: 1, Quality: 2, Element: "thuy",
			Lines:   []store.ItemLine{{Stat: "phys_attack", Value: 5}, {Stat: "hp", Value: 43}, {Stat: "magic_defense", Value: 3}},
			Hidden:  []store.ItemLine{{Stat: "hp", Value: 86}, {Stat: "phys_defense", Value: 4}},
			Enhance: 9, Pity: []int{0, 0, 0, 0, 1, 2, 3, 0, 7, 12, 30, 75, 80, 85, 90},
			Gems: []string{"gem_hp_1", "", `odd "code", \{}`}},
	}
}

// The enhancement migration gives existing item rows an empty pity, which
// loads as nil (all zero), and its Down drops the column again.
func TestMigrationAddsPity(t *testing.T) {
	url, schema := storetest.Schema(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*schemaConfig(t, url, schema))
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	var acc, char int64
	if err := db.QueryRowContext(ctx, `INSERT INTO accounts (username, password_hash) VALUES ('old', 'h') RETURNING id`).Scan(&acc); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO characters (account_id, slot, name, name_key, class, level, map_id, pos_x, pos_y)
		VALUES ($1, 1, 'old', 'old', 'viem', 7, 'kumotori_1', 5.5, 5.5) RETURNING id`, acc).Scan(&char); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO character_items (character_id, equipped, slot, code, count, enhance)
		VALUES ($1, true, 0, 'eq10_weapon_viem', 1, 6)`, char); err != nil {
		t.Fatal(err)
	}

	st := openMigrated(t, url, schema)
	c, err := st.GetCharacter(ctx, acc, char)
	want := []store.ItemRow{{Equipped: true, Slot: 0, Code: "eq10_weapon_viem", Count: 1, Enhance: 6,
		Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}}}
	if err != nil || !reflect.DeepEqual(c.Items, want) {
		t.Fatalf("migrated items %+v, %v, want the weapon with no pity", c.Items, err)
	}

	if _, err := p.DownTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	var cols int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'character_items' AND column_name = 'pity'`, schema).Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("after down: %d pity columns, %v", cols, err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
}

// The sockets migration gives 3b-era item rows no holes, which load as nil,
// and its Down drops the column again.
func TestMigrationAddsGems(t *testing.T) {
	url, schema := storetest.Schema(t)
	ctx := context.Background()
	db := stdlib.OpenDB(*schemaConfig(t, url, schema))
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	var acc, char int64
	if err := db.QueryRowContext(ctx, `INSERT INTO accounts (username, password_hash) VALUES ('old', 'h') RETURNING id`).Scan(&acc); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO characters (account_id, slot, name, name_key, class, level, map_id, pos_x, pos_y)
		VALUES ($1, 1, 'old', 'old', 'viem', 7, 'kumotori_1', 5.5, 5.5) RETURNING id`, acc).Scan(&char); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO character_items (character_id, equipped, slot, code, count, enhance, pity)
		VALUES ($1, true, 0, 'eq10_weapon_viem', 1, 6, '{0,0,0,0,0,0,4,0,0,0,0,0,0,0,0}')`, char); err != nil {
		t.Fatal(err)
	}

	st := openMigrated(t, url, schema)
	c, err := st.GetCharacter(ctx, acc, char)
	want := []store.ItemRow{{Equipped: true, Slot: 0, Code: "eq10_weapon_viem", Count: 1, Enhance: 6,
		Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}, Pity: []int{0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0}}}
	if err != nil || !reflect.DeepEqual(c.Items, want) {
		t.Fatalf("migrated items %+v, %v, want the weapon with no holes", c.Items, err)
	}

	if _, err := p.DownTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	var cols int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'character_items' AND column_name = 'gems'`, schema).Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("after down: %d gems columns, %v", cols, err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSaveAndLoadItems(t *testing.T) {
	url, schema := storetest.Schema(t)
	st := openMigrated(t, url, schema)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	c, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Xu != 0 || c.Items == nil || len(c.Items) != 0 {
		t.Fatalf("new character %+v, want 0 Xu and an empty, non-nil item list", c)
	}
	other, _ := st.CreateCharacter(ctx, newChar(acc, 2, "Zenitsu"))
	otherItems := []store.ItemRow{{Slot: 3, Code: "stone_refine", Count: 9, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}}}
	if err := st.SaveCharacterState(ctx, store.CharState{ID: other.ID, MapID: "kumotori_1", Level: 1, HP: -1, MP: -1,
		Xu: 7, Items: otherItems}); err != nil {
		t.Fatal(err)
	}

	state := store.CharState{ID: c.ID, MapID: "kumotori_1", Level: 12, HP: -1, MP: -1, Xu: 123456789012, Items: testItems()}
	if err := st.SaveCharacterState(ctx, state); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCharacter(ctx, acc, c.ID)
	if err != nil || got.Xu != state.Xu || !reflect.DeepEqual(got.Items, state.Items) {
		t.Fatalf("after save: xu %d items %+v, %v\nwant xu %d items %+v", got.Xu, got.Items, err, state.Xu, state.Items)
	}

	// Saving replaces the rows; the list also loads them per character.
	state.Xu, state.Items = 5, state.Items[3:]
	if err := st.SaveCharacterState(ctx, state); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListCharacters(ctx, acc)
	if err != nil || len(list) != 2 {
		t.Fatalf("list %+v, %v", list, err)
	}
	if list[0].Xu != 5 || !reflect.DeepEqual(list[0].Items, state.Items) {
		t.Fatalf("listed %+v, want 5 Xu and only the worn weapon", list[0])
	}
	if list[1].Xu != 7 || !reflect.DeepEqual(list[1].Items, otherItems) {
		t.Fatalf("other character %+v, want its own Xu and items untouched", list[1])
	}

	// Nil lines save as empty lists, an empty pity and no holes load as nil;
	// an empty item list deletes every row.
	state.Items = []store.ItemRow{{Slot: 1, Code: "stone_enhance", Count: 1, Pity: []int{}, Gems: []string{}}}
	if err := st.SaveCharacterState(ctx, state); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetCharacter(ctx, acc, c.ID)
	want := []store.ItemRow{{Slot: 1, Code: "stone_enhance", Count: 1, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}}}
	if err != nil || !reflect.DeepEqual(got.Items, want) {
		t.Fatalf("after saving nil lines: %+v, %v", got.Items, err)
	}
	state.Items = nil
	if err := st.SaveCharacterState(ctx, state); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetCharacter(ctx, acc, c.ID); err != nil || got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("after saving no items: %+v, %v", got.Items, err)
	}

	// Deleting a character takes its items with it.
	if err := st.DeleteCharacter(ctx, acc, other.ID); err != nil {
		t.Fatal(err)
	}
	raw, err := pgx.ConnectConfig(ctx, schemaConfig(t, url, schema))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close(ctx)
	var n int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM character_items WHERE character_id = $1`, other.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d item rows left after delete, %v", n, err)
	}
}

// ______________________TEST DEV LEVEL_____________________

func TestDevSetLevel(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	// Slot 2 is created first: the first character is the one in the lowest slot.
	second, _ := st.CreateCharacter(ctx, newChar(acc, 2, "Zenitsu"))
	first, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}

	res, err := st.DevSetLevel(ctx, "owner", 90)
	if err != nil {
		t.Fatal(err)
	}
	if res != (store.DevResult{Name: "Tanjiro", Level: 90, FreePoints: 890, SkillPoints: 89}) {
		t.Fatalf("level 90 on a fresh character: %+v", res)
	}
	got, err := st.GetCharacter(ctx, acc, first.ID)
	if err != nil || got.Level != 90 || got.Exp != 0 || got.FreePoints != 890 || got.SkillPoints != 89 || got.HP != -1 || got.MP != -1 {
		t.Fatalf("stored %+v, %v", got, err)
	}
	if other, _ := st.GetCharacter(ctx, acc, second.ID); other.Level != 1 {
		t.Fatalf("the slot 2 character changed: %+v", other)
	}

	// Spent potential points are refunded; skills (3 + 2 levels) fit in 19 points and stay.
	learned := map[string]int{"viem_1": 3, "viem_2": 2}
	if err := st.SaveCharacterState(ctx, store.CharState{ID: first.ID, MapID: "kumotori_1", Level: 30, Exp: 500,
		Points: combat.Points{Str: 10, Agi: 20, Vit: 30, Int: 40}, FreePoints: 190, HP: 77, MP: 3,
		SkillPoints: 24, Skills: learned}); err != nil {
		t.Fatal(err)
	}
	res, err = st.DevSetLevel(ctx, "owner", 20)
	if err != nil || res != (store.DevResult{Name: "Tanjiro", Level: 20, FreePoints: 190, SkillPoints: 14}) {
		t.Fatalf("level 20 with skills: %+v, %v", res, err)
	}
	got, err = st.GetCharacter(ctx, acc, first.ID)
	if err != nil || got.Level != 20 || got.Exp != 0 || got.Points != (combat.Points{}) || got.FreePoints != 190 ||
		got.SkillPoints != 14 || !maps.Equal(got.Skills, learned) || got.HP != -1 || got.MP != -1 {
		t.Fatalf("stored %+v, %v", got, err)
	}

	// At level 5 the 5 learned levels do not fit in 4 points: every skill goes.
	res, err = st.DevSetLevel(ctx, "owner", 5)
	if err != nil || res != (store.DevResult{Name: "Tanjiro", Level: 5, FreePoints: 40, SkillPoints: 4}) {
		t.Fatalf("level 5 with skills: %+v, %v", res, err)
	}
	got, err = st.GetCharacter(ctx, acc, first.ID)
	if err != nil || got.Level != 5 || got.SkillPoints != 4 || len(got.Skills) != 0 {
		t.Fatalf("stored %+v, %v", got, err)
	}
}

func TestDevSetLevelErrors(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	if _, err := st.DevSetLevel(ctx, "nobody", 10); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing account: %v, want ErrNotFound", err)
	}
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	if _, err := st.DevSetLevel(ctx, "owner", 10); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("account without a character: %v, want ErrNotFound", err)
	}
	c, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}
	for _, level := range []int{0, 91} {
		if _, err := st.DevSetLevel(ctx, "owner", level); err == nil {
			t.Errorf("level %d: no error", level)
		}
	}
	if got, err := st.GetCharacter(ctx, acc, c.ID); err != nil || got.Level != 1 {
		t.Fatalf("after refused levels: %+v, %v", got, err)
	}
}

// ______________________TEST DEV ITEM_____________________

func TestDevGiveItem(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	second, _ := st.CreateCharacter(ctx, newChar(acc, 2, "Zenitsu"))
	first, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}
	// An enhanced piece keeps its level and pity (phase 3b spec 7) and its
	// holes and gems (phase 3c spec 7) through place.
	worn := store.ItemRow{Equipped: true, Slot: 4, Code: "eq10_armor", Count: 1, Quality: 2, Element: "thuy", Enhance: 9,
		Lines: []store.ItemLine{{Stat: "hp", Value: 40}}, Hidden: []store.ItemLine{{Stat: "hp", Value: 50}, {Stat: "hp", Value: 50}},
		Pity: []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0}, Gems: []string{"gem_hp_1", ""}}
	stones := store.ItemRow{Slot: 0, Code: "stone_enhance", Count: 7, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{}}
	if err := st.SaveCharacterState(ctx, store.CharState{ID: first.ID, MapID: "kumotori_1", Level: 1, HP: -1, MP: -1,
		Xu: 100, Items: []store.ItemRow{stones, worn}}); err != nil {
		t.Fatal(err)
	}

	// place sees the current rows and its list replaces them; Xu is added.
	sword := store.ItemRow{Slot: 1, Code: "eq10_weapon_viem", Count: 1, Lines: []store.ItemLine{}, Hidden: []store.ItemLine{},
		Gems: []string{"", "", ""}}
	var seen []store.ItemRow
	res, err := st.DevGiveItem(ctx, "owner", func(rows []store.ItemRow) ([]store.ItemRow, error) {
		seen = rows
		merged := stones
		merged.Count = 12
		return []store.ItemRow{merged, sword, worn}, nil
	}, 500)
	if err != nil || res != (store.DevItemResult{Name: "Tanjiro", Xu: 600}) {
		t.Fatalf("give: %+v, %v", res, err)
	}
	if !reflect.DeepEqual(seen, []store.ItemRow{stones, worn}) {
		t.Fatalf("place saw %+v", seen)
	}
	got, err := st.GetCharacter(ctx, acc, first.ID)
	merged := stones
	merged.Count = 12
	if err != nil || got.Xu != 600 || !reflect.DeepEqual(got.Items, []store.ItemRow{merged, sword, worn}) {
		t.Fatalf("stored xu %d items %+v, %v", got.Xu, got.Items, err)
	}
	if other, _ := st.GetCharacter(ctx, acc, second.ID); other.Xu != 0 || len(other.Items) != 0 {
		t.Fatalf("the slot 2 character changed: %+v", other)
	}

	// A nil place only adds Xu.
	res, err = st.DevGiveItem(ctx, "owner", nil, 500)
	if err != nil || res.Xu != 1100 {
		t.Fatalf("xu only: %+v, %v", res, err)
	}
	if got, _ := st.GetCharacter(ctx, acc, first.ID); got.Xu != 1100 || len(got.Items) != 3 {
		t.Fatalf("after xu only: xu %d, %d items", got.Xu, len(got.Items))
	}

	// A place error (a full bag) writes nothing, not even the Xu.
	full := errors.New("bag full")
	if _, err := st.DevGiveItem(ctx, "owner", func([]store.ItemRow) ([]store.ItemRow, error) { return nil, full }, 500); !errors.Is(err, full) {
		t.Fatalf("place error: %v, want %v", err, full)
	}
	if got, _ := st.GetCharacter(ctx, acc, first.ID); got.Xu != 1100 || len(got.Items) != 3 {
		t.Fatalf("after a place error: xu %d, %d items", got.Xu, len(got.Items))
	}
}

func TestDevGiveItemErrors(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	if _, err := st.DevGiveItem(ctx, "nobody", nil, 10); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing account: %v, want ErrNotFound", err)
	}
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	if _, err := st.DevGiveItem(ctx, "owner", nil, 10); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("account without a character: %v, want ErrNotFound", err)
	}
	c, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DevGiveItem(ctx, "owner", nil, -1); err == nil {
		t.Fatal("negative xu: no error")
	}
	if got, err := st.GetCharacter(ctx, acc, c.ID); err != nil || got.Xu != 0 {
		t.Fatalf("after refused calls: %+v, %v", got, err)
	}
}

// ______________________TEST DEV PLACE_____________________

func TestDevSetPlace(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	second, _ := st.CreateCharacter(ctx, newChar(acc, 2, "Zenitsu"))
	first, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCharacterState(ctx, store.CharState{ID: first.ID, MapID: "kumotori_1", X: 5.5, Y: 5.5, Level: 7,
		Exp: 12, HP: 40, MP: 3, Xu: 9}); err != nil {
		t.Fatal(err)
	}

	res, err := st.DevSetPlace(ctx, "owner", "ubuyashiki", 8.5, 16.25)
	if err != nil || res != (store.DevResult{Name: "Tanjiro", Level: 7}) {
		t.Fatalf("place: %+v, %v", res, err)
	}
	// Only the map and position change.
	got, err := st.GetCharacter(ctx, acc, first.ID)
	if err != nil || got.MapID != "ubuyashiki" || got.X != 8.5 || got.Y != 16.25 || got.Level != 7 || got.Exp != 12 ||
		got.HP != 40 || got.MP != 3 || got.Xu != 9 {
		t.Fatalf("stored %+v, %v", got, err)
	}
	if other, _ := st.GetCharacter(ctx, acc, second.ID); other.MapID != "kumotori_1" || other.X != 5.5 || other.Y != 5.5 {
		t.Fatalf("the slot 2 character moved: %+v", other)
	}
}

func TestDevSetPlaceErrors(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	if _, err := st.DevSetPlace(ctx, "nobody", "ubuyashiki", 8.5, 16.5); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing account: %v, want ErrNotFound", err)
	}
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	if _, err := st.DevSetPlace(ctx, "owner", "ubuyashiki", 8.5, 16.5); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("account without a character: %v, want ErrNotFound", err)
	}
	c, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DevSetPlace(ctx, "owner", "", 8.5, 16.5); err == nil {
		t.Fatal("empty map id: no error")
	}
	if got, err := st.GetCharacter(ctx, acc, c.ID); err != nil || got.MapID != "kumotori_1" || got.X != 5.5 {
		t.Fatalf("after refused calls: %+v, %v", got, err)
	}
}
