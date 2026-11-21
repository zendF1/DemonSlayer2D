package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"

	"demonslayer2d/internal/combat"
)

type Character struct {
	ID        int64
	AccountID int64
	Slot      int
	Name      string
	Class     string
	Level     int
	Exp       int64
	MapID     string
	X, Y      float32
	// Points are the spent potential points; FreePoints are the unspent ones.
	Points     combat.Points
	FreePoints int
	// HP and MP are -1 when stored as NULL, which means full.
	HP, MP int
	// SkillPoints are the unspent skill points; Skills maps a learned skill's
	// id to its level. Skills is never nil on a loaded character.
	SkillPoints int
	Skills      map[string]int
	// Xu is the character's money. Items are its bag and worn items, ordered
	// bag first, then by slot; never nil on a loaded character.
	Xu    int64
	Items []ItemRow
	// PkOn is whether PK mode is on.
	PkOn bool
	// Quests are the quest log rows, ordered by quest id; never nil on a
	// loaded character.
	Quests []QuestRow
	// SkillBookWaiver is the highest learn level whose skills need no book.
	SkillBookWaiver int
	// Account is the account part (Lượng, stash); only GetCharacter fills it.
	Account AccountState
}

// QuestRow is one stored quest log entry. State is 1 (in progress) or 2
// (done); Day is 0 for the main line.
type QuestRow struct {
	QuestID  string
	State    int16
	Progress int
	Day      int32
}

// AccountState is the account-owned data a character save may carry: Lượng,
// the stash size and the stash rows (Equipped is always false, Slot is the
// stash slot).
type AccountState struct {
	ID        int64
	Luong     int64
	StashSize int
	Stash     []ItemRow
}

// ItemRow is one stored item: a bag slot (Equipped false) or a worn slot
// (Equipped true). Element is "" for none. Lines and Hidden are never nil on
// a loaded row.
type ItemRow struct {
	Equipped      bool
	Slot          int
	Code          string
	Count         int
	Quality       int
	Element       string
	Enhance       int
	Lines, Hidden []ItemLine
	// Pity is the hidden enhancement bonus per target level: entry k is the
	// bonus in percentage points for target k + 1. Empty means all zero, and
	// an empty stored array loads as nil. The store does not check its
	// length or values; the world does.
	Pity []int
	// Gems has one entry per punched hole: a gem code, or "" for an empty
	// hole. No holes load as nil. The store does not check the codes or the
	// length; the world does.
	Gems []string
}

// ItemLine is an item line as stored in the lines and hidden jsonb columns.
type ItemLine struct {
	Stat  string `json:"stat"`
	Value int    `json:"value"`
}

type NewCharacter struct {
	AccountID int64
	Slot      int
	Name      string
	NameKey   string
	Class     string
	MapID     string
	X, Y      float32
}

// CharState is the part of a character the game loop changes and saves.
type CharState struct {
	ID    int64
	MapID string
	X, Y  float32
	Level int
	Exp   int64

	Points     combat.Points
	FreePoints int
	// HP and MP are saved as given; -1 (full) is stored as NULL.
	HP, MP int
	// Skills replaces every stored skill of the character; nil saves none.
	SkillPoints int
	Skills      map[string]int
	// Items replaces every stored item of the character; nil saves none.
	Xu    int64
	Items []ItemRow
	// PkOn is saved with every state.
	PkOn bool
	// Quests replaces every stored quest row of the character; nil saves
	// none. SkillBookWaiver is saved with every state.
	Quests          []QuestRow
	SkillBookWaiver int
	// Account, when not nil, is saved in the same transaction; nil writes
	// nothing to the account.
	Account *AccountState
}

const charColumns = `id, account_id, slot, name, class, level, exp, map_id, pos_x, pos_y,
	str, agi, vit, int, free_points, COALESCE(hp, -1), COALESCE(mp, -1), skill_points, xu, pk_on, skill_book_waiver`

// scanCharacter reads charColumns; the skills and items come from loadExtras.
func scanCharacter(row pgx.Row) (Character, error) {
	c := Character{Skills: map[string]int{}, Items: []ItemRow{}, Quests: []QuestRow{}}
	err := row.Scan(&c.ID, &c.AccountID, &c.Slot, &c.Name, &c.Class, &c.Level, &c.Exp, &c.MapID, &c.X, &c.Y,
		&c.Points.Str, &c.Points.Agi, &c.Points.Vit, &c.Points.Int, &c.FreePoints, &c.HP, &c.MP, &c.SkillPoints, &c.Xu, &c.PkOn, &c.SkillBookWaiver)
	return c, err
}

