package gamedata

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"demonslayer2d/internal/mapdata"
)

const validJSON = `{
  "kinds": [{"id":"a","name":"A","attack_range":1.5,"attack_sec":1.5,"speed":0.8,"detect_range":5,"chase_range":10,"shoot_sec":2,"shoot_windup_sec":0.5,"region_margin":3,"chase_speed":1.2,"safe_wait_sec":5,"margin_grace_sec":2,"grudge_sec":10,"respawn_sec":8}],
  "levels": [{"level":1,"hp":56,"attack":30,"defense":8,"exp":6},{"level":2,"hp":104,"attack":39,"defense":16,"exp":11}]
}`

// ______________________TEST PARSE_____________________

func TestParseOK(t *testing.T) {
	d, err := Parse([]byte(validJSON))
	if err != nil {
		t.Fatal(err)
	}
	if k := d.Kinds["a"]; k.Name != "A" || k.Speed != 0.8 || k.RespawnSec != 8 {
		t.Fatalf("kind: %+v", k)
	}
	if l := d.Levels[2]; l != (Level{Level: 2, HP: 104, Attack: 39, Defense: 16, Exp: 11}) {
		t.Fatalf("level: %+v", l)
	}
}

func TestParseKindFields(t *testing.T) {
	d, err := Parse([]byte(validJSON))
	if err != nil {
		t.Fatal(err)
	}
	want := Kind{ID: "a", Name: "A", AttackRange: 1.5, AttackSec: 1.5, Speed: 0.8, DetectRange: 5, ChaseRange: 10,
		ShootSec: 2, ShootWindupSec: 0.5, RegionMargin: 3, ChaseSpeed: 1.2, SafeWaitSec: 5,
		MarginGraceSec: 2, GrudgeSec: 10, RespawnSec: 8}
	if k := d.Kinds["a"]; k != want {
		t.Fatalf("kind %+v, want %+v", k, want)
	}
}

func kindWith(field, value string) string {
	fields := map[string]string{"attack_range": "1.5", "attack_sec": "1.5", "speed": "0.8", "detect_range": "5",
		"chase_range": "10", "shoot_sec": "2", "shoot_windup_sec": "0.5", "region_margin": "3", "chase_speed": "1.2", "safe_wait_sec": "5",
		"margin_grace_sec": "2", "grudge_sec": "10", "respawn_sec": "8"}
	fields[field] = value
	var parts []string
	for k, v := range fields {
		parts = append(parts, `"`+k+`":`+v)
	}
	return `{"kinds":[{"id":"a","name":"A",` + strings.Join(parts, ",") +
		`}],"levels":[{"level":1,"hp":56,"attack":30,"defense":8,"exp":6}]}`
}

func TestParseKindFieldValues(t *testing.T) {
	for _, f := range []string{"attack_range", "attack_sec", "speed", "detect_range", "chase_range", "shoot_sec",
		"shoot_windup_sec", "chase_speed", "safe_wait_sec", "margin_grace_sec", "grudge_sec", "respawn_sec"} {
		for _, v := range []string{"0", "-1"} {
			_, err := Parse([]byte(kindWith(f, v)))
			if err == nil || !strings.Contains(err.Error(), f) {
				t.Errorf("%s = %s: err %v, want one naming the field", f, v, err)
			}
		}
	}
	for _, v := range []string{"-1"} {
		if _, err := Parse([]byte(kindWith("region_margin", v))); err == nil || !strings.Contains(err.Error(), "region_margin") {
			t.Errorf("region_margin = %s: err %v", v, err)
		}
	}
	if _, err := Parse([]byte(kindWith("region_margin", "0"))); err != nil {
		t.Fatalf("region_margin 0 must be accepted: %v", err)
	}
}

