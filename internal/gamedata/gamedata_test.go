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
  "kinds": [{"id":"a","name":"A","attack_range":1.5,"attack_sec":1.5,"speed":0.8,"aggro_range":4,"leash_range":10,"respawn_sec":8}],
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

func TestParseErrors(t *testing.T) {
	kind := func(id, speed string) string {
		return `{"id":"` + id + `","name":"A","attack_range":1.5,"attack_sec":1.5,"speed":` + speed +
			`,"aggro_range":4,"leash_range":10,"respawn_sec":8}`
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

func mapWithCamp(monster string, min, max int) map[string]*mapdata.Map {
	return map[string]*mapdata.Map{"m": {Camps: []mapdata.Camp{{ID: "c", Monster: monster, MinLevel: min, MaxLevel: max}}}}
}

func TestValidateMissingKindOrLevel(t *testing.T) {
	d, err := Parse([]byte(validJSON))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(mapWithCamp("a", 1, 2)); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if err := d.Validate(mapWithCamp("ghost", 1, 2)); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("missing kind: %v", err)
	}
	if err := d.Validate(mapWithCamp("a", 1, 3)); err == nil || !strings.Contains(err.Error(), "3") {
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
