package gamedata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"demonslayer2d/internal/combat"
)

// Skill shapes.
const (
	ShapeSingle       = "single"
	ShapeCone         = "cone"
	ShapeCircleSelf   = "circle_self"
	ShapeCircleTarget = "circle_target"
	ShapeLine         = "line"
)

// Passive keys. Percent keys are whole percents; PassiveCritBP is in basis
// points (100 = 1%).
const (
	PassivePhysDefPct     = "phys_def_pct"
	PassiveMagicDefPct    = "magic_def_pct"
	PassiveMaxHPPct       = "max_hp_pct"
	PassiveEvasionPct     = "evasion_pct"
	PassiveAccuracyPct    = "accuracy_pct"
	PassiveMagicAttackPct = "magic_attack_pct"
	PassiveCritBP         = "crit_bp"
)

// Damage kinds.
const (
	DamagePhys  = "phys"
	DamageMagic = "magic"
)

// SkillLevels is how many levels a skill has; every per-level array has
// exactly this many entries.
const SkillLevels = 10

// Fixed skill layout of a class: slot to learn level. Slot 0 is the passive.
const (
	activeSlots    = 4
	skillsPerClass = activeSlots + 1
	// passiveIndex is the passive's place in a ClassSkills list.
	passiveIndex = activeSlots
)

var slotLearnLevel = map[int]int{1: 10, 2: 20, 3: 40, 4: 50, 0: 30}

var classList = []string{"viem", "thuy", "minh", "nham", "phong"}

var classes = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range classList {
		m[c] = true
	}
	return m
}()

var passiveKeys = map[string]bool{
	PassivePhysDefPct: true, PassiveMagicDefPct: true, PassiveMaxHPPct: true, PassiveEvasionPct: true,
	PassiveAccuracyPct: true, PassiveMagicAttackPct: true, PassiveCritBP: true,
}

// Skill is one skill of a class (spec 2.1). Zero fields are absent fields.
type Skill struct {
	ID         string `json:"id"`
	Class      string `json:"class"`
	Name       string `json:"name"`
	Form       string `json:"form"`
	LearnLevel int    `json:"learn_level"`
	// Slot is 1-4 for actives and 0 for the passive.
	Slot      int     `json:"slot"`
	Damage    string  `json:"damage"`
	Shape     string  `json:"shape"`
	CastRange float64 `json:"cast_range"`
	Radius    float64 `json:"radius"`
	ConeDeg   float64 `json:"cone_deg"`
	Reach     float64 `json:"reach"`
	Length    float64 `json:"length"`
	Width     float64 `json:"width"`
	Dash      bool    `json:"dash"`
	// Per-level arrays, index 0 = skill level 1.
	CoefPct    []int `json:"coef_pct"`
	MP         []int `json:"mp"`
	CooldownMS []int `json:"cooldown_ms"`
	MaxTargets []int `json:"max_targets"`
	// Passive maps a passive key to its per-level values.
	Passive map[string][]int `json:"passive"`
}

// Active reports whether the skill is cast (slots 1-4), not a passive.
func (s Skill) Active() bool { return s.Slot > 0 }

type skillsFile struct {
	Skills []Skill `json:"skills"`
}

func decodeStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}

// ParseSkills decodes and validates skills.json, returning skills by id.
func ParseSkills(b []byte) (map[string]Skill, error) {
	var f skillsFile
	if err := decodeStrict(b, &f); err != nil {
		return nil, fmt.Errorf("gamedata: skills: decode: %w", err)
	}
	// A second pass tells absent fields from present-but-empty ones.
	var raw struct {
		Skills []map[string]json.RawMessage `json:"skills"`
	}
	if err := json.Unmarshal(b, &raw); err != nil || len(raw.Skills) != len(f.Skills) {
		return nil, fmt.Errorf("gamedata: skills: decode: %v", err)
	}
	out := make(map[string]Skill, len(f.Skills))
	slots := map[string]map[int]bool{}
	for i, s := range f.Skills {
		if err := validateSkill(s, raw.Skills[i]); err != nil {
			return nil, err
		}
		if _, dup := out[s.ID]; dup {
			return nil, fmt.Errorf("gamedata: duplicate skill %q", s.ID)
		}
		if slots[s.Class] == nil {
			slots[s.Class] = map[int]bool{}
		}
		if slots[s.Class][s.Slot] {
			return nil, fmt.Errorf("gamedata: skill %q: class %q has two skills in slot %d", s.ID, s.Class, s.Slot)
		}
		slots[s.Class][s.Slot] = true
		out[s.ID] = s
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("gamedata: skills: no skills")
	}
	for _, c := range classList {
		got := slots[c]
		for slot := range slotLearnLevel {
			if !got[slot] {
				return nil, fmt.Errorf("gamedata: class %q has no skill in slot %d", c, slot)
			}
		}
	}
	return out, nil
}

func validateSkill(s Skill, present map[string]json.RawMessage) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("gamedata: skill %q: %s", s.ID, fmt.Sprintf(format, args...))
	}
	if s.ID == "" {
		return fmt.Errorf("gamedata: skill with empty id")
	}
	if !classes[s.Class] {
		return bad("class %q is not valid", s.Class)
	}
	want, ok := slotLearnLevel[s.Slot]
	if !ok {
		return bad("slot %d must be 0-4", s.Slot)
	}
	if s.LearnLevel != want {
		return bad("learn_level %d must be %d for slot %d", s.LearnLevel, want, s.Slot)
	}
	if s.Slot == 0 {
		return validatePassive(s, present, bad)
	}
	return validateActive(s, present, bad)
}

