package combat

import (
	"slices"
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

// ______________________TEST EXP TABLE_____________________

// testTable is a synthetic table: level L needs 100 * L EXP.
func testTable() ExpTable {
	t := make(ExpTable, MaxLevel-1)
	for i := range t {
		t[i] = 100 * int64(i+1)
	}
	return t
}

func TestExpTableToNext(t *testing.T) {
	tb := testTable()
	for _, c := range []struct {
		level int
		want  int64
	}{{1, 100}, {89, 8900}, {MaxLevel, 0}, {0, 0}, {-1, 0}, {MaxLevel + 1, 0}} {
		if got := tb.ToNext(c.level); got != c.want {
			t.Errorf("ToNext(%d) = %d, want %d", c.level, got, c.want)
		}
	}
	if got := tb[:3].ToNext(4); got != 0 {
		t.Fatalf("ToNext past a short table = %d, want 0", got)
	}
}

func TestExpTableGain(t *testing.T) {
	tb := testTable()
	if l, e := tb.Gain(1, 0, 100+7); l != 2 || e != 7 {
		t.Fatalf("carry: got %d, %d", l, e)
	}
	if l, e := tb.Gain(1, 5, 10); l != 1 || e != 15 {
		t.Fatalf("no level-up: got %d, %d", l, e)
	}
	if l, e := tb.Gain(1, 0, 100+200+3); l != 3 || e != 3 {
		t.Fatalf("two levels at once: got %d, %d", l, e)
	}
	if l, e := tb.Gain(89, 0, 1<<50); l != MaxLevel || e != 0 {
		t.Fatalf("cap: got %d, %d", l, e)
	}
	if l, e := tb.Gain(MaxLevel, 0, 1000); l != MaxLevel || e != 0 {
		t.Fatalf("at max: got %d, %d", l, e)
	}
	// A dev character above its level's need levels up on the next gain.
	if l, e := tb.Gain(2, 450, 1); l != 3 || e != 251 {
		t.Fatalf("over the need: got %d, %d", l, e)
	}
	if l, e := tb[:2].Gain(3, 0, 1000); l != 3 || e != 1000 {
		t.Fatalf("past a short table: got %d, %d, want the EXP kept at level 3", l, e)
	}
}

func TestExpEligible(t *testing.T) {
	for _, c := range []struct {
		player, monster int
		want            bool
	}{{20, 10, true}, {10, 20, true}, {10, 10, true}, {21, 10, false}, {10, 21, false}, {12, 1, false}, {11, 1, true}} {
		if got := ExpEligible(c.player, c.monster); got != c.want {
			t.Errorf("ExpEligible(%d, %d) = %v, want %v", c.player, c.monster, got, c.want)
		}
	}
}

// ______________________TEST EXP SHARE_____________________

func TestExpShare(t *testing.T) {
	for _, c := range []struct {
		total      int64
		maxHP, dmg int
		want       int64
	}{
		{100, 1000, 100, 10}, // the design example
		{10, 56, 14, 2},      // 2.5 rounds down
		{10, 56, 55, 9},      // 9.8 rounds down
		{10, 56, 56, 10},     // all of it
		{10, 56, 200, 10},    // capped at maxHP
		{10, 56, 0, 0},
		{10, 56, -3, 0},
		{10, 0, 14, 0},
		{0, 56, 14, 0},
		{900, 1 << 30, 1 << 30, 900}, // no overflow
	} {
		if got := ExpShare(c.total, c.maxHP, c.dmg); got != c.want {
			t.Errorf("ExpShare(%d, %d, %d) = %d, want %d", c.total, c.maxHP, c.dmg, got, c.want)
		}
	}
}

// Paying the share of the summed damage less what was paid loses nothing: a
// monster of 56 HP and 10 EXP slashed for 14 four times pays 2, 3, 2, 3.
func TestExpShareCumulative(t *testing.T) {
	var paid int64
	var got []int64
	for sum := 14; sum <= 56; sum += 14 {
		share := ExpShare(10, 56, sum)
		got = append(got, share-paid)
		paid = share
	}
	if !slices.Equal(got, []int64{2, 3, 2, 3}) || paid != 10 {
		t.Fatalf("payments %v total %d, want [2 3 2 3] and 10", got, paid)
	}
}

// ______________________TEST RANGED_____________________

func TestMonsterRangedDamage(t *testing.T) {
	for _, c := range []struct{ base, def, lvl, want int }{
		{30, 0, 1, 15},
		{30, 8, 1, 10},
		{30, 16, 1, 8},
		{1, 0, 1, 1},    // Damage 1 still gives 1
		{2, 1000, 1, 1}, // floors to 0, clamped to 1
	} {
		if got := MonsterRangedDamage(c.base, c.def, c.lvl); got != c.want {
			t.Errorf("MonsterRangedDamage(%d, %d, %d) = %d, want %d", c.base, c.def, c.lvl, got, c.want)
		}
	}
}
