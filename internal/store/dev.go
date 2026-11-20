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
		var accountID int64
		err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE username = $1`, username).Scan(&accountID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no account %q: %w", username, ErrNotFound)
		}
		if err != nil {
			return err
		}
		var charID int64
		err = tx.QueryRow(ctx,
			`SELECT id, name FROM characters WHERE account_id = $1 ORDER BY slot LIMIT 1 FOR UPDATE`,
			accountID).Scan(&charID, &res.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("account %q has no character: %w", username, ErrNotFound)
		}
		if err != nil {
			return err
		}
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
