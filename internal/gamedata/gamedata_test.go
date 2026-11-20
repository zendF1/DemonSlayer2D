package gamedata

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
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
  "kinds": [{"id":"a","name":"A","attack_range":1.5,"attack_sec":1.5,"speed":0.8,"detect_range":5,"chase_range":10,"shoot_sec":2,"shoot_windup_sec":0.5,"region_margin":3,"chase_speed":1.2,"safe_wait_sec":5,"margin_grace_sec":2,"grudge_sec":10,"respawn_sec":8,"melee_windup_ms":200,"shot_speed":15}],
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
		MarginGraceSec: 2, GrudgeSec: 10, RespawnSec: 8, MeleeWindupMS: 200, ShotSpeed: 15}
	if k := d.Kinds["a"]; k != want {
		t.Fatalf("kind %+v, want %+v", k, want)
	}
}

func kindWith(field, value string) string {
	fields := map[string]string{"attack_range": "1.5", "attack_sec": "1.5", "speed": "0.8", "detect_range": "5",
		"chase_range": "10", "shoot_sec": "2", "shoot_windup_sec": "0.5", "region_margin": "3", "chase_speed": "1.2", "safe_wait_sec": "5",
		"margin_grace_sec": "2", "grudge_sec": "10", "respawn_sec": "8", "melee_windup_ms": "200", "shot_speed": "15"}
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
			`,"detect_range":5,"chase_range":10,"shoot_sec":2,"shoot_windup_sec":0.5,"region_margin":3,"chase_speed":1.2,"safe_wait_sec":5,"margin_grace_sec":2,"grudge_sec":10,"respawn_sec":8,"melee_windup_ms":200,"shot_speed":15}`
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
