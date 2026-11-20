package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"demonslayer2d/internal/combat"
)

// DevResult is what DevSetLevel left the character with.
type DevResult struct {
	Name                           string
	Level, FreePoints, SkillPoints int
}

// DevSetLevel puts the first character (lowest slot) of username at level,
// for development only (phase 2 spec 2.3). In one transaction it sets EXP to
// 0, refunds every spent potential point (free points become 10 × (level −
// 1)), sets the skill points to (level − 1) minus the learned skill levels,
// or deletes every learned skill and sets them to level − 1 when that is
// negative, and fills HP and MP. The character must be offline: a running
// server's next save of it overwrites the change.
func (s *Store) DevSetLevel(ctx context.Context, username string, level int) (DevResult, error) {
	if level < 1 || level > combat.MaxLevel {
		return DevResult{}, fmt.Errorf("store: dev set level: level %d is outside 1-%d", level, combat.MaxLevel)
	}
	var res DevResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		charID, name, err := lockFirstCharacter(ctx, tx, username)
		if err != nil {
			return err
		}
		res.Name = name
		var spent int
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(level), 0) FROM character_skills WHERE character_id = $1`,
			charID).Scan(&spent); err != nil {
			return err
		}
		earned := level - 1
		res.Level, res.FreePoints, res.SkillPoints = level, combat.PointsPerLevel*earned, earned-spent
		if res.SkillPoints < 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM character_skills WHERE character_id = $1`, charID); err != nil {
				return err
			}
			res.SkillPoints = earned
		}
		_, err = tx.Exec(ctx,
			`UPDATE characters SET level = $2, exp = 0, str = 0, agi = 0, vit = 0, int = 0, free_points = $3,
			 skill_points = $4, hp = NULL, mp = NULL, updated_at = now() WHERE id = $1`,
			charID, res.Level, res.FreePoints, res.SkillPoints)
		return err
	})
	if err != nil {
		return DevResult{}, fmt.Errorf("store: dev set level: %w", err)
	}
	return res, nil
}

// lockFirstCharacter finds the first character (lowest slot) of username and
// locks its row for the rest of tx.
func lockFirstCharacter(ctx context.Context, tx pgx.Tx, username string) (id int64, name string, err error) {
	var accountID int64
	err = tx.QueryRow(ctx, `SELECT id FROM accounts WHERE username = $1`, username).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", fmt.Errorf("no account %q: %w", username, ErrNotFound)
	}
	if err != nil {
		return 0, "", err
	}
	err = tx.QueryRow(ctx,
		`SELECT id, name FROM characters WHERE account_id = $1 ORDER BY slot LIMIT 1 FOR UPDATE`,
		accountID).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", fmt.Errorf("account %q has no character: %w", username, ErrNotFound)
	}
	return id, name, err
}

// DevItemResult is what DevGiveItem left the character with.
type DevItemResult struct {
	Name string
	Xu   int64
}

// DevGiveItem changes the items and Xu of the first character (lowest slot)
// of username, for development only (phase 3a spec 7). In one transaction it
// locks the character, loads its item rows (bag first, then worn, each by
// slot) and, when place is not nil, replaces them all with the list place
// returns; then it adds xu (0 or more) to the character's Xu. The item rules
// (which slot, merging stacks, rolling lines) are the caller's, so place
// returns the full new list rather than one row. An error from place aborts
// the transaction: nothing is written. The character must be offline: a
// running server's next save of it overwrites the change.
func (s *Store) DevGiveItem(ctx context.Context, username string, place func(rows []ItemRow) ([]ItemRow, error), xu int64) (DevItemResult, error) {
	if xu < 0 {
		return DevItemResult{}, fmt.Errorf("store: dev give item: xu %d is negative", xu)
	}
	var res DevItemResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		charID, name, err := lockFirstCharacter(ctx, tx, username)
		if err != nil {
			return err
		}
		res.Name = name
		if place != nil {
			chars := []Character{{ID: charID, Items: []ItemRow{}}}
			if err := loadItems(ctx, tx, chars); err != nil {
				return err
			}
			rows, err := place(chars[0].Items)
			if err != nil {
				return err
			}
			if err := saveItems(ctx, tx, CharState{ID: charID, Items: rows}); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, `UPDATE characters SET xu = xu + $2, updated_at = now() WHERE id = $1 RETURNING xu`,
			charID, xu).Scan(&res.Xu)
	})
	if err != nil {
		return DevItemResult{}, fmt.Errorf("store: dev give item: %w", err)
	}
	return res, nil
}
