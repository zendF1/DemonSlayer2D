package gamedata

import (
	"fmt"
	"maps"
	"slices"

	charclass "demonslayer2d/internal/classes"
)

// Equipment slot ids, in slot index order (phase 3a spec 2.1). The index is
// also how many levels above its tier a piece is worn.
const (
	SlotWeapon    = "weapon"
	SlotBoots     = "boots"
	SlotBracers   = "bracers"
	SlotPants     = "pants"
	SlotHelmet    = "helmet"
	SlotArmor     = "armor"
	SlotNecklace  = "necklace"
	SlotPendant   = "pendant"
	SlotRingLeft  = "ring_left"
	SlotRingRight = "ring_right"
)

// SlotIDs lists the slot ids by index.
var SlotIDs = [10]string{SlotWeapon, SlotBoots, SlotBracers, SlotPants, SlotHelmet,
	SlotArmor, SlotNecklace, SlotPendant, SlotRingLeft, SlotRingRight}

// Line stat keys: the stats an item line can raise.
const (
	StatHP           = "hp"
	StatPhysAttack   = "phys_attack"
	StatMagicAttack  = "magic_attack"
	StatPhysDefense  = "phys_defense"
	StatMagicDefense = "magic_defense"
)

var statKeys = map[string]bool{StatHP: true, StatPhysAttack: true, StatMagicAttack: true,
	StatPhysDefense: true, StatMagicDefense: true}

// Monster ranks; each has a drop table.
const (
	RankNormal = "normal"
	RankElite  = "elite"
	RankBoss   = "boss"
)

var rankList = []string{RankNormal, RankElite, RankBoss}

// EnhanceLevels is how many enhance levels (+0 .. +15) an equipment piece
// has; every equipment stat array has exactly this many entries.
const EnhanceLevels = 16

const (
	qualityCount = 5
	// firstHiddenQuality is the first quality (Vàng) with an element and
	// hidden lines; the ones below have neither.
	firstHiddenQuality = 2
	maxBagSize         = 200
	maxChanceBP        = 10000
)

// Slot is one of the ten equipment slots.
type Slot struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Index int    `json:"index"`
}

// Quality is an item quality; its index in Items.Qualities is the quality
// number used by the protocol and the database. Lines is the (min, max)
// number of lines; HiddenPct > 0 means an element and two hidden lines.
type Quality struct {
	Name      string
	Rank      string
	Lines     [2]int
	HiddenPct int
}

// Tier is an equipment tier; LineStandard is the standard line value S of
// each line stat.
type Tier struct {
	Tier         int            `json:"tier"`
	Name         string         `json:"name"`
	LineStandard map[string]int `json:"line_standard"`
}

// Equipment is one equipment piece. Class is set on weapons only. The five
// stat arrays are indexed by enhance level (+0 .. +15).
type Equipment struct {
	Code         string `json:"code"`
	Tier         int    `json:"tier"`
	Slot         string `json:"slot"`
	Class        string `json:"class"`
	Name         string `json:"name"`
	Level        int    `json:"level"`
	HP           []int  `json:"hp"`
	PhysAttack   []int  `json:"phys_attack"`
	MagicAttack  []int  `json:"magic_attack"`
	PhysDefense  []int  `json:"phys_defense"`
	MagicDefense []int  `json:"magic_defense"`
	// SlotIndex is the index of Slot, filled after parsing.
	SlotIndex int `json:"-"`
}

// Stackable is an item that stacks in the bag (stones).
type Stackable struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Drop is the drop table of one monster rank (phase 3a spec 4.3). Xu is
// rolled in XuPerLevel[0..1] times the monster level.
type Drop struct {
	XuPerLevel [2]int
	Equipment  DropEquipment
	Stacks     []DropStack
}

// DropEquipment drops Count pieces with ChanceBP / 10000; QualityBP is the
// chance of each quality, in basis points summing to 10000.
type DropEquipment struct {
	ChanceBP  int   `json:"chance_bp"`
	Count     int   `json:"count"`
	QualityBP []int `json:"quality_bp"`
}