// loadExtras fills the skills and items of chars, one query each.
func (s *Store) loadExtras(ctx context.Context, chars []Character) error {
	if err := s.loadSkills(ctx, chars); err != nil {
		return err
	}
	if err := loadItems(ctx, s.pool, chars); err != nil {
		return err
	}
	return loadQuests(ctx, s.pool, chars)
}

// loadQuests fills the Quests of chars with one query, ordered by quest id.
func loadQuests(ctx context.Context, q querier, chars []Character) error {
	if len(chars) == 0 {
		return nil
	}
	byID := make(map[int64]*Character, len(chars))
	ids := make([]int64, len(chars))
	for i := range chars {
		byID[chars[i].ID] = &chars[i]
		ids[i] = chars[i].ID
	}
	rows, err := q.Query(ctx, `SELECT character_id, quest_id, state, progress, day
		FROM character_quests WHERE character_id = ANY($1) ORDER BY character_id, quest_id`, ids)
	if err != nil {
		return err
	}
	var (
		charID int64
		r      QuestRow
	)
	_, err = pgx.ForEachRow(rows, []any{&charID, &r.QuestID, &r.State, &r.Progress, &r.Day}, func() error {
		c := byID[charID]
		c.Quests = append(c.Quests, r)
		return nil
	})
	return err
}

// saveQuests replaces the character's quest rows: a delete, then one INSERT
// from unnest.
func saveQuests(ctx context.Context, tx pgx.Tx, id int64, quests []QuestRow) error {
	if _, err := tx.Exec(ctx, `DELETE FROM character_quests WHERE character_id = $1`, id); err != nil {
		return err
	}
	if len(quests) == 0 {
		return nil
	}
	n := len(quests)
	var (
		qid      = make([]string, n)
		state    = make([]int16, n)
		progress = make([]int32, n)
		day      = make([]int32, n)
	)
	for i, r := range quests {
		qid[i], state[i], progress[i], day[i] = r.QuestID, r.State, int32(r.Progress), r.Day
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO character_quests (character_id, quest_id, state, progress, day)
		 SELECT $1, * FROM unnest($2::text[], $3::smallint[], $4::int[], $5::int[])`,
		id, qid, state, progress, day)
	return err
}

// querier is what loadItems needs from a pool or a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// loadItems fills the Items of chars with one query, bag first, then by slot.
func loadItems(ctx context.Context, q querier, chars []Character) error {
	if len(chars) == 0 {
		return nil
	}
	byID := make(map[int64]*Character, len(chars))
	ids := make([]int64, len(chars))
	for i := range chars {
		byID[chars[i].ID] = &chars[i]
		ids[i] = chars[i].ID
	}
	rows, err := q.Query(ctx, `SELECT character_id, equipped, slot, code, count, quality, element, enhance, lines, hidden, pity, gems
		FROM character_items WHERE character_id = ANY($1) ORDER BY character_id, equipped, slot`, ids)
	if err != nil {
		return err
	}
	var (
		charID int64
		r      ItemRow
	)
	_, err = pgx.ForEachRow(rows, []any{&charID, &r.Equipped, &r.Slot, &r.Code, &r.Count, &r.Quality, &r.Element,
		&r.Enhance, &r.Lines, &r.Hidden, &r.Pity, &r.Gems}, func() error {
		// pgx decodes each jsonb and array value into a fresh slice, so rows
		// share nothing.
		if len(r.Pity) == 0 {
			r.Pity = nil
		}
		if len(r.Gems) == 0 {
			r.Gems = nil
		}
		c := byID[charID]
		c.Items = append(c.Items, r)
		return nil
	})
	return err
}

// loadSkills fills the Skills of chars with one query.
func (s *Store) loadSkills(ctx context.Context, chars []Character) error {
	if len(chars) == 0 {
		return nil
	}
	byID := make(map[int64]*Character, len(chars))
	ids := make([]int64, len(chars))
	for i := range chars {
		byID[chars[i].ID] = &chars[i]
		ids[i] = chars[i].ID
	}
	rows, err := s.pool.Query(ctx, `SELECT character_id, skill_id, level FROM character_skills WHERE character_id = ANY($1)`, ids)
	if err != nil {
		return err
	}
	var (
		charID int64
		skill  string
		level  int
	)
	_, err = pgx.ForEachRow(rows, []any{&charID, &skill, &level}, func() error {
		byID[charID].Skills[skill] = level
		return nil
	})
	return err
}

func (s *Store) ListCharacters(ctx context.Context, accountID int64) ([]Character, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+charColumns+` FROM characters WHERE account_id = $1 ORDER BY slot`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list characters: %w", err)
	}
	chars, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Character, error) { return scanCharacter(r) })
	if err == nil {
		err = s.loadExtras(ctx, chars)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list characters: %w", err)
	}
	return chars, nil
}

func (s *Store) GetCharacter(ctx context.Context, accountID, charID int64) (Character, error) {
	c, err := scanCharacter(s.pool.QueryRow(ctx,
		`SELECT `+charColumns+` FROM characters WHERE id = $1 AND account_id = $2`, charID, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Character{}, ErrNotFound
	}
	if err == nil {
		chars := []Character{c}
		err = s.loadExtras(ctx, chars)
		c = chars[0]
	}
	if err == nil {
		err = s.loadAccount(ctx, &c)
	}
	if err != nil {
		return Character{}, fmt.Errorf("store: get character: %w", err)
	}
	return c, nil
}

func (s *Store) CreateCharacter(ctx context.Context, nc NewCharacter) (Character, error) {
	c, err := scanCharacter(s.pool.QueryRow(ctx,
		`INSERT INTO characters (account_id, slot, name, name_key, class, map_id, pos_x, pos_y)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+charColumns,
		nc.AccountID, nc.Slot, nc.Name, nc.NameKey, nc.Class, nc.MapID, nc.X, nc.Y))
	if constraint, ok := uniqueViolation(err); ok {
		switch constraint {
		case "characters_name_key_key":
			return Character{}, ErrNameTaken
		case "characters_account_slot_key":
			return Character{}, ErrSlotOccupied
		}
	}
	if err != nil {
		return Character{}, fmt.Errorf("store: create character: %w", err)
	}
	return c, nil
}

