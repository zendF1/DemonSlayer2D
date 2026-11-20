package gamedata

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/mapdata"
)

const validJSON = `{
  "kinds": [{"id":"a","name":"A","attack_range":1.5,"attack_sec":1.5,"speed":0.8,"detect_range":5,"chase_range":10,"shoot_sec":2,"shoot_windup_sec":0.5,"region_margin":3,"chase_speed":1.2,"safe_wait_sec":5,"margin_grace_sec":2,"grudge_sec":10,"respawn_sec":8,"melee_windup_ms":200,"shot_speed":15,"rank":"normal"}],
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
		MarginGraceSec: 2, GrudgeSec: 10, RespawnSec: 8, MeleeWindupMS: 200, ShotSpeed: 15, Rank: RankNormal}
	if k := d.Kinds["a"]; k != want {
		t.Fatalf("kind %+v, want %+v", k, want)
	}
}

func kindWith(field, value string) string {
	fields := map[string]string{"attack_range": "1.5", "attack_sec": "1.5", "speed": "0.8", "detect_range": "5",
		"chase_range": "10", "shoot_sec": "2", "shoot_windup_sec": "0.5", "region_margin": "3", "chase_speed": "1.2", "safe_wait_sec": "5",
		"margin_grace_sec": "2", "grudge_sec": "10", "respawn_sec": "8", "melee_windup_ms": "200", "shot_speed": "15", "rank": `"normal"`}
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
		"shoot_windup_sec", "chase_speed", "safe_wait_sec", "margin_grace_sec", "grudge_sec", "respawn_sec", "melee_windup_ms", "shot_speed"} {
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
	for _, f := range []string{"chase_speed", "safe_wait_sec", "margin_grace_sec", "grudge_sec", "melee_windup_ms", "shot_speed"} {
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

// A kind's rank must be normal, elite or boss (phase 3a spec 2.2).
func TestParseKindRank(t *testing.T) {
	for _, r := range []string{RankNormal, RankElite, RankBoss} {
		d, err := Parse([]byte(kindWith("rank", `"`+r+`"`)))
		if err != nil || d.Kinds["a"].Rank != r {
			t.Fatalf("rank %s: %v", r, err)
		}
	}
	for _, v := range []string{`"champion"`, `""`} {
		if _, err := Parse([]byte(kindWith("rank", v))); err == nil || !strings.Contains(err.Error(), "rank") {
			t.Errorf("rank %s: err %v, want one naming rank", v, err)
		}
	}
	doc := strings.Replace(kindWith("speed", "0.8"), `,"rank":"normal"`, "", 1)
	doc = strings.Replace(doc, `"rank":"normal",`, "", 1)
	if strings.Contains(doc, "rank") {
		t.Fatalf("setup: rank still in %s", doc)
	}
	if _, err := Parse([]byte(doc)); err == nil || !strings.Contains(err.Error(), "rank") {
		t.Errorf("without rank: err %v, want one naming rank", err)
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
			`,"detect_range":5,"chase_range":10,"shoot_sec":2,"shoot_windup_sec":0.5,"region_margin":3,"chase_speed":1.2,"safe_wait_sec":5,"margin_grace_sec":2,"grudge_sec":10,"respawn_sec":8,"melee_windup_ms":200,"shot_speed":15,"rank":"normal"}`
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
		{"item_stats.py", "items.json"},
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

// From level 10 the monsters are built against the reference +0 gear and
// match game-design 5.9; level 30 is 4,336 HP (phase 3a spec 1.4).
func TestRepoMonstersMatchGameDesign(t *testing.T) {
	d, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	for lvl, want := range map[int][3]int{
		10: {750, 95, 80}, 20: {2704, 319, 160}, 30: {4336, 508, 240}, 40: {6240, 694, 320}, 50: {8408, 934, 400},
		60: {10452, 1211, 480}, 70: {11466, 1276, 560}, 80: {12466, 1344, 640}, 90: {13478, 1412, 720},
	} {
		if l := d.Levels[lvl]; [3]int{l.HP, l.Attack, l.Defense} != want {
			t.Errorf("level %d: %d/%d/%d, want %v", lvl, l.HP, l.Attack, l.Defense, want)
		}
	}
	if l := d.Levels[10]; l.Exp != 143 {
		t.Errorf("level 10 exp %d, want 143", l.Exp)
	}
	for id, k := range d.Kinds {
		if k.Rank != "normal" {
			t.Errorf("kind %s: rank %q, want normal", id, k.Rank)
		}
	}
}

// docs/balance/exp-table.csv and data/exp.json come from the same run of
// exp_table.py; the table the server reads must match the CSV people read.
func TestRepoExpMatchesCSV(t *testing.T) {
	d, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("../../docs/balance/exp-table.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 90 || rows[0][0] != "level" || rows[0][1] != "exp_to_next" { // header + levels 1-89
		t.Fatalf("%d rows, header %v", len(rows), rows[0])
	}
	for _, r := range rows[1:] {
		level, err1 := strconv.Atoi(r[0])
		want, err2 := strconv.ParseInt(r[1], 10, 64)
		if err1 != nil || err2 != nil {
			t.Fatalf("row %v: %v %v", r, err1, err2)
		}
		if got := d.Exp.ToNext(level); got != want {
			t.Errorf("ToNext(%d) = %d, the CSV says %d", level, got, want)
		}
	}
	if got := d.Exp.ToNext(combat.MaxLevel); got != 0 {
		t.Fatalf("ToNext(90) = %d", got)
	}
}

// EXP per damage point rises with monster level (phase 2 spec 2.4).
func TestRepoExpPerDamageRisesWithLevel(t *testing.T) {
	d, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	rate := func(level int) float64 {
		l := d.Levels[level]
		return float64(l.Exp) / float64(l.HP)
	}
	if !(rate(50) > rate(10) && rate(10) > rate(1)) {
		t.Fatalf("EXP per damage: level 1 %.3f, 10 %.3f, 50 %.3f; want it rising", rate(1), rate(10), rate(50))
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

// singleRange and areaRange are the range tables of spec 2.1: single-target
// skills, and cone, line and circle_target.
var (
	singleRange = []float64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}
	areaRange   = []float64{4, 4, 4, 5, 5, 5, 6, 6, 6, 6}
)

// validSkills is a minimal valid skills.json: for each of the five classes
// four actives (single, cone, circle_target, circle_self) and a passive, in
// that order of index 5c+0..4 (c = class number).
func validSkills() []obj {
	var out []obj
	for _, class := range []string{"viem", "thuy", "minh", "nham", "phong"} {
		active := func(n, slot, learn int, shape string, extra obj) obj {
			s := obj{"id": fmt.Sprintf("%s_%d", class, n), "class": class, "name": class, "form": "I",
				"learn_level": learn, "slot": slot, "damage": "phys", "shape": shape,
				"coef_pct": ten(100), "mp": ten(10), "cooldown_ms": ten(1000),
				"windup_ms": 250, "projectile_speed": 20}
			for k, v := range extra {
				s[k] = v
			}
			return s
		}
		out = append(out,
			active(1, 1, 10, "single", obj{"range": singleRange, "dash": true, "windup_ms": 100, "projectile_speed": 25}),
			active(2, 2, 20, "cone", obj{"cone_deg": 120, "range": areaRange, "max_targets": ten(2)}),
			obj{"id": class + "_3", "class": class, "name": "p", "form": "III", "learn_level": 30, "slot": 0,
				"passive": obj{"phys_def_pct": ten(2)}},
			active(4, 3, 40, "circle_target", obj{"range": areaRange, "radius": 1.5, "max_targets": ten(2)}),
			active(5, 4, 50, "circle_self", obj{"radius": 2, "max_targets": ten(2), "windup_ms": 300, "projectile_speed": 0}),
		)
	}
	return out
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
	if len(m) != 25 || !m["viem_1"].Dash || !m["viem_1"].Active() || m["viem_3"].Active() {
		t.Fatalf("parsed: %+v", m)
	}
	if !slices.Equal(m["viem_2"].Range, areaRange) || m["viem_5"].Range != nil {
		t.Fatalf("ranges: cone %v, circle_self %v", m["viem_2"].Range, m["viem_5"].Range)
	}
}

func TestParseSkillsRejects(t *testing.T) {
	// idx within viem: single=0, cone=1, passive=2, circle_target=3, circle_self=4.
	set := func(i int, k string, v any) func([]obj) []obj {
		return func(s []obj) []obj { s[i][k] = v; return s }
	}
	del := func(i int, k string) func([]obj) []obj {
		return func(s []obj) []obj { delete(s[i], k); return s }
	}
	// asLine turns viem's circle_target (index 3) into a line skill, then applies mutate.
	asLine := func(mutate func(obj)) func([]obj) []obj {
		return func(s []obj) []obj {
			x := s[3]
			delete(x, "radius")
			x["shape"], x["width"] = "line", 1.5
			mutate(x)
			return s
		}
	}
	tests := []struct {
		name   string
		mutate func([]obj) []obj
		want   string
	}{
		{"unknown field", set(0, "bogus", 1), "bogus"},
		{"duplicate id", set(1, "id", "viem_1"), "duplicate skill"},
		{"bad class", set(0, "class", "ghost"), `class "ghost" is not valid`},
		{"missing class", func(s []obj) []obj { return s[:20] }, `class "phong" has no skill`},
		{"missing radius", del(4, "radius"), "radius must be positive"},
		{"radius on single", set(0, "radius", 2), "radius is not allowed"},
		{"old cast_range", set(0, "cast_range", 5), `unknown field "cast_range"`},
		{"old reach", set(1, "reach", 4), `unknown field "reach"`},
		{"old length", asLine(func(x obj) { x["length"] = 6 }), `unknown field "length"`},
		{"single missing range", del(0, "range"), "range is required for shape single"},
		{"cone missing range", del(1, "range"), "range is required for shape cone"},
		{"circle_target missing range", del(3, "range"), "range is required for shape circle_target"},
		{"line missing range", asLine(func(x obj) { delete(x, "range") }), "range is required for shape line"},
		{"null range on single", set(0, "range", nil), "range has 0 entries"},
		{"range on circle_self", set(4, "range", areaRange), "range is not allowed for shape circle_self"},
		{"null range on circle_self", set(4, "range", nil), "range is not allowed for shape circle_self"},
		{"range on passive", set(2, "range", areaRange), "passive must not have active fields"},
		{"null range on passive", set(2, "range", nil), "passive must not have active fields"},
		{"range of 9", set(1, "range", areaRange[:9]), "range has 9 entries"},
		{"range of 11", set(1, "range", append(slices.Clone(areaRange), 6)), "range has 11 entries"},
		{"zero range", set(0, "range", []float64{0, 5, 5, 5, 5, 5, 5, 5, 5, 5}), "range must be positive"},
		{"negative range", set(1, "range", []float64{4, 4, 4, 5, 5, -5, 6, 6, 6, 6}), "range must be positive"},
		{"decreasing range", set(1, "range", []float64{4, 4, 4, 5, 5, 5, 6, 6, 6, 5.5}), "range must not decrease"},
		{"decreasing range early", set(3, "range", []float64{5, 4, 4, 5, 5, 5, 6, 6, 6, 6}), "range must not decrease"},
		{"missing windup_ms", del(0, "windup_ms"), "windup_ms is required"},
		{"zero windup_ms", set(0, "windup_ms", 0), "windup_ms must be positive"},
		{"negative windup_ms", set(1, "windup_ms", -5), "windup_ms must be positive"},
		{"missing projectile_speed", del(1, "projectile_speed"), "projectile_speed is required"},
		{"negative projectile_speed", set(1, "projectile_speed", -1), "projectile_speed must not be negative"},
		{"dash with speed 0", set(0, "projectile_speed", 0), "positive for a dash"},
		{"circle_self with speed 0.5", set(4, "projectile_speed", 0.5), "projectile_speed must be 0 for shape circle_self"},
		{"circle_self missing speed", del(4, "projectile_speed"), "projectile_speed is required"},
		{"passive with windup_ms", set(2, "windup_ms", 250), "passive must not have active fields"},
		{"passive with zero windup_ms", set(2, "windup_ms", 0), "passive must not have active fields"},
		{"passive with projectile_speed", set(2, "projectile_speed", 0), "passive must not have active fields"},
		{"array of 9", set(0, "mp", ten(10)[:9]), "mp has 9 entries"},
		{"zero in mp", set(0, "mp", append([]int{0}, ten(10)[1:]...)), "mp must be positive"},
		{"dash on cone", set(1, "dash", true), "dash needs shape single"},
		{"max_targets on single", set(0, "max_targets", ten(2)), "max_targets is not allowed"},
		{"null max_targets on single", set(0, "max_targets", nil), "max_targets is not allowed"},
		{"empty max_targets on single", set(0, "max_targets", []int{}), "max_targets is not allowed"},
		{"missing max_targets on cone", del(1, "max_targets"), "max_targets has 0 entries"},
		{"line missing max_targets", asLine(func(x obj) { delete(x, "max_targets") }), "max_targets has 0 entries"},
		{"line with radius", asLine(func(x obj) { x["radius"] = 1 }), "radius is not allowed for shape line"},
		{"line missing width", asLine(func(x obj) { delete(x, "width") }), "width must be positive"},
		{"passive at level 20", set(2, "learn_level", 20), "learn_level 20 must be 30"},
		{"passive with shape", set(2, "shape", "single"), "passive must not have active fields"},
		{"passive with null shape", set(2, "shape", nil), "passive must not have active fields"},
		{"unknown passive key", set(2, "passive", obj{"speed_pct": ten(1)}), `passive key "speed_pct"`},
		{"empty passive", set(2, "passive", obj{}), "at least one key"},
		{"negative passive", set(2, "passive", obj{"phys_def_pct": append([]int{-1}, ten(1)[1:]...)}), "must not be negative"},
		{"passive array of 9", set(2, "passive", obj{"phys_def_pct": ten(1)[:9]}), "passive.phys_def_pct has 9"},
		{"two slot 2 skills", func(s []obj) []obj { s[3]["slot"], s[3]["learn_level"] = 2, 20; return s }, "two skills in slot 2"},
		{"missing slot 4", func(s []obj) []obj { return append(s[:4:4], s[5:]...) }, `class "viem" has no skill in slot 4`},
		{"cone_deg 0", set(1, "cone_deg", 0), "cone_deg must be positive"},
		{"cone_deg 400", set(1, "cone_deg", 400), "cone_deg 400 must be in (0, 360]"},
		{"bad damage", set(0, "damage", "fire"), `damage "fire"`},
		{"bad shape", set(0, "shape", "ring"), `shape "ring" is not valid`},
		{"active with passive", set(0, "passive", obj{"phys_def_pct": ten(1)}), "active skill must not have passive"},
		{"active with empty passive", set(0, "passive", obj{}), "active skill must not have passive"},
		{"wrong learn level", set(0, "learn_level", 11), "learn_level 11 must be 10"},
		{"bad slot", set(0, "slot", 5), "slot 5 must be 0-4"},
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
	if _, err := ParseSkills(append(skillsJSON(t, validSkills()), []byte(` {"skills":[]}`)...)); err == nil ||
		!strings.Contains(err.Error(), "after the JSON value") {
		t.Fatalf("trailing data: %v", err)
	}
}

func TestRepoData(t *testing.T) {
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
	// Every class has the same ranges (spec 2.1).
	for id, s := range d.Skills {
		want := areaRange
		switch {
		case !s.Active() || s.Shape == ShapeCircleSelf:
			want = nil
		case s.Shape == ShapeSingle:
			want = singleRange
		}
		if !slices.Equal(s.Range, want) {
			t.Fatalf("%s (%s): range %v, want %v", id, s.Shape, s.Range, want)
		}
	}
	// Every active's timing follows the phases spec 2.3 table.
	for id, s := range d.Skills {
		if !s.Active() {
			continue
		}
		wantWindup, wantSpeed := 250, 20.0
		switch {
		case s.Dash:
			wantWindup, wantSpeed = 100, 25
		case s.Shape == ShapeCircleSelf:
			wantWindup, wantSpeed = 300, 0
		case s.Shape == ShapeCircleTarget:
			wantWindup, wantSpeed = 300, 20
		case s.Shape == ShapeSingle && s.Slot == 4:
			wantWindup, wantSpeed = 400, 20
		}
		if s.WindupMS != wantWindup || s.ProjectileSpeed != wantSpeed {
			t.Fatalf("%s (%s, slot %d): windup %d speed %v, want %d %v",
				id, s.Shape, s.Slot, s.WindupMS, s.ProjectileSpeed, wantWindup, wantSpeed)
		}
	}
	for id, k := range d.Kinds {
		if k.MeleeWindupMS != 200 || k.ShotSpeed != 15 {
			t.Fatalf("kind %s: melee windup %d, shot speed %v", id, k.MeleeWindupMS, k.ShotSpeed)
		}
	}
	if combat.SlashWindup != 150*time.Millisecond {
		t.Fatalf("SlashWindup %v", combat.SlashWindup)
	}
	if len(d.Exp) != 89 || d.Exp[0] != 80 {
		t.Fatalf("exp: %d values, first %v", len(d.Exp), d.Exp[:1])
	}
}

// Every passive key lands on its own combat.Passive field, at the learned level.
func TestPassiveAt(t *testing.T) {
	s := Skill{ID: "p", Passive: map[string][]int{}}
	keys := []string{PassivePhysDefPct, PassiveMagicDefPct, PassiveMaxHPPct, PassiveEvasionPct,
		PassiveAccuracyPct, PassiveMagicAttackPct, PassiveCritBP}
	for i, k := range keys {
		v := make([]int, SkillLevels)
		for l := range v {
			v[l] = (i+1)*100 + l + 1 // key i, level l+1
		}
		s.Passive[k] = v
	}
	want := combat.Passive{PhysDefPct: 103, MagicDefPct: 203, MaxHPPct: 303, EvasionPct: 403,
		AccuracyPct: 503, MagicAttackPct: 603, CritBP: 703}
	if got := s.PassiveAt(3); got != want {
		t.Fatalf("level 3: %+v, want %+v", got, want)
	}
	if got := s.PassiveAt(10); got.CritBP != 710 || got.PhysDefPct != 110 {
		t.Fatalf("level 10: %+v", got)
	}
	for _, l := range []int{0, -1, 11} {
		if got := s.PassiveAt(l); got != (combat.Passive{}) {
			t.Fatalf("level %d: %+v, want none", l, got)
		}
	}
	if got := (Skill{ID: "a", Slot: 1}).PassiveAt(5); got != (combat.Passive{}) {
		t.Fatalf("active skill: %+v, want none", got)
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
	valid := exp(89, nil)
	withExtra := append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"x":1}`)...)
	for _, tt := range []struct {
		name string
		b    []byte
		want string
	}{
		{"88 values", exp(88, nil), "88 values, want 89"},
		{"90 values", exp(90, nil), "90 values, want 89"},
		{"non-increasing", exp(89, func(a []int64) { a[5] = a[4] }), "level 6: value must be greater"},
		{"zero", exp(89, func(a []int64) { a[0] = 0 }), "level 1: value must be positive"},
		{"unknown field", withExtra, `unknown field "x"`},
		{"trailing data", append(append([]byte{}, valid...), []byte(" 1")...), "after the JSON value"},
	} {
		if _, err := ParseExp(tt.b); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: want error containing %q, got %v", tt.name, tt.want, err)
		}
	}
}

