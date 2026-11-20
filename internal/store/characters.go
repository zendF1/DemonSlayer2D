package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

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
}

const charColumns = `id, account_id, slot, name, class, level, exp, map_id, pos_x, pos_y,
	str, agi, vit, int, free_points, COALESCE(hp, -1), COALESCE(mp, -1), skill_points, xu`

// scanCharacter reads charColumns; the skills and items come from loadExtras.
func scanCharacter(row pgx.Row) (Character, error) {
	c := Character{Skills: map[string]int{}, Items: []ItemRow{}}
	err := row.Scan(&c.ID, &c.AccountID, &c.Slot, &c.Name, &c.Class, &c.Level, &c.Exp, &c.MapID, &c.X, &c.Y,
		&c.Points.Str, &c.Points.Agi, &c.Points.Vit, &c.Points.Int, &c.FreePoints, &c.HP, &c.MP, &c.SkillPoints, &c.Xu)
	return c, err
}

// loadExtras fills the skills and items of chars, one query each.
func (s *Store) loadExtras(ctx context.Context, chars []Character) error {
	if err := s.loadSkills(ctx, chars); err != nil {
		return err
	}
	return s.loadItems(ctx, chars)
}

// loadItems fills the Items of chars with one query, bag first, then by slot.
func (s *Store) loadItems(ctx context.Context, chars []Character) error {
	if len(chars) == 0 {
		return nil
	}
	byID := make(map[int64]*Character, len(chars))
	ids := make([]int64, len(chars))
	for i := range chars {
		byID[chars[i].ID] = &chars[i]
		ids[i] = chars[i].ID
	}
	rows, err := s.pool.Query(ctx, `SELECT character_id, equipped, slot, code, count, quality, element, enhance, lines, hidden
		FROM character_items WHERE character_id = ANY($1) ORDER BY character_id, equipped, slot`, ids)
	if err != nil {
		return err
	}
	var (
		charID int64
		r      ItemRow
	)
	_, err = pgx.ForEachRow(rows, []any{&charID, &r.Equipped, &r.Slot, &r.Code, &r.Count, &r.Quality, &r.Element,
		&r.Enhance, &r.Lines, &r.Hidden}, func() error {
		// pgx decodes each jsonb value into a fresh slice, so rows share nothing.
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
// points, Xu, skills and items in one transaction. A missing character is not
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
		 hp = NULLIF($12::int, -1), mp = NULLIF($13::int, -1), skill_points = $14, xu = $15, updated_at = now() WHERE id = $1`,
		st.ID, st.MapID, st.X, st.Y, st.Level, st.Exp,
		st.Points.Str, st.Points.Agi, st.Points.Vit, st.Points.Int, st.FreePoints, st.HP, st.MP, st.SkillPoints, st.Xu)
	if err != nil || tag.RowsAffected() == 0 {
		return err // a deleted character has no skills or items to replace
	}
	if err := saveSkills(ctx, tx, st); err != nil {
		return err
	}
	return saveItems(ctx, tx, st)
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
// to jsonb per row.
func saveItems(ctx context.Context, tx pgx.Tx, st CharState) error {
	if _, err := tx.Exec(ctx, `DELETE FROM character_items WHERE character_id = $1`, st.ID); err != nil {
		return err
	}
	if len(st.Items) == 0 {
		return nil
	}
	n := len(st.Items)
	var (
		equipped                             = make([]bool, n)
		slot, count, quality, enhance        = make([]int32, n), make([]int32, n), make([]int32, n), make([]int32, n)
		code, element, linesJSON, hiddenJSON = make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	)
	for i, r := range st.Items {
		equipped[i], code[i], element[i] = r.Equipped, r.Code, r.Element
		slot[i], count[i], quality[i], enhance[i] = int32(r.Slot), int32(r.Count), int32(r.Quality), int32(r.Enhance)
		var err error
		if linesJSON[i], err = linesText(r.Lines); err != nil {
			return err
		}
		if hiddenJSON[i], err = linesText(r.Hidden); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO character_items (character_id, equipped, slot, code, count, quality, element, enhance, lines, hidden)
		 SELECT $1, e, s, c, n, q, el, en, l::jsonb, h::jsonb
		 FROM unnest($2::bool[], $3::int[], $4::text[], $5::int[], $6::int[], $7::text[], $8::int[], $9::text[], $10::text[])
		   AS u(e, s, c, n, q, el, en, l, h)`,
		st.ID, equipped, slot, code, count, quality, element, enhance, linesJSON, hiddenJSON)
	return err
}

// linesText is ls as a JSON array; nil is "[]".
func linesText(ls []ItemLine) (string, error) {
	if ls == nil {
		ls = []ItemLine{}
	}
	b, err := json.Marshal(ls)
	return string(b), err
}