func validatePassive(s Skill, present map[string]json.RawMessage, bad func(string, ...any) error) error {
	if len(s.Passive) == 0 {
		return bad("passive needs at least one key in passive")
	}
	if s.Damage != "" || s.Shape != "" || s.CastRange != 0 || s.Radius != 0 || s.ConeDeg != 0 ||
		s.Reach != 0 || s.Length != 0 || s.Width != 0 || s.Dash ||
		s.CoefPct != nil || s.MP != nil || s.CooldownMS != nil || s.MaxTargets != nil ||
		hasAny(present, "damage", "shape", "cast_range", "radius", "cone_deg", "reach", "length", "width",
			"dash", "coef_pct", "mp", "cooldown_ms", "max_targets") {
		return bad("passive must not have active fields")
	}
	for k, v := range s.Passive {
		if !passiveKeys[k] {
			return bad("passive key %q is unknown", k)
		}
		if len(v) != SkillLevels {
			return bad("passive.%s has %d entries, want %d", k, len(v), SkillLevels)
		}
		for _, n := range v {
			if n < 0 {
				return bad("passive.%s must not be negative", k)
			}
		}
	}
	return nil
}

func validateActive(s Skill, present map[string]json.RawMessage, bad func(string, ...any) error) error {
	if _, ok := present["passive"]; ok {
		return bad("active skill must not have passive")
	}
	if s.Damage != DamagePhys && s.Damage != DamageMagic {
		return bad("damage %q must be phys or magic", s.Damage)
	}
	// Each shape has exactly its own measures, all positive; the rest stay zero.
	measures := map[string]float64{
		"cast_range": s.CastRange, "radius": s.Radius, "cone_deg": s.ConeDeg,
		"reach": s.Reach, "length": s.Length, "width": s.Width,
	}
	var need []string
	switch s.Shape {
	case ShapeSingle:
		need = []string{"cast_range"}
	case ShapeCone:
		need = []string{"cone_deg", "reach"}
	case ShapeCircleSelf:
		need = []string{"radius"}
	case ShapeCircleTarget:
		need = []string{"cast_range", "radius"}
	case ShapeLine:
		need = []string{"length", "width"}
	default:
		return bad("shape %q is not valid", s.Shape)
	}
	for _, n := range need {
		// The negated comparison also rejects NaN.
		if !(measures[n] > 0) {
			return bad("%s must be positive for shape %s", n, s.Shape)
		}
		delete(measures, n)
	}
	for n, v := range measures {
		if v != 0 {
			return bad("%s is not allowed for shape %s", n, s.Shape)
		}
	}
	if s.ConeDeg > 360 {
		return bad("cone_deg %v must be in (0, 360]", s.ConeDeg)
	}
	if s.Dash && s.Shape != ShapeSingle {
		return bad("dash needs shape single")
	}
	for name, arr := range map[string][]int{"coef_pct": s.CoefPct, "mp": s.MP, "cooldown_ms": s.CooldownMS} {
		if err := positiveArray(name, arr, bad); err != nil {
			return err
		}
	}
	if s.Shape == ShapeSingle {
		if _, ok := present["max_targets"]; ok {
			return bad("max_targets is not allowed for shape single")
		}
	} else if err := positiveArray("max_targets", s.MaxTargets, bad); err != nil {
		return err
	}
	return nil
}

func hasAny(m map[string]json.RawMessage, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func positiveArray(name string, a []int, bad func(string, ...any) error) error {
	if len(a) != SkillLevels {
		return bad("%s has %d entries, want %d", name, len(a), SkillLevels)
	}
	for _, n := range a {
		if n <= 0 {
			return bad("%s must be positive", name)
		}
	}
	return nil
}

// classSkills groups skills by class, ordered by slot 1-4 with the passive
// (slot 0) last. That is also learn-level order except for the passive.
func classSkills(skills map[string]Skill) map[string][]Skill {
	out := map[string][]Skill{}
	for _, s := range skills {
		l := out[s.Class]
		if l == nil {
			l = make([]Skill, skillsPerClass)
		}
		i := s.Slot - 1
		if s.Slot == 0 {
			i = passiveIndex
		}
		l[i] = s
		out[s.Class] = l
	}
	return out
}

type expFile struct {
	ExpToNext combat.ExpTable `json:"exp_to_next"`
}

// ParseExp decodes and validates exp.json: the EXP needed to leave each
// level, index 0 = level 1.
func ParseExp(b []byte) (combat.ExpTable, error) {
	var f expFile
	if err := decodeStrict(b, &f); err != nil {
		return nil, fmt.Errorf("gamedata: exp: decode: %w", err)
	}
	if len(f.ExpToNext) != combat.MaxLevel-1 {
		return nil, fmt.Errorf("gamedata: exp: %d values, want %d", len(f.ExpToNext), combat.MaxLevel-1)
	}
	for i, v := range f.ExpToNext {
		if v <= 0 {
			return nil, fmt.Errorf("gamedata: exp: level %d: value must be positive", i+1)
		}
		if i > 0 && v <= f.ExpToNext[i-1] {
			return nil, fmt.Errorf("gamedata: exp: level %d: value must be greater than level %d", i+1, i)
		}
	}
	return f.ExpToNext, nil
}
