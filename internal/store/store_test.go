package store_test

import (
	"context"
	"errors"
	"maps"
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

	// Down removes both again, and Up comes back cleanly.
	if _, err := p.Down(ctx); err != nil {
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