// ______________________TEST ITEMS DATA_____________________

func sixteen(v int) []int {
	a := make([]int, EnhanceLevels)
	for i := range a {
		a[i] = v + i
	}
	return a
}

// validItems is a minimal valid items.json: the five drop tiers (10-50),
// each with its nine shared pieces and five weapons, three stackables and
// the three drop tables. equipment[0..4] are tier 10's weapons of viem,
// thuy, minh, nham, phong; equipment[5..13] its shared pieces in slot order
// (boots .. ring_right); the other tiers follow in the same order.
func validItems() obj { return itemsWithTiers(MinTier, 20, 30, 40, MaxTier) }

// itemsWithTiers is validItems with only the given tiers and their pieces.
func itemsWithTiers(tiers ...int) obj {
	var slots []any
	for i, id := range SlotIDs {
		regen := RegenHP
		if i == 0 || i >= 6 {
			regen = RegenMP
		}
		slots = append(slots, obj{"id": id, "name": "s" + id, "index": i, "regen": regen})
	}
	quality := func(name string, min, max, hidden int) obj {
		return obj{"name": name, "rank": "r", "lines": []int{min, max}, "hidden_pct": hidden}
	}
	piece := func(code string, tier int, slot, class string, level int) obj {
		zero := make([]int, EnhanceLevels)
		return obj{"code": code, "tier": tier, "slot": slot, "class": class, "name": "n " + code, "level": level,
			"hp": sixteen(10), "phys_attack": zero, "magic_attack": zero, "phys_defense": sixteen(1), "magic_defense": sixteen(1)}
	}
	var equipment, tierList []any
	for _, tier := range tiers {
		prefix := fmt.Sprintf("eq%d_", tier)
		for _, c := range classList {
			w := piece(prefix+"weapon_"+c, tier, SlotWeapon, c, tier)
			if c == "thuy" || c == "phong" {
				w["magic_attack"] = sixteen(25)
			} else {
				w["phys_attack"] = sixteen(25)
			}
			equipment = append(equipment, w)
		}
		for i, id := range SlotIDs[1:] {
			equipment = append(equipment, piece(prefix+id, tier, id, "", tier+i+1))
		}
		tierList = append(tierList, obj{"tier": tier, "name": fmt.Sprintf("Bậc %d", tier), "line_standard": obj{
			StatHP: 86, StatPhysAttack: 4, StatMagicAttack: 4, StatPhysDefense: 1, StatMagicDefense: 1}})
	}
	tierList[0].(obj)["name"] = "Lữ Hành"
	drop := func(xuMin, xuMax, chance, count int, q []int, stacks ...obj) obj {
		st := []any{}
		for _, s := range stacks {
			st = append(st, s)
		}
		return obj{"xu_per_level": []int{xuMin, xuMax},
			"equipment": obj{"chance_bp": chance, "count": count, "quality_bp": q}, "stacks": st}
	}
	stack := func(code string, chance, count int) obj {
		return obj{"code": code, "chance_bp": chance, "count": count}
	}
	return obj{
		"bag_size": 60, "stack_max": 999, "ground_sec": 60,
		"slots": slots,
		"qualities": []any{quality("Trắng", 0, 0, 0), quality("Xanh", 1, 2, 0), quality("Vàng", 3, 4, 100),
			quality("Cam", 4, 4, 125), quality("Đỏ", 4, 4, 150)},
		"line_stats":        []any{StatHP, StatPhysAttack, StatMagicAttack, StatPhysDefense, StatMagicDefense},
		"line_roll_min_pct": 50,
		"tiers":             tierList,
		"equipment":         equipment,
		"stackables": append([]any{obj{"code": "stone_enhance", "name": "Đá cường hóa"}, obj{"code": "stone_refine", "name": "Đá tẩy luyện"},
			obj{"code": "charm_protect", "name": "Bùa bảo vệ"}}, gemList()...),
		"drops": obj{
			RankNormal: drop(3, 5, 500, 1, []int{8000, 1700, 300, 0, 0}, stack("stone_enhance", 2000, 1)),
			RankElite:  drop(18, 30, 10000, 1, []int{0, 7500, 2500, 0, 0}, stack("stone_enhance", 10000, 3), stack("stone_refine", 1000, 1)),
			RankBoss:   drop(75, 125, 10000, 2, []int{0, 2000, 7500, 500, 0}, stack("stone_enhance", 10000, 10), stack("stone_refine", 10000, 1)),
		},
		"enhance": obj{"pity_pct": 1, "max_times": 100, "levels": enhanceLevels()},
		"regen": []any{obj{"enhance": 7, "hp_bp": 5, "mp": 1}, obj{"enhance": 10, "hp_bp": 10, "mp": 2},
			obj{"enhance": 12, "hp_bp": 20, "mp": 3}, obj{"enhance": 15, "hp_bp": 30, "mp": 5}},
		"resonance": obj{"activator": specActivator(), "slots": specSlots()},
		"sockets":   obj{"max": 3, "punch_xu_per_tier": []int{100, 200, 400}, "unsocket_xu_per_level": 500},
		"refine":    obj{"min_quality": 2, "stones": 1, "stones_per_lock": 1, "xu_per_tier": 20, "xu_per_tier_per_lock": 20},
	}
}

