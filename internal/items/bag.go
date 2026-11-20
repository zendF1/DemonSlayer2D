package items

import (
	"cmp"
	"slices"

	"demonslayer2d/internal/gamedata"
)

// FirstFree is the first empty bag slot, or -1 when the bag is full.
func (inv *Inventory) FirstFree() int {
	return slices.Index(inv.Bag, nil)
}

// Add puts a copy of it into the bag (spec 4.1): a stack first tops up the
// stacks of the same code in slot order, then fills empty slots; equipment
// takes the first empty slot. It is all or nothing: when the bag cannot hold
// everything, nothing changes and the error is ErrBagFull. changed lists the
// bag slots that changed, ascending.
func (inv *Inventory) Add(d *gamedata.Items, it Item) (changed []int, err error) {
	if it.Count < 1 {
		return nil, ErrBadCount
	}
	if d.IsEquipment(it.Code) {
		if it.Count != 1 {
			return nil, ErrBadCount
		}
		slot := inv.FirstFree()
		if slot < 0 {
			return nil, ErrBagFull
		}
		c := it.Clone()
		inv.Bag[slot] = &c
		return []int{slot}, nil
	}
	if _, ok := d.Stackables[it.Code]; !ok {
		return nil, ErrNotFound
	}

	// Plan every slot's new count first, then apply.
	type put struct{ slot, count int }
	var plan []put
	left := it.Count
	for i, b := range inv.Bag {
		if left == 0 {
			break
		}
		if b != nil && b.Code == it.Code && b.Count < d.StackMax {
			n := min(left, d.StackMax-b.Count)
			plan = append(plan, put{i, b.Count + n})
			left -= n
		}
	}
	for i, b := range inv.Bag {
		if left == 0 {
			break
		}
		if b == nil {
			n := min(left, d.StackMax)
			plan = append(plan, put{i, n})
			left -= n
		}
	}
	if left > 0 {
		return nil, ErrBagFull
	}
	for _, p := range plan {
		if inv.Bag[p.slot] == nil {
			inv.Bag[p.slot] = &Item{Code: it.Code}
		}
		inv.Bag[p.slot].Count = p.count
		changed = append(changed, p.slot)
	}
	slices.Sort(changed)
	return changed, nil
}

// Equip wears the piece in bagSlot, swapping with whatever is worn in its
// slot (spec 4.7). Checks, in order: slot range, empty slot, not equipment,
// weapon of another class, level too low. It returns the worn slot.
func (inv *Inventory) Equip(d *gamedata.Items, bagSlot, level int, class string) (equipSlot int, err error) {
	if bagSlot < 0 || bagSlot >= len(inv.Bag) {
		return 0, ErrBadSlot
	}
	it := inv.Bag[bagSlot]
	if it == nil {
		return 0, ErrNotFound
	}
	e, ok := d.Equipment[it.Code]
	if !ok {
		return 0, ErrNotEquipment
	}
	if e.Class != "" && e.Class != class {
		return 0, ErrWrongClass
	}
	if level < e.Level {
		return 0, ErrLevelTooLow
	}
	inv.Bag[bagSlot], inv.Worn[e.SlotIndex] = inv.Worn[e.SlotIndex], it
	return e.SlotIndex, nil
}

// Unequip moves the worn item in equipSlot to the first empty bag slot.
// Checks, in order: slot range, empty slot, full bag.
func (inv *Inventory) Unequip(equipSlot int) (bagSlot int, err error) {
	if equipSlot < 0 || equipSlot >= EquipSlots {
		return 0, ErrBadSlot
	}
	if inv.Worn[equipSlot] == nil {
		return 0, ErrNotFound
	}
	bagSlot = inv.FirstFree()
	if bagSlot < 0 {
		return 0, ErrBagFull
	}
	inv.Bag[bagSlot], inv.Worn[equipSlot] = inv.Worn[equipSlot], nil
	return bagSlot, nil
}

// Discard destroys count of the item in bagSlot; count 0 destroys the whole
// slot. Checks, in order: slot range, empty slot, count below 0 or above the
// slot's count.
func (inv *Inventory) Discard(bagSlot, count int) error {
	if bagSlot < 0 || bagSlot >= len(inv.Bag) {
		return ErrBadSlot
	}
	it := inv.Bag[bagSlot]
	if it == nil {
		return ErrNotFound
	}
	if count < 0 || count > it.Count {
		return ErrBadCount
	}
	if count == 0 || count == it.Count {
		inv.Bag[bagSlot] = nil
		return nil
	}
	it.Count -= count
	return nil
}

// Sort merges stacks of the same code (each up to stack_max) and lays the bag
// out from slot 0 (spec 4.7): equipment first, by tier descending, slot index,
// quality descending, enhance descending, then code; then stacks in the
// stackables' file order, larger first. Ties keep their bag order. A code in
// neither list (not expected: the world drops them at login) stays in its
// slot and the rest fill the other slots. If the result would not fit (only
// possible with a stack above stack_max), the bag is left as it was: Sort
// never deletes an item.
func (inv *Inventory) Sort(d *gamedata.Items) {
	var equip []*Item
	totals := map[string]int{}
	free := 0 // slots not held by an unknown code
	for _, it := range inv.Bag {
		switch {
		case it == nil:
		case d.IsEquipment(it.Code):
			equip = append(equip, it)
		case isStackable(d, it.Code):
			totals[it.Code] += it.Count
		default:
			continue
		}
		free++
	}
	slices.SortStableFunc(equip, func(a, b *Item) int {
		ea, eb := d.Equipment[a.Code], d.Equipment[b.Code]
		return cmp.Or(
			cmp.Compare(eb.Tier, ea.Tier),
			cmp.Compare(ea.SlotIndex, eb.SlotIndex),
			cmp.Compare(b.Quality, a.Quality),
			cmp.Compare(b.Enhance, a.Enhance),
			cmp.Compare(a.Code, b.Code),
		)
	})
	sorted := equip
	for _, code := range d.StackOrder {
		for n := totals[code]; n > 0; n -= d.StackMax {
			sorted = append(sorted, &Item{Code: code, Count: min(n, d.StackMax)})
		}
	}
	if len(sorted) > free {
		return
	}
	for i, it := range inv.Bag {
		if it != nil && !d.IsEquipment(it.Code) && !isStackable(d, it.Code) {
			continue // unknown code: keep it where it is
		}
		inv.Bag[i] = nil
		if len(sorted) > 0 {
			inv.Bag[i], sorted = sorted[0], sorted[1:]
		}
	}
}

func isStackable(d *gamedata.Items, code string) bool {
	_, ok := d.Stackables[code]
	return ok
}
