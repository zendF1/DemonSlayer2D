package items

import (
	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
)

// HiddenActive reports which hidden lines of the worn items are lit (phase
// 3c spec 4.1): line j of the piece X in slot s lights when the piece Y in
// slot Resonance.Slots[s][j] exists, both count (see counts) and Y's element
// is the activator of X's. X needs an element and both hidden lines.
// GearBonus adds the lit lines; the result is recomputed on every call.
func HiddenActive(d *gamedata.Items, inv *Inventory, level int, class string) [EquipSlots][HiddenLines]bool {
	var lit [EquipSlots][HiddenLines]bool
	for s, x := range inv.Worn {
		if !counts(d, x, s, level, class) || len(x.Hidden) != HiddenLines {
			continue
		}
		want := d.Resonance.Activator[x.Element]
		if want == "" {
			continue // no element (or an unknown one)
		}
		for j, by := range d.Resonance.Slots[s] {
			y := inv.Worn[by]
			lit[s][j] = counts(d, y, by, level, class) && y.Element == want
		}
	}
	return lit
}

// counts reports whether the worn item in slot gives stats: it is there, the
// character may wear it, and it is a piece of that slot (not, through bad
// data, another slot's piece).
func counts(d *gamedata.Items, it *Item, slot, level int, class string) bool {
	return it != nil && Wearable(d, it, level, class) && d.Equipment[it.Code].SlotIndex == slot
}

// gemBonus sums the stats of the gems socketed in gems; empty holes and
// codes that are not gems add nothing.
func gemBonus(d *gamedata.Items, gems []string) combat.Bonus {
	var b combat.Bonus
	for _, code := range gems {
		g := d.Stackables[code].Gem
		if g == nil {
			continue
		}
		for stat, v := range g.Stats {
			b = b.Add(lineBonus(Line{Stat: stat, Value: v}))
		}
	}
	return b
}