// specActivator is the phase 3c spec 2.1 activator table: the cycle
// Thủy → Phong → Viêm → Nham → Minh → Thủy.
func specActivator() obj {
	return obj{"viem": "phong", "nham": "viem", "minh": "nham", "thuy": "minh", "phong": "thuy"}
}

// specSlots is the phase 3c spec 2.1 activating-slot table (game-design 3.2).
func specSlots() obj {
	return obj{
		SlotWeapon: []any{SlotHelmet, SlotArmor}, SlotHelmet: []any{SlotNecklace, SlotRingLeft},
		SlotArmor: []any{SlotNecklace, SlotRingRight}, SlotPants: []any{SlotArmor, SlotPendant},
		SlotBracers: []any{SlotWeapon, SlotRingLeft}, SlotBoots: []any{SlotPants, SlotPendant},
		SlotNecklace: []any{SlotWeapon, SlotBracers}, SlotPendant: []any{SlotHelmet, SlotBoots},
		SlotRingLeft: []any{SlotPants, SlotRingRight}, SlotRingRight: []any{SlotBoots, SlotBracers},
	}
}

// gemKinds lists each gem kind with the stats it raises.
var gemKinds = []struct {
	kind  string
	stats []string
}{{GemAttack, []string{StatPhysAttack, StatMagicAttack}}, {GemHP, []string{StatHP}},
	{GemDefense, []string{StatPhysDefense, StatMagicDefense}}}

