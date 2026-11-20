package items

import (
	"errors"
	"math/rand/v2"
	"slices"

	"demonslayer2d/internal/gamedata"
)

// Errors of enhancement; the zone maps them to pb.ErrorCode.
var (
	ErrMaxed       = errors.New("items: already at the highest enhance level")
	ErrNoMaterials = errors.New("items: not enough stones or charms")
	ErrNoXu        = errors.New("items: not enough xu")
)

// Ref is an item in the bag (Equipped false, Slot a bag slot) or worn
// (Slot a worn slot index).
type Ref struct {
	Equipped bool
	Slot     int
}

// At is the item r points to: ErrBadSlot when the slot is out of range,
// ErrNotFound when it is empty.
func (inv *Inventory) At(r Ref) (*Item, error) {
	if r.Equipped {
		if r.Slot < 0 || r.Slot >= EquipSlots {
			return nil, ErrBadSlot
		}
		if it := inv.Worn[r.Slot]; it != nil {
			return it, nil
		}
		return nil, ErrNotFound
	}
	if r.Slot < 0 || r.Slot >= len(inv.Bag) {
		return nil, ErrBadSlot
	}
	if it := inv.Bag[r.Slot]; it != nil {
		return it, nil
	}
	return nil, ErrNotFound
}

// Count is how many of code the bag holds, over all its stacks.
func (inv *Inventory) Count(code string) int {
	n := 0
	for _, it := range inv.Bag {
		if it != nil && it.Code == code {
			n += it.Count
		}
	}
	return n
}

// Take removes n of code from the bag's stacks, lowest slots first; a stack
// taken to 0 leaves its slot empty. The caller checks Count first. changed
// lists the bag slots that changed, ascending.
func (inv *Inventory) Take(code string, n int) (changed []int) {
	for i, it := range inv.Bag {
		if n <= 0 {
			break
		}
		if it == nil || it.Code != code {
			continue
		}
		k := min(n, it.Count)
		if it.Count -= k; it.Count == 0 {
			inv.Bag[i] = nil
		}
		n -= k
		changed = append(changed, i)
	}
	return changed
}

// EnhanceResult is the outcome of one enhancement request: how many attempts
// were made, whether the last one succeeded, the item's final level and the
// bag slots that changed, ascending (the materials taken and, for a bag item,
// its own slot).
type EnhanceResult struct {
	Attempts int
	Success  bool
	Level    int
	Changed  []int
}

// Enhance makes up to times attempts (0 means 1) at raising the item r points
// to by one level (phase 3b spec 4.1), paying each attempt's stones and Xu
// from the bag and *xu, and a charm when protect is set and the target level
// can drop. Checks, in order, the first failing one returned with nothing
// changed: ErrBadSlot (r out of range), ErrBadCount (times below 0 or above
// max_times), ErrNotFound, ErrNotEquipment, ErrMaxed, ErrNoMaterials (stones,
// or a charm when one would be taken), ErrNoXu. The dead and safe-zone checks
// are the zone's.
//
// Each attempt rolls rng.IntN(100) once and succeeds below success_pct plus
// the item's pity for the target level. A success raises the level and
// clears that pity; a failure adds pity_pct to it (capped where the rate
// reaches 100) and, at a target that drops without a charm, lowers the level
// by one. Attempts go on only after a failure that kept the level, while the
// next one is affordable.
func (inv *Inventory) Enhance(rng *rand.Rand, d *gamedata.Items, r Ref, protect bool, times int, xu *int64) (EnhanceResult, error) {
	if r.Equipped && (r.Slot < 0 || r.Slot >= EquipSlots) || !r.Equipped && (r.Slot < 0 || r.Slot >= len(inv.Bag)) {
		return EnhanceResult{}, ErrBadSlot
	}
	if times < 0 || times > d.Enhance.MaxTimes {
		return EnhanceResult{}, ErrBadCount
	}
	times = max(times, 1)
	it, err := inv.At(r)
	if err != nil {
		return EnhanceResult{}, err
	}
	e, ok := d.Equipment[it.Code]
	if !ok {
		return EnhanceResult{}, ErrNotEquipment
	}
	// A level outside 0-14 (never stored: the database checks 0-15) is
	// refused rather than indexing out of range.
	if it.Enhance < 0 || it.Enhance >= MaxEnhance {
		return EnhanceResult{}, ErrMaxed
	}
	// Until the loop ends the level is kept, so every attempt has the same
	// target, cost and charm use.
	n := it.Enhance
	lvl := d.Enhance.Levels[n]
	charm := protect && lvl.Drop
	cost := int64(lvl.XuPerTier) * int64(e.Tier)
	afford := func() error {
		if inv.Count(gamedata.StoneEnhance) < lvl.Stones || charm && inv.Count(gamedata.CharmProtect) < 1 {
			return ErrNoMaterials
		}
		if *xu < cost {
			return ErrNoXu
		}
		return nil
	}
	if err := afford(); err != nil {
		return EnhanceResult{}, err
	}

	var res EnhanceResult
	changed := map[int]bool{}
	if !r.Equipped {
		changed[r.Slot] = true
	}
	for {
		for _, s := range inv.Take(gamedata.StoneEnhance, lvl.Stones) {
			changed[s] = true
		}
		if charm {
			for _, s := range inv.Take(gamedata.CharmProtect, 1) {
				changed[s] = true
			}
		}
		*xu -= cost
		res.Attempts++
		if rng.IntN(100) < min(100, lvl.SuccessPct+it.Pity[n]) {
			it.Enhance, it.Pity[n], res.Success = n+1, 0, true
			break
		}
		it.Pity[n] = min(it.Pity[n]+d.Enhance.PityPct, 100-lvl.SuccessPct)
		if lvl.Drop && !charm {
			it.Enhance = n - 1
			break
		}
		if res.Attempts >= times || afford() != nil {
			break
		}
	}
	res.Level = it.Enhance
	for s := range changed {
		res.Changed = append(res.Changed, s)
	}
	slices.Sort(res.Changed)
	return res, nil
}

// Milestone is the highest regen step at or below enhance, false below the
// first step (+7).
func Milestone(d *gamedata.Items, enhance int) (step gamedata.RegenStep, ok bool) {
	for _, s := range d.Regen {
		if s.Enhance <= enhance {
			step, ok = s, true
		}
	}
	return step, ok
}

// Regen sums the per-second regen of the wearable worn items (phase 3b spec
// 4.2): hpBP, in 1/10000 of max HP, from the pieces in HP slots and mp from
// those in MP slots, each at its highest milestone. Like GearBonus it skips a
// piece that is not wearable or sits in another piece's slot.
func Regen(d *gamedata.Items, inv *Inventory, level int, class string) (hpBP, mp int) {
	for slot, it := range inv.Worn {
		if it == nil || !Wearable(d, it, level, class) || d.Equipment[it.Code].SlotIndex != slot {
			continue
		}
		s, ok := Milestone(d, it.Enhance)
		if !ok {
			continue
		}
		switch d.Slots[slot].Regen {
		case gamedata.RegenHP:
			hpBP += s.HPBP
		case gamedata.RegenMP:
			mp += s.MP
		}
	}
	return hpBP, mp
}
