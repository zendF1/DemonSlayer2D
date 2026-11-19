package zone

import (
	"slices"
	"strings"
	"testing"
	"time"

	"demonslayer2d/internal/pb"
)

// newFightZone builds a zone on a 60x60 open grid with the safe rect (0,0,10,10)
// and a single monster of the given level standing at home.
func newFightZone(t *testing.T, level int, home Vec2) (*Zone, *monster) {
	t.Helper()
	cfg := testConfig(10)
	area := Area{
		Grid:    openGrid(60, 60),
		Safe:    []Rect{{X: 0, Y: 0, W: 10, H: 10}},
		Respawn: Vec2{5.5, 5.5},
		Camps:   []Camp{{ID: "c1", Tiles: []Vec2{home}, Count: 1, MinLevel: level, MaxLevel: level, Kind: testKind}},
		Levels:  testLevels,
	}
	z := New(cfg, area, &fakeSaver{}, quietLog())
	if len(z.monsters) != 1 {
		t.Fatalf("monsters = %d, want 1", len(z.monsters))
	}
	for _, m := range z.monsters {
		return z, m
	}
	panic("unreachable")
}

func joinHP(t *testing.T, z *Zone, id uint64, x, y float32, hp int) *fakeOut {
	t.Helper()
	in := info(id, x, y)
	in.HP = hp
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func ticksIn(envs []*pb.Envelope) []*pb.WorldTick {
	var wts []*pb.WorldTick
	for _, e := range envs {
		if wt := e.GetWorldTick(); wt != nil {
			wts = append(wts, wt)
		}
	}
	return wts
}

func hitsIn(envs []*pb.Envelope) []*pb.Hit {
	var hs []*pb.Hit
	for _, wt := range ticksIn(envs) {
		hs = append(hs, wt.Hits...)
	}
	return hs
}

func deathsIn(envs []*pb.Envelope) []uint64 {
	var ids []uint64
	for _, wt := range ticksIn(envs) {
		for _, d := range wt.Deaths {
			ids = append(ids, d.Id)
		}
	}
	return ids
}

func hpChangesIn(envs []*pb.Envelope) []*pb.HpChange {
	var cs []*pb.HpChange
	for _, wt := range ticksIn(envs) {
		cs = append(cs, wt.HpChanges...)
	}
	return cs
}

// ______________________TEST MONSTER AI_____________________

func TestMonsterAggroWithinRange(t *testing.T) {
	for _, c := range []struct {
		dx    float32
		chase bool
	}{{3.9, true}, {4.1, false}} {
		z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
		joinNow(t, z, 1, 30.5+c.dx, 30.5)
		z.advance(time.Now())
		if got := m.target == 1 && m.state == chase; got != c.chase {
			t.Fatalf("player %.1f tiles away: target %d state %v, want chase=%v", c.dx, m.target, m.state, c.chase)
		}
		if !c.chase && (m.target != 0 || m.state != idle) {
			t.Fatalf("player out of aggro range: target %d state %v, want idle with no target", m.target, m.state)
		}
	}

	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	joinNow(t, z, 1, 30.5, 33.5)
	joinNow(t, z, 2, 28.5, 30.5)
	joinNow(t, z, 3, 33.0, 30.5)
	z.advance(time.Now())
	if m.target != 2 {
		t.Fatalf("target %d, want the nearest player 2", m.target)
	}
}

func TestMonsterIgnoresPlayerInSafeZone(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{11.5, 5.5})
	a := joinNow(t, z, 1, 9.5, 5.5) // tile 9 is the safe rect's last column
	now := time.Now()
	for i := 0; i < 50; i++ {
		z.advance(now)
		if m.target != 0 || m.state != idle {
			t.Fatalf("tick %d: target %d state %v, want an idle monster", i, m.target, m.state)
		}
	}
	if hs := hitsIn(a.take()); len(hs) != 0 || z.players[1].hp != 500 {
		t.Fatalf("a player in the safe zone must not be hit, hits %v hp %d", hs, z.players[1].hp)
	}
}

