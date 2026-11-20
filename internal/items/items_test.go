package items

import (
	"errors"
	"maps"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"sync"
	"testing"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
)

var (
	dataOnce sync.Once
	data     *gamedata.Items
	dataErr  error
)

// realData is the committed data/items.json.
func realData(t *testing.T) *gamedata.Items {
	t.Helper()
	dataOnce.Do(func() {
		var b []byte
		if b, dataErr = os.ReadFile("../../data/items.json"); dataErr == nil {
			data, dataErr = gamedata.ParseItems(b)
		}
	})
	if dataErr != nil {
		t.Fatal(dataErr)
	}
	return data
}

func seeded(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) }

func ceilDiv(a, b int) int { return (a + b - 1) / b }

// ______________________TEST ROLL_____________________

func TestDropTier(t *testing.T) {
	for _, c := range []struct{ level, tier int }{{1, 10}, {9, 10}, {10, 10}, {19, 10}, {20, 20}, {39, 30}, {55, 50}, {90, 50}} {
		if got := DropTier(c.level); got != c.tier {
			t.Errorf("DropTier(%d) = %d, want %d", c.level, got, c.tier)
		}
	}
}

// checkEquipment checks one rolled piece against spec 4.4.
func checkEquipment(t *testing.T, d *gamedata.Items, it Item, code string, quality int) {
	t.Helper()
	e := d.Equipment[code]
	q := d.Qualities[quality]
	std := d.Tiers[e.Tier].LineStandard
	if it.Code != code || it.Count != 1 || it.Quality != quality || it.Enhance != 0 {
		t.Fatalf("%+v: wrong code, count, quality or enhance", it)
	}
	if n := len(it.Lines); n < q.Lines[0] || n > q.Lines[1] {
		t.Fatalf("%+v: %d lines, quality %d wants %v", it, n, quality, q.Lines)
	}
	seen := map[string]bool{}
	for _, l := range it.Lines {
		if seen[l.Stat] {
			t.Fatalf("%+v: stat %q twice", it, l.Stat)
		}
		seen[l.Stat] = true
		s, ok := std[l.Stat]
		if !ok {
			t.Fatalf("%+v: unknown stat %q", it, l.Stat)
		}
		if lo := ceilDiv(s*d.LineRollMinPct, 100); l.Value < lo || l.Value > s {
			t.Fatalf("%+v: %s %d outside [%d, %d]", it, l.Stat, l.Value, lo, s)
		}
	}
	if q.HiddenPct == 0 {
		if it.Element != "" || len(it.Hidden) != 0 {
			t.Fatalf("%+v: quality %d has no element or hidden lines", it, quality)
		}
		return
	}
	if !slices.Contains(Elements[:], it.Element) {
		t.Fatalf("%+v: element %q", it, it.Element)
	}
	if len(it.Hidden) != HiddenLines {
		t.Fatalf("%+v: %d hidden lines", it, len(it.Hidden))
	}
	for _, h := range it.Hidden {
		s, ok := std[h.Stat]
		if !ok {
			t.Fatalf("%+v: unknown hidden stat %q", it, h.Stat)
		}
		if want := (s*q.HiddenPct + 50) / 100; h.Value != want {
			t.Fatalf("%+v: hidden %s %d, want %d", it, h.Stat, h.Value, want)
		}
	}
}

func TestNewEquipmentLines(t *testing.T) {
	d := realData(t)
	codes := slices.Sorted(maps.Keys(d.Equipment))
	rng := seeded(1)
	for quality, q := range d.Qualities {
		lineCounts := map[int]int{}
		stats := map[string]int{}
		values := map[[2]int]bool{} // {tier, value} seen for HP
		elements := map[string]bool{}
		hiddenDup := 0
		for range 10000 {
			code := codes[rng.IntN(len(codes))]
			it := NewEquipment(rng, d, code, quality)
			checkEquipment(t, d, it, code, quality)
			lineCounts[len(it.Lines)]++
			for _, l := range it.Lines {
				stats[l.Stat]++
				if l.Stat == gamedata.StatHP {
					values[[2]int{d.Equipment[code].Tier, l.Value}] = true
				}
			}
			if it.Element != "" {
				elements[it.Element] = true
			}
			if len(it.Hidden) == 2 && it.Hidden[0].Stat == it.Hidden[1].Stat {
				hiddenDup++
			}
		}
		// Every count in the range happens.
		for n := q.Lines[0]; n <= q.Lines[1]; n++ {
			if lineCounts[n] == 0 {
				t.Errorf("quality %d: never rolled %d lines", quality, n)
			}
		}
		if q.Lines[1] > 0 {
			for _, s := range d.LineStats {
				if stats[s] == 0 {
					t.Errorf("quality %d: never rolled stat %q", quality, s)
				}
			}
			// Both ends of the HP range at tier 10 happen.
			s := d.Tiers[10].LineStandard[gamedata.StatHP]
			if lo := ceilDiv(s*d.LineRollMinPct, 100); !values[[2]int{10, lo}] || !values[[2]int{10, s}] {
				t.Errorf("quality %d: tier 10 HP lines never hit %d or %d", quality, lo, s)
			}
		}
		if q.HiddenPct > 0 {
			if len(elements) != len(Elements) {
				t.Errorf("quality %d: elements %v", quality, elements)
			}
			// Two hidden lines may share a stat (about one in five).
			if hiddenDup < 1500 || hiddenDup > 2500 {
				t.Errorf("quality %d: %d hidden pairs share a stat, want about 2000", quality, hiddenDup)
			}
		}
	}
}

func TestNewEquipmentHiddenValues(t *testing.T) {
	d := realData(t)
	// Tier-10 HP standard 86: hidden 86, 108, 129 for Vàng, Cam, Đỏ.
	for quality, want := range map[int]int{2: 86, 3: 108, 4: 129} {
		rng := seeded(uint64(quality))
		found := false
		for range 200 {
			it := NewEquipment(rng, d, "eq10_armor", quality)
			for _, h := range it.Hidden {
				if h.Stat == gamedata.StatHP {
					found = true
					if h.Value != want {
						t.Fatalf("quality %d: hidden HP %d, want %d", quality, h.Value, want)
					}
				}
			}
		}
		if !found {
			t.Fatalf("quality %d: no hidden HP line in 200 rolls", quality)
		}
	}
}

func TestNewEquipmentSeeded(t *testing.T) {
	d := realData(t)
	a := NewEquipment(seeded(7), d, "eq30_helmet", 4)
	b := NewEquipment(seeded(7), d, "eq30_helmet", 4)
	if !slices.Equal(a.Lines, b.Lines) || !slices.Equal(a.Hidden, b.Hidden) || a.Element != b.Element {
		t.Fatalf("same seed gave %+v and %+v", a, b)
	}
}

