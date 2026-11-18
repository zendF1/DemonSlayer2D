package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
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
}

const charColumns = `id, account_id, slot, name, class, level, exp, map_id, pos_x, pos_y`

func scanCharacter(row pgx.Row) (Character, error) {
	var c Character
	err := row.Scan(&c.ID, &c.AccountID, &c.Slot, &c.Name, &c.Class, &c.Level, &c.Exp, &c.MapID, &c.X, &c.Y)
	return c, err
}

func (s *Store) ListCharacters(ctx context.Context, accountID int64) ([]Character, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+charColumns+` FROM characters WHERE account_id = $1 ORDER BY slot`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list characters: %w", err)
	}
	chars, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Character, error) { return scanCharacter(r) })
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

// SaveCharacterState writes position, level and EXP. A missing character is not an error.
func (s *Store) SaveCharacterState(ctx context.Context, st CharState) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE characters SET map_id = $2, pos_x = $3, pos_y = $4, level = $5, exp = $6, updated_at = now() WHERE id = $1`,
		st.ID, st.MapID, st.X, st.Y, st.Level, st.Exp)
	if err != nil {
		return fmt.Errorf("store: save character state: %w", err)
	}
	return nil
}