func TestMonsterAttacksOnInterval(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 31.5, 30.5)
	v := joinNow(t, z, 2, 30.5, 36.5) // in view, out of aggro range
	now := time.Now()
	var hitTicks []int
	for i := 1; i <= 31; i++ {
		z.advance(now)
		ha, hv := hitsIn(a.take()), hitsIn(v.take())
		if len(ha) != len(hv) {
			t.Fatalf("tick %d: target got %d hits, viewer %d", i, len(ha), len(hv))
		}
		for j, h := range ha {
			// Level-1 monster (attack 30) on a level-1 player (defense 10): 30*(1-10/30) = 20.
			want := 500 - 20*(len(hitTicks)+1)
			if h.AttackerId != m.id || h.TargetId != 1 || h.Damage != 20 || h.HpAfter != int32(want) {
				t.Fatalf("tick %d: hit %v, want %d hits 1 for 20 leaving %d", i, h, m.id, want)
			}
			if hv[j].String() != h.String() {
				t.Fatalf("tick %d: viewer got %v, target %v", i, hv[j], h)
			}
			hitTicks = append(hitTicks, i)
		}
	}
	if !slices.Equal(hitTicks, []int{1, 16, 31}) {
		t.Fatalf("hits on ticks %v, want 1, 16 and 31 (every 15 ticks)", hitTicks)
	}
	if hp := z.players[1].hp; hp != 440 {
		t.Fatalf("hp = %d, want 440", hp)
	}
	if m.pos != (Vec2{30.5, 30.5}) {
		t.Fatalf("a monster within 1.3 tiles must not move, at %v", m.pos)
	}
}

func TestMonsterLeashesAndHeals(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home)
	m.hp = 10
	a := joinNow(t, z, 1, 33.5, 30.5)
	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{1, 0}}) // lure it east, faster than it walks
	step := testKind.Speed * 4 / 10
	now := time.Now()
	var far float64
	returned, healed := false, false
	for i := 0; i < 200; i++ {
		z.advance(now)
		far = max(far, dist(m.pos, home))
		for _, c := range hpChangesIn(a.take()) {
			if c.Id == m.id && c.Hp == 56 && c.MaxHp == 56 {
				healed = true
			}
		}
		if m.state == returning {
			if m.target != 0 || len(m.attackers) != 0 {
				t.Fatalf("tick %d: a returning monster keeps target %d attackers %v", i, m.target, m.attackers)
			}
			if !returned {
				z.move(moveCmd{id: 1, seq: 2, dir: Vec2{}})
				returned = true
			}
		}
		if returned && m.state == idle {
			break
		}
	}
	if !returned || m.state != idle {
		t.Fatalf("returned=%v state %v, want back home and idle", returned, m.state)
	}
	if far <= 10 || far > 10+step+1e-3 {
		t.Fatalf("farthest from home %.3f, want just beyond the leash range 10", far)
	}
	if m.pos != home || m.hp != 56 || !healed {
		t.Fatalf("at %v hp %d healed event %v, want home with full HP and an HpChange", m.pos, m.hp, healed)
	}
	if m.nextWander <= z.tickNo {
		t.Fatalf("nextWander %d, want a fresh one after tick %d", m.nextWander, z.tickNo)
	}
}

func TestReturningMonsterStuckTeleportsHome(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home)
	rows := make([]string, 60)
	for y := range rows {
		r := []byte(strings.Repeat(".", 60))
		if y >= 20 && y <= 40 {
			r[33] = '#' // between it and home, too long to slide around
		}
		rows[y] = string(r)
	}
	z.area.Grid = gridFrom(rows...)
	z.mgrid = z.area.monsterGrid()
	m.pos, m.hp, m.state = Vec2{35.5, 30.5}, 10, returning
	z.updateCell(&m.entity)
	v := joinNow(t, z, 1, 36.5, 36.5) // in view, out of aggro range of home
	now := time.Now()
	idleAt := 0
	var snap *pb.EntityMove
	var heal *pb.HpChange
	for i := 1; i <= 60 && idleAt == 0; i++ {
		z.advance(now)
		envs := v.take()
		if mv := moveFor(lastTick(envs), m.id); mv != nil && mv.Snap {
			snap = mv
		}
		for _, c := range hpChangesIn(envs) {
			if c.Id == m.id {
				heal = c
			}
		}
		if m.state == idle {
			idleAt = i
		}
	}
	// About 4 ticks to reach the wall, then 20 ticks (2 s) stuck.
	if idleAt < 20 || idleAt > 30 {
		t.Fatalf("idle at tick %d, want teleported home after 2 s stuck", idleAt)
	}
	if m.pos != home || m.hp != m.maxHP || m.snap {
		t.Fatalf("at %v hp %d/%d snap %v, want home, full HP and snap cleared after the tick", m.pos, m.hp, m.maxHP, m.snap)
	}
	if snap == nil || snap.X != home.X || snap.Y != home.Y {
		t.Fatalf("viewer move %v, want a snap to home", snap)
	}
	if heal == nil || heal.Hp != int32(m.maxHP) {
		t.Fatalf("viewer HpChange %v, want full HP", heal)
	}
}