// DropStack drops one stack of Count of Code with ChanceBP / 10000.
type DropStack struct {
	Code     string `json:"code"`
	ChanceBP int    `json:"chance_bp"`
	Count    int    `json:"count"`
}

// Items is the parsed items.json.
type Items struct {
	BagSize, StackMax int
	// GroundSec is how long a dropped item stays on the ground.
	GroundSec      float64
	Slots          []Slot
	Qualities      []Quality
	LineStats      []string
	LineRollMinPct int
	Tiers          map[int]Tier
	Equipment      map[string]Equipment
	Stackables     map[string]Stackable
	// StackOrder lists the stackable codes in file order.
	StackOrder []string
	// Drops is keyed by monster rank.
	Drops map[string]Drop

	pieces map[pieceKey]string
}

// pieceKey finds a piece: weapons by tier and class, the rest by tier and
// slot index with an empty class.
type pieceKey struct {
	tier, slot int
	class      string
}

type itemsFile struct {
	BagSize        int                 `json:"bag_size"`
	StackMax       int                 `json:"stack_max"`
	GroundSec      float64             `json:"ground_sec"`
	Slots          []Slot              `json:"slots"`
	Qualities      []qualityFile       `json:"qualities"`
	LineStats      []string            `json:"line_stats"`
	LineRollMinPct int                 `json:"line_roll_min_pct"`
	Tiers          []Tier              `json:"tiers"`
	Equipment      []Equipment         `json:"equipment"`
	Stackables     []Stackable         `json:"stackables"`
	Drops          map[string]dropFile `json:"drops"`
}

// qualityFile and dropFile take pairs as slices so a wrong length is an
// error rather than silently truncated or zero-filled.
type qualityFile struct {
	Name      string `json:"name"`
	Rank      string `json:"rank"`
	Lines     []int  `json:"lines"`
	HiddenPct int    `json:"hidden_pct"`
}

type dropFile struct {
	XuPerLevel []int         `json:"xu_per_level"`
	Equipment  DropEquipment `json:"equipment"`
	Stacks     []DropStack   `json:"stacks"`
}

// IsEquipment reports whether code is an equipment piece.
func (it *Items) IsEquipment(code string) bool {
	_, ok := it.Equipment[code]
	return ok
}

// Weapon is the code of the tier's weapon for a class, or "" if none.
func (it *Items) Weapon(tier int, class string) string {
	return it.pieces[pieceKey{tier: tier, slot: 0, class: class}]
}

// Piece is the code of the tier's shared piece in slot index 1-9, or "" if
// none; weapons (slot 0) need a class, see Weapon.
func (it *Items) Piece(tier, slotIndex int) string {
	if slotIndex == 0 {
		return ""
	}
	return it.pieces[pieceKey{tier: tier, slot: slotIndex}]
}

// ParseItems decodes and validates items.json (phase 3a spec 2.1).
func ParseItems(b []byte) (*Items, error) {
	var f itemsFile
	if err := decodeStrict(b, &f); err != nil {
		return nil, fmt.Errorf("gamedata: items: decode: %w", err)
	}
	it := &Items{BagSize: f.BagSize, StackMax: f.StackMax, GroundSec: f.GroundSec, Slots: f.Slots,
		LineStats: f.LineStats, LineRollMinPct: f.LineRollMinPct}
	for _, step := range []func(*Items, *itemsFile) error{
		parseScalars, parseSlots, parseLineStats, parseQualities, parseTiers, parseEquipment, parseStackables, parseDrops,
	} {
		if err := step(it, &f); err != nil {
			return nil, fmt.Errorf("gamedata: items: %w", err)
		}
	}
	return it, nil
}