// gemList is 15 valid gems (three kinds, levels 1-5), each stat worth its
// level; stackables[3+5k+g-1] is kind k at level g.
func gemList() []any {
	var out []any
	for _, k := range gemKinds {
		for lv := 1; lv <= MaxGemLevel; lv++ {
			stats := obj{}
			for _, s := range k.stats {
				stats[s] = lv
			}
			out = append(out, obj{"code": fmt.Sprintf("gem_%s_%d", k.kind, lv), "name": fmt.Sprintf("Ngọc %s %d", k.kind, lv),
				"gem": obj{"kind": k.kind, "level": lv, "stats": stats}})
		}
	}
	return out
}

// enhanceLevels is the phase 3b spec 2.1 table: entry i is target +i+1.
func enhanceLevels() []any {
	rows := [][5]int{ // level, success_pct, drop (0/1), stones, xu_per_tier
		{1, 100, 0, 1, 10}, {2, 100, 0, 1, 15}, {3, 100, 0, 1, 20}, {4, 100, 0, 1, 25},
		{5, 90, 0, 2, 40}, {6, 80, 0, 2, 50}, {7, 70, 0, 2, 60},
		{8, 60, 1, 3, 80}, {9, 50, 1, 3, 100}, {10, 40, 1, 3, 120}, {11, 30, 1, 4, 150},
		{12, 25, 1, 4, 180}, {13, 20, 1, 5, 220}, {14, 15, 1, 5, 260}, {15, 10, 1, 5, 300},
	}
	var out []any
	for _, r := range rows {
		out = append(out, obj{"level": r[0], "success_pct": r[1], "drop": r[2] == 1, "stones": r[3], "xu_per_tier": r[4]})
	}
	return out
}

