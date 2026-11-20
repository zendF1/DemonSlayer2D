package items

import (
	"fmt"
	"math/rand/v2"

	"demonslayer2d/internal/gamedata"
)

const (
	bpScale = 10000 // chances are in basis points
	minTier = 10
	maxTier = 50
)

// DropTier is the equipment tier a monster of the level drops (spec 4.3).
func DropTier(monsterLevel int) int {
	return min(maxTier, max(minTier, 10*(monsterLevel/10)))
}

// RollDrops rolls what a monster of rank and level gives its killer (spec
// 4.3): Xu, then the equipment pieces, then each stack, in that order. Weapons
// are of the killer's class. An unknown rank gives nothing.
func RollDrops(rng *rand.Rand, d *gamedata.Items, rank string, monsterLevel int, killerClass string) (xu int64, drops []Item) {
	t, ok := d.Drops[rank]
	if !ok {
		return 0, nil
	}
	lo, hi := t.XuPerLevel[0]*monsterLevel, t.XuPerLevel[1]*monsterLevel
	xu = int64(lo + rng.IntN(hi-lo+1))

	if chance(rng, t.Equipment.ChanceBP) {
		tier := DropTier(monsterLevel)
		for range t.Equipment.Count {
			slot := rng.IntN(EquipSlots)
			code := d.Piece(tier, slot)
			if slot == 0 {
				code = d.Weapon(tier, killerClass)
			}
			quality := pickBP(rng, t.Equipment.QualityBP)
			if code == "" {
				continue // no such tier or class in the data
			}
			drops = append(drops, NewEquipment(rng, d, code, quality))
		}
	}
	for _, s := range t.Stacks {
		if chance(rng, s.ChanceBP) {
			drops = append(drops, Item{Code: s.Code, Count: s.Count})
		}
	}
	return xu, drops
}

// NewEquipment makes a piece of quality with rolled lines (spec 4.4): the
// line count, then each line's stat (no repeats) and value, then, from Vàng
// up, the element and the hidden lines. code must be an equipment code and
// quality a quality index; anything else is a programming error and panics.
func NewEquipment(rng *rand.Rand, d *gamedata.Items, code string, quality int) Item {
	e, ok := d.Equipment[code]
	if !ok || quality < 0 || quality >= len(d.Qualities) {
		panic(fmt.Sprintf("items: NewEquipment(%q, %d)", code, quality))
	}
	q := d.Qualities[quality]
	std := d.Tiers[e.Tier].LineStandard
	it := Item{Code: code, Count: 1, Quality: quality}

	n := q.Lines[0] + rng.IntN(q.Lines[1]-q.Lines[0]+1)
	pool := append([]string(nil), d.LineStats...)
	for range n {
		i := rng.IntN(len(pool))
		stat := pool[i]
		pool = append(pool[:i], pool[i+1:]...)
		s := std[stat]
		lo := (s*d.LineRollMinPct + 99) / 100 // ceil
		it.Lines = append(it.Lines, Line{Stat: stat, Value: lo + rng.IntN(s-lo+1)})
	}

	if q.HiddenPct > 0 {
		it.Element = Elements[rng.IntN(len(Elements))]
		for range HiddenLines {
			stat := d.LineStats[rng.IntN(len(d.LineStats))]
			it.Hidden = append(it.Hidden, Line{Stat: stat, Value: (std[stat]*q.HiddenPct + 50) / 100}) // round half up
		}
	}
	return it
}

func chance(rng *rand.Rand, bp int) bool { return rng.IntN(bpScale) < bp }

// pickBP picks an index with the weights bp (basis points summing to 10000).
func pickBP(rng *rand.Rand, bp []int) int {
	r := rng.IntN(bpScale)
	for i, w := range bp {
		if r < w {
			return i
		}
		r -= w
	}
	return len(bp) - 1
}
