package items

import (
	"errors"
	"maps"
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
			x.Enhance == y.Enhance && slices.Equal(x.Lines, y.Lines) && slices.Equal(x.Hidden, y.Hidden)
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
	inv.Worn[5] = &Item{Code: "eq10_armor", Count: 1, Lines: []Line{{"hp", 60}}}
	c := inv.Clone()
	if !sameInventory(inv, c) {
		t.Fatal("clone differs")
	}
	c.Bag[1].Lines[0].Value = 1
	c.Bag[1].Hidden[1].Value = 1
	c.Worn[5].Lines[0].Value = 1
	c.Bag[0] = sword()
	if inv.Bag[1].Lines[0].Value != 50 || inv.Bag[1].Hidden[1].Value != 4 || inv.Worn[5].Lines[0].Value != 60 || inv.Bag[0] != nil {
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
