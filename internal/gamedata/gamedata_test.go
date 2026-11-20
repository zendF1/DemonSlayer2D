package gamedata

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"demonslayer2d/internal/mapdata"
)

const validJSON = `{
  "kinds": [{"id":"a","name":"A","attack_range":1.5,"attack_sec":1.5,"speed":0.8,"detect_range":5,"chase_range":10,"shoot_sec":2,"shoot_windup_sec":0.5,"region_margin":3,"respawn_sec":8}],
  "levels": [{"level":1,"hp":56,"attack":30,"defense":8},{"level":2,"hp":104,"attack":39,"defense":16}]
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
	if l := d.Levels[2]; l != (Level{Level: 2, HP: 104, Attack: 39, Defense: 16}) {
		t.Fatalf("level: %+v", l)
	}
}

func TestParseKindFields(t *testing.T) {
	d, err := Parse([]byte(validJSON))
	if err != nil {
		t.Fatal(err)
	}
	want := Kind{ID: "a", Name: "A", AttackRange: 1.5, AttackSec: 1.5, Speed: 0.8, DetectRange: 5, ChaseRange: 10,
		ShootSec: 2, ShootWindupSec: 0.5, RegionMargin: 3, RespawnSec: 8}
	if k := d.Kinds["a"]; k != want {
		t.Fatalf("kind %+v, want %+v", k, want)
	}
}

func kindWith(field, value string) string {
	fields := map[string]string{"attack_range": "1.5", "attack_sec": "1.5", "speed": "0.8", "detect_range": "5",
		"chase_range": "10", "shoot_sec": "2", "shoot_windup_sec": "0.5", "region_margin": "3", "respawn_sec": "8"}
	fields[field] = value
	var parts []string
	for k, v := range fields {
		parts = append(parts, `"`+k+`":`+v)
	}
	return `{"kinds":[{"id":"a","name":"A",` + strings.Join(parts, ",") +
		`}],"levels":[{"level":1,"hp":56,"attack":30,"defense":8}]}`
}

func TestParseKindFieldValues(t *testing.T) {
	for _, f := range []string{"attack_range", "attack_sec", "speed", "detect_range", "chase_range", "shoot_sec",
		"shoot_windup_sec", "respawn_sec"} {
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
			`,"detect_range":5,"chase_range":10,"shoot_sec":2,"shoot_windup_sec":0.5,"region_margin":3,"respawn_sec":8}`
	}
	lvl := `{"level":1,"hp":56,"attack":30,"defense":8}`
	doc := func(kinds, levels string) string { return `{"kinds":[` + kinds + `],"levels":[` + levels + `]}` }
	tests := []struct{ name, in string }{
		{"duplicate kind", doc(kind("a", "0.8")+","+kind("a", "0.8"), lvl)},
		{"zero speed", doc(kind("a", "0"), lvl)},
		{"empty kind id", doc(kind("", "0.8"), lvl)},
		{"duplicate level", doc(kind("a", "0.8"), lvl+","+lvl)},
		{"zero hp", doc(kind("a", "0.8"), `{"level":1,"hp":0,"attack":30,"defense":8}`)},
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

func TestRepoDataMatchesScript(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	got, err := exec.Command(py, "../../docs/balance/monster_stats.py", "--print-json").Output()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../data/monsters.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("data/monsters.json differs from monster_stats.py output; run it with --write-json")
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