func (s *Store) DeleteCharacter(ctx context.Context, accountID, charID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM characters WHERE id = $1 AND account_id = $2`, charID, accountID)
	if err != nil {
		return fmt.Errorf("store: delete character: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveCharacterState writes position, level, EXP, points, HP, MP, skill
// points, Xu, skills, items and quests in one transaction. A missing character is not
// an error.
func (s *Store) SaveCharacterState(ctx context.Context, st CharState) error {
	if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return saveState(ctx, tx, st) }); err != nil {
		return fmt.Errorf("store: save character state: %w", err)
	}
	return nil
}

func saveState(ctx context.Context, tx pgx.Tx, st CharState) error {
	tag, err := tx.Exec(ctx,
		`UPDATE characters SET map_id = $2, pos_x = $3, pos_y = $4, level = $5, exp = $6,
		 str = $7, agi = $8, vit = $9, int = $10, free_points = $11,
		 hp = NULLIF($12::int, -1), mp = NULLIF($13::int, -1), skill_points = $14, xu = $15, pk_on = $16, skill_book_waiver = $17, updated_at = now() WHERE id = $1`,
		st.ID, st.MapID, st.X, st.Y, st.Level, st.Exp,
		st.Points.Str, st.Points.Agi, st.Points.Vit, st.Points.Int, st.FreePoints, st.HP, st.MP, st.SkillPoints, st.Xu, st.PkOn, st.SkillBookWaiver)
	if err != nil || tag.RowsAffected() == 0 {
		return err // a deleted character has no skills or items to replace
	}
	if err := saveSkills(ctx, tx, st); err != nil {
		return err
	}
	if err := saveItems(ctx, tx, st); err != nil {
		return err
	}
	if err := saveQuests(ctx, tx, st.ID, st.Quests); err != nil {
		return err
	}
	return saveAccount(ctx, tx, st.Account)
}

// saveAccount writes Lượng, the stash size and the stash rows; nil does nothing.
func saveAccount(ctx context.Context, tx pgx.Tx, a *AccountState) error {
	if a == nil {
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE accounts SET luong = $2, stash_size = $3 WHERE id = $1`, a.ID, a.Luong, a.StashSize)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM account_stash WHERE account_id = $1`, a.ID); err != nil {
		return err
	}
	return insertItems(ctx, tx, "account_stash", "account_id", a.ID, false, a.Stash)
}

// loadAccount fills c.Account: the account columns, then the stash rows.
func (s *Store) loadAccount(ctx context.Context, c *Character) error {
	a := AccountState{ID: c.AccountID, Stash: []ItemRow{}}
	if err := s.pool.QueryRow(ctx, `SELECT luong, stash_size FROM accounts WHERE id = $1`, c.AccountID).
		Scan(&a.Luong, &a.StashSize); err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `SELECT slot, code, count, quality, element, enhance, lines, hidden, pity, gems
		FROM account_stash WHERE account_id = $1 ORDER BY slot`, c.AccountID)
	if err != nil {
		return err
	}
	var r ItemRow
	_, err = pgx.ForEachRow(rows, []any{&r.Slot, &r.Code, &r.Count, &r.Quality, &r.Element,
		&r.Enhance, &r.Lines, &r.Hidden, &r.Pity, &r.Gems}, func() error {
		if len(r.Pity) == 0 {
			r.Pity = nil
		}
		if len(r.Gems) == 0 {
			r.Gems = nil
		}
		a.Stash = append(a.Stash, r)
		return nil
	})
	if err != nil {
		return err
	}
	c.Account = a
	return nil
}

func saveSkills(ctx context.Context, tx pgx.Tx, st CharState) error {
	if _, err := tx.Exec(ctx, `DELETE FROM character_skills WHERE character_id = $1`, st.ID); err != nil {
		return err
	}
	if len(st.Skills) == 0 {
		return nil
	}
	ids := slices.Sorted(maps.Keys(st.Skills))
	levels := make([]int32, len(ids))
	for i, id := range ids {
		levels[i] = int32(st.Skills[id])
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO character_skills (character_id, skill_id, level) SELECT $1, * FROM unnest($2::text[], $3::int[])`,
		st.ID, ids, levels)
	return err
}

