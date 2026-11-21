package combat

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// ______________________TEST STATS_____________________

func TestComputeViemLevel10(t *testing.T) {
	s := Compute(false, 10, Points{Str: 60, Vit: 30}, Bonus{}, Passive{})
	if s.PhysAttack != 185 || s.MaxHP != 1640 || s.PhysDefense != 88 || s.MaxMP != 280 {
		t.Fatalf("got %+v", s)
	}
}

func TestComputeThuyLevel10(t *testing.T) {
	s := Compute(true, 10, Points{Int: 60, Vit: 30}, Bonus{}, Passive{})
	if s.MagicAttack != 185 || s.MaxHP != 1430 || s.MaxMP != 580 {
		t.Fatalf("got %+v", s)
	}
}

func TestComputeLevel1Base(t *testing.T) {
	want := Stats{MaxHP: 500, MaxMP: 100, PhysAttack: 20, MagicAttack: 20,
		PhysDefense: 10, MagicDefense: 10, Accuracy: 100, Evasion: 50, CritRateBP: 500}
	if got := Compute(false, 1, Points{}, Bonus{}, Passive{}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestComputeAgi(t *testing.T) {
	base := Compute(false, 1, Points{}, Bonus{}, Passive{})
	s := Compute(false, 1, Points{Agi: 100}, Bonus{}, Passive{})
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

// ______________________TEST PASSIVE_____________________

func TestComputeZeroPassiveKeepsPhase1Stats(t *testing.T) {
	for _, magic := range []bool{false, true} {
		for _, level := range []int{1, 10, 50, 90} {
			for _, pts := range []Points{{}, {Str: 60, Vit: 30}, {Int: 45, Agi: 20}, {Str: 5, Agi: 7, Vit: 9, Int: 11}} {
				l := level - 1
				hpA, hpB := 60, 20
				if magic {
					hpA, hpB = 50, 16
				}
				def := 10 + 2*l + 2*pts.Vit
				want := Stats{
					MaxHP: 500 + hpA*l + hpB*pts.Vit, MaxMP: 100 + 20*l + 5*pts.Int,
					PhysAttack: 20 + 5*l + 2*pts.Str, MagicAttack: 20 + 5*l + 2*pts.Int,
					PhysDefense: def, MagicDefense: def,
					Accuracy: 100 + 5*l + 3*pts.Agi, Evasion: 50 + 5*l + 2*pts.Agi,
					CritRateBP: 500 + 2*pts.Agi,
				}
				if got := Compute(magic, level, pts, Bonus{}, Passive{}); got != want {
					t.Fatalf("magic=%v level=%d pts=%+v: got %+v, want %+v", magic, level, pts, got, want)
				}
			}
		}
	}
}

func TestComputeNhamPassive(t *testing.T) {
	pts := Points{Str: 60, Vit: 30}
	base := Compute(false, 10, pts, Bonus{}, Passive{})
	s := Compute(false, 10, pts, Bonus{}, Passive{MaxHPPct: 20, PhysDefPct: 20})
	if base.MaxHP != 1640 || base.PhysDefense != 88 {
		t.Fatalf("base %+v", base)
	}
	if s.MaxHP != 1640+328 || s.PhysDefense != 88+17 {
		t.Fatalf("got hp %d def %d", s.MaxHP, s.PhysDefense)
	}
	if s.MagicDefense != base.MagicDefense || s.PhysAttack != base.PhysAttack || s.MaxMP != base.MaxMP {
		t.Fatalf("other stats changed: %+v", s)
	}
}

func TestComputePassiveFloors(t *testing.T) {
	// Evasion 50 + 5*l + 2*agi = 57 with l=1, agi=1.
	s := Compute(false, 2, Points{Agi: 1}, Bonus{}, Passive{})
	if s.Evasion != 57 {
		t.Fatalf("setup: evasion %d", s.Evasion)
	}
	if got := Compute(false, 2, Points{Agi: 1}, Bonus{}, Passive{EvasionPct: 3}).Evasion; got != 58 {
		t.Fatalf("57 with 3%% = %d, want 58", got)
	}
}

func TestComputePassiveAllFields(t *testing.T) {
	pts := Points{Int: 40, Agi: 10, Vit: 10}
	b := Compute(true, 20, pts, Bonus{}, Passive{})
	s := Compute(true, 20, pts, Bonus{}, Passive{MagicDefPct: 10, AccuracyPct: 10, MagicAttackPct: 10, EvasionPct: 10})
	if s.MagicDefense != b.MagicDefense+b.MagicDefense*10/100 ||
		s.Accuracy != b.Accuracy+b.Accuracy*10/100 ||
		s.MagicAttack != b.MagicAttack+b.MagicAttack*10/100 ||
		s.Evasion != b.Evasion+b.Evasion*10/100 {
		t.Fatalf("got %+v from %+v", s, b)
	}
	if s.PhysDefense != b.PhysDefense || s.MaxHP != b.MaxHP || s.PhysAttack != b.PhysAttack {
		t.Fatalf("untargeted stats changed: %+v", s)
	}
}

func TestComputePassiveCritBP(t *testing.T) {
	base := Compute(false, 5, Points{Agi: 3}, Bonus{}, Passive{})
	s := Compute(false, 5, Points{Agi: 3}, Bonus{}, Passive{CritBP: 1000})
	if s.CritRateBP != base.CritRateBP+1000 {
		t.Fatalf("got %d, base %d", s.CritRateBP, base.CritRateBP)
	}
}

func TestPassiveAdd(t *testing.T) {
	a := Passive{MaxHPPct: 5, CritBP: 100, PhysDefPct: 1}
	b := Passive{MaxHPPct: 15, CritBP: 400, MagicDefPct: 2, EvasionPct: 3, AccuracyPct: 4, MagicAttackPct: 6}
	want := Passive{PhysDefPct: 1, MagicDefPct: 2, MaxHPPct: 20, EvasionPct: 3, AccuracyPct: 4, MagicAttackPct: 6, CritBP: 500}
	if got := a.Add(b); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// ______________________TEST SKILL BASE_____________________

func TestSkillBase(t *testing.T) {
	s := Stats{PhysAttack: 285, MagicAttack: 120}
	if got := SkillBase(s, false, 150); got != 427 {
		t.Fatalf("phys = %d, want 427", got)
	}
	if got := SkillBase(s, true, 150); got != 180 {
		t.Fatalf("magic = %d, want 180", got)
	}
}

// ______________________TEST GEAR BONUS_____________________

// phase2Stats is Compute as it was before gear: the base block, then the
// passives (phase 2 spec 3.2).
func phase2Stats(magic bool, level int, p Points, pas Passive) Stats {
	l := level - 1
	hpA, hpB := 60, 20
	if magic {
		hpA, hpB = 50, 16
	}
	def := 10 + 2*l + 2*p.Vit
	hp, matk := 500+hpA*l+hpB*p.Vit, 20+5*l+2*p.Int
	acc, eva := 100+5*l+3*p.Agi, 50+5*l+2*p.Agi
	return Stats{
		MaxHP: hp + hp*pas.MaxHPPct/100, MaxMP: 100 + 20*l + 5*p.Int,
		PhysAttack: 20 + 5*l + 2*p.Str, MagicAttack: matk + matk*pas.MagicAttackPct/100,
		PhysDefense: def + def*pas.PhysDefPct/100, MagicDefense: def + def*pas.MagicDefPct/100,
		Accuracy: acc + acc*pas.AccuracyPct/100, Evasion: eva + eva*pas.EvasionPct/100,
		CritRateBP: 500 + 2*p.Agi + pas.CritBP,
	}
}

func TestComputeZeroGearKeepsStats(t *testing.T) {
	passives := []Passive{{}, {MaxHPPct: 20, PhysDefPct: 20}, {MagicDefPct: 7, AccuracyPct: 3, MagicAttackPct: 15, EvasionPct: 9, CritBP: 250}}
	for _, magic := range []bool{false, true} {
		for _, level := range []int{1, 10, 31, 90} {
			for _, pts := range []Points{{}, {Str: 60, Vit: 30}, {Int: 45, Agi: 20}, {Str: 5, Agi: 7, Vit: 9, Int: 11}} {
				for _, pas := range passives {
					want := phase2Stats(magic, level, pts, pas)
					if got := Compute(magic, level, pts, Bonus{}, pas); got != want {
						t.Fatalf("magic=%v level=%d pts=%+v pas=%+v: got %+v, want %+v", magic, level, pts, pas, got, want)
					}
				}
			}
		}
	}
}

// Phase 3a spec 4.2: Viêm level 10, 60 STR/30 VIT with the Lữ Hành sword.
func TestComputeGearViemSword(t *testing.T) {
	s := Compute(false, 10, Points{Str: 60, Vit: 30}, Bonus{PhysAttack: 25}, Passive{})
	if s.PhysAttack != 210 || s.MaxHP != 1640 {
		t.Fatalf("got attack %d hp %d, want 210 1640", s.PhysAttack, s.MaxHP)
	}
}

// Each gear field lands on its own stat and nothing else.
func TestComputeGearFields(t *testing.T) {
	pts := Points{Str: 5, Agi: 7, Vit: 9, Int: 11}
	base := Compute(true, 20, pts, Bonus{}, Passive{})
	s := Compute(true, 20, pts, Bonus{HP: 1, PhysAttack: 20, MagicAttack: 300, PhysDefense: 4000, MagicDefense: 50000}, Passive{})
	want := base
	want.MaxHP += 1
	want.PhysAttack += 20
	want.MagicAttack += 300
	want.PhysDefense += 4000
	want.MagicDefense += 50000
	if s != want {
		t.Fatalf("got %+v, want %+v", s, want)
	}
}

// Passives apply to the gear too (phase 3a spec 4.2): Nham's 20% HP passive
// with +200 HP of gear gives 240 more HP than without the gear.
func TestComputeGearBeforePassives(t *testing.T) {
	pts := Points{Str: 60, Vit: 30}
	pas := Passive{MaxHPPct: 20, PhysDefPct: 20, MagicDefPct: 10, MagicAttackPct: 10}
	without := Compute(false, 10, pts, Bonus{}, pas)
	with := Compute(false, 10, pts, Bonus{HP: 200, PhysDefense: 10, MagicDefense: 10, MagicAttack: 10}, pas)
	if with.MaxHP != (1640+200)*120/100 || with.MaxHP-without.MaxHP != 240 {
		t.Fatalf("max hp %d (without gear %d), want %d, 240 more", with.MaxHP, without.MaxHP, (1640+200)*120/100)
	}
	if with.PhysDefense != (88+10)*120/100 || with.MagicDefense != (88+10)*110/100 || with.MagicAttack != (65+10)*110/100 {
		t.Fatalf("got %+v", with)
	}
}

func TestBonusAdd(t *testing.T) {
	a := Bonus{HP: 1, PhysAttack: 2, MagicAttack: 3, PhysDefense: 4, MagicDefense: 5}
	b := Bonus{HP: 10, PhysAttack: 20, MagicAttack: 30, PhysDefense: 40, MagicDefense: 50}
	if got := a.Add(b); got != (Bonus{HP: 11, PhysAttack: 22, MagicAttack: 33, PhysDefense: 44, MagicDefense: 55}) {
		t.Fatalf("got %+v", got)
	}
}

// ______________________TEST PVP_____________________

func TestPvPAllowed(t *testing.T) {
	for _, c := range []struct{ a, b, party, want bool }{
		{false, false, false, false}, {true, false, false, true},
		{false, true, false, true}, {true, true, false, true},
		{false, false, true, false}, {true, false, true, false},
		{false, true, true, false}, {true, true, true, false},
	} {
		if got := PvPAllowed(c.a, c.b, c.party); got != c.want {
			t.Errorf("PvPAllowed(%v,%v,%v) = %v, want %v", c.a, c.b, c.party, got, c.want)
		}
	}
}

func TestPkExpLoss(t *testing.T) {
	for _, c := range []struct{ in, want int64 }{{64000, 3200}, {130, 6}, {0, 0}, {-5, 0}} {
		if got := PkExpLoss(c.in); got != c.want {
			t.Errorf("PkExpLoss(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPvPDamageReference(t *testing.T) {
	viem := Compute(false, 10, Points{Str: 60, Vit: 30}, Bonus{}, Passive{})
	slash := PvPDamage(SlashBase(viem), viem.PhysDefense, 10)
	if slash != 128 {
		t.Fatalf("slash = %d, want 128", slash)
	}
	bth := PvPDamage(SkillBase(viem, false, 150), viem.PhysDefense, 10)
	if bth != 192 {
		t.Fatalf("Bat Tri Hoa = %d, want 192", bth)
	}
	if 9*bth < viem.MaxHP || 8*bth >= viem.MaxHP {
		t.Fatalf("9 hits %d, 8 hits %d vs %d HP", 9*bth, 8*bth, viem.MaxHP)
	}
	thuy := Compute(true, 10, Points{Int: 60, Vit: 30}, Bonus{}, Passive{})
	if thuy.MaxHP != 1430 || thuy.PhysDefense != 88 || thuy.MagicDefense != 88 {
		t.Fatalf("thuy %+v", thuy)
	}
	if 8*PvPDamage(SkillBase(viem, false, 150), thuy.PhysDefense, 10) < thuy.MaxHP {
		t.Fatal("eight Bat Tri Hoa should kill Thuy")
	}
}

func TestPvPDamageMinimumOne(t *testing.T) {
	if got := PvPDamage(1, 100000, 1); got != 1 {
		t.Fatalf("got %d", got)
	}
	if got := PvPDamage(0, 0, 1); got != 1 {
		t.Fatalf("got %d", got)
	}
}

// ______________________TEST PARTY BONUS_____________________

func TestPartyBonusExample(t *testing.T) {
	var carry, total int64
	for _, g := range []int64{1, 2, 1, 2} {
		var b int64
		b, carry = PartyBonus(carry, g)
		total += b
	}
	if total != 0 || carry != 30 {
		t.Fatalf("after 1,2,1,2: bonus %d carry %d, want 0 and 30", total, carry)
	}
	b, carry := PartyBonus(carry, 143)
	if b != 7 || carry != 45 {
		t.Fatalf("143 from 30: bonus %d carry %d, want 7 and 45", b, carry)
	}
}

func TestPartyBonusSumsExactly(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var carry, bonus, gains int64
	for i := 0; i < 10000; i++ {
		g := r.Int64N(5000)
		var b int64
		b, carry = PartyBonus(carry, g)
		bonus += b
		gains += g
		if want := PartyBonusPct * gains / 100; bonus != want {
			t.Fatalf("step %d: summed bonus %d, want %d", i, bonus, want)
		}
	}
}

// ______________________TEST SCALE PCT_____________________

// ScalePct rounds up in integers and leaves a value alone at 100 (phase 4d
// spec 2.4).
func TestScalePct(t *testing.T) {
	for _, c := range []struct{ v, pct, want int }{
		{1000, 80, 800}, {1001, 80, 801}, {7, 100, 7}, {10, 120, 12}, {13280, 80, 10624}, {1398, 120, 1678}, {0, 80, 0},
	} {
		if got := ScalePct(c.v, c.pct); got != c.want {
			t.Fatalf("ScalePct(%d, %d) = %d, want %d", c.v, c.pct, got, c.want)
		}
	}
}