func parseScalars(it *Items, _ *itemsFile) error {
	if it.BagSize < 1 || it.BagSize > maxBagSize {
		return fmt.Errorf("bag_size %d must be in 1-%d", it.BagSize, maxBagSize)
	}
	if it.StackMax < 1 {
		return fmt.Errorf("stack_max %d must be at least 1", it.StackMax)
	}
	// The negated comparison also rejects NaN.
	if !(it.GroundSec > 0) {
		return fmt.Errorf("ground_sec must be positive")
	}
	if it.LineRollMinPct < 1 || it.LineRollMinPct > 100 {
		return fmt.Errorf("line_roll_min_pct %d must be in 1-100", it.LineRollMinPct)
	}
	return nil
}

func parseSlots(it *Items, _ *itemsFile) error {
	if len(it.Slots) != len(SlotIDs) {
		return fmt.Errorf("slots has %d entries, want %d", len(it.Slots), len(SlotIDs))
	}
	for i, s := range it.Slots {
		if s.ID != SlotIDs[i] {
			return fmt.Errorf("slots[%d] is %q, want %q", i, s.ID, SlotIDs[i])
		}
		if s.Index != i {
			return fmt.Errorf("slot %q: index %d, want %d", s.ID, s.Index, i)
		}
		if s.Name == "" {
			return fmt.Errorf("slot %q: name is empty", s.ID)
		}
	}
	return nil
}

func parseLineStats(it *Items, _ *itemsFile) error {
	seen := map[string]bool{}
	for _, s := range it.LineStats {
		if !statKeys[s] {
			return fmt.Errorf("line stat %q is unknown", s)
		}
		if seen[s] {
			return fmt.Errorf("line stat %q is listed twice", s)
		}
		seen[s] = true
	}
	return nil
}

func parseQualities(it *Items, f *itemsFile) error {
	if len(f.Qualities) != qualityCount {
		return fmt.Errorf("qualities has %d entries, want %d", len(f.Qualities), qualityCount)
	}
	for i, q := range f.Qualities {
		if q.Name == "" {
			return fmt.Errorf("quality %d: name is empty", i)
		}
		if len(q.Lines) != 2 {
			return fmt.Errorf("quality %d %q: lines has %d entries, want 2", i, q.Name, len(q.Lines))
		}
		if min, max := q.Lines[0], q.Lines[1]; min < 0 || min > max || max > len(it.LineStats) {
			return fmt.Errorf("quality %d %q: lines %v must satisfy 0 <= min <= max <= %d", i, q.Name, q.Lines, len(it.LineStats))
		}
		if i >= firstHiddenQuality && q.HiddenPct <= 0 {
			return fmt.Errorf("quality %d %q: hidden_pct %d must be positive", i, q.Name, q.HiddenPct)
		}
		if i < firstHiddenQuality && q.HiddenPct != 0 {
			return fmt.Errorf("quality %d %q: hidden_pct %d must be 0", i, q.Name, q.HiddenPct)
		}
		it.Qualities = append(it.Qualities, Quality{Name: q.Name, Rank: q.Rank, Lines: [2]int{q.Lines[0], q.Lines[1]}, HiddenPct: q.HiddenPct})
	}
	return nil
}