// saveItems replaces the character's item rows: a delete, then one INSERT
// from unnest over column arrays, with the lines sent as JSON text and cast
// to jsonb per row and the pity and gems sent as array text and cast to
// smallint[] and text[] (unnest would flatten an array of arrays).
func saveItems(ctx context.Context, tx pgx.Tx, st CharState) error {
	if _, err := tx.Exec(ctx, `DELETE FROM character_items WHERE character_id = $1`, st.ID); err != nil {
		return err
	}
	return insertItems(ctx, tx, "character_items", "character_id", st.ID, true, st.Items)
}

// insertItems inserts rows into table (a constant: character_items or
// account_stash) under owner id, in one INSERT from unnest. withEquipped is
// false for the stash, which has no equipped column.
func insertItems(ctx context.Context, tx pgx.Tx, table, idCol string, id int64, withEquipped bool, items []ItemRow) error {
	if len(items) == 0 {
		return nil
	}
	n := len(items)
	var (
		equipped                             = make([]bool, n)
		slot, count, quality, enhance        = make([]int32, n), make([]int32, n), make([]int32, n), make([]int32, n)
		code, element, linesJSON, hiddenJSON = make([]string, n), make([]string, n), make([]string, n), make([]string, n)
		pity, gems                           = make([]string, n), make([]string, n)
	)
	for i, r := range items {
		equipped[i], code[i], element[i] = r.Equipped, r.Code, r.Element
		slot[i], count[i], quality[i], enhance[i] = int32(r.Slot), int32(r.Count), int32(r.Quality), int32(r.Enhance)
		var err error
		if linesJSON[i], err = linesText(r.Lines); err != nil {
			return err
		}
		if hiddenJSON[i], err = linesText(r.Hidden); err != nil {
			return err
		}
		pity[i], gems[i] = pityText(r.Pity), gemsText(r.Gems)
	}
	cols, sel := "slot", "s"
	if withEquipped {
		cols, sel = "equipped, slot", "e, s"
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO `+table+` (`+idCol+`, `+cols+`, code, count, quality, element, enhance, lines, hidden, pity, gems)
		 SELECT $1, `+sel+`, c, n, q, el, en, l::jsonb, h::jsonb, p::smallint[], g::text[]
		 FROM unnest($2::bool[], $3::int[], $4::text[], $5::int[], $6::int[], $7::text[], $8::int[], $9::text[], $10::text[],
		   $11::text[], $12::text[])
		   AS u(e, s, c, n, q, el, en, l, h, p, g)`,
		id, equipped, slot, code, count, quality, element, enhance, linesJSON, hiddenJSON, pity, gems)
	return err
}

// pityText is p as a Postgres array literal; nil is "{}".
func pityText(p []int) string {
	b := []byte{'{'}
	for i, v := range p {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendInt(b, int64(v), 10)
	}
	return string(append(b, '}'))
}

// gemsText is g as a Postgres array literal, every element quoted so an
// empty hole stays an empty string; nil is "{}".
func gemsText(g []string) string {
	b := []byte{'{'}
	for i, v := range g {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '"')
		for j := 0; j < len(v); j++ {
			if v[j] == '"' || v[j] == '\\' {
				b = append(b, '\\')
			}
			b = append(b, v[j])
		}
		b = append(b, '"')
	}
	return string(append(b, '}'))
}

// linesText is ls as a JSON array; nil is "[]".
func linesText(ls []ItemLine) (string, error) {
	if ls == nil {
		ls = []ItemLine{}
	}
	b, err := json.Marshal(ls)
	return string(b), err
}