func itemsJSON(t *testing.T, d obj) []byte {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseItemsValid(t *testing.T) {
	it, err := ParseItems(itemsJSON(t, validItems()))
	if err != nil {
		t.Fatal(err)
	}
	if it.BagSize != 60 || it.StackMax != 999 || it.GroundSec != 60 || it.LineRollMinPct != 50 {
		t.Fatalf("scalars: %+v", it)
	}
	if len(it.Slots) != 10 || it.Slots[9] != (Slot{ID: SlotRingRight, Name: "sring_right", Index: 9, Regen: RegenMP}) {
		t.Fatalf("slots: %+v", it.Slots)
	}
	if q := it.Qualities[1]; q.Name != "Xanh" || q.Lines != [2]int{1, 2} || q.HiddenPct != 0 {
		t.Fatalf("quality 1: %+v", q)
	}
	if s := it.Tiers[10]; s.Name != "Lữ Hành" || s.LineStandard[StatHP] != 86 {
		t.Fatalf("tier 10: %+v", s)
	}
	if e := it.Equipment["eq10_armor"]; e.SlotIndex != 5 || e.Level != 15 || e.HP[3] != 13 {
		t.Fatalf("armor: %+v", e)
	}
	if it.Weapon(10, "nham") != "eq10_weapon_nham" || it.Weapon(60, "nham") != "" || it.Weapon(10, "lua") != "" {
		t.Fatal("Weapon lookup")
	}
	if it.Piece(10, 5) != "eq10_armor" || it.Piece(10, 0) != "" || it.Piece(60, 5) != "" || it.Piece(50, 5) != "eq50_armor" {
		t.Fatal("Piece lookup")
	}
	if !it.IsEquipment("eq10_ring_left") || it.IsEquipment("stone_enhance") {
		t.Fatal("IsEquipment")
	}
	if !slices.Equal(it.StackOrder[:4], []string{"stone_enhance", "stone_refine", "charm_protect", "gem_attack_1"}) || len(it.StackOrder) != 18 ||
		it.Stackables["stone_refine"].Name != "Đá tẩy luyện" {
		t.Fatalf("stackables: %v %+v", it.StackOrder, it.Stackables)
	}
	b := it.Drops[RankBoss]
	if b.XuPerLevel != [2]int{75, 125} || b.Equipment.Count != 2 || len(b.Stacks) != 2 || b.Stacks[0].Count != 10 {
		t.Fatalf("boss drop: %+v", b)
	}
}

func TestParseItemsRejects(t *testing.T) {
	list := func(d obj, k string) []any { return d[k].([]any) }
	at := func(d obj, k string, i int) obj { return list(d, k)[i].(obj) }
	set := func(k string, v any) func(obj) {
		return func(d obj) { d[k] = v }
	}
	// setIn sets field f of entry i of list k.
	setIn := func(k string, i int, f string, v any) func(obj) {
		return func(d obj) { at(d, k, i)[f] = v }
	}
	delIn := func(k string, i int) func(obj) {
		return func(d obj) { d[k] = slices.Delete(slices.Clone(list(d, k)), i, i+1) }
	}
	std := func(f func(obj)) func(obj) {
		return func(d obj) { f(at(d, "tiers", 0)["line_standard"].(obj)) }
	}
	drop := func(rank string, f func(obj)) func(obj) {
		return func(d obj) { f(d["drops"].(obj)[rank].(obj)) }
	}
	dropEq := func(rank, f string, v any) func(obj) {
		return drop(rank, func(x obj) { x["equipment"].(obj)[f] = v })
	}
	dropStack := func(rank string, i int, f string, v any) func(obj) {
		return drop(rank, func(x obj) { x["stacks"].([]any)[i].(obj)[f] = v })
	}
	// tiersOnly replaces the tiers and their pieces with those of tiers.
	tiersOnly := func(tiers ...int) func(obj) {
		return func(d obj) {
			x := itemsWithTiers(tiers...)
			d["tiers"], d["equipment"] = x["tiers"], x["equipment"]
		}
	}
	// equipment indices: 0 viem, 1 thuy, ... 4 phong weapons; 5 boots .. 13 ring_right; 9 armor.
	const viem, thuy, boots, armor, ringRight = 0, 1, 5, 9, 13
	ones := make([]int, EnhanceLevels)
	for i := range ones {
		ones[i] = 1
	}
	tests := []struct {
		name   string
		mutate func(obj)
		want   string
	}{
		{"unknown field", set("bogus", 1), `unknown field "bogus"`},
		{"unknown nested field", setIn("equipment", 0, "enhance", 1), `unknown field "enhance"`},
		{"bag_size 0", set("bag_size", 0), "bag_size 0 must be in 1-200"},
		{"bag_size 201", set("bag_size", 201), "bag_size 201 must be in 1-200"},
		{"stack_max 0", set("stack_max", 0), "stack_max 0 must be at least 1"},
		{"ground_sec 0", set("ground_sec", 0), "ground_sec must be positive"},
		{"line_roll_min_pct 0", set("line_roll_min_pct", 0), "line_roll_min_pct 0 must be in 1-100"},
		{"line_roll_min_pct 101", set("line_roll_min_pct", 101), "line_roll_min_pct 101 must be in 1-100"},

		{"9 slots", delIn("slots", 9), "slots has 9 entries, want 10"},
		{"slots out of order", func(d obj) {
			s := list(d, "slots")
			s[0].(obj)["id"], s[1].(obj)["id"] = SlotBoots, SlotWeapon
		}, `slots[0] is "boots", want "weapon"`},
		{"slot index", setIn("slots", 3, "index", 4), `slot "pants": index 4, want 3`},
		{"slot without name", setIn("slots", 3, "name", ""), `slot "pants": name is empty`},

		{"4 qualities", delIn("qualities", 4), "qualities has 4 entries, want 5"},
		{"Vàng with hidden_pct 0", setIn("qualities", 2, "hidden_pct", 0), `quality 2 "Vàng": hidden_pct 0 must be positive`},
		{"Cam with negative hidden_pct", setIn("qualities", 3, "hidden_pct", -5), `quality 3 "Cam": hidden_pct -5 must be positive`},
		{"Xanh with hidden_pct 10", setIn("qualities", 1, "hidden_pct", 10), `quality 1 "Xanh": hidden_pct 10 must be 0`},
		{"lines max above 5", setIn("qualities", 4, "lines", []int{4, 6}), `quality 4 "Đỏ": lines [4 6] must satisfy 0 <= min <= max <= 5`},
		{"lines min above max", setIn("qualities", 1, "lines", []int{2, 1}), `quality 1 "Xanh": lines [2 1] must satisfy`},
		{"lines min negative", setIn("qualities", 1, "lines", []int{-1, 1}), `quality 1 "Xanh": lines [-1 1] must satisfy`},
		{"lines of 3", setIn("qualities", 1, "lines", []int{1, 2, 2}), `quality 1 "Xanh": lines has 3 entries, want 2`},
		{"quality without name", setIn("qualities", 1, "name", ""), "quality 1: name is empty"},

		{"duplicate line stat", set("line_stats", []any{StatHP, StatHP, StatMagicAttack, StatPhysDefense, StatMagicDefense}), `line stat "hp" is listed twice`},
		// Without line stats every quality has 0 lines and the tiers no
		// standards, which is otherwise consistent; hidden lines would then
		// pick from an empty list.
		{"empty line_stats", func(d obj) {
			d["line_stats"] = []any{}
			for _, q := range list(d, "qualities") {
				q.(obj)["lines"] = []int{0, 0}
			}
			for _, tr := range list(d, "tiers") {
				tr.(obj)["line_standard"] = obj{}
			}
		}, "line_stats is empty"},
		{"unknown line stat", set("line_stats", []any{StatHP, "mp", StatMagicAttack, StatPhysDefense, StatMagicDefense}), `line stat "mp" is unknown`},

		{"tier missing a line standard", std(func(s obj) { delete(s, StatMagicDefense) }), `tier 10: line_standard has no "magic_defense"`},
		{"standard 0", std(func(s obj) { s[StatPhysAttack] = 0 }), `tier 10: line_standard "phys_attack" 0 must be at least 1`},
		{"standard of unknown stat", std(func(s obj) { s["mp"] = 3 }), `tier 10: line_standard "mp" is not a line stat`},
		{"duplicate tier", func(d obj) { d["tiers"] = append(list(d, "tiers"), list(d, "tiers")[0]) }, "tier 10 is listed twice"},
		{"tier 0", setIn("tiers", 0, "tier", 0), "tier 0 must be positive"},
		// Every tier DropTier can return must exist, and no other: a missing
		// one would silently drop nothing.
		{"missing tier 50", tiersOnly(MinTier, 20, 30, 40), "tiers has no 50"},
		{"missing tier 10", tiersOnly(20, 30, 40, MaxTier), "tiers has no 10"},
		{"tier 60", tiersOnly(MinTier, 20, 30, 40, MaxTier, 60), "tier 60 is not a drop tier (10-50, step 10)"},
		{"tier 15", tiersOnly(MinTier, 15, 20, 30, 40, MaxTier), "tier 15 is not a drop tier"},
		{"tier without name", setIn("tiers", 0, "name", ""), "tier 10: name is empty"},

		{"duplicate code", setIn("equipment", boots, "code", "eq10_weapon_viem"), `duplicate item code "eq10_weapon_viem"`},
		{"empty code", setIn("equipment", boots, "code", ""), "equipment with empty code"},
		{"unknown tier", func(d obj) { e := at(d, "equipment", boots); e["tier"], e["level"] = 60, 61 }, `equipment "eq10_boots": tier 60 is not in tiers`},
		{"unknown slot", setIn("equipment", boots, "slot", "cape"), `equipment "eq10_boots": slot "cape" is unknown`},
		{"weapon without class", setIn("equipment", viem, "class", ""), `equipment "eq10_weapon_viem": class "" is not valid`},
		{"weapon with bad class", setIn("equipment", viem, "class", "lua"), `equipment "eq10_weapon_viem": class "lua" is not valid`},
		{"armour with class", setIn("equipment", armor, "class", "viem"), `equipment "eq10_armor": class must be empty`},
		{"piece without name", setIn("equipment", armor, "name", ""), `equipment "eq10_armor": name is empty`},
		{"wrong level", setIn("equipment", armor, "level", 14), `equipment "eq10_armor": level 14, want 15`},
		{"array of 15", setIn("equipment", armor, "hp", sixteen(10)[:15]), `equipment "eq10_armor": hp has 15 entries, want 16`},
		{"array of 17", setIn("equipment", armor, "phys_defense", append(sixteen(1), 99)), `equipment "eq10_armor": phys_defense has 17 entries, want 16`},
		{"missing array", func(d obj) { delete(at(d, "equipment", armor), "magic_defense") }, `equipment "eq10_armor": magic_defense has 0 entries`},
		{"decreasing array", setIn("equipment", armor, "hp", append(sixteen(10)[:15], 0)), `equipment "eq10_armor": hp must not decrease at +15`},
		{"negative value", setIn("equipment", armor, "phys_defense", append([]int{-1}, sixteen(1)[1:]...)), `equipment "eq10_armor": phys_defense must not be negative`},
		{"physical-class weapon with magic attack", setIn("equipment", viem, "magic_attack", ones), `equipment "eq10_weapon_viem": magic_attack must be 0 for physical class "viem"`},
		{"magic-class weapon with physical attack", setIn("equipment", thuy, "phys_attack", ones), `equipment "eq10_weapon_thuy": phys_attack must be 0 for magic class "thuy"`},
		{"tier missing a piece", delIn("equipment", ringRight), `tier 10 has no "ring_right" piece`},
		{"tier missing a weapon", delIn("equipment", thuy), `tier 10 has no "weapon" piece for class "thuy"`},
		{"two pieces in a slot", func(d obj) {
			e := maps.Clone(at(d, "equipment", ringRight))
			e["code"] = "eq10_ring_right_b"
			d["equipment"] = append(list(d, "equipment"), e)
		}, `equipment "eq10_ring_right_b": tier 10 already has a "ring_right" piece`},

		{"duplicate stackable", setIn("stackables", 1, "code", "stone_enhance"), `duplicate item code "stone_enhance"`},
		{"stackable code equal to an equipment code", setIn("stackables", 1, "code", "eq10_armor"), `duplicate item code "eq10_armor"`},
		{"stackable without name", setIn("stackables", 1, "name", ""), `stackable "stone_refine": name is empty`},
		{"stackable with empty code", setIn("stackables", 1, "code", ""), "stackable with empty code"},

		{"missing boss drop", func(d obj) { delete(d["drops"].(obj), RankBoss) }, `drops has no "boss"`},
		{"extra drop rank", func(d obj) { d["drops"].(obj)["champion"] = d["drops"].(obj)[RankBoss] }, `drops "champion" is not a rank`},
		{"xu min above max", drop(RankElite, func(x obj) { x["xu_per_level"] = []int{31, 30} }), `drops "elite": xu_per_level [31 30] must satisfy 0 <= min <= max`},
		{"xu negative", drop(RankElite, func(x obj) { x["xu_per_level"] = []int{-1, 30} }), `drops "elite": xu_per_level [-1 30] must satisfy`},
		{"xu of 1", drop(RankElite, func(x obj) { x["xu_per_level"] = []int{3} }), `drops "elite": xu_per_level has 1 entries, want 2`},
		{"equipment chance_bp 10001", dropEq(RankNormal, "chance_bp", 10001), `drops "normal": equipment chance_bp 10001 must be in 0-10000`},
		{"equipment chance_bp -1", dropEq(RankNormal, "chance_bp", -1), `drops "normal": equipment chance_bp -1 must be in 0-10000`},
		{"equipment count 0", dropEq(RankBoss, "count", 0), `drops "boss": equipment count 0 must be at least 1`},
		{"quality_bp sum 9999", dropEq(RankNormal, "quality_bp", []int{7999, 1700, 300, 0, 0}), `drops "normal": quality_bp sums to 9999, want 10000`},
		{"quality_bp negative", dropEq(RankNormal, "quality_bp", []int{8001, 1700, 300, 0, -1}), `drops "normal": quality_bp must not be negative`},
		{"quality_bp of 4", dropEq(RankNormal, "quality_bp", []int{8000, 1700, 300, 0}), `drops "normal": quality_bp has 4 entries, want 5`},
		{"stack drop of an unknown code", dropStack(RankElite, 1, "code", "stone_gem"), `drops "elite": stack "stone_gem" is not a stackable`},
		{"stack drop of an equipment code", dropStack(RankElite, 1, "code", "eq10_armor"), `drops "elite": stack "eq10_armor" is not a stackable`},
		{"stack chance_bp 10001", dropStack(RankElite, 0, "chance_bp", 10001), `drops "elite": stack "stone_enhance" chance_bp 10001 must be in 0-10000`},
		{"stack chance_bp -1", dropStack(RankElite, 0, "chance_bp", -1), `drops "elite": stack "stone_enhance" chance_bp -1 must be in 0-10000`},
		{"stack count 0", dropStack(RankBoss, 1, "count", 0), `drops "boss": stack "stone_refine" count 0 must be at least 1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validItems()
			tt.mutate(d)
			_, err := ParseItems(itemsJSON(t, d))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want error containing %q, got %v", tt.want, err)
			}
		})
	}
	if _, err := ParseItems(append(itemsJSON(t, validItems()), []byte(` {}`)...)); err == nil ||
		!strings.Contains(err.Error(), "after the JSON value") {
		t.Fatalf("trailing data: %v", err)
	}
}

// The committed items.json loads (spec 2.1).
func TestRepoItems(t *testing.T) {
	d, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	it := d.Items
	if it == nil || len(it.Equipment) != 70 || len(it.Stackables) != 18 || len(it.Drops) != 3 || len(it.Tiers) != 5 {
		t.Fatalf("items: %d equipment, %d stackables, %d drops", len(it.Equipment), len(it.Stackables), len(it.Drops))
	}
	perTier := map[int]int{}
	for _, e := range it.Equipment {
		perTier[e.Tier]++
	}
	for _, tier := range []int{10, 20, 30, 40, 50} {
		if perTier[tier] != 14 {
			t.Errorf("tier %d: %d pieces, want 14", tier, perTier[tier])
		}
	}
	for _, code := range []string{"eq10_weapon_viem", "eq50_ring_right"} {
		if !it.IsEquipment(code) {
			t.Errorf("%s missing", code)
		}
	}
	if e := it.Equipment[it.Weapon(50, "thuy")]; e.Name != "Thủy Nguyệt Xích Cương Kiếm" || e.Class != "thuy" || e.SlotIndex != 0 {
		t.Fatalf("Weapon(50, thuy): %+v", e)
	}
	if e := it.Equipment["eq10_weapon_viem"]; e.PhysAttack[0] != 25 || e.MagicAttack[15] != 0 || e.Level != 10 {
		t.Fatalf("eq10_weapon_viem: %+v", e)
	}
	if e := it.Equipment[it.Piece(50, 9)]; e.Code != "eq50_ring_right" || e.Level != 59 {
		t.Fatalf("Piece(50, 9): %+v", e)
	}
	for _, r := range []string{RankNormal, RankElite, RankBoss} {
		if _, ok := it.Drops[r]; !ok {
			t.Errorf("no %s drop", r)
		}
	}
	if s := it.Tiers[50].LineStandard; s[StatHP] != 469 || s[StatPhysAttack] != 32 || s[StatMagicDefense] != 5 {
		t.Fatalf("tier 50 standard: %v", s)
	}
}

// ______________________TEST ENHANCE DATA_____________________

func TestParseEnhanceData(t *testing.T) {
	it, err := ParseItems(itemsJSON(t, validItems()))
	if err != nil {
		t.Fatal(err)
	}
	e := it.Enhance
	if e.PityPct != 1 || e.MaxTimes != 100 || len(e.Levels) != 15 {
		t.Fatalf("enhance: %+v", e)
	}
	if e.Levels[0] != (EnhanceLevel{Level: 1, SuccessPct: 100, Stones: 1, XuPerTier: 10}) {
		t.Fatalf("+1: %+v", e.Levels[0])
	}
	if e.Levels[7] != (EnhanceLevel{Level: 8, SuccessPct: 60, Drop: true, Stones: 3, XuPerTier: 80}) {
		t.Fatalf("+8: %+v", e.Levels[7])
	}
	if !slices.Equal(it.Regen, []RegenStep{{7, 5, 1}, {10, 10, 2}, {12, 20, 3}, {15, 30, 5}}) {
		t.Fatalf("regen: %+v", it.Regen)
	}
	if it.Slots[0].Regen != RegenMP || it.Slots[1].Regen != RegenHP || it.Slots[5].Regen != RegenHP || it.Slots[6].Regen != RegenMP {
		t.Fatalf("slot regen: %+v", it.Slots)
	}
	if _, ok := it.Stackables[CharmProtect]; !ok {
		t.Fatal("no charm")
	}
}

func TestParseEnhanceRejects(t *testing.T) {
	enh := func(f func(obj)) func(obj) {
		return func(d obj) { f(d["enhance"].(obj)) }
	}
	level := func(i int, f string, v any) func(obj) {
		return enh(func(e obj) { e["levels"].([]any)[i].(obj)[f] = v })
	}
	regen := func(i int, f string, v any) func(obj) {
		return func(d obj) { d["regen"].([]any)[i].(obj)[f] = v }
	}
	tests := []struct {
		name   string
		mutate func(obj)
		want   string
	}{
		{"14 levels", enh(func(e obj) { e["levels"] = e["levels"].([]any)[:14] }), "enhance levels has 14 entries, want 15"},
		{"16 levels", enh(func(e obj) { e["levels"] = append(e["levels"].([]any), e["levels"].([]any)[14]) }), "enhance levels has 16 entries, want 15"},
		{"no enhance", func(d obj) { delete(d, "enhance") }, "enhance pity_pct 0 must be at least 1"},
		{"levels out of order", func(d obj) {
			l := d["enhance"].(obj)["levels"].([]any)
			l[9], l[10] = l[10], l[9]
		}, "enhance levels[9] is level 11, want 10"},
		{"rising success_pct", level(9, "success_pct", 55), "enhance +10: success_pct 55 must not be above +9's 50"},
		{"success_pct 0", level(14, "success_pct", 0), "enhance +15: success_pct 0 must be in 1-100"},
		{"success_pct 101", level(0, "success_pct", 101), "enhance +1: success_pct 101 must be in 1-100"},
		{"+4 below 100", level(3, "success_pct", 95), "enhance +4: success_pct 95 must be 100"},
		{"drop at +3", level(2, "drop", true), "enhance +3: drop must be false"},
		{"drop with 100%", func(d obj) {
			level(4, "success_pct", 100)(d)
			level(4, "drop", true)(d)
		}, "enhance +5: drop needs success_pct below 100"},
		{"stones 0", level(5, "stones", 0), "enhance +6: stones 0 must be at least 1"},
		{"xu_per_tier 0", level(0, "xu_per_tier", 0), "enhance +1: xu_per_tier 0 must be at least 1"},
		{"falling xu_per_tier", level(10, "xu_per_tier", 99), "enhance +11: xu_per_tier 99 must not be below +10's 120"},
		{"pity_pct 0", enh(func(e obj) { e["pity_pct"] = 0 }), "enhance pity_pct 0 must be at least 1"},
		{"max_times 0", enh(func(e obj) { e["max_times"] = 0 }), "enhance max_times 0 must be in 1-1000"},
		{"max_times 1001", enh(func(e obj) { e["max_times"] = 1001 }), "enhance max_times 1001 must be in 1-1000"},
		{"unknown enhance field", enh(func(e obj) { e["bogus"] = 1 }), `unknown field "bogus"`},

		{"no regen", func(d obj) { delete(d, "regen") }, "regen is empty"},
		{"regen out of order", func(d obj) {
			r := d["regen"].([]any)
			r[1], r[2] = r[2], r[1]
		}, "regen[2]: enhance 10, hp_bp 10 and mp 2 must each be above the step before"},
		{"regen equal enhance", regen(1, "enhance", 7), "regen[1]: enhance 7, hp_bp 10 and mp 2 must each be above the step before"},
		{"regen hp_bp not rising", regen(3, "hp_bp", 20), "regen[3]: enhance 15, hp_bp 20 and mp 5 must each be above the step before"},
		{"regen mp not rising", regen(2, "mp", 2), "regen[2]: enhance 12, hp_bp 20 and mp 2 must each be above the step before"},
		{"regen enhance 16", regen(3, "enhance", 16), "regen[3]: enhance 16 must be in 1-15"},
		{"regen enhance 0", regen(0, "enhance", 0), "regen[0]: enhance 0 must be in 1-15"},
		{"regen hp_bp 0", regen(0, "hp_bp", 0), "regen[0]: hp_bp 0 and mp 1 must be positive"},
		{"regen mp 0", regen(0, "mp", 0), "regen[0]: hp_bp 5 and mp 0 must be positive"},

		{"slot without regen", func(d obj) { delete(d["slots"].([]any)[4].(obj), "regen") }, `slot "helmet": regen "" must be "hp" or "mp"`},
		{"slot with bad regen", func(d obj) { d["slots"].([]any)[0].(obj)["regen"] = "sp" }, `slot "weapon": regen "sp" must be "hp" or "mp"`},
		{"missing charm", func(d obj) { d["stackables"] = slices.Delete(d["stackables"].([]any), 2, 3) }, `stackables has no "charm_protect"`},
		{"missing stone", func(d obj) {
			d["stackables"] = d["stackables"].([]any)[1:]
			d["drops"] = obj{RankNormal: dropNoStacks(d, RankNormal), RankElite: dropNoStacks(d, RankElite), RankBoss: dropNoStacks(d, RankBoss)}
		}, `stackables has no "stone_enhance"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validItems()
			tt.mutate(d)
			_, err := ParseItems(itemsJSON(t, d))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want error containing %q, got %v", tt.want, err)
			}
		})
	}
}