func parseTiers(it *Items, f *itemsFile) error {
	it.Tiers = make(map[int]Tier, len(f.Tiers))
	for _, t := range f.Tiers {
		if t.Tier <= 0 {
			return fmt.Errorf("tier %d must be positive", t.Tier)
		}
		if _, dup := it.Tiers[t.Tier]; dup {
			return fmt.Errorf("tier %d is listed twice", t.Tier)
		}
		if t.Name == "" {
			return fmt.Errorf("tier %d: name is empty", t.Tier)
		}
		for _, s := range it.LineStats {
			v, ok := t.LineStandard[s]
			if !ok {
				return fmt.Errorf("tier %d: line_standard has no %q", t.Tier, s)
			}
			if v < 1 {
				return fmt.Errorf("tier %d: line_standard %q %d must be at least 1", t.Tier, s, v)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(t.LineStandard)) {
			if !slices.Contains(it.LineStats, k) {
				return fmt.Errorf("tier %d: line_standard %q is not a line stat", t.Tier, k)
			}
		}
		it.Tiers[t.Tier] = t
	}
	return nil
}

func parseEquipment(it *Items, f *itemsFile) error {
	slotIndex := map[string]int{}
	for i, id := range SlotIDs {
		slotIndex[id] = i
	}
	it.Equipment = make(map[string]Equipment, len(f.Equipment))
	it.pieces = map[pieceKey]string{}
	for _, e := range f.Equipment {
		if e.Code == "" {
			return fmt.Errorf("equipment with empty code")
		}
		if _, dup := it.Equipment[e.Code]; dup {
			return fmt.Errorf("duplicate item code %q", e.Code)
		}
		idx, ok := slotIndex[e.Slot]
		if !ok {
			return fmt.Errorf("equipment %q: slot %q is unknown", e.Code, e.Slot)
		}
		e.SlotIndex = idx
		if err := validateEquipment(it, e); err != nil {
			return fmt.Errorf("equipment %q: %w", e.Code, err)
		}
		key := pieceKey{tier: e.Tier, slot: idx, class: e.Class}
		if _, dup := it.pieces[key]; dup {
			return fmt.Errorf("equipment %q: tier %d already has a %q piece%s", e.Code, e.Tier, e.Slot, forClass(e.Class))
		}
		it.pieces[key] = e.Code
		it.Equipment[e.Code] = e
	}
	// Every tier has its nine shared pieces and one weapon per class.
	for _, tier := range slices.Sorted(maps.Keys(it.Tiers)) {
		for idx, slot := range SlotIDs {
			wanted := []string{""}
			if idx == 0 {
				wanted = classList
			}
			for _, class := range wanted {
				if _, ok := it.pieces[pieceKey{tier: tier, slot: idx, class: class}]; !ok {
					return fmt.Errorf("tier %d has no %q piece%s", tier, slot, forClass(class))
				}
			}
		}
	}
	return nil
}

func forClass(class string) string {
	if class == "" {
		return ""
	}
	return fmt.Sprintf(" for class %q", class)
}

// validateEquipment checks one piece whose code and slot are already known.
func validateEquipment(it *Items, e Equipment) error {
	if _, ok := it.Tiers[e.Tier]; !ok {
		return fmt.Errorf("tier %d is not in tiers", e.Tier)
	}
	if e.Name == "" {
		return fmt.Errorf("name is empty")
	}
	weapon := e.Slot == SlotWeapon
	if weapon && !classes[e.Class] {
		return fmt.Errorf("class %q is not valid", e.Class)
	}
	if !weapon && e.Class != "" {
		return fmt.Errorf("class must be empty, got %q (only weapons have one)", e.Class)
	}
	if want := e.Tier + e.SlotIndex; e.Level != want {
		return fmt.Errorf("level %d, want %d", e.Level, want)
	}
	arrays := []struct {
		name string
		a    []int
	}{{StatHP, e.HP}, {StatPhysAttack, e.PhysAttack}, {StatMagicAttack, e.MagicAttack},
		{StatPhysDefense, e.PhysDefense}, {StatMagicDefense, e.MagicDefense}}
	for _, arr := range arrays {
		if len(arr.a) != EnhanceLevels {
			return fmt.Errorf("%s has %d entries, want %d", arr.name, len(arr.a), EnhanceLevels)
		}
		for n, v := range arr.a {
			if v < 0 {
				return fmt.Errorf("%s must not be negative", arr.name)
			}
			if n > 0 && v < arr.a[n-1] {
				return fmt.Errorf("%s must not decrease at +%d", arr.name, n)
			}
		}
	}
	if weapon {
		// A weapon's attack is all physical or all magic, by its class.
		if charclass.IsMagic(charclass.ToPB(e.Class)) {
			if slices.ContainsFunc(e.PhysAttack, func(v int) bool { return v != 0 }) {
				return fmt.Errorf("phys_attack must be 0 for magic class %q", e.Class)
			}
		} else if slices.ContainsFunc(e.MagicAttack, func(v int) bool { return v != 0 }) {
			return fmt.Errorf("magic_attack must be 0 for physical class %q", e.Class)
		}
	}
	return nil
}

func parseStackables(it *Items, f *itemsFile) error {
	it.Stackables = make(map[string]Stackable, len(f.Stackables))
	for _, s := range f.Stackables {
		if s.Code == "" {
			return fmt.Errorf("stackable with empty code")
		}
		if _, dup := it.Stackables[s.Code]; dup || it.IsEquipment(s.Code) {
			return fmt.Errorf("duplicate item code %q", s.Code)
		}
		if s.Name == "" {
			return fmt.Errorf("stackable %q: name is empty", s.Code)
		}
		it.Stackables[s.Code] = s
		it.StackOrder = append(it.StackOrder, s.Code)
	}
	return nil
}

func parseDrops(it *Items, f *itemsFile) error {
	for _, r := range slices.Sorted(maps.Keys(f.Drops)) {
		if !slices.Contains(rankList, r) {
			return fmt.Errorf("drops %q is not a rank", r)
		}
	}
	it.Drops = make(map[string]Drop, len(rankList))
	for _, r := range rankList {
		d, ok := f.Drops[r]
		if !ok {
			return fmt.Errorf("drops has no %q", r)
		}
		drop, err := parseDrop(it, d)
		if err != nil {
			return fmt.Errorf("drops %q: %w", r, err)
		}
		it.Drops[r] = drop
	}
	return nil
}

func parseDrop(it *Items, d dropFile) (Drop, error) {
	if len(d.XuPerLevel) != 2 {
		return Drop{}, fmt.Errorf("xu_per_level has %d entries, want 2", len(d.XuPerLevel))
	}
	if min, max := d.XuPerLevel[0], d.XuPerLevel[1]; min < 0 || min > max {
		return Drop{}, fmt.Errorf("xu_per_level %v must satisfy 0 <= min <= max", d.XuPerLevel)
	}
	eq := d.Equipment
	if eq.ChanceBP < 0 || eq.ChanceBP > maxChanceBP {
		return Drop{}, fmt.Errorf("equipment chance_bp %d must be in 0-%d", eq.ChanceBP, maxChanceBP)
	}
	if eq.Count < 1 {
		return Drop{}, fmt.Errorf("equipment count %d must be at least 1", eq.Count)
	}
	if len(eq.QualityBP) != len(it.Qualities) {
		return Drop{}, fmt.Errorf("quality_bp has %d entries, want %d", len(eq.QualityBP), len(it.Qualities))
	}
	sum := 0
	for _, v := range eq.QualityBP {
		if v < 0 {
			return Drop{}, fmt.Errorf("quality_bp must not be negative")
		}
		sum += v
	}
	if sum != maxChanceBP {
		return Drop{}, fmt.Errorf("quality_bp sums to %d, want %d", sum, maxChanceBP)
	}
	for _, s := range d.Stacks {
		if _, ok := it.Stackables[s.Code]; !ok {
			return Drop{}, fmt.Errorf("stack %q is not a stackable", s.Code)
		}
		if s.ChanceBP < 0 || s.ChanceBP > maxChanceBP {
			return Drop{}, fmt.Errorf("stack %q chance_bp %d must be in 0-%d", s.Code, s.ChanceBP, maxChanceBP)
		}
		if s.Count < 1 {
			return Drop{}, fmt.Errorf("stack %q count %d must be at least 1", s.Code, s.Count)
		}
	}
	return Drop{XuPerLevel: [2]int{d.XuPerLevel[0], d.XuPerLevel[1]}, Equipment: eq, Stacks: d.Stacks}, nil
}
