package combat

import (
	"encoding/csv"
	"os"
	"strconv"
	"testing"
)

// ______________________TEST STATS_____________________

func TestComputeViemLevel10(t *testing.T) {
	s := Compute(false, 10, Points{Str: 60, Vit: 30})
	if s.PhysAttack != 185 || s.MaxHP != 1640 || s.PhysDefense != 88 || s.MaxMP != 280 {
		t.Fatalf("got %+v", s)
	}
}

func TestComputeThuyLevel10(t *testing.T) {
	s := Compute(true, 10, Points{Int: 60, Vit: 30})
	if s.MagicAttack != 185 || s.MaxHP != 1430 || s.MaxMP != 580 {
		t.Fatalf("got %+v", s)
	}
}

func TestComputeLevel1Base(t *testing.T) {
	want := Stats{MaxHP: 500, MaxMP: 100, PhysAttack: 20, MagicAttack: 20,
		PhysDefense: 10, MagicDefense: 10, Accuracy: 100, Evasion: 50, CritRateBP: 500}
	if got := Compute(false, 1, Points{}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestComputeAgi(t *testing.T) {
	base := Compute(false, 1, Points{})
	s := Compute(false, 1, Points{Agi: 100})
	if s.Accuracy-base.Accuracy != 300 || s.Evasion-base.Evasion != 200 || s.CritRateBP-base.CritRateBP != 200 {
		t.Fatalf("got %+v", s)
	}
}

func TestPointsTotal(t *testing.T) {
	if got := (Points{1, 2, 3, 4}).Total(); got != 10 {
		t.Fatalf("got %d", got)
	}
}

// ______________________TEST DAMAGE_____________________

func TestDamageReference(t *testing.T) {
	if got := Damage(185, 88, 10); got != 128 {
		t.Fatalf("Damage(185,88,10) = %d", got)
	}
	if got := Damage(20, 8, 1); got != 14 {
		t.Fatalf("Damage(20,8,1) = %d", got)
	}
}

func TestDamageMinimumOne(t *testing.T) {
	if got := Damage(20, 1_000_000, 1); got != 1 {
		t.Fatalf("got %d", got)
	}
}

func TestSlashBase(t *testing.T) {
	if got := SlashBase(Stats{PhysAttack: 30, MagicAttack: 50}); got != 50 {
		t.Fatalf("got %d", got)
	}
	if got := SlashBase(Stats{PhysAttack: 60, MagicAttack: 50}); got != 60 {
		t.Fatalf("got %d", got)
	}
}

// ______________________TEST EXP_____________________

func TestExpToNextMatchesCSV(t *testing.T) {
	f, err := os.Open("../../docs/balance/exp-table.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 90 { // header + levels 1-89
		t.Fatalf("rows = %d", len(rows))
	}
	for _, r := range rows[1:] {
		level, _ := strconv.Atoi(r[0])
		want, _ := strconv.ParseInt(r[1], 10, 64)
		if got := ExpToNext(level); got != want {
			t.Errorf("ExpToNext(%d) = %d, want %d", level, got, want)
		}
	}
	if got := ExpToNext(MaxLevel); got != 0 {
		t.Fatalf("ExpToNext(90) = %d", got)
	}
}

func TestGainExp(t *testing.T) {
	need1 := ExpToNext(1)
	if l, e := GainExp(1, 0, need1+7); l != 2 || e != 7 {
		t.Fatalf("carry: got %d, %d", l, e)
	}
	if l, e := GainExp(1, 5, 10); l != 1 || e != 15 {
		t.Fatalf("no level-up: got %d, %d", l, e)
	}
	multi := ExpToNext(1) + ExpToNext(2) + ExpToNext(3) + 3
	if l, e := GainExp(1, 0, multi); l != 4 || e != 3 {
		t.Fatalf("several levels: got %d, %d", l, e)
	}
	if l, e := GainExp(89, 0, 1<<50); l != MaxLevel || e != 0 {
		t.Fatalf("cap: got %d, %d", l, e)
	}
	if l, e := GainExp(MaxLevel, 0, 1000); l != MaxLevel || e != 0 {
		t.Fatalf("at max: got %d, %d", l, e)
	}
}

func TestMonsterExp(t *testing.T) {
	if got := MonsterExp(7); got != 70 {
		t.Fatalf("got %d", got)
	}
}
