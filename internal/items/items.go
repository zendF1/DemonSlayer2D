// Package items holds the item model and its rules (phase 3a spec 4): rolling
// drops and lines, the bag and worn slots, and the stats worn gear gives. It
// is pure: no I/O, and every random choice goes through the caller's RNG.
//
// Classes and elements are the lower-case ids ("viem" ... "phong"); the zone
// converts from pb.CharClass with the classes package.
package items

import (
	"errors"
	"slices"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
)

// Errors of the bag and equip operations; the zone maps them to pb.ErrorCode.
var (
	ErrBagFull      = errors.New("items: bag full")
	ErrNotFound     = errors.New("items: no such item")
	ErrBadSlot      = errors.New("items: slot out of range")
	ErrNotEquipment = errors.New("items: not equipment")
	ErrWrongClass   = errors.New("items: weapon of another class")
	ErrLevelTooLow  = errors.New("items: level too low")
	ErrBadCount     = errors.New("items: bad count")
)

// EquipSlots is the number of worn slots.
const EquipSlots = len(gamedata.SlotIDs)

// HiddenLines is how many hidden lines a piece with an element has.
const HiddenLines = 2

// Elements are the five elements a Vàng or better piece can have.
var Elements = [5]string{"viem", "thuy", "minh", "nham", "phong"}

// Line is an item line: a stat and how much it adds.
type Line struct {
	Stat  string
	Value int
}

// MaxEnhance is the highest enhance level.
const MaxEnhance = gamedata.EnhanceLevels - 1

// Item is one bag or worn item, or a stack. Count is 1 for equipment;
// stackables have no quality, element, enhance, lines or pity. Element is ""
// for none. Lines keep their roll order; Hidden is empty or HiddenLines long.
// Pity[k] is the hidden success bonus, in percentage points, of the attempt
// at target level +k+1 (phase 3b spec 4.1); it is never sent to clients.
type Item struct {
	Code          string
	Count         int
	Quality       int
	Element       string
	Enhance       int
	Lines, Hidden []Line
	Pity          [MaxEnhance]int
}

// Clone returns a deep copy of it (Pity is an array, copied by value).
func (it Item) Clone() Item {
	it.Lines = slices.Clone(it.Lines)
	it.Hidden = slices.Clone(it.Hidden)
	return it
}

// Inventory is a player's bag and worn items; a nil entry is an empty slot.
// Worn is indexed by slot index (gamedata.SlotIDs).
type Inventory struct {
	Bag  []*Item
	Worn [EquipSlots]*Item
}

// NewInventory returns an empty inventory with bagSize bag slots.
func NewInventory(bagSize int) *Inventory {
	return &Inventory{Bag: make([]*Item, bagSize)}
}

// Clone returns a deep copy of inv.
func (inv *Inventory) Clone() *Inventory {
	c := &Inventory{Bag: make([]*Item, len(inv.Bag))}
	for i, it := range inv.Bag {
		c.Bag[i] = clonePtr(it)
	}
	for i, it := range inv.Worn {
		c.Worn[i] = clonePtr(it)
	}
	return c
}

func clonePtr(it *Item) *Item {
	if it == nil {
		return nil
	}
	c := it.Clone()
	return &c
}

// Wearable reports whether a character of level and class may wear it and
// get its stats: it is equipment, the level is enough and a weapon is of the
// character's class (spec 4.2).
func Wearable(d *gamedata.Items, it *Item, level int, class string) bool {
	e, ok := d.Equipment[it.Code]
	return ok && level >= e.Level && (e.Class == "" || e.Class == class)
}

// GearBonus sums the stats of the wearable worn items: each piece's base
// stats at its enhance level plus its lines (spec 4.2). A worn item that is
// not wearable (level too low after devchar, another class) or sits in
// another piece's slot adds nothing.
func GearBonus(d *gamedata.Items, inv *Inventory, level int, class string) combat.Bonus {
	var b combat.Bonus
	for slot, it := range inv.Worn {
		if it == nil || !Wearable(d, it, level, class) {
			continue
		}
		e := d.Equipment[it.Code]
		if e.SlotIndex != slot {
			continue
		}
		n := min(max(it.Enhance, 0), gamedata.EnhanceLevels-1)
		b = b.Add(combat.Bonus{HP: e.HP[n], PhysAttack: e.PhysAttack[n], MagicAttack: e.MagicAttack[n],
			PhysDefense: e.PhysDefense[n], MagicDefense: e.MagicDefense[n]})
		for _, l := range it.Lines {
			b = b.Add(lineBonus(l))
		}
		for _, l := range activeHidden(inv, slot) {
			b = b.Add(lineBonus(l))
		}
	}
	return b
}

// activeHidden returns the hidden lines of the worn item in slot that count
// towards its stats. Hidden lines are stored from 3a but do nothing until
// resonance (phase 3c) turns them on here.
func activeHidden(_ *Inventory, _ int) []Line { return nil }

func lineBonus(l Line) combat.Bonus {
	switch l.Stat {
	case gamedata.StatHP:
		return combat.Bonus{HP: l.Value}
	case gamedata.StatPhysAttack:
		return combat.Bonus{PhysAttack: l.Value}
	case gamedata.StatMagicAttack:
		return combat.Bonus{MagicAttack: l.Value}
	case gamedata.StatPhysDefense:
		return combat.Bonus{PhysDefense: l.Value}
	case gamedata.StatMagicDefense:
		return combat.Bonus{MagicDefense: l.Value}
	}
	return combat.Bonus{}
}
