package items

import "demonslayer2d/internal/gamedata"

// SellValue is the Xu one unit of it sells for (phase 4b spec 4.3): an
// equipment piece sells for its tier times the Xu per tier of its quality
// (enhance level and gems do not change it); a stackable for its sell price.
// An unknown code, or a quality out of range, is worth 0.
func SellValue(d *gamedata.Items, it *Item) int64 {
	if e, ok := d.Equipment[it.Code]; ok {
		if it.Quality < 0 || it.Quality >= len(d.SellEquipmentXuPerTier) {
			return 0
		}
		return int64(e.Tier) * d.SellEquipmentXuPerTier[it.Quality]
	}
	return d.Stackables[it.Code].Sell
}

// TakeSlot removes count of the item in bagSlot and returns what was
// removed; count 0 takes the whole slot. Checks, in order, as Discard: slot
// range (ErrBadSlot), empty slot (ErrNotFound), count below 0 or above the
// slot's count (ErrBadCount). Take, the by-code remover, is a separate method.
func (inv *Inventory) TakeSlot(bagSlot, count int) (Item, error) {
	if bagSlot < 0 || bagSlot >= len(inv.Bag) {
		return Item{}, ErrBadSlot
	}
	it := inv.Bag[bagSlot]
	if it == nil {
		return Item{}, ErrNotFound
	}
	if count < 0 || count > it.Count {
		return Item{}, ErrBadCount
	}
	if count == 0 || count == it.Count {
		inv.Bag[bagSlot] = nil
		return *it, nil
	}
	it.Count -= count
	out := it.Clone()
	out.Count = count
	return out, nil
}

// HealAmount is what a potion restores before capping at the maximum
// (spec 4.4): hp_pct heals max(1, floor(maxHP * value / 100)); mp heals value.
func HealAmount(u gamedata.Use, maxHP, _ int) (hp, mp int) {
	switch u.Effect {
	case gamedata.EffectHPPct:
		return max(1, maxHP*u.Value/100), 0
	case gamedata.EffectMP:
		return 0, u.Value
	}
	return 0, 0
}
