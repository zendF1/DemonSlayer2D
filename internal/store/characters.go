package store

import (
	"context"
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
}

const charColumns = `id, account_id, slot, name, class, level, exp, map_id, pos_x, pos_y,
	str, agi, vit, int, free_points, COALESCE(hp, -1), COALESCE(mp, -1), skill_points`

// scanCharacter reads charColumns; the skills come from loadSkills.
func scanCharacter(row pgx.Row) (Character, error) {
	c := Character{Skills: map[string]int{}}
	err := row.Scan(&c.ID, &c.AccountID, &c.Slot, &c.Name, &c.Class, &c.Level, &c.Exp, &c.MapID, &c.X, &c.Y,
		&c.Points.Str, &c.Points.Agi, &c.Points.Vit, &c.Points.Int, &c.FreePoints, &c.HP, &c.MP, &c.SkillPoints)
	return c, err
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
		err = s.loadSkills(ctx, chars)
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
		err = s.loadSkills(ctx, []Character{c})
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
// points and skills in one transaction. A missing character is not an error.
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
		 hp = NULLIF($12::int, -1), mp = NULLIF($13::int, -1), skill_points = $14, updated_at = now() WHERE id = $1`,
		st.ID, st.MapID, st.X, st.Y, st.Level, st.Exp,
		st.Points.Str, st.Points.Agi, st.Points.Vit, st.Points.Int, st.FreePoints, st.HP, st.MP, st.SkillPoints)
	if err != nil || tag.RowsAffected() == 0 {
		return err // a deleted character has no skills to replace
	}
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
	_, err = tx.Exec(ctx,
		`INSERT INTO character_skills (character_id, skill_id, level) SELECT $1, * FROM unnest($2::text[], $3::int[])`,
		st.ID, ids, levels)
	return err
}