func TestMonsterKeepsTargetWhenOthersPass(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 31.5, 30.5)
	z.advance(time.Now())
	if m.target != 1 {
		t.Fatalf("target %d, want 1", m.target)
	}
	b := joinNow(t, z, 2, 27.5, 30.8)
	z.move(moveCmd{id: 2, seq: 1, dir: Vec2{1, 0}}) // walks right past the monster
	now := time.Now()
	for i := 0; i < 20; i++ {
		z.advance(now)
		if m.target != 1 {
			t.Fatalf("tick %d: target switched to %d", i, m.target)
		}
		for _, h := range hitsIn(b.take()) {
			if h.TargetId != 1 {
				t.Fatalf("tick %d: hit %v, want only the first target hit", i, h)
			}
		}
	}
	if len(hitsIn(a.take())) == 0 {
		t.Fatal("the first target should keep being hit")
	}
}

func TestMonsterSwitchesToAttackerWhenTargetDies(t *testing.T) {
	for _, c := range []struct {
		name     string
		attacker bool // B (3.5 tiles away) has hit the monster
		near     bool // C (2.1 tiles away) stands within aggro range
		target   uint64
		state    monsterState
	}{
		{"attacker before nearer player", true, true, 2, chase},
		{"nearest in aggro range", false, true, 3, chase},
		{"nobody left", false, false, 0, returning},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
			m.pos = Vec2{31, 30.5} // off home, so a return takes more than a tick
			joinNow(t, z, 1, 31.5, 30.5)
			z.advance(time.Now())
			if m.target != 1 {
				t.Fatalf("target %d, want 1", m.target)
			}
			if c.attacker {
				joinNow(t, z, 2, 30.5, 34.0)
				m.attackers = map[uint64]struct{}{2: {}}
			}
			if c.near {
				joinNow(t, z, 3, 30.5, 28.5)
			}
			z.damagePlayer(z.players[1], m.id, 10000)
			z.advance(time.Now())
			if m.target != c.target || m.state != c.state {
				t.Fatalf("target %d state %v, want %d %v", m.target, m.state, c.target, c.state)
			}
		})
	}
}

func TestPlayerDiesAtZeroHP(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinHP(t, z, 1, 31.5, 30.5, 20)
	v := joinNow(t, z, 2, 30.5, 36.5)
	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{0, 1}})
	z.advance(time.Now())
	p := z.players[1]
	if !p.dead || p.hp != 0 || p.dir != (Vec2{}) {
		t.Fatalf("hp %d dead %v dir %v, want dead at 0 HP, standing still", p.hp, p.dead, p.dir)
	}
	if want := z.tickNo + z.ticks(60); p.deadUntil != want {
		t.Fatalf("deadUntil %d, want %d", p.deadUntil, want)
	}
	ea, ev := a.take(), v.take()
	if hs := hitsIn(ea); len(hs) != 1 || hs[0].HpAfter != 0 || hs[0].AttackerId != m.id {
		t.Fatalf("hits %v, want one hit leaving 0 HP", hs)
	}
	if !slices.Equal(deathsIn(ea), []uint64{1}) || !slices.Equal(deathsIn(ev), []uint64{1}) {
		t.Fatalf("deaths: player %v viewer %v, want [1] for both", deathsIn(ea), deathsIn(ev))
	}

	pos := p.pos
	z.move(moveCmd{id: 1, seq: 2, dir: Vec2{1, 0}})
	now := time.Now()
	for i := 0; i < 20; i++ {
		z.advance(now)
		envs := a.take()
		if wt := lastTick(envs); wt.AckInputSeq != 2 {
			t.Fatalf("ack %d, want 2: a dead player's input is still acked", wt.AckInputSeq)
		}
		if hs := hitsIn(envs); len(hs) != 0 {
			t.Fatalf("a dead player was hit again: %v", hs)
		}
	}
	if p.pos != pos || p.dir != (Vec2{}) {
		t.Fatalf("dead player at %v dir %v, want still at %v", p.pos, p.dir, pos)
	}
	if m.target == 1 {
		t.Fatal("the monster must drop a dead target")
	}
}

func TestEventsOnlyForVisible(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinHP(t, z, 1, 31.5, 30.5, 20)
	f := joinNow(t, z, 2, 5.5, 5.5) // safe zone, three cells away
	z.advance(time.Now())
	if len(hitsIn(a.take())) != 1 {
		t.Fatal("the target must see its own hit")
	}
	ef := f.take()
	if hs, ds := hitsIn(ef), deathsIn(ef); len(hs) != 0 || len(ds) != 0 {
		t.Fatalf("a far viewer got hits %v deaths %v", hs, ds)
	}
}