// A kind without one of the anti-kite fields is rejected: all must be set.
func TestParseRequiresAntiKiteFields(t *testing.T) {
	for _, f := range []string{"chase_speed", "safe_wait_sec", "margin_grace_sec", "grudge_sec"} {
		doc := strings.Replace(kindWith(f, "1"), `"`+f+`":1`, `"x_unused":0`, 1)
		doc = strings.Replace(doc, `,"x_unused":0`, "", 1)
		doc = strings.Replace(doc, `"x_unused":0,`, "", 1)
		if strings.Contains(doc, f) {
			t.Fatalf("setup: %s still in %s", f, doc)
		}
		if _, err := Parse([]byte(doc)); err == nil || !strings.Contains(err.Error(), f) {
			t.Errorf("without %s: err %v, want one naming it", f, err)
		}
	}
}

func TestParseRejectsOldRangeFields(t *testing.T) {
	for _, f := range []string{"aggro_range", "leash_range"} {
		doc := strings.Replace(kindWith("speed", "0.8"), `"speed"`, `"`+f+`":4,"speed"`, 1)
		if _, err := Parse([]byte(doc)); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("%s must be rejected as an unknown field, got %v", f, err)
		}
	}
}

func TestParseErrors(t *testing.T) {
	kind := func(id, speed string) string {
		return `{"id":"` + id + `","name":"A","attack_range":1.5,"attack_sec":1.5,"speed":` + speed +
			`,"detect_range":5,"chase_range":10,"shoot_sec":2,"shoot_windup_sec":0.5,"region_margin":3,"chase_speed":1.2,"safe_wait_sec":5,"margin_grace_sec":2,"grudge_sec":10,"respawn_sec":8}`
	}
	lvl := `{"level":1,"hp":56,"attack":30,"defense":8,"exp":6}`
	doc := func(kinds, levels string) string { return `{"kinds":[` + kinds + `],"levels":[` + levels + `]}` }
	tests := []struct{ name, in string }{
		{"duplicate kind", doc(kind("a", "0.8")+","+kind("a", "0.8"), lvl)},
		{"zero speed", doc(kind("a", "0"), lvl)},
		{"empty kind id", doc(kind("", "0.8"), lvl)},
		{"duplicate level", doc(kind("a", "0.8"), lvl+","+lvl)},
		{"zero hp", doc(kind("a", "0.8"), `{"level":1,"hp":0,"attack":30,"defense":8,"exp":6}`)},
		{"zero exp", doc(kind("a", "0.8"), `{"level":1,"hp":56,"attack":30,"defense":8,"exp":0}`)},
		{"missing exp", doc(kind("a", "0.8"), `{"level":1,"hp":56,"attack":30,"defense":8}`)},
		{"bad json", `{"kinds":`},
		{"unknown field", `{"kinds":[],"levels":[],"extra":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.in)); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("want error")
	}
}

// ______________________TEST VALIDATE_____________________

func TestValidateRegions(t *testing.T) {
	d, err := Parse([]byte(validJSON))
	if err != nil {
		t.Fatal(err)
	}
	withRegion := func(monster string, min, max int) map[string]*mapdata.Map {
		return map[string]*mapdata.Map{"m": {Regions: []mapdata.Region{{ID: "r", Monster: monster, MinLevel: min, MaxLevel: max}}}}
	}
	if err := d.Validate(withRegion("a", 1, 2)); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if err := d.Validate(withRegion("ghost", 1, 2)); err == nil || !strings.Contains(err.Error(), "ghost") || !strings.Contains(err.Error(), `region "r"`) {
		t.Fatalf("unknown monster: %v", err)
	}
	if err := d.Validate(withRegion("a", 1, 3)); err == nil || !strings.Contains(err.Error(), "level 3") {
		t.Fatalf("missing level: %v", err)
	}
}

// ______________________TEST GENERATED DATA_____________________

func TestRepoDataMatchesScripts(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	for _, tt := range []struct{ script, file string }{
		{"monster_stats.py", "monsters.json"},
		{"skill_stats.py", "skills.json"},
		{"exp_table.py", "exp.json"},
	} {
		t.Run(tt.file, func(t *testing.T) {
			got, err := exec.Command(py, "../../docs/balance/"+tt.script, "--print-json").Output()
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile("../../data/" + tt.file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("data/%s differs from %s output; run it with --write-json", tt.file, tt.script)
			}
		})
	}
}

func TestRepoMonstersCoverLevels1To90(t *testing.T) {
	d, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	for lvl := 1; lvl <= 90; lvl++ {
		if l, ok := d.Levels[lvl]; !ok || l.Exp <= 0 {
			t.Fatalf("level %d: %+v", lvl, l)
		}
	}
	if len(d.Levels) != 90 {
		t.Fatalf("levels: %d", len(d.Levels))
	}
}

func TestRepoDataValidatesAgainstMaps(t *testing.T) {
	d, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	maps, err := mapdata.LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(maps); err != nil {
		t.Fatal(err)
	}
}

// ______________________TEST SKILLS DATA_____________________

type obj = map[string]any

func ten(v int) []int {
	a := make([]int, SkillLevels)
	for i := range a {
		a[i] = v + i
	}
	return a
}

// validSkills is a minimal valid skills.json for the class viem: four
// actives (single, cone, circle_target, circle_self) and a passive.
func validSkills() []obj {
	active := func(id string, slot, learn int, shape string, extra obj) obj {
		s := obj{"id": id, "class": "viem", "name": id, "form": "I", "learn_level": learn, "slot": slot,
			"damage": "phys", "shape": shape, "coef_pct": ten(100), "mp": ten(10), "cooldown_ms": ten(1000)}
		for k, v := range extra {
			s[k] = v
		}
		return s
	}
	return []obj{
		active("viem_1", 1, 10, "single", obj{"cast_range": 4, "dash": true}),
		active("viem_2", 2, 20, "cone", obj{"cone_deg": 120, "reach": 2, "max_targets": ten(2)}),
		obj{"id": "viem_3", "class": "viem", "name": "p", "form": "III", "learn_level": 30, "slot": 0,
			"passive": obj{"phys_def_pct": ten(2)}},
		active("viem_4", 3, 40, "circle_target", obj{"cast_range": 5, "radius": 1.5, "max_targets": ten(2)}),
		active("viem_5", 4, 50, "circle_self", obj{"radius": 2, "max_targets": ten(2)}),
	}
}

func skillsJSON(t *testing.T, skills []obj) []byte {
	t.Helper()
	b, err := json.Marshal(obj{"skills": skills})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseSkillsValid(t *testing.T) {
	m, err := ParseSkills(skillsJSON(t, validSkills()))
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 5 || !m["viem_1"].Dash || !m["viem_1"].Active() || m["viem_3"].Active() {
		t.Fatalf("parsed: %+v", m)
	}
}

func TestParseSkillsRejects(t *testing.T) {
	// idx: viem_1 single=0, viem_2 cone=1, viem_3 passive=2, viem_4 circle_target=3, viem_5 circle_self=4.
	set := func(i int, k string, v any) func([]obj) []obj {
		return func(s []obj) []obj { s[i][k] = v; return s }
	}
	del := func(i int, k string) func([]obj) []obj {
		return func(s []obj) []obj { delete(s[i], k); return s }
	}
	tests := []struct {
		name   string
		mutate func([]obj) []obj
		want   string
	}{
		{"unknown field", set(0, "bogus", 1), "bogus"},
		{"duplicate id", set(1, "id", "viem_1"), "duplicate"},
		{"bad class", set(0, "class", "ghost"), "class"},
		{"missing radius", del(4, "radius"), "radius"},
		{"reach on single", set(0, "reach", 2), "reach"},
		{"array of 9", set(0, "mp", ten(10)[:9]), "mp"},
		{"zero in mp", set(0, "mp", append([]int{0}, ten(10)[1:]...)), "mp"},
		{"dash on cone", set(1, "dash", true), "dash"},
		{"max_targets on single", set(0, "max_targets", ten(2)), "max_targets"},
		{"missing max_targets on cone", del(1, "max_targets"), "max_targets"},
		{"passive at level 20", set(2, "learn_level", 20), "learn_level"},
		{"passive with shape", set(2, "shape", "single"), "active fields"},
		{"unknown passive key", set(2, "passive", obj{"speed_pct": ten(1)}), "speed_pct"},
		{"empty passive", set(2, "passive", obj{}), "passive"},
		{"negative passive", set(2, "passive", obj{"phys_def_pct": append([]int{-1}, ten(1)[1:]...)}), "negative"},
		{"passive array of 9", set(2, "passive", obj{"phys_def_pct": ten(1)[:9]}), "phys_def_pct"},
		{"two slot 2 skills", func(s []obj) []obj { s[3]["slot"], s[3]["learn_level"] = 2, 20; return s }, "slot 2"},
		{"missing slot 4", func(s []obj) []obj { return s[:4] }, "slot 4"},
		{"cone_deg 0", set(1, "cone_deg", 0), "cone_deg"},
		{"cone_deg 400", set(1, "cone_deg", 400), "cone_deg"},
		{"bad damage", set(0, "damage", "fire"), "damage"},
		{"bad shape", set(0, "shape", "ring"), "shape"},
		{"active with passive", set(0, "passive", obj{"phys_def_pct": ten(1)}), "passive"},
		{"wrong learn level", set(0, "learn_level", 11), "learn_level"},
		{"bad slot", set(0, "slot", 5), "slot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSkills(skillsJSON(t, tt.mutate(validSkills())))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want error containing %q, got %v", tt.want, err)
			}
		})
	}
	if _, err := ParseSkills([]byte(`{"skills":[]}`)); err == nil {
		t.Fatal("empty skills accepted")
	}
}

func TestRepoSkills(t *testing.T) {
	d, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Skills) != 25 || len(d.ClassSkills) != 5 {
		t.Fatalf("skills %d, classes %d", len(d.Skills), len(d.ClassSkills))
	}
	for class, list := range d.ClassSkills {
		if len(list) != 5 {
			t.Fatalf("%s: %d skills", class, len(list))
		}
		for i, learn := range []int{10, 20, 40, 50} {
			if s := list[i]; s.Class != class || s.Slot != i+1 || s.LearnLevel != learn || !s.Active() {
				t.Fatalf("%s index %d: %+v", class, i, s)
			}
		}
		if p := list[4]; p.Slot != 0 || p.LearnLevel != 30 || p.Active() || len(p.Passive) == 0 {
			t.Fatalf("%s passive: %+v", class, p)
		}
	}
	if s := d.Skills["viem_1"]; s.Shape != ShapeSingle || !s.Dash || s.CoefPct[0] != 150 {
		t.Fatalf("viem_1: %+v", s)
	}
}

func TestParseExp(t *testing.T) {
	exp := func(n int, mutate func([]int64)) []byte {
		a := make([]int64, n)
		for i := range a {
			a[i] = int64(i+1) * 10
		}
		if mutate != nil {
			mutate(a)
		}
		b, _ := json.Marshal(obj{"exp_to_next": a})
		return b
	}
	got, err := ParseExp(exp(89, nil))
	if err != nil || len(got) != 89 || got[0] != 10 {
		t.Fatalf("valid: %v %v", len(got), err)
	}
	for name, b := range map[string][]byte{
		"88 values":      exp(88, nil),
		"90 values":      exp(90, nil),
		"non-increasing": exp(89, func(a []int64) { a[5] = a[4] }),
		"zero":           exp(89, func(a []int64) { a[0] = 0 }),
		"unknown field":  []byte(`{"exp_to_next":[],"x":1}`),
	} {
		if _, err := ParseExp(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	d, err := Load("../../data")
	if err != nil || len(d.Exp) != 89 || d.Exp[0] != 80 {
		t.Fatalf("repo exp: %v", err)
	}
}
