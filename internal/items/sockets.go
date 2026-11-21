package items

import (
	"errors"
	"slices"

	"demonslayer2d/internal/gamedata"
)

// Errors of punching, socketing and refining; the zone maps them to
// pb.ErrorCode (ErrBadRequest to BAD_REQUEST).
var (
	ErrHolesMaxed = errors.New("items: no more holes can be punched")
	ErrGemTooHigh = errors.New("items: gem level above the item tier")
	ErrBadRequest = errors.New("items: bad hole, gem or lock")
)

// inRange reports whether r points inside the bag or the worn slots.
func (inv *Inventory) inRange(r Ref) bool {
	if r.Equipped {
		return r.Slot >= 0 && r.Slot < EquipSlots
	}
	return r.Slot >= 0 && r.Slot < len(inv.Bag)
}

// equipmentAt is the equipment piece r points to and its data: ErrBadSlot,
// ErrNotFound or ErrNotEquipment otherwise.
func (inv *Inventory) equipmentAt(d *gamedata.Items, r Ref) (*Item, gamedata.Equipment, error) {
	it, err := inv.At(r)
	if err != nil {
		return nil, gamedata.Equipment{}, err
	}
	e, ok := d.Equipment[it.Code]
	if !ok {
		return nil, gamedata.Equipment{}, ErrNotEquipment
	}
	return it, e, nil
}

// withOwnSlot adds r's bag slot (none for a worn item) to changed, keeping it
// ascending and without repeats.
func withOwnSlot(r Ref, changed []int) []int {
	if !r.Equipped && !slices.Contains(changed, r.Slot) {
		changed = append(changed, r.Slot)
		slices.Sort(changed)
	}
	return changed
}

// Punch adds one empty hole to the piece r points to (phase 3c spec 4.2),
// paying punch_xu_per_tier[holes so far] times its tier from *xu. Checks, in
// order, the first failing one returned with nothing changed: ErrBadSlot,
// ErrNotFound, ErrNotEquipment, ErrHolesMaxed, ErrNoXu. The dead and
// safe-zone checks are the zone's. changed is r's bag slot, if any.
func (inv *Inventory) Punch(d *gamedata.Items, r Ref, xu *int64) (changed []int, err error) {
	it, e, err := inv.equipmentAt(d, r)
	if err != nil {
		return nil, err
	}
	n := len(it.Gems)
	if n >= d.Sockets.Max {
		return nil, ErrHolesMaxed
	}
	cost := int64(d.Sockets.PunchXuPerTier[n]) * int64(e.Tier)
	if *xu < cost {
		return nil, ErrNoXu
	}
	*xu -= cost
	it.Gems = append(it.Gems, "")
	return withOwnSlot(r, nil), nil
}

// Socket moves one gem from bag slot gemSlot into the empty hole of the
// piece r points to (phase 3c spec 4.3); it costs no Xu. Checks, in order,
// the first failing one returned with nothing changed: ErrBadSlot (r or
// gemSlot out of range), ErrNotFound (r or gemSlot empty), ErrNotEquipment,
// ErrBadRequest (gemSlot holds no gem, hole outside the holes or filled),
// ErrGemTooHigh (gem level above the tier / 10). changed lists the gem's
// slot and r's bag slot, ascending.
func (inv *Inventory) Socket(d *gamedata.Items, r Ref, hole, gemSlot int) (changed []int, err error) {
	if !inv.inRange(r) || gemSlot < 0 || gemSlot >= len(inv.Bag) {
		return nil, ErrBadSlot
	}
	it, err := inv.At(r)
	if err != nil {
		return nil, err
	}
	g := inv.Bag[gemSlot]
	if g == nil {
		return nil, ErrNotFound
	}
	e, ok := d.Equipment[it.Code]
	if !ok {
		return nil, ErrNotEquipment
	}
	gd := d.Stackables[g.Code].Gem
	if gd == nil || hole < 0 || hole >= len(it.Gems) || it.Gems[hole] != "" {
		return nil, ErrBadRequest
	}
	if gd.Level > e.Tier/gamedata.TierStep {
		return nil, ErrGemTooHigh
	}
	it.Gems[hole] = g.Code
	if g.Count--; g.Count <= 0 {
		inv.Bag[gemSlot] = nil
	}
	return withOwnSlot(r, []int{gemSlot}), nil
}

// Unsocket returns the gem in hole of the piece r points to to the bag
// (phase 3c spec 4.4), paying unsocket_xu_per_level times the gem level from
// *xu; the hole stays, empty. Checks, in order, the first failing one
// returned with nothing changed: ErrBadSlot, ErrNotFound, ErrNotEquipment,
// ErrBadRequest (hole outside the holes or empty, or not a gem), ErrBagFull
// (the bag cannot take the gem by the Add rule), ErrNoXu. changed lists the
// bag slots the gem went to and r's bag slot, ascending.
func (inv *Inventory) Unsocket(d *gamedata.Items, r Ref, hole int, xu *int64) (changed []int, err error) {
	it, _, err := inv.equipmentAt(d, r)
	if err != nil {
		return nil, err
	}
	if hole < 0 || hole >= len(it.Gems) {
		return nil, ErrBadRequest
	}
	code := it.Gems[hole]
	g := d.Stackables[code].Gem
	if g == nil {
		return nil, ErrBadRequest
	}
	plan, err := planStack(d, inv.Bag, code, 1)
	if err != nil {
		return nil, err
	}
	cost := int64(d.Sockets.UnsocketXuPerLevel) * int64(g.Level)
	if *xu < cost {
		return nil, ErrNoXu
	}
	*xu -= cost
	it.Gems[hole] = ""
	return withOwnSlot(r, applyStack(inv.Bag, code, plan)), nil
}