// dropNoStacks is the rank's drop table of d without its stack drops.
func dropNoStacks(d obj, rank string) obj {
	x := maps.Clone(d["drops"].(obj)[rank].(obj))
	x["stacks"] = []any{}
	return x
}

// The committed enhancement data is the spec 2.1 table.
func TestRepoEnhanceData(t *testing.T) {
	d, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	it := d.Items
	if got := it.Enhance.Levels[14]; got != (EnhanceLevel{Level: 15, SuccessPct: 10, Drop: true, Stones: 5, XuPerTier: 300}) {
		t.Fatalf("+15: %+v", got)
	}
	var want []EnhanceLevel
	for _, l := range enhanceLevels() {
		o := l.(obj)
		want = append(want, EnhanceLevel{Level: o["level"].(int), SuccessPct: o["success_pct"].(int), Drop: o["drop"].(bool),
			Stones: o["stones"].(int), XuPerTier: o["xu_per_tier"].(int)})
	}
	if !slices.Equal(it.Enhance.Levels, want) || it.Enhance.PityPct != 1 || it.Enhance.MaxTimes != 100 {
		t.Fatalf("enhance: %+v", it.Enhance)
	}
	if !slices.Equal(it.Regen, []RegenStep{{7, 5, 1}, {10, 10, 2}, {12, 20, 3}, {15, 30, 5}}) {
		t.Fatalf("regen: %+v", it.Regen)
	}
	wantRegen := map[string]string{SlotWeapon: RegenMP, SlotBoots: RegenHP, SlotBracers: RegenHP, SlotPants: RegenHP,
		SlotHelmet: RegenHP, SlotArmor: RegenHP, SlotNecklace: RegenMP, SlotPendant: RegenMP, SlotRingLeft: RegenMP, SlotRingRight: RegenMP}
	for _, s := range it.Slots {
		if s.Regen != wantRegen[s.ID] {
			t.Errorf("slot %s regen %q, want %q", s.ID, s.Regen, wantRegen[s.ID])
		}
	}
	if it.Stackables[CharmProtect].Name != "Bùa bảo vệ" || it.StackOrder[2] != CharmProtect || it.StackOrder[0] != StoneEnhance {
		t.Fatalf("stackables: %v", it.StackOrder)
	}
}

