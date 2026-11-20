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

// Stackable codes the enhancement rules (phase 3b spec 2.1) and refining
// (phase 3c spec 2.1) use.
const (
	StoneEnhance = "stone_enhance"
	CharmProtect = "charm_protect"
	StoneRefine  = "stone_refine"
)

// Gem kinds (phase 3c spec 2.1). A gem of level g fits equipment of tier
// 10g or more, so MaxGemLevel is the highest tier's.
const (
	GemAttack   = "attack"
	GemHP       = "hp"
	GemDefense  = "defense"
	MaxGemLevel = MaxTier / TierStep
)

var gemKindList = []string{GemAttack, GemHP, GemDefense}

// Slot regen kinds: worn pieces at an enhance milestone regenerate HP
// (armour) or MP (weapon and jewellery) every second (game-design 2.7).
const (
	RegenHP = "hp"
	RegenMP = "mp"
)

// Equipment tiers: a monster drops tier 10 * (level / 10), clamped to
// MinTier..MaxTier (phase 3a spec 4.3, items.DropTier). items.json must list
// exactly these tiers, so no drop finds a missing tier.
const (
	MinTier  = 10
	MaxTier  = 50
	TierStep = 10
)

// HiddenLineCount is how many hidden lines a piece with an element has, and
// so how many activating slots each slot has.
const HiddenLineCount = 2

const (
	qualityCount = 5
	// firstHiddenQuality is the first quality (Vàng) with an element and
	// hidden lines; the ones below have neither.
	firstHiddenQuality = 2
	maxBagSize         = 200
	maxChanceBP        = 10000
	// alwaysSucceed is the last target level that must succeed every time.
	alwaysSucceed   = 4
	maxEnhanceTimes = 1000
)

// Slot is one of the ten equipment slots. Regen is RegenHP or RegenMP.
type Slot struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Index int    `json:"index"`
	Regen string `json:"regen"`
}

// EnhanceTable is the enhancement rules (phase 3b spec 2.1). Levels[i] is
// the attempt at target level +i+1; PityPct is the hidden rate a failure
// adds; MaxTimes caps the attempts of one request.
type EnhanceTable struct {
	PityPct  int            `json:"pity_pct"`
	MaxTimes int            `json:"max_times"`
	Levels   []EnhanceLevel `json:"levels"`
}

// EnhanceLevel is one attempt at target Level: SuccessPct is the base rate;
// Drop means an unprotected failure drops one level; each attempt costs
// Stones enhance stones and XuPerTier times the item tier in Xu.
type EnhanceLevel struct {
	Level      int  `json:"level"`
	SuccessPct int  `json:"success_pct"`
	Drop       bool `json:"drop"`
	Stones     int  `json:"stones"`
	XuPerTier  int  `json:"xu_per_tier"`
}

// RegenStep is an enhance milestone: a worn piece at Enhance or more (up to
// the next step) regenerates HPBP / 10000 of max HP (armour) or MP (weapon
// and jewellery) every second.
type RegenStep struct {
	Enhance int `json:"enhance"`
	HPBP    int `json:"hp_bp"`
	MP      int `json:"mp"`
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

// Stackable is an item that stacks in the bag (stones, charms, gems). Gem
// is set on gems only.
type Stackable struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Gem  *Gem   `json:"gem,omitempty"`
}

// Gem is what a socketed gem adds (phase 3c spec 2.1): Stats by line stat
// key. Level is 1-MaxGemLevel.
type Gem struct {
	Kind  string         `json:"kind"`
	Level int            `json:"level"`
	Stats map[string]int `json:"stats"`
}

// Resonance is the hidden-line table (phase 3c spec 2.1): Activator[e] is
// the element whose worn piece lights the hidden lines of a piece of
// element e; Slots[s][j] is the slot index whose piece lights hidden line
// j+1 of the piece in slot index s.
type Resonance struct {
	Activator map[string]string
	Slots     [10][2]int
}

// Sockets is the socket rules: at most Max holes; punching hole n+1 costs
// PunchXuPerTier[n] times the item tier in Xu; unsocketing a gem costs
// UnsocketXuPerLevel times its level.
type Sockets struct {
	Max                int   `json:"max"`
	PunchXuPerTier     []int `json:"punch_xu_per_tier"`
	UnsocketXuPerLevel int   `json:"unsocket_xu_per_level"`
}