func TestRollDropsNormal(t *testing.T) {
	d := realData(t)
	rng := seeded(42)
	// 500,000 kills: about 25,000 pieces, so the ±1 point quality bound is
	// beyond 3.5 standard deviations.
	const kills = 500000
	equip, stones := 0, 0
	qualities := make([]int, len(d.Qualities))
	for _, class := range []string{"viem", "thuy", "minh", "nham", "phong"} {
		for range kills / 5 {
			xu, drops := RollDrops(rng, d, gamedata.RankNormal, 15, class)
			if xu < 45 || xu > 75 {
				t.Fatalf("xu %d outside [45, 75]", xu)
			}
			for _, it := range drops {
				if e, ok := d.Equipment[it.Code]; ok {
					equip++
					qualities[it.Quality]++
					if e.Tier != 10 {
						t.Fatalf("%s: tier %d, want 10", it.Code, e.Tier)
					}
					if e.Slot == gamedata.SlotWeapon && e.Class != class {
						t.Fatalf("%s dropped for a %s", it.Code, class)
					}
					checkEquipment(t, d, it, it.Code, it.Quality)
					continue
				}
				if it.Code != "stone_enhance" || it.Count != 1 {
					t.Fatalf("unexpected drop %+v", it)
				}
				stones++
			}
		}
	}
	pct := func(n, of int) float64 { return 100 * float64(n) / float64(of) }
	if r := pct(equip, kills); r < 4.7 || r > 5.3 {
		t.Errorf("equipment rate %.2f%%, want 5 ± 0.3", r)
	}
	if r := pct(stones, kills); r < 19.5 || r > 20.5 {
		t.Errorf("stone rate %.2f%%, want 20 ± 0.5", r)
	}
	for q, want := range []float64{80, 17, 3, 0, 0} {
		if r := pct(qualities[q], equip); r < want-1 || r > want+1 {
			t.Errorf("quality %d share %.2f%%, want %.0f ± 1", q, r, want)
		}
	}
}

func TestRollDropsXuEnds(t *testing.T) {
	d := realData(t)
	rng := seeded(3)
	seen := map[int64]bool{}
	for range 5000 {
		xu, _ := RollDrops(rng, d, gamedata.RankNormal, 2, "viem")
		seen[xu] = true
	}
	// Level 2: [6, 10], every value happens.
	for xu := int64(6); xu <= 10; xu++ {
		if !seen[xu] {
			t.Errorf("xu %d never rolled", xu)
		}
	}
	if len(seen) != 5 {
		t.Errorf("xu values %v", seen)
	}
}

func TestRollDropsBoss(t *testing.T) {
	d := realData(t)
	rng := seeded(9)
	slotsSeen := map[int]bool{}
	for range 2000 {
		xu, drops := RollDrops(rng, d, gamedata.RankBoss, 47, "nham")
		if xu < 75*47 || xu > 125*47 {
			t.Fatalf("xu %d", xu)
		}
		// Boss: two pieces, then 10 enhance stones and 1 refine stone, in order.
		if len(drops) != 4 {
			t.Fatalf("boss drops %+v", drops)
		}
		for _, it := range drops[:2] {
			e, ok := d.Equipment[it.Code]
			if !ok || e.Tier != 40 || it.Quality == 0 || it.Quality == 4 {
				t.Fatalf("boss piece %+v", it)
			}
			slotsSeen[e.SlotIndex] = true
		}
		if drops[2].Code != "stone_enhance" || drops[2].Count != 10 || drops[3].Code != "stone_refine" || drops[3].Count != 1 {
			t.Fatalf("boss stacks %+v", drops[2:])
		}
	}
	if len(slotsSeen) != 10 {
		t.Fatalf("slots %v, want all ten", slotsSeen)
	}
}

func TestRollDropsSeeded(t *testing.T) {
	d := realData(t)
	r1, r2 := seeded(5), seeded(5)
	for range 200 {
		x1, d1 := RollDrops(r1, d, gamedata.RankElite, 33, "phong")
		x2, d2 := RollDrops(r2, d, gamedata.RankElite, 33, "phong")
		if x1 != x2 || len(d1) != len(d2) {
			t.Fatalf("same seed differs: %d %+v vs %d %+v", x1, d1, x2, d2)
		}
		for i := range d1 {
			if d1[i].Code != d2[i].Code || !slices.Equal(d1[i].Lines, d2[i].Lines) {
				t.Fatalf("same seed differs: %+v vs %+v", d1[i], d2[i])
			}
		}
	}
}

// ______________________TEST BAG_____________________

func stone(n int) *Item { return &Item{Code: "stone_enhance", Count: n} }

func sword() *Item { return &Item{Code: "eq10_weapon_viem", Count: 1} }

func TestAddStackFillsExistingFirst(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	inv.Bag[0] = sword()
	inv.Bag[2] = stone(998)
	changed, err := inv.Add(d, *stone(3))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []int{1, 2}) {
		t.Fatalf("changed %v, want [1 2]", changed)
	}
	if inv.Bag[2].Count != 999 || inv.Bag[1] == nil || inv.Bag[1].Count != 2 || inv.Bag[1].Code != "stone_enhance" {
		t.Fatalf("bag %+v %+v", inv.Bag[1], inv.Bag[2])
	}
}

func TestAddStackOnlyMerges(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	inv.Bag[5] = stone(10)
	inv.Bag[6] = &Item{Code: "stone_refine", Count: 1}
	changed, err := inv.Add(d, *stone(4))
	if err != nil || !slices.Equal(changed, []int{5}) || inv.Bag[5].Count != 14 || inv.Bag[0] != nil {
		t.Fatalf("changed %v err %v slot5 %+v slot0 %+v", changed, err, inv.Bag[5], inv.Bag[0])
	}
}

func TestAddEquipmentFirstFree(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	inv.Bag[0] = stone(5)
	piece := NewEquipment(seeded(1), d, "eq10_armor", 2)
	changed, err := inv.Add(d, piece)
	if err != nil || !slices.Equal(changed, []int{1}) {
		t.Fatalf("changed %v err %v", changed, err)
	}
	if inv.Bag[1].Code != "eq10_armor" || !slices.Equal(inv.Bag[1].Lines, piece.Lines) {
		t.Fatalf("slot 1 %+v", inv.Bag[1])
	}
	// The bag holds its own copy.
	piece.Lines[0].Value = -1
	if inv.Bag[1].Lines[0].Value == -1 {
		t.Fatal("bag shares the added item's lines")
	}
}

func TestAddBagFull(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	for i := range inv.Bag {
		inv.Bag[i] = sword()
	}
	inv.Bag[7] = stone(998)
	before := inv.Clone()
	if _, err := inv.Add(d, *sword()); !errors.Is(err, ErrBagFull) {
		t.Fatalf("equipment into a full bag: %v", err)
	}
	// One fits into the stack, the second has nowhere to go: nothing changes.
	if _, err := inv.Add(d, *stone(2)); !errors.Is(err, ErrBagFull) {
		t.Fatalf("stack into a full bag: %v", err)
	}
	if inv.Bag[7].Count != 998 || !sameInventory(inv, before) {
		t.Fatal("a failed Add changed the bag")
	}
	// Exactly one more fits.
	if changed, err := inv.Add(d, *stone(1)); err != nil || !slices.Equal(changed, []int{7}) || inv.Bag[7].Count != 999 {
		t.Fatalf("changed %v err %v", changed, err)
	}
}

func TestAddLargeStackSplits(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	changed, err := inv.Add(d, *stone(2500))
	if err != nil || !slices.Equal(changed, []int{0, 1, 2}) {
		t.Fatalf("changed %v err %v", changed, err)
	}
	if inv.Bag[0].Count != 999 || inv.Bag[1].Count != 999 || inv.Bag[2].Count != 502 {
		t.Fatalf("counts %d %d %d", inv.Bag[0].Count, inv.Bag[1].Count, inv.Bag[2].Count)
	}
}