// ______________________TEST FORGE DATA_____________________

// slotIdx is the index of each slot id.
func slotIdx(id string) int { return slices.Index(SlotIDs[:], id) }

func TestParseForgeData(t *testing.T) {
	it, err := ParseItems(itemsJSON(t, validItems()))
	if err != nil {
		t.Fatal(err)
	}
	r := it.Resonance
	if r.Activator["viem"] != "phong" || r.Activator["phong"] != "thuy" || len(r.Activator) != 5 {
		t.Fatalf("activator: %v", r.Activator)
	}
	if r.Slots[slotIdx(SlotWeapon)] != [2]int{slotIdx(SlotHelmet), slotIdx(SlotArmor)} {
		t.Fatalf("weapon slots: %v", r.Slots[0])
	}
	if r.Slots[slotIdx(SlotRingRight)] != [2]int{slotIdx(SlotBoots), slotIdx(SlotBracers)} {
		t.Fatalf("ring_right slots: %v", r.Slots[9])
	}
	if it.Sockets.Max != 3 || !slices.Equal(it.Sockets.PunchXuPerTier, []int{100, 200, 400}) || it.Sockets.UnsocketXuPerLevel != 500 {
		t.Fatalf("sockets: %+v", it.Sockets)
	}
	if it.Refine != (Refine{MinQuality: 2, Stones: 1, StonesPerLock: 1, XuPerTier: 20, XuPerTierPerLock: 20}) {
		t.Fatalf("refine: %+v", it.Refine)
	}
	if !it.IsGem("gem_hp_5") || it.IsGem("stone_refine") || it.IsGem("eq10_armor") || it.IsGem("nope") {
		t.Fatal("IsGem")
	}
	g := it.Stackables["gem_attack_4"].Gem
	if g == nil || g.Kind != GemAttack || g.Level != 4 || !maps.Equal(g.Stats, map[string]int{StatPhysAttack: 4, StatMagicAttack: 4}) {
		t.Fatalf("gem_attack_4: %+v", g)
	}
	if it.Stackables[StoneRefine].Gem != nil {
		t.Fatal("stone has a gem block")
	}
}