// Refine is the refining rules: items of quality MinQuality or more; a roll
// costs Stones refine stones and XuPerTier times the tier in Xu, plus
// StonesPerLock and XuPerTierPerLock times the tier per locked line.
type Refine struct {
	MinQuality       int `json:"min_quality"`
	Stones           int `json:"stones"`
	StonesPerLock    int `json:"stones_per_lock"`
	XuPerTier        int `json:"xu_per_tier"`
	XuPerTierPerLock int `json:"xu_per_tier_per_lock"`
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
	// Enhance is the enhancement table; Regen the milestones, ascending.
	Enhance EnhanceTable
	Regen   []RegenStep
	// Resonance, Sockets and Refine are the phase 3c forge rules.
	Resonance Resonance
	Sockets   Sockets
	Refine    Refine

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
	Enhance        EnhanceTable        `json:"enhance"`
	Regen          []RegenStep         `json:"regen"`
	Resonance      resonanceFile       `json:"resonance"`
	Sockets        Sockets             `json:"sockets"`
	Refine         Refine              `json:"refine"`
}

type resonanceFile struct {
	Activator map[string]string   `json:"activator"`
	Slots     map[string][]string `json:"slots"`
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

// IsGem reports whether code is a gem.
func (it *Items) IsGem(code string) bool {
	return it.Stackables[code].Gem != nil
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

// ParseItems decodes and validates items.json (phase 3a spec 2.1, phase 3b
// spec 2.1, phase 3c spec 2.1).
func ParseItems(b []byte) (*Items, error) {
	var f itemsFile
	if err := decodeStrict(b, &f); err != nil {
		return nil, fmt.Errorf("gamedata: items: decode: %w", err)
	}
	it := &Items{BagSize: f.BagSize, StackMax: f.StackMax, GroundSec: f.GroundSec, Slots: f.Slots,
		LineStats: f.LineStats, LineRollMinPct: f.LineRollMinPct, Enhance: f.Enhance, Regen: f.Regen,
		Sockets: f.Sockets, Refine: f.Refine}
	for _, step := range []func(*Items, *itemsFile) error{
		parseScalars, parseSlots, parseLineStats, parseQualities, parseTiers, parseEquipment, parseStackables, parseDrops,
		parseEnhance, parseRegen, parseResonance, parseSockets, parseRefine,
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
		if s.Regen != RegenHP && s.Regen != RegenMP {
			return fmt.Errorf("slot %q: regen %q must be %q or %q", s.ID, s.Regen, RegenHP, RegenMP)
		}
	}
	return nil
}

func parseLineStats(it *Items, _ *itemsFile) error {
	// Hidden lines pick among the line stats, so the list cannot be empty.
	if len(it.LineStats) == 0 {
		return fmt.Errorf("line_stats is empty")
	}
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
		if t.Tier < MinTier || t.Tier > MaxTier || t.Tier%TierStep != 0 {
			return fmt.Errorf("tier %d is not a drop tier (%d-%d, step %d)", t.Tier, MinTier, MaxTier, TierStep)
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
	for tier := MinTier; tier <= MaxTier; tier += TierStep {
		if _, ok := it.Tiers[tier]; !ok {
			return fmt.Errorf("tiers has no %d", tier)
		}
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
	return parseGems(it)
}

// parseGems checks the gem blocks (phase 3c spec 2.1): a known kind, level
// 1-MaxGemLevel, at least one stat, every stat a line stat with a positive
// value, and exactly one gem of each kind and level.
func parseGems(it *Items) error {
	type kindLevel struct {
		kind  string
		level int
	}
	seen := map[kindLevel]string{}
	for _, code := range it.StackOrder {
		g := it.Stackables[code].Gem
		if g == nil {
			continue
		}
		if !slices.Contains(gemKindList, g.Kind) {
			return fmt.Errorf("gem %q: kind %q is unknown", code, g.Kind)
		}
		if g.Level < 1 || g.Level > MaxGemLevel {
			return fmt.Errorf("gem %q: level %d must be in 1-%d", code, g.Level, MaxGemLevel)
		}
		if len(g.Stats) == 0 {
			return fmt.Errorf("gem %q: stats is empty", code)
		}
		for _, k := range slices.Sorted(maps.Keys(g.Stats)) {
			if !slices.Contains(it.LineStats, k) {
				return fmt.Errorf("gem %q: stat %q is not a line stat", code, k)
			}
			if v := g.Stats[k]; v <= 0 {
				return fmt.Errorf("gem %q: stat %q %d must be positive", code, k, v)
			}
		}
		key := kindLevel{g.Kind, g.Level}
		if other, dup := seen[key]; dup {
			return fmt.Errorf("gem %q: %s level %d is already %q", code, g.Kind, g.Level, other)
		}
		seen[key] = code
	}
	for _, k := range gemKindList {
		for lv := 1; lv <= MaxGemLevel; lv++ {
			if _, ok := seen[kindLevel{k, lv}]; !ok {
				return fmt.Errorf("stackables has no %s gem of level %d", k, lv)
			}
		}
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

// parseEnhance checks the enhancement table (phase 3b spec 2.1): 15 levels
// +1..+15, rates in 1-100 and never rising, +1..+4 always succeed and never
// drop, a drop only where a failure is possible, stones and Xu at least 1
// with Xu never falling; and the stone and the charm exist.
func parseEnhance(it *Items, _ *itemsFile) error {
	e := it.Enhance
	if e.PityPct < 1 {
		return fmt.Errorf("enhance pity_pct %d must be at least 1", e.PityPct)
	}
	if e.MaxTimes < 1 || e.MaxTimes > maxEnhanceTimes {
		return fmt.Errorf("enhance max_times %d must be in 1-%d", e.MaxTimes, maxEnhanceTimes)
	}
	if len(e.Levels) != EnhanceLevels-1 {
		return fmt.Errorf("enhance levels has %d entries, want %d", len(e.Levels), EnhanceLevels-1)
	}
	for i, l := range e.Levels {
		if l.Level != i+1 {
			return fmt.Errorf("enhance levels[%d] is level %d, want %d", i, l.Level, i+1)
		}
		if l.SuccessPct < 1 || l.SuccessPct > 100 {
			return fmt.Errorf("enhance +%d: success_pct %d must be in 1-100", l.Level, l.SuccessPct)
		}
		if l.Stones < 1 {
			return fmt.Errorf("enhance +%d: stones %d must be at least 1", l.Level, l.Stones)
		}
		if l.XuPerTier < 1 {
			return fmt.Errorf("enhance +%d: xu_per_tier %d must be at least 1", l.Level, l.XuPerTier)
		}
		if l.Level <= alwaysSucceed {
			if l.SuccessPct != 100 {
				return fmt.Errorf("enhance +%d: success_pct %d must be 100", l.Level, l.SuccessPct)
			}
			if l.Drop {
				return fmt.Errorf("enhance +%d: drop must be false", l.Level)
			}
		}
		if l.Drop && l.SuccessPct == 100 {
			return fmt.Errorf("enhance +%d: drop needs success_pct below 100", l.Level)
		}
		if i == 0 {
			continue
		}
		prev := e.Levels[i-1]
		if l.SuccessPct > prev.SuccessPct {
			return fmt.Errorf("enhance +%d: success_pct %d must not be above +%d's %d", l.Level, l.SuccessPct, prev.Level, prev.SuccessPct)
		}
		if l.XuPerTier < prev.XuPerTier {
			return fmt.Errorf("enhance +%d: xu_per_tier %d must not be below +%d's %d", l.Level, l.XuPerTier, prev.Level, prev.XuPerTier)
		}
	}
	for _, code := range []string{StoneEnhance, CharmProtect} {
		if _, ok := it.Stackables[code]; !ok {
			return fmt.Errorf("stackables has no %q", code)
		}
	}
	return nil
}

// parseRegen checks the regen milestones: at least one, enhance in 1-15,
// hp_bp and mp positive, and all three strictly rising step by step.
func parseRegen(it *Items, _ *itemsFile) error {
	if len(it.Regen) == 0 {
		return fmt.Errorf("regen is empty")
	}
	for i, r := range it.Regen {
		if r.Enhance < 1 || r.Enhance > EnhanceLevels-1 {
			return fmt.Errorf("regen[%d]: enhance %d must be in 1-%d", i, r.Enhance, EnhanceLevels-1)
		}
		if r.HPBP <= 0 || r.MP <= 0 {
			return fmt.Errorf("regen[%d]: hp_bp %d and mp %d must be positive", i, r.HPBP, r.MP)
		}
		if i > 0 {
			prev := it.Regen[i-1]
			if r.Enhance <= prev.Enhance || r.HPBP <= prev.HPBP || r.MP <= prev.MP {
				return fmt.Errorf("regen[%d]: enhance %d, hp_bp %d and mp %d must each be above the step before", i, r.Enhance, r.HPBP, r.MP)
			}
		}
	}
	return nil
}

// parseResonance checks the resonance table (phase 3c spec 2.1): the
// activator maps each of the five elements to another, as one cycle through
// all five; every slot has two different activating slots, neither itself.
func parseResonance(it *Items, f *itemsFile) error {
	a := f.Resonance.Activator
	for _, e := range slices.Sorted(maps.Keys(a)) {
		if !classes[e] {
			return fmt.Errorf("resonance activator: %q is not an element", e)
		}
	}
	by := map[string]string{} // activator -> the element it activates
	for _, e := range classList {
		v, ok := a[e]
		if !ok {
			return fmt.Errorf("resonance activator has no %q", e)
		}
		if !classes[v] {
			return fmt.Errorf("resonance activator: %q is activated by %q, not an element", e, v)
		}
		if v == e {
			return fmt.Errorf("resonance activator: %q activates itself", e)
		}
		if _, dup := by[v]; dup {
			return fmt.Errorf("resonance activator: %q activates more than one element", v)
		}
		by[v] = e
	}
	// A permutation without fixed points of five elements is one 5-cycle or a
	// 2-cycle and a 3-cycle; walk from the first element to tell them apart.
	e, steps := classList[0], 0
	for {
		e, steps = a[e], steps+1
		if e == classList[0] {
			break
		}
	}
	if steps != len(classList) {
		return fmt.Errorf("resonance activator must be one cycle through all five elements")
	}
	it.Resonance.Activator = maps.Clone(a)

	slots := f.Resonance.Slots
	for _, id := range slices.Sorted(maps.Keys(slots)) {
		if !slices.Contains(SlotIDs[:], id) {
			return fmt.Errorf("resonance slots %q is not a slot", id)
		}
	}
	for s, id := range SlotIDs {
		on, ok := slots[id]
		if !ok {
			return fmt.Errorf("resonance slots has no %q", id)
		}
		if len(on) != HiddenLineCount {
			return fmt.Errorf("resonance slots %q: %d slots, want %d", id, len(on), HiddenLineCount)
		}
		for j, o := range on {
			idx := slices.Index(SlotIDs[:], o)
			if idx < 0 {
				return fmt.Errorf("resonance slots %q: %q is not a slot", id, o)
			}
			if idx == s {
				return fmt.Errorf("resonance slots %q: activated by itself", id)
			}
			it.Resonance.Slots[s][j] = idx
		}
		if on[0] == on[1] {
			return fmt.Errorf("resonance slots %q: both lines activated by %q", id, on[0])
		}
	}
	return nil
}

// parseSockets checks the socket rules: at least one hole, one positive
// punch price per hole and a positive unsocket price.
func parseSockets(it *Items, _ *itemsFile) error {
	s := it.Sockets
	if s.Max < 1 {
		return fmt.Errorf("sockets max %d must be at least 1", s.Max)
	}
	if len(s.PunchXuPerTier) != s.Max {
		return fmt.Errorf("sockets max %d needs %d punch_xu_per_tier entries, got %d", s.Max, s.Max, len(s.PunchXuPerTier))
	}
	for i, v := range s.PunchXuPerTier {
		if v <= 0 {
			return fmt.Errorf("sockets punch_xu_per_tier[%d] %d must be positive", i, v)
		}
	}
	if s.UnsocketXuPerLevel <= 0 {
		return fmt.Errorf("sockets unsocket_xu_per_level %d must be positive", s.UnsocketXuPerLevel)
	}
	return nil
}

// parseRefine checks the refining rules: min_quality is a quality above the
// lowest, at least one stone per roll, the rest not negative; and the
// refine stone exists.
func parseRefine(it *Items, _ *itemsFile) error {
	r := it.Refine
	if r.MinQuality < 1 || r.MinQuality > qualityCount-1 {
		return fmt.Errorf("refine min_quality %d must be in 1-%d", r.MinQuality, qualityCount-1)
	}
	if r.Stones < 1 {
		return fmt.Errorf("refine stones %d must be at least 1", r.Stones)
	}
	for _, v := range []struct {
		name string
		v    int
	}{{"stones_per_lock", r.StonesPerLock}, {"xu_per_tier", r.XuPerTier}, {"xu_per_tier_per_lock", r.XuPerTierPerLock}} {
		if v.v < 0 {
			return fmt.Errorf("refine %s %d must not be negative", v.name, v.v)
		}
	}
	if _, ok := it.Stackables[StoneRefine]; !ok {
		return fmt.Errorf("stackables has no %q", StoneRefine)
	}
	return nil
}