func TestAddBadItems(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	if _, err := inv.Add(d, Item{Code: "nope", Count: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown code: %v", err)
	}
	if _, err := inv.Add(d, *stone(0)); !errors.Is(err, ErrBadCount) {
		t.Fatalf("zero count: %v", err)
	}
	if _, err := inv.Add(d, Item{Code: "eq10_armor", Count: 2}); !errors.Is(err, ErrBadCount) {
		t.Fatalf("equipment count 2: %v", err)
	}
	if inv.FirstFree() != 0 {
		t.Fatal("a failed Add changed the bag")
	}
}

func TestFirstFree(t *testing.T) {
	inv := NewInventory(3)
	if inv.FirstFree() != 0 {
		t.Fatal("empty bag")
	}
	inv.Bag[0], inv.Bag[2] = sword(), sword()
	if inv.FirstFree() != 1 {
		t.Fatal("hole at 1")
	}
	inv.Bag[1] = sword()
	if inv.FirstFree() != -1 {
		t.Fatal("full bag")
	}
}

func sameInventory(a, b *Inventory) bool {
	same := func(x, y *Item) bool {
		if x == nil || y == nil {
			return x == y
		}
		return x.Code == y.Code && x.Count == y.Count && x.Quality == y.Quality && x.Element == y.Element &&
			x.Enhance == y.Enhance && x.Pity == y.Pity && slices.Equal(x.Lines, y.Lines) && slices.Equal(x.Hidden, y.Hidden)
	}
	if len(a.Bag) != len(b.Bag) {
		return false
	}
	for i := range a.Bag {
		if !same(a.Bag[i], b.Bag[i]) {
			return false
		}
	}
	for i := range a.Worn {
		if !same(a.Worn[i], b.Worn[i]) {
			return false
		}
	}
	return true
}

func TestCloneIsDeep(t *testing.T) {
	inv := NewInventory(4)
	inv.Bag[1] = &Item{Code: "eq10_armor", Count: 1, Quality: 2, Element: "thuy",
		Lines: []Line{{"hp", 50}}, Hidden: []Line{{"hp", 86}, {"phys_attack", 4}}}
	inv.Worn[5] = &Item{Code: "eq10_armor", Count: 1, Lines: []Line{{"hp", 60}}, Pity: [MaxEnhance]int{7: 3}}
	c := inv.Clone()
	if !sameInventory(inv, c) {
		t.Fatal("clone differs")
	}
	c.Bag[1].Lines[0].Value = 1
	c.Bag[1].Hidden[1].Value = 1
	c.Worn[5].Lines[0].Value = 1
	c.Worn[5].Pity[7] = 9
	c.Bag[0] = sword()
	if inv.Bag[1].Lines[0].Value != 50 || inv.Bag[1].Hidden[1].Value != 4 || inv.Worn[5].Lines[0].Value != 60 || inv.Bag[0] != nil ||
		inv.Worn[5].Pity[7] != 3 {
		t.Fatal("clone shares state with the original")
	}
	it := Item{Code: "x", Lines: []Line{{"hp", 1}}}
	ic := it.Clone()
	ic.Lines[0].Value = 2
	if it.Lines[0].Value != 1 {
		t.Fatal("Item.Clone shares lines")
	}
}

func TestEquipSwaps(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	inv.Bag[3] = &Item{Code: "eq20_armor", Count: 1, Quality: 1, Lines: []Line{{"hp", 100}}}
	inv.Worn[5] = &Item{Code: "eq10_armor", Count: 1}
	slot, err := inv.Equip(d, 3, 25, "minh")
	if err != nil || slot != 5 {
		t.Fatalf("slot %d err %v", slot, err)
	}
	if inv.Worn[5].Code != "eq20_armor" || inv.Bag[3] == nil || inv.Bag[3].Code != "eq10_armor" {
		t.Fatalf("worn %+v bag %+v", inv.Worn[5], inv.Bag[3])
	}
	// Into an empty slot the bag slot becomes empty.
	inv.Bag[0] = sword()
	if slot, err := inv.Equip(d, 0, 10, "viem"); err != nil || slot != 0 || inv.Bag[0] != nil || inv.Worn[0].Code != "eq10_weapon_viem" {
		t.Fatalf("slot %d err %v bag %+v", slot, err, inv.Bag[0])
	}
}

func TestEquipErrorOrder(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	inv.Bag[0] = stone(3)
	inv.Bag[1] = &Item{Code: "eq20_weapon_thuy", Count: 1} // level 20, Thủy
	inv.Bag[2] = &Item{Code: "eq20_ring_left", Count: 1}   // level 28
	cases := []struct {
		name  string
		slot  int
		level int
		class string
		want  error
	}{
		{"negative slot", -1, 1, "viem", ErrBadSlot},
		{"slot past the bag", d.BagSize, 1, "viem", ErrBadSlot},
		{"empty slot", 5, 1, "viem", ErrNotFound},
		{"stack", 0, 99, "viem", ErrNotEquipment},
		// Wrong class beats too low a level.
		{"wrong class and level", 1, 1, "viem", ErrWrongClass},
		{"wrong class", 1, 99, "viem", ErrWrongClass},
		{"level too low", 1, 19, "thuy", ErrLevelTooLow},
		{"ring level too low", 2, 27, "viem", ErrLevelTooLow},
	}
	for _, c := range cases {
		before := inv.Clone()
		if _, err := inv.Equip(d, c.slot, c.level, c.class); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
		if !sameInventory(inv, before) {
			t.Fatalf("%s: a failed Equip changed the inventory", c.name)
		}
	}
	if _, err := inv.Equip(d, 1, 20, "thuy"); err != nil {
		t.Fatalf("level 20 Thủy: %v", err)
	}
	if _, err := inv.Equip(d, 2, 28, "viem"); err != nil {
		t.Fatalf("level 28 ring: %v", err)
	}
}

func TestUnequip(t *testing.T) {
	inv := NewInventory(3)
	inv.Bag[0] = stone(1)
	inv.Worn[8] = &Item{Code: "eq10_ring_left", Count: 1}
	for _, c := range []struct {
		slot int
		want error
	}{{-1, ErrBadSlot}, {10, ErrBadSlot}, {7, ErrNotFound}} {
		if _, err := inv.Unequip(c.slot); !errors.Is(err, c.want) {
			t.Errorf("slot %d: %v, want %v", c.slot, err, c.want)
		}
	}
	slot, err := inv.Unequip(8)
	if err != nil || slot != 1 || inv.Worn[8] != nil || inv.Bag[1].Code != "eq10_ring_left" {
		t.Fatalf("slot %d err %v", slot, err)
	}
	inv.Bag[2] = stone(1)
	inv.Worn[0] = sword()
	if _, err := inv.Unequip(0); !errors.Is(err, ErrBagFull) {
		t.Fatalf("full bag: %v", err)
	}
	if inv.Worn[0] == nil {
		t.Fatal("a failed Unequip removed the item")
	}
}

func TestDiscard(t *testing.T) {
	inv := NewInventory(4)
	inv.Bag[0] = stone(5)
	inv.Bag[1] = sword()
	for _, c := range []struct {
		name        string
		slot, count int
		want        error
	}{
		{"negative slot", -1, 0, ErrBadSlot},
		{"slot past the bag", 4, 0, ErrBadSlot},
		{"empty slot", 2, 0, ErrNotFound},
		{"negative count", 0, -1, ErrBadCount},
		{"too many", 0, 6, ErrBadCount},
	} {
		if err := inv.Discard(c.slot, c.count); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	if inv.Bag[0].Count != 5 {
		t.Fatal("a failed Discard changed the stack")
	}
	if err := inv.Discard(0, 2); err != nil || inv.Bag[0].Count != 3 {
		t.Fatalf("discard 2 of 5: %v, %+v", err, inv.Bag[0])
	}
	if err := inv.Discard(0, 3); err != nil || inv.Bag[0] != nil {
		t.Fatalf("discard all 3: %v, %+v", err, inv.Bag[0])
	}
	if err := inv.Discard(1, 0); err != nil || inv.Bag[1] != nil {
		t.Fatalf("discard the whole slot: %v", err)
	}
}

func TestSort(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	inv.Bag[0] = stone(600)
	inv.Bag[3] = &Item{Code: "stone_refine", Count: 4}
	inv.Bag[4] = &Item{Code: "eq10_armor", Count: 1, Quality: 4}
	inv.Bag[9] = stone(600)
	inv.Bag[10] = &Item{Code: "eq30_boots", Count: 1, Quality: 0}
	inv.Bag[11] = &Item{Code: "eq10_armor", Count: 1, Quality: 1, Enhance: 2}
	inv.Bag[12] = &Item{Code: "eq10_armor", Count: 1, Quality: 1, Enhance: 5}
	inv.Bag[20] = &Item{Code: "eq10_weapon_viem", Count: 1}
	inv.Bag[21] = &Item{Code: "eq30_weapon_thuy", Count: 1}
	inv.Bag[30] = &Item{Code: "eq10_armor", Count: 1, Quality: 4, Lines: []Line{{"hp", 50}}}
	inv.Bag[40] = &Item{Code: "stone_refine", Count: 9}
	inv.Sort(d)
	want := []struct {
		code          string
		count, q, enh int
		lineHP        int // the first line's value, when not 0
	}{
		{"eq30_weapon_thuy", 1, 0, 0, 0},
		{"eq30_boots", 1, 0, 0, 0},
		{"eq10_weapon_viem", 1, 0, 0, 0},
		{"eq10_armor", 1, 4, 0, 0}, // ties keep their bag order
		{"eq10_armor", 1, 4, 0, 50},
		{"eq10_armor", 1, 1, 5, 0},
		{"eq10_armor", 1, 1, 2, 0},
		{"stone_enhance", 999, 0, 0, 0},
		{"stone_enhance", 201, 0, 0, 0},
		{"stone_refine", 13, 0, 0, 0},
	}
	for i, w := range want {
		it := inv.Bag[i]
		if it == nil || it.Code != w.code || it.Count != w.count || it.Quality != w.q || it.Enhance != w.enh {
			t.Fatalf("slot %d: %+v, want %+v", i, it, w)
		}
		if w.lineHP != 0 && (len(it.Lines) == 0 || it.Lines[0].Value != w.lineHP) {
			t.Fatalf("slot %d: lines %v", i, it.Lines)
		}
	}
	for i := len(want); i < len(inv.Bag); i++ {
		if inv.Bag[i] != nil {
			t.Fatalf("slot %d not empty: %+v", i, inv.Bag[i])
		}
	}
}

func TestSortStackOrder(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	// Refine comes after enhance (file order); 2,500 enhance stones become 999, 999, 502.
	inv.Bag[0] = &Item{Code: "stone_refine", Count: 1}
	inv.Bag[1] = stone(500)
	inv.Bag[2] = stone(999)
	inv.Bag[3] = stone(1001)
	inv.Sort(d)
	got := []int{}
	for _, it := range inv.Bag[:4] {
		got = append(got, it.Count)
	}
	if !slices.Equal(got, []int{999, 999, 502, 1}) || inv.Bag[3].Code != "stone_refine" || inv.Bag[4] != nil {
		t.Fatalf("counts %v, last %+v", got, inv.Bag[3])
	}
}

func TestSortKeepsTiesInBagOrder(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	// Forty pieces with the same sort key, told apart by their line value.
	for i := range 40 {
		code := "eq10_armor"
		if i%3 == 0 {
			code = "eq20_boots"
		}
		inv.Bag[59-i] = &Item{Code: code, Count: 1, Quality: 1, Lines: []Line{{"hp", i + 1}}}
	}
	inv.Sort(d)
	// Bag order runs from slot 20 (value 40) down to slot 59 (value 1).
	last := map[string]int{"eq10_armor": 41, "eq20_boots": 41}
	for _, it := range inv.Bag[:40] {
		if v := it.Lines[0].Value; v >= last[it.Code] {
			t.Fatalf("%s: line %d after %d, ties must keep bag order", it.Code, v, last[it.Code])
		}
		last[it.Code] = it.Lines[0].Value
	}
}

// Codes the data does not know (the world drops them at login, so this is
// only a safety net) stay in their slots; the known items sort around them.
func TestSortKeepsUnknownCodes(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	inv.Bag[0] = &Item{Code: "old_gem", Count: 7}
	inv.Bag[2] = stone(5)
	inv.Bag[3] = &Item{Code: "eq99_armor", Count: 1, Quality: 3}
	inv.Bag[5] = &Item{Code: "eq10_armor", Count: 1}
	inv.Bag[59] = &Item{Code: "old_gem", Count: 2}
	inv.Sort(d)
	if b := inv.Bag; b[0].Code != "old_gem" || b[0].Count != 7 || b[3].Code != "eq99_armor" || b[3].Quality != 3 ||
		b[59].Code != "old_gem" || b[59].Count != 2 {
		t.Fatalf("unknown codes moved or lost: %+v %+v %+v", b[0], b[3], b[59])
	}
	if b := inv.Bag; b[1].Code != "eq10_armor" || b[2].Code != "stone_enhance" || b[2].Count != 5 || b[4] != nil || b[5] != nil {
		t.Fatalf("known items: %+v %+v %+v %+v", b[1], b[2], b[4], b[5])
	}
}

// A layout that does not fit (stacks above stack_max in a full bag) leaves
// the bag as it was rather than dropping the overflow.
func TestSortNeverDeletes(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	for i := range inv.Bag {
		inv.Bag[i] = &Item{Code: "eq10_armor", Count: 1}
	}
	inv.Bag[0] = &Item{Code: "old_gem", Count: 1}
	inv.Bag[1] = stone(d.StackMax + 1) // needs two slots after the merge
	before := inv.Clone()
	inv.Sort(d)
	for i := range inv.Bag {
		if a, b := inv.Bag[i], before.Bag[i]; a.Code != b.Code || a.Count != b.Count {
			t.Fatalf("slot %d changed: %+v, was %+v", i, inv.Bag[i], before.Bag[i])
		}
	}
}

// ______________________TEST GEAR_____________________

func TestWearable(t *testing.T) {
	d := realData(t)
	for _, c := range []struct {
		code  string
		level int
		class string
		want  bool
	}{
		{"eq10_weapon_viem", 10, "viem", true},
		{"eq10_weapon_viem", 9, "viem", false},
		{"eq10_weapon_viem", 50, "thuy", false},
		{"eq10_ring_right", 19, "thuy", true},
		{"eq10_ring_right", 18, "thuy", false},
		{"stone_enhance", 90, "viem", false},
		{"nope", 90, "viem", false},
	} {
		if got := Wearable(d, &Item{Code: c.code, Count: 1}, c.level, c.class); got != c.want {
			t.Errorf("Wearable(%s, %d, %s) = %v", c.code, c.level, c.class, got)
		}
	}
}

func TestGearBonus(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	inv.Worn[0] = sword()
	if got := GearBonus(d, inv, 10, "viem"); got != (combat.Bonus{PhysAttack: 25}) {
		t.Fatalf("Lữ Hành sword on a Viêm: %+v", got)
	}
	if got := GearBonus(d, inv, 10, "thuy"); got != (combat.Bonus{}) {
		t.Fatalf("Viêm sword on a Thủy: %+v", got)
	}
	if got := GearBonus(d, inv, 9, "viem"); got != (combat.Bonus{}) {
		t.Fatalf("sword below its level: %+v", got)
	}
	inv.Worn[0] = nil
	inv.Worn[9] = &Item{Code: "eq50_ring_right", Count: 1}
	if got := GearBonus(d, inv, 30, "viem"); got != (combat.Bonus{}) {
		t.Fatalf("tier-50 ring at level 30: %+v", got)
	}
	ring := d.Equipment["eq50_ring_right"]
	if got := GearBonus(d, inv, 59, "viem"); got != (combat.Bonus{HP: ring.HP[0], PhysAttack: ring.PhysAttack[0], MagicAttack: ring.MagicAttack[0]}) {
		t.Fatalf("tier-50 ring at level 59: %+v", got)
	}
	// Lines add their values; hidden lines do nothing in 3a; the enhance level
	// picks the array entry.
	inv.Worn[9] = nil
	inv.Worn[0] = &Item{Code: "eq10_weapon_viem", Count: 1, Quality: 2, Element: "viem", Enhance: 3,
		Lines:  []Line{{"hp", 50}, {"phys_defense", 1}, {"magic_attack", 3}},
		Hidden: []Line{{"hp", 86}, {"phys_attack", 4}}}
	inv.Worn[5] = &Item{Code: "eq10_armor", Count: 1, Quality: 1, Lines: []Line{{"hp", 70}, {"magic_defense", 1}}}
	armor := d.Equipment["eq10_armor"]
	want := combat.Bonus{
		HP:           armor.HP[0] + 50 + 70,
		PhysAttack:   28 + armor.PhysAttack[0],
		MagicAttack:  3 + armor.MagicAttack[0],
		PhysDefense:  1 + armor.PhysDefense[0],
		MagicDefense: 1 + armor.MagicDefense[0],
	}
	if got := GearBonus(d, inv, 15, "viem"); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// Only worn items count.
	inv.Bag[0] = &Item{Code: "eq10_boots", Count: 1}
	if got := GearBonus(d, inv, 15, "viem"); got != want {
		t.Fatalf("a bag item counted: %+v", got)
	}
}

func TestGearBonusSlotMismatch(t *testing.T) {
	d := realData(t)
	inv := NewInventory(d.BagSize)
	// A ring in the weapon slot (bad data) adds nothing.
	inv.Worn[0] = &Item{Code: "eq10_ring_left", Count: 1}
	if got := GearBonus(d, inv, 90, "viem"); got != (combat.Bonus{}) {
		t.Fatalf("misplaced ring: %+v", got)
	}
}

// ______________________TEST ENHANCE_____________________

// rollSource is a rand.Source whose rand.Rand.IntN(100) results are rolls,
// in order, then cycled: each value sits in the middle of its 1/100 band so
// no rejection step ever draws again.
type rollSource struct {
	rolls []int
	n     int
}

func (s *rollSource) Uint64() uint64 {
	r := s.rolls[s.n%len(s.rolls)]
	s.n++
	q := ^uint64(0) / 100
	return uint64(r)*q + q/2
}

func rolls(r ...int) *rand.Rand { return rand.New(&rollSource{rolls: r}) }

func TestRollSource(t *testing.T) {
	all := make([]int, 100)
	for i := range all {
		all[i] = i
	}
	rng := rolls(all...)
	for i := range 200 {
		if got := rng.IntN(100); got != i%100 {
			t.Fatalf("roll %d: got %d", i, got)
		}
	}
}

const (
	// t10 is a tier 10 piece (Xu per attempt is 10 x xu_per_tier), t50 a tier 50 one.
	t10 = "eq10_weapon_viem"
	t50 = "eq50_armor"
	// rich is more Xu than any test spends.
	rich = int64(1) << 40
)

func charm(n int) *Item { return &Item{Code: gamedata.CharmProtect, Count: n} }

// forge is an inventory with the piece at bag slot 0 at level n, then stones
// stacks at slots 1.. and, when charms > 0, a charm stack after them.
func forge(code string, n int, stones []int, charms int) (*Inventory, *Item) {
	inv := NewInventory(60)
	it := &Item{Code: code, Count: 1, Enhance: n}
	inv.Bag[0] = it
	for i, c := range stones {
		inv.Bag[1+i] = stone(c)
	}
	if charms > 0 {
		inv.Bag[1+len(stones)] = charm(charms)
	}
	return inv, it
}

var bag0 = Ref{Slot: 0}

func TestEnhanceFirstLevel(t *testing.T) {
	d := realData(t)
	// +1 is 100%: even the highest roll succeeds.
	for _, code := range []string{t10, t50} {
		inv, it := forge(code, 0, []int{3}, 0)
		xu := rich
		res, err := inv.Enhance(rolls(99), d, bag0, false, 1, &xu)
		if err != nil {
			t.Fatal(err)
		}
		if res.Attempts != 1 || !res.Success || res.Level != 1 || it.Enhance != 1 || !slices.Equal(res.Changed, []int{0, 1}) {
			t.Fatalf("%s: %+v, item +%d", code, res, it.Enhance)
		}
		tier := d.Equipment[code].Tier
		if inv.Bag[1].Count != 2 || xu != rich-int64(10*tier) {
			t.Fatalf("%s: %d stones left, spent %d Xu, want 1 stone and %d Xu", code, inv.Bag[1].Count, rich-xu, 10*tier)
		}
		if it.Pity != [MaxEnhance]int{} {
			t.Fatalf("%s: pity %v", code, it.Pity)
		}
	}
}

func TestEnhanceFailureAddsPity(t *testing.T) {
	d := realData(t)
	// +4 -> +5 is 90%: a roll of 95 fails, the level stays, pity[4] grows.
	inv, it := forge(t10, 4, []int{10}, 0)
	xu := rich
	res, err := inv.Enhance(rolls(95), d, bag0, false, 1, &xu)
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 1 || res.Success || res.Level != 4 || it.Enhance != 4 || it.Pity != [MaxEnhance]int{4: 1} {
		t.Fatalf("%+v, item +%d pity %v", res, it.Enhance, it.Pity)
	}
	if inv.Bag[1].Count != 8 || rich-xu != 400 {
		t.Fatalf("%d stones, %d Xu spent", inv.Bag[1].Count, rich-xu)
	}
	// The pity counts on the next attempt: 90 + 1 = 91, so 90 succeeds and 91 fails.
	inv, it = forge(t10, 4, []int{10}, 0)
	it.Pity[4] = 1
	if res, _ := inv.Enhance(rolls(90), d, bag0, false, 1, &xu); !res.Success || it.Pity[4] != 0 {
		t.Fatalf("roll 90 with pity 1: %+v pity %v", res, it.Pity)
	}
	inv, it = forge(t10, 4, []int{10}, 0)
	it.Pity[4] = 1
	if res, _ := inv.Enhance(rolls(91), d, bag0, false, 1, &xu); res.Success || it.Pity[4] != 2 {
		t.Fatalf("roll 91 with pity 1: %+v pity %v", res, it.Pity)
	}
}

func TestEnhanceDropKeepsPity(t *testing.T) {
	d := realData(t)
	// +7 -> +8 is 60% and drops on failure: a roll of 99 takes it to +6.
	inv, it := forge(t10, 7, []int{50}, 0)
	it.Pity[6], it.Pity[9] = 4, 5
	xu := rich
	res, err := inv.Enhance(rolls(99), d, bag0, false, 1, &xu)
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 1 || res.Success || res.Level != 6 || it.Enhance != 6 {
		t.Fatalf("%+v, item +%d", res, it.Enhance)
	}
	if it.Pity != [MaxEnhance]int{6: 4, 7: 1, 9: 5} {
		t.Fatalf("pity %v", it.Pity)
	}
	// Back to +7 (resets pity[6] only), then +8 with a roll that only the
	// kept pity makes succeed: 60 + 1 > 60.
	if res, _ := inv.Enhance(rolls(0), d, bag0, false, 1, &xu); !res.Success || it.Enhance != 7 {
		t.Fatalf("+6 -> +7: %+v", res)
	}
	if res, _ := inv.Enhance(rolls(60), d, bag0, false, 1, &xu); !res.Success || it.Enhance != 8 {
		t.Fatalf("+7 -> +8 with pity 1 and roll 60: %+v", res)
	}
	if it.Pity != [MaxEnhance]int{9: 5} {
		t.Fatalf("pity after the two successes %v, want only [9] = 5", it.Pity)
	}
}

func TestEnhancePityCap(t *testing.T) {
	d := realData(t)
	// +15 is 10%: 90 protected failures in a row raise pity[14] to 90, its
	// cap, and the 91st attempt succeeds whatever the roll.
	inv, it := forge(t10, 14, []int{999}, 999)
	xu := rich
	res, err := inv.Enhance(rolls(99), d, bag0, true, 90, &xu)
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 90 || res.Success || it.Enhance != 14 || it.Pity[14] != 90 {
		t.Fatalf("%+v, item +%d pity %d", res, it.Enhance, it.Pity[14])
	}
	res, _ = inv.Enhance(rolls(99), d, bag0, true, 1, &xu)
	if !res.Success || it.Enhance != 15 || it.Pity[14] != 0 {
		t.Fatalf("with pity 90: %+v, item +%d pity %d", res, it.Enhance, it.Pity[14])
	}
	// One request does the same: 91 attempts, the last one succeeds.
	inv, it = forge(t10, 14, []int{999}, 999)
	if res, _ := inv.Enhance(rolls(99), d, bag0, true, 100, &xu); res.Attempts != 91 || !res.Success || it.Enhance != 15 {
		t.Fatalf("times 100: %+v", res)
	}

	// With a bigger pity step the cap binds below the step: 84 + 7 -> 90.
	big := *d
	big.Enhance.PityPct = 7
	inv, it = forge(t10, 14, []int{999}, 999)
	it.Pity[14] = 84
	if res, _ := inv.Enhance(rolls(99), &big, bag0, true, 1, &xu); res.Success || it.Pity[14] != 90 {
		t.Fatalf("pity step 7 from 84: %+v pity %d, want capped at 90", res, it.Pity[14])
	}
	// +5 (90%) caps at 10.
	inv, it = forge(t10, 4, []int{999}, 0)
	it.Pity[4] = 9
	if res, _ := inv.Enhance(rolls(99), &big, bag0, false, 1, &xu); res.Success || it.Pity[4] != 10 {
		t.Fatalf("+5 pity step 7 from 9: %+v pity %d, want 10", res, it.Pity[4])
	}
}

func TestEnhanceCharm(t *testing.T) {
	d := realData(t)
	xu := rich
	// Target +7 cannot drop: protect takes no charm and needs none.
	inv, it := forge(t10, 6, []int{10}, 5)
	res, err := inv.Enhance(rolls(99), d, bag0, true, 1, &xu)
	if err != nil || res.Success || it.Enhance != 6 || inv.Bag[2].Count != 5 || !slices.Equal(res.Changed, []int{0, 1}) {
		t.Fatalf("+7 with protect: %+v %v, charms %d", res, err, inv.Bag[2].Count)
	}
	inv, _ = forge(t10, 6, []int{10}, 0)
	if _, err := inv.Enhance(rolls(99), d, bag0, true, 1, &xu); err != nil {
		t.Fatalf("+7 with protect and no charm: %v", err)
	}
	// Target +8: protect takes one charm and a failure keeps +7.
	inv, it = forge(t10, 7, []int{10}, 5)
	res, err = inv.Enhance(rolls(99), d, bag0, true, 1, &xu)
	if err != nil || res.Success || res.Level != 7 || it.Enhance != 7 || inv.Bag[2].Count != 4 || it.Pity[7] != 1 ||
		!slices.Equal(res.Changed, []int{0, 1, 2}) {
		t.Fatalf("+8 with protect: %+v %v, charms %d pity %v", res, err, inv.Bag[2].Count, it.Pity)
	}
	// A success with protect also uses a charm.
	res, _ = inv.Enhance(rolls(0), d, bag0, true, 1, &xu)
	if !res.Success || it.Enhance != 8 || inv.Bag[2].Count != 3 {
		t.Fatalf("+8 success with protect: %+v charms %d", res, inv.Bag[2].Count)
	}
	// Without protect no charm is taken and the failure drops.
	inv, it = forge(t10, 7, []int{10}, 5)
	res, _ = inv.Enhance(rolls(99), d, bag0, false, 1, &xu)
	if res.Level != 6 || it.Enhance != 6 || inv.Bag[2].Count != 5 {
		t.Fatalf("+8 without protect: %+v charms %d", res, inv.Bag[2].Count)
	}
}

func TestEnhanceTimesStops(t *testing.T) {
	d := realData(t)
	cost := func(target int) int64 { return int64(d.Enhance.Levels[target-1].XuPerTier) * 10 }
	type run struct {
		name           string
		level          int
		stones         []int
		charms         int
		protect        bool
		times          int
		xu             int64
		rolls          []int
		attempts       int
		success        bool
		wantLevel      int
		stonesLeft     int // at bag slot 1
		charmsLeftSlot int // bag slot of the charm stack, 0 for none
		charmsLeft     int
	}
	for _, r := range []run{
		// +5 -> +6 (80%): fail, fail, succeed.
		{name: "first success", level: 5, stones: []int{100}, times: 100, xu: rich, rolls: []int{99, 99, 0},
			attempts: 3, success: true, wantLevel: 6, stonesLeft: 94},
		// +8 -> +9 without protect: the first failure drops and stops.
		{name: "drop", level: 8, stones: []int{100}, times: 100, xu: rich, rolls: []int{99},
			attempts: 1, wantLevel: 7, stonesLeft: 97},
		// +4 -> +5 takes 2 stones: 5 stones pay for two attempts.
		{name: "stones", level: 4, stones: []int{5}, times: 100, xu: rich, rolls: []int{99},
			attempts: 2, wantLevel: 4, stonesLeft: 1},
		// +8 -> +9 protected: 3 charms pay for three attempts.
		{name: "charms", level: 8, stones: []int{100}, charms: 3, protect: true, times: 100, xu: rich, rolls: []int{99},
			attempts: 3, wantLevel: 8, stonesLeft: 91, charmsLeftSlot: 2, charmsLeft: 0},
		// +15 Xu for exactly three attempts and one short of a fourth.
		{name: "xu", level: 14, stones: []int{100}, charms: 10, protect: true, times: 100, xu: 4*cost(15) - 1, rolls: []int{99},
			attempts: 3, wantLevel: 14, stonesLeft: 85, charmsLeftSlot: 2, charmsLeft: 7},
		{name: "times", level: 14, stones: []int{999}, charms: 999, protect: true, times: 50, xu: rich, rolls: []int{99},
			attempts: 50, wantLevel: 14, stonesLeft: 749, charmsLeftSlot: 2, charmsLeft: 949},
		{name: "times 0 is 1", level: 5, stones: []int{100}, times: 0, xu: rich, rolls: []int{99},
			attempts: 1, wantLevel: 5, stonesLeft: 98},
		// Stones run out over two stacks; the emptied stack leaves its slot.
		{name: "two stacks", level: 4, stones: []int{1, 4}, times: 100, xu: rich, rolls: []int{99},
			attempts: 2, wantLevel: 4, stonesLeft: -1},
	} {
		t.Run(r.name, func(t *testing.T) {
			inv, it := forge(t10, r.level, r.stones, r.charms)
			xu := r.xu
			res, err := inv.Enhance(rolls(r.rolls...), d, bag0, r.protect, r.times, &xu)
			if err != nil {
				t.Fatal(err)
			}
			if res.Attempts != r.attempts || res.Success != r.success || res.Level != r.wantLevel || it.Enhance != r.wantLevel {
				t.Fatalf("%+v, item +%d", res, it.Enhance)
			}
			if r.stonesLeft >= 0 && inv.Bag[1].Count != r.stonesLeft {
				t.Fatalf("%d stones left, want %d", inv.Bag[1].Count, r.stonesLeft)
			}
			if r.charmsLeftSlot > 0 && inv.Count(gamedata.CharmProtect) != r.charmsLeft {
				t.Fatalf("%d charms left, want %d", inv.Count(gamedata.CharmProtect), r.charmsLeft)
			}
			if want := int64(r.attempts) * cost(r.level+1); r.xu-xu != want {
				t.Fatalf("spent %d Xu, want %d", r.xu-xu, want)
			}
		})
	}
}

func TestEnhanceErrors(t *testing.T) {
	d := realData(t)
	type tc struct {
		name    string
		setup   func(inv *Inventory) // on forge(t10, 7, {100}, 0)
		ref     Ref
		protect bool
		times   int
		xu      int64
		want    error
	}
	full := int64(1000) // +8 costs 800 Xu for tier 10
	for _, c := range []tc{
		{"bag slot -1", nil, Ref{Slot: -1}, false, -1, 0, ErrBadSlot},
		{"bag slot 60", nil, Ref{Slot: 60}, false, 101, 0, ErrBadSlot},
		{"worn slot 10", nil, Ref{Equipped: true, Slot: 10}, false, 1, full, ErrBadSlot},
		{"worn slot -1", nil, Ref{Equipped: true, Slot: -1}, false, 1, full, ErrBadSlot},
		{"times -1", nil, Ref{Slot: 5}, false, -1, 0, ErrBadCount},
		{"times 101", nil, bag0, false, 101, full, ErrBadCount},
		{"empty bag slot", nil, Ref{Slot: 5}, false, 1, 0, ErrNotFound},
		{"empty worn slot", nil, Ref{Equipped: true, Slot: 3}, false, 1, 0, ErrNotFound},
		{"stone", nil, Ref{Slot: 1}, false, 1, 0, ErrNotEquipment},
		{"maxed", func(inv *Inventory) { inv.Bag[0].Enhance = 15; inv.Bag[1] = nil }, bag0, false, 1, 0, ErrMaxed},
		{"no stones", func(inv *Inventory) { inv.Bag[1] = nil }, bag0, false, 1, 0, ErrNoMaterials},
		{"too few stones", func(inv *Inventory) { inv.Bag[1].Count = 2 }, bag0, false, 1, 0, ErrNoMaterials},
		{"protect without charm", nil, bag0, true, 1, 0, ErrNoMaterials},
		{"xu one short", nil, bag0, false, 1, 799, ErrNoXu},
		{"xu one short with charm", func(inv *Inventory) { inv.Bag[2] = charm(1) }, bag0, true, 1, 799, ErrNoXu},
	} {
		t.Run(c.name, func(t *testing.T) {
			inv, _ := forge(t10, 7, []int{100}, 0)
			if c.setup != nil {
				c.setup(inv)
			}
			before := inv.Clone()
			xu := c.xu
			res, err := inv.Enhance(rolls(0), d, c.ref, c.protect, c.times, &xu)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			if res.Attempts != 0 || !sameInventory(inv, before) || xu != c.xu {
				t.Fatalf("an error changed something: %+v xu %d", res, xu)
			}
		})
	}
	// A worn piece is enhanced in place and only material slots change.
	inv, _ := forge(t10, 0, []int{5}, 0)
	inv.Worn[0], inv.Bag[0] = inv.Bag[0], nil
	xu := rich
	res, err := inv.Enhance(rolls(0), d, Ref{Equipped: true, Slot: 0}, false, 1, &xu)
	if err != nil || inv.Worn[0].Enhance != 1 || !slices.Equal(res.Changed, []int{1}) {
		t.Fatalf("worn: %+v %v", res, err)
	}
	// times = max_times is allowed.
	inv, _ = forge(t10, 0, []int{5}, 0)
	if _, err := inv.Enhance(rolls(0), d, bag0, false, d.Enhance.MaxTimes, &xu); err != nil {
		t.Fatalf("times %d: %v", d.Enhance.MaxTimes, err)
	}
}

func TestEnhanceTakesLowestSlots(t *testing.T) {
	d := realData(t)
	inv := NewInventory(60)
	inv.Bag[3] = stone(4)
	inv.Bag[7] = stone(5)
	inv.Bag[9] = charm(1)
	inv.Bag[10] = &Item{Code: t10, Count: 1, Enhance: 7}
	inv.Bag[12] = charm(4)
	inv.Bag[20] = stone(9)
	xu := rich
	// +8 takes 3 stones and a charm per attempt: two attempts take slot 3's
	// four stones, two of slot 7's, and slot 9's charm then one of slot 12's.
	res, err := inv.Enhance(rolls(99, 0), d, Ref{Slot: 10}, true, 100, &xu)
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 2 || !res.Success || res.Level != 8 {
		t.Fatalf("%+v", res)
	}
	if inv.Bag[3] != nil || inv.Bag[7].Count != 3 || inv.Bag[20].Count != 9 || inv.Bag[9] != nil || inv.Bag[12].Count != 3 {
		t.Fatalf("bag 3 %+v, 7 %+v, 9 %+v, 12 %+v, 20 %+v", inv.Bag[3], inv.Bag[7], inv.Bag[9], inv.Bag[12], inv.Bag[20])
	}
	if !slices.Equal(res.Changed, []int{3, 7, 9, 10, 12}) {
		t.Fatalf("changed %v", res.Changed)
	}
}

func TestCountTakeAt(t *testing.T) {
	inv := NewInventory(6)
	inv.Bag[1], inv.Bag[2], inv.Bag[4] = stone(3), sword(), stone(5)
	inv.Worn[0] = sword()
	if inv.Count(gamedata.StoneEnhance) != 8 || inv.Count(gamedata.CharmProtect) != 0 {
		t.Fatal("Count")
	}
	if ch := inv.Take(gamedata.StoneEnhance, 3); !slices.Equal(ch, []int{1}) || inv.Bag[1] != nil || inv.Bag[4].Count != 5 {
		t.Fatalf("Take 3: %v", ch)
	}
	inv.Bag[0] = stone(2)
	if ch := inv.Take(gamedata.StoneEnhance, 4); !slices.Equal(ch, []int{0, 4}) || inv.Bag[0] != nil || inv.Bag[4].Count != 3 {
		t.Fatalf("Take 4: %v", ch)
	}
	if ch := inv.Take(gamedata.StoneEnhance, 0); len(ch) != 0 {
		t.Fatalf("Take 0: %v", ch)
	}
	for _, c := range []struct {
		r    Ref
		want *Item
		err  error
	}{
		{Ref{Slot: 2}, inv.Bag[2], nil},
		{Ref{Equipped: true, Slot: 0}, inv.Worn[0], nil},
		{Ref{Slot: 3}, nil, ErrNotFound},
		{Ref{Equipped: true, Slot: 9}, nil, ErrNotFound},
		{Ref{Slot: 6}, nil, ErrBadSlot},
		{Ref{Slot: -1}, nil, ErrBadSlot},
		{Ref{Equipped: true, Slot: 10}, nil, ErrBadSlot},
	} {
		if got, err := inv.At(c.r); got != c.want || !errors.Is(err, c.err) {
			t.Errorf("At(%+v) = %p, %v; want %p, %v", c.r, got, err, c.want, c.err)
		}
	}
}

// climb enhances a fresh tier 10 piece from +0 to target with a seeded RNG,
// times 100 per request and materials topped up, and returns the attempts.
func climb(rng *rand.Rand, d *gamedata.Items, target int, protect bool) int {
	inv := NewInventory(4)
	it := &Item{Code: t10, Count: 1}
	inv.Bag[0] = it
	attempts := 0
	for it.Enhance < target {
		inv.Bag[1], inv.Bag[2] = stone(999), charm(999)
		xu := rich
		res, err := inv.Enhance(rng, d, bag0, protect, 100, &xu)
		if err != nil {
			panic(err)
		}
		attempts += res.Attempts
	}
	return attempts
}

// The mean attempts to +7 match the exact expectation of the pity rule,
// and +15 without charms averages about 6,500 (game-design 3.3; spec 2.1).
func TestEnhanceMeanAttempts(t *testing.T) {
	d := realData(t)
	// Exact mean attempts at one kept-level target from pity 0: the n-th
	// attempt happens when the n-1 before failed.
	keptMean := func(pct int) float64 {
		mean, reach := 0.0, 1.0
		for p := 0; reach > 0; p += d.Enhance.PityPct {
			mean += reach
			reach *= 1 - float64(min(100, pct+p))/100
		}
		return mean
	}
	want7 := 4.0 // +1 .. +4 always succeed
	for _, l := range d.Enhance.Levels[4:7] {
		want7 += keptMean(l.SuccessPct)
	}
	const runs7 = 40000
	rng := seeded(7)
	var sum, sumSq float64
	for range runs7 {
		a := float64(climb(rng, d, 7, false))
		sum += a
		sumSq += a * a
	}
	mean := sum / runs7
	se := math.Sqrt((sumSq/runs7 - mean*mean) / runs7)
	if math.Abs(mean-want7) > 4*se {
		t.Fatalf("+7 mean attempts %.3f, want %.3f +- %.3f", mean, want7, 4*se)
	}

	const runs15 = 1000
	rng = seeded(15)
	sum = 0
	for range runs15 {
		sum += float64(climb(rng, d, 15, false))
	}
	if mean := sum / runs15; mean < 5500 || mean > 7500 {
		t.Fatalf("+15 without charms: mean attempts %.0f, want 5,500-7,500", mean)
	}
	// Protected from +8 it is a few dozen (spec 2.1: about 37).
	rng = seeded(16)
	sum = 0
	for range 4000 {
		sum += float64(climb(rng, d, 15, true))
	}
	if mean := sum / 4000; mean < 35 || mean > 39 {
		t.Fatalf("+15 with charms: mean attempts %.1f, want about 37", mean)
	}
}

// The first +15 attempt from pity 0 succeeds 10% of the time.
func TestEnhanceRate(t *testing.T) {
	d := realData(t)
	rng := seeded(10)
	const n = 100000
	wins := 0
	for range n {
		inv, _ := forge(t10, 14, []int{5}, 1)
		xu := rich
		if res, _ := inv.Enhance(rng, d, bag0, true, 1, &xu); res.Success {
			wins++
		}
	}
	if p := float64(wins) / n; math.Abs(p-0.10) > 4*math.Sqrt(0.1*0.9/n) {
		t.Fatalf("+15 success rate %.4f, want 0.10", p)
	}
}

// ______________________TEST REGEN_____________________

func TestMilestone(t *testing.T) {
	d := realData(t)
	for _, c := range []struct {
		enhance int
		ok      bool
		want    gamedata.RegenStep
	}{
		{0, false, gamedata.RegenStep{}}, {6, false, gamedata.RegenStep{}},
		{7, true, gamedata.RegenStep{Enhance: 7, HPBP: 5, MP: 1}},
		{9, true, gamedata.RegenStep{Enhance: 7, HPBP: 5, MP: 1}},
		{10, true, gamedata.RegenStep{Enhance: 10, HPBP: 10, MP: 2}},
		{11, true, gamedata.RegenStep{Enhance: 10, HPBP: 10, MP: 2}},
		{12, true, gamedata.RegenStep{Enhance: 12, HPBP: 20, MP: 3}},
		{14, true, gamedata.RegenStep{Enhance: 12, HPBP: 20, MP: 3}},
		{15, true, gamedata.RegenStep{Enhance: 15, HPBP: 30, MP: 5}},
	} {
		if got, ok := Milestone(d, c.enhance); ok != c.ok || got != c.want {
			t.Errorf("Milestone(+%d) = %+v %v, want %+v %v", c.enhance, got, ok, c.want, c.ok)
		}
	}
}

// fullSet wears the tier 50 set for class at enhance n.
func fullSet(d *gamedata.Items, class string, n int) *Inventory {
	inv := NewInventory(10)
	inv.Worn[0] = &Item{Code: d.Weapon(50, class), Count: 1, Enhance: n}
	for slot := 1; slot < EquipSlots; slot++ {
		inv.Worn[slot] = &Item{Code: d.Piece(50, slot), Count: 1, Enhance: n}
	}
	return inv
}

func TestRegen(t *testing.T) {
	d := realData(t)
	if hp, mp := Regen(d, fullSet(d, "viem", 15), 90, "viem"); hp != 150 || mp != 25 {
		t.Fatalf("ten +15 pieces: %d bp %d MP, want 150 and 25", hp, mp)
	}
	if hp, mp := Regen(d, fullSet(d, "viem", 6), 90, "viem"); hp != 0 || mp != 0 {
		t.Fatalf("ten +6 pieces: %d bp %d MP", hp, mp)
	}
	inv := fullSet(d, "viem", 0)
	inv.Worn[1].Enhance = 7  // boots: HP
	inv.Worn[5].Enhance = 12 // armour: HP
	inv.Worn[0].Enhance = 10 // weapon: MP
	inv.Worn[8].Enhance = 15 // left ring: MP
	inv.Bag[0] = &Item{Code: d.Piece(50, 4), Count: 1, Enhance: 15}
	if hp, mp := Regen(d, inv, 90, "viem"); hp != 25 || mp != 7 {
		t.Fatalf("mixed: %d bp %d MP, want 25 and 7", hp, mp)
	}
	// Unwearable worn pieces add nothing: a weapon of another class, and
	// everything at a level below the tier 50 pieces' wear levels.
	if hp, mp := Regen(d, fullSet(d, "viem", 15), 90, "thuy"); hp != 150 || mp != 20 {
		t.Fatalf("another class's weapon: %d bp %d MP, want 150 and 20", hp, mp)
	}
	// Level 54 wears slots 0-4 (levels 50-54) only: the weapon and four armour pieces.
	if hp, mp := Regen(d, fullSet(d, "viem", 15), 54, "viem"); hp != 120 || mp != 5 {
		t.Fatalf("level 54: %d bp %d MP, want 120 and 5", hp, mp)
	}
	if hp, mp := Regen(d, fullSet(d, "viem", 15), 1, "viem"); hp != 0 || mp != 0 {
		t.Fatalf("level 1: %d bp %d MP", hp, mp)
	}
}