func TestParseForgeRejects(t *testing.T) {
	res := func(f func(obj)) func(obj) {
		return func(d obj) { f(d["resonance"].(obj)) }
	}
	act := func(f func(obj)) func(obj) {
		return res(func(r obj) { f(r["activator"].(obj)) })
	}
	slot := func(id string, v any) func(obj) {
		return res(func(r obj) { r["slots"].(obj)[id] = v })
	}
	sock := func(f string, v any) func(obj) {
		return func(d obj) { d["sockets"].(obj)[f] = v }
	}
	ref := func(f string, v any) func(obj) {
		return func(d obj) { d["refine"].(obj)[f] = v }
	}
	// gem sets field f of the gem block of stackables[3+i] (gem_attack_1 is 0).
	gem := func(i int, f string, v any) func(obj) {
		return func(d obj) { d["stackables"].([]any)[3+i].(obj)["gem"].(obj)[f] = v }
	}
	tests := []struct {
		name   string
		mutate func(obj)
		want   string
	}{
		{"element activating itself", act(func(a obj) { a["viem"] = "viem" }), `resonance activator: "viem" activates itself`},
		{"two cycles", act(func(a obj) {
			// viem <-> phong, nham -> minh -> thuy -> nham.
			a["viem"], a["phong"], a["nham"], a["minh"], a["thuy"] = "phong", "viem", "thuy", "nham", "minh"
		}), "resonance activator must be one cycle through all five elements"},
		{"not a permutation", act(func(a obj) { a["viem"] = "thuy" }), `resonance activator: "thuy" activates more than one element`},
		{"missing element", act(func(a obj) { delete(a, "minh") }), `resonance activator has no "minh"`},
		{"unknown element key", act(func(a obj) { a["lua"] = "viem" }), `resonance activator: "lua" is not an element`},
		{"unknown activator", act(func(a obj) { a["viem"] = "lua" }), `resonance activator: "viem" is activated by "lua", not an element`},
		{"slot activating itself", slot(SlotArmor, []any{SlotNecklace, SlotArmor}), `resonance slots "armor": activated by itself`},
		{"slot activated twice by one slot", slot(SlotArmor, []any{SlotNecklace, SlotNecklace}), `resonance slots "armor": both lines activated by "necklace"`},
		{"unknown activating slot", slot(SlotArmor, []any{SlotNecklace, "cape"}), `resonance slots "armor": "cape" is not a slot`},
		{"three activating slots", slot(SlotArmor, []any{SlotNecklace, SlotRingLeft, SlotRingRight}), `resonance slots "armor": 3 slots, want 2`},
		{"missing slot", res(func(r obj) { delete(r["slots"].(obj), SlotPendant) }), `resonance slots has no "pendant"`},
		{"unknown slot", slot("cape", []any{SlotNecklace, SlotRingLeft}), `resonance slots "cape" is not a slot`},
		{"no resonance", func(d obj) { delete(d, "resonance") }, "resonance activator has no"},

		{"max 3 with two prices", sock("punch_xu_per_tier", []int{100, 200}), "sockets max 3 needs 3 punch_xu_per_tier entries, got 2"},
		{"max 0", func(d obj) { sock("max", 0)(d); sock("punch_xu_per_tier", []int{})(d) }, "sockets max 0 must be at least 1"},
		{"punch price 0", sock("punch_xu_per_tier", []int{100, 0, 400}), "sockets punch_xu_per_tier[1] 0 must be positive"},
		{"unsocket 0", sock("unsocket_xu_per_level", 0), "sockets unsocket_xu_per_level 0 must be positive"},

		{"min_quality 0", ref("min_quality", 0), "refine min_quality 0 must be in 1-4"},
		{"min_quality 5", ref("min_quality", 5), "refine min_quality 5 must be in 1-4"},
		{"stones 0", ref("stones", 0), "refine stones 0 must be at least 1"},
		{"negative stones_per_lock", ref("stones_per_lock", -1), "refine stones_per_lock -1 must not be negative"},
		{"negative xu_per_tier", ref("xu_per_tier", -1), "refine xu_per_tier -1 must not be negative"},
		{"negative xu_per_tier_per_lock", ref("xu_per_tier_per_lock", -1), "refine xu_per_tier_per_lock -1 must not be negative"},
		{"missing refine stone", func(d obj) {
			d["stackables"] = slices.Delete(d["stackables"].([]any), 1, 2)
			for _, r := range []string{RankElite, RankBoss} {
				x := d["drops"].(obj)[r].(obj)
				x["stacks"] = x["stacks"].([]any)[:1]
			}
		}, `stackables has no "stone_refine"`},

		{"duplicate gem kind and level", gem(1, "level", 1), `gem "gem_attack_2": attack level 1 is already "gem_attack_1"`},
		{"missing gem", func(d obj) { d["stackables"] = slices.Delete(d["stackables"].([]any), 3+14, 3+15) }, `no defense gem of level 5`},
		{"gem stat mp", gem(5, "stats", obj{"mp": 10}), `gem "gem_hp_1": stat "mp" is not a line stat`},
		{"gem stat 0", gem(5, "stats", obj{StatHP: 0}), `gem "gem_hp_1": stat "hp" 0 must be positive`},
		{"gem without stats", gem(5, "stats", obj{}), `gem "gem_hp_1": stats is empty`},
		{"gem kind", gem(0, "kind", "pierce"), `gem "gem_attack_1": kind "pierce" is unknown`},
		{"gem level 6", gem(4, "level", 6), `gem "gem_attack_5": level 6 must be in 1-5`},
		{"gem level 0", gem(0, "level", 0), `gem "gem_attack_1": level 0 must be in 1-5`},
		{"unknown gem field", gem(0, "bogus", 1), `unknown field "bogus"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validItems()
			tt.mutate(d)
			_, err := ParseItems(itemsJSON(t, d))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want error containing %q, got %v", tt.want, err)
			}
		})
	}
}

// The committed resonance, socket, refine and gem data is the phase 3c spec
// 2.1 tables.
func TestRepoForgeData(t *testing.T) {
	d, err := Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	it := d.Items
	if g := it.Stackables["gem_hp_5"].Gem; g == nil || !maps.Equal(g.Stats, map[string]int{StatHP: 469}) {
		t.Fatalf("gem_hp_5: %+v", g)
	}
	for id, v := range specSlots() {
		a := v.([]any)
		if got, want := it.Resonance.Slots[slotIdx(id)], [2]int{slotIdx(a[0].(string)), slotIdx(a[1].(string))}; got != want {
			t.Errorf("slots %s: %v, want %v", id, got, want)
		}
	}
	for e, a := range specActivator() {
		if it.Resonance.Activator[e] != a {
			t.Errorf("activator %s: %q, want %q", e, it.Resonance.Activator[e], a)
		}
	}
	if it.Sockets.Max != 3 || !slices.Equal(it.Sockets.PunchXuPerTier, []int{100, 200, 400}) || it.Sockets.UnsocketXuPerLevel != 500 {
		t.Fatalf("sockets: %+v", it.Sockets)
	}
	if it.Refine != (Refine{MinQuality: 2, Stones: 1, StonesPerLock: 1, XuPerTier: 20, XuPerTierPerLock: 20}) {
		t.Fatalf("refine: %+v", it.Refine)
	}
	// Spec 2.1 table: level -> attack, HP, defense.
	want := map[int][3]int{1: {4, 86, 1}, 2: {8, 156, 1}, 3: {14, 220, 2}, 4: {22, 320, 3}, 5: {32, 469, 5}}
	for lv, v := range want {
		for _, c := range []struct {
			kind  string
			stats map[string]int
		}{
			{GemAttack, map[string]int{StatPhysAttack: v[0], StatMagicAttack: v[0]}},
			{GemHP, map[string]int{StatHP: v[1]}},
			{GemDefense, map[string]int{StatPhysDefense: v[2], StatMagicDefense: v[2]}},
		} {
			code := fmt.Sprintf("gem_%s_%d", c.kind, lv)
			s := it.Stackables[code]
			if s.Gem == nil || s.Gem.Kind != c.kind || s.Gem.Level != lv || !maps.Equal(s.Gem.Stats, c.stats) || s.Name == "" {
				t.Errorf("%s: %+v", code, s)
			}
		}
	}
	// The gems follow the charm, attack then HP then defense, by level.
	if got := it.StackOrder[3:]; len(got) != 15 || got[0] != "gem_attack_1" || got[5] != "gem_hp_1" || got[14] != "gem_defense_5" {
		t.Fatalf("gem order: %v", got)
	}
	if it.Stackables["gem_attack_1"].Name != "Ngọc Công cấp 1" || it.Stackables["gem_hp_2"].Name != "Ngọc Sinh Lực cấp 2" ||
		it.Stackables["gem_defense_3"].Name != "Ngọc Hộ Thể cấp 3" {
		t.Fatal("gem names")
	}
}
