package items

import (
	"errors"
	"math/rand/v2"
	"slices"

	"demonslayer2d/internal/gamedata"
)

// ErrNotRefinable is a piece below the refine quality (Trắng, Xanh).
var ErrNotRefinable = errors.New("items: quality too low to refine")

// RefineRoll pays for one refine of the piece r points to and returns its
// candidate sub-lines (phase 3c spec 4.5), leaving the piece as it is: the
// zone keeps the result until the player keeps or takes it (RefineApply).
// locked are the indices of the lines to keep. Checks, in order, the first
// failing one returned with nothing changed: ErrBadSlot, ErrNotFound,
// ErrNotEquipment, ErrNotRefinable (quality below min_quality),
// ErrBadRequest (a lock outside the lines or repeated, or every line
// locked), ErrNoMaterials (stones + stones_per_lock x locks refine stones),
// ErrNoXu ((xu_per_tier + xu_per_tier_per_lock x locks) x tier). The dead and
// safe-zone checks are the zone's.
//
// Locked lines keep their stat, value and index. Each unlocked index, in
// order, draws a stat uniformly from the line stats not yet in the set (the
// locked lines and the lines drawn before it) and a value as a dropped
// line's (NewEquipment). changed lists the bag slots the stones came from.
func (inv *Inventory) RefineRoll(rng *rand.Rand, d *gamedata.Items, r Ref, locked []int, xu *int64) (lines []Line, changed []int, err error) {
	it, e, err := inv.equipmentAt(d, r)
	if err != nil {
		return nil, nil, err
	}
	if it.Quality < d.Refine.MinQuality {
		return nil, nil, ErrNotRefinable
	}
	n := len(it.Lines)
	keep := make([]bool, n)
	for _, i := range locked {
		if i < 0 || i >= n || keep[i] {
			return nil, nil, ErrBadRequest
		}
		keep[i] = true
	}
	// More lines than line stats (never stored: drops and refining keep the
	// stats distinct) would leave a line with nothing to draw.
	if len(locked) >= n || n > len(d.LineStats) {
		return nil, nil, ErrBadRequest
	}
	k := len(locked)
	stones := d.Refine.Stones + d.Refine.StonesPerLock*k
	if inv.Count(gamedata.StoneRefine) < stones {
		return nil, nil, ErrNoMaterials
	}
	cost := int64(d.Refine.XuPerTier+d.Refine.XuPerTierPerLock*k) * int64(e.Tier)
	if *xu < cost {
		return nil, nil, ErrNoXu
	}
	changed = inv.Take(gamedata.StoneRefine, stones)
	*xu -= cost

	std := d.Tiers[e.Tier].LineStandard
	used := map[string]bool{}
	for _, i := range locked {
		used[it.Lines[i].Stat] = true
	}
	lines = slices.Clone(it.Lines)
	for i := range lines {
		if keep[i] {
			continue
		}
		var pool []string
		for _, s := range d.LineStats {
			if !used[s] {
				pool = append(pool, s)
			}
		}
		stat := pool[rng.IntN(len(pool))]
		used[stat] = true
		lines[i] = Line{Stat: stat, Value: rollLineValue(rng, d, std[stat])}
	}
	return lines, changed, nil
}

// RefineApply gives the piece r points to the sub-lines a RefineRoll
// returned; nothing else of it changes. ErrBadSlot or ErrNotFound for r, and
// ErrBadRequest when the line count differs (the slot no longer holds the
// rolled piece).
func (inv *Inventory) RefineApply(r Ref, lines []Line) error {
	it, err := inv.At(r)
	if err != nil {
		return err
	}
	if len(lines) != len(it.Lines) {
		return ErrBadRequest
	}
	it.Lines = slices.Clone(lines)
	return nil
}
