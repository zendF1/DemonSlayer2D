package zone

import (
	"slices"
	"strings"
	"testing"
	"time"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
)

// newFightZone builds a zone on a 60x60 open grid with the safe rect (0,0,10,10)
// and a single testKind monster of the given level standing at home.
func newFightZone(t *testing.T, level int, home Vec2) (*Zone, *monster) {
	t.Helper()
	return newFightZoneOf(t, testKind, level, home)
}

func newFightZoneOf(t *testing.T, kind gamedata.Kind, level int, home Vec2) (*Zone, *monster) {
	t.Helper()
	cfg := testConfig(10)
	area := Area{
		Grid:    openGrid(60, 60),
		Safe:    []Rect{{X: 0, Y: 0, W: 10, H: 10}},
		Respawn: Vec2{5.5, 5.5},
		Camps:   []Camp{{ID: "c1", Tiles: []Vec2{home}, Count: 1, MinLevel: level, MaxLevel: level, Kind: kind}},
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
	if m.attackers == nil {
		t.Fatal("attackers must stay allocated after a return")
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

// The chase stops inside the attack range even when the range is not above
// the 1.3-tile chase stop, so float error cannot leave it chasing forever.
func TestMonsterHitsWithShortAttackRange(t *testing.T) {
	// Approaches from several angles: each rounds the stop point differently.
	starts := []Vec2{{33.5, 30.5}, {33.1, 32.3}, {32.7, 33.2}, {28.1, 32.9}, {31.9, 27.4}}
	for _, reach := range []float64{1.3, 1.2, 1.0} {
		for _, p := range starts {
			k := testKind
			k.AttackRange = reach
			z, m := newFightZoneOf(t, k, 1, Vec2{30.5, 30.5})
			joinNow(t, z, 1, p.X, p.Y)
			now := time.Now()
			hit := false
			for i := 0; i < 40 && !hit; i++ {
				z.advance(now)
				hit = len(z.events.hits) > 0
			}
			if !hit {
				t.Fatalf("attack range %v from %v: never hit, %v at %.7f tiles", reach, p, m.state, dist(m.pos, z.players[1].pos))
			}
		}
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

// A viewer who sees only the attacker still gets the Hit; the client ignores
// ids it has no EntityEnter for.
func TestHitWithOnlyAttackerVisible(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{39.5, 30.5}) // AOI cell (3,3)
	joinNow(t, z, 1, 40.5, 30.5)                 // target in cell (4,3)
	v := joinNow(t, z, 2, 20.5, 30.5)            // cell (2,3): sees (3,3), not (4,3)
	z.advance(time.Now())
	envs := v.take()
	if slices.Contains(enters(envs), 1) || !slices.Contains(enters(envs), m.id) {
		t.Fatalf("viewer enters %v, want the monster but not the target", enters(envs))
	}
	if hs := hitsIn(envs); len(hs) != 1 || hs[0].AttackerId != m.id || hs[0].TargetId != 1 {
		t.Fatalf("viewer hits %v, want the monster's hit on 1", hs)
	}
}

// ______________________TEST COMBAT_____________________

// attackNow queues an attack by id on target with seq and runs one tick.
func attackNow(t *testing.T, z *Zone, id uint64, seq uint32, target uint64) {
	t.Helper()
	if !z.Attack(id, seq, target) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
}

func errorFor(envs []*pb.Envelope, seq uint32) *pb.ErrorResp {
	for _, e := range envs {
		if er := e.GetError(); er != nil && e.Seq == seq {
			return er
		}
	}
	return nil
}

func attackRespAt(envs []*pb.Envelope, seq uint32) int {
	for i, e := range envs {
		if e.GetAttackResp() != nil && e.Seq == seq {
			return i
		}
	}
	return -1
}

func selfStatsIn(envs []*pb.Envelope) []*pb.SelfStats {
	var ss []*pb.SelfStats
	for _, e := range envs {
		if s := e.GetSelfStats(); s != nil {
			ss = append(ss, s)
		}
	}
	return ss
}

func TestAttackValidation(t *testing.T) {
	home := Vec2{30.5, 30.5}
	for _, c := range []struct {
		name   string
		setup  func(z *Zone, m *monster) uint64 // returns the target id
		player Vec2
		code   pb.ErrorCode
	}{
		{"dead attacker", func(z *Zone, m *monster) uint64 { z.players[1].dead = true; return m.id },
			Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_DEAD},
		{"attacker in a safe zone", func(z *Zone, m *monster) uint64 {
			m.pos, m.home = Vec2{11.0, 5.5}, Vec2{11.0, 5.5}
			z.updateCell(&m.entity)
			return m.id
		}, Vec2{9.5, 5.5}, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE},
		{"unknown id", func(z *Zone, m *monster) uint64 { return 999999 },
			Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"a player id", func(z *Zone, m *monster) uint64 { joinNow(t, z, 2, 31.5, 31.5); return 2 },
			Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"dead monster", func(z *Zone, m *monster) uint64 { m.dead = true; return m.id },
			Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"returning monster", func(z *Zone, m *monster) uint64 {
			m.pos, m.state = Vec2{31.0, 30.5}, returning
			return m.id
		}, Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_INVALID_TARGET},
		{"2.1 tiles away", func(z *Zone, m *monster) uint64 { return m.id },
			Vec2{32.6, 30.5}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{"a wall in between", func(z *Zone, m *monster) uint64 {
			rows := make([]string, 60)
			for y := range rows {
				r := []byte(strings.Repeat(".", 60))
				if y == 30 {
					r[29] = '#'
				}
				rows[y] = string(r)
			}
			z.area.Grid = gridFrom(rows...)
			z.mgrid = z.area.monsterGrid()
			return m.id
		}, Vec2{28.6, 30.5}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
		{"a second attack after 7 ticks", func(z *Zone, m *monster) uint64 {
			attackNow(t, z, 1, 41, m.id)
			for i := 0; i < 6; i++ {
				z.advance(time.Now())
			}
			return m.id
		}, Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_ON_COOLDOWN},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, m := newFightZone(t, 1, home)
			a := joinNow(t, z, 1, c.player.X, c.player.Y)
			target := c.setup(z, m)
			hp := m.hp
			a.take()
			attackNow(t, z, 1, 42, target)
			envs := a.take()
			er := errorFor(envs, 42)
			if er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v with seq 42", er, c.code)
			}
			if attackRespAt(envs, 42) >= 0 || m.hp != hp {
				t.Fatalf("a rejected attack must not land: hp %d, want %d", m.hp, hp)
			}
		})
	}
}

func TestAttackHitsAndAggro(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	// Too short-sighted to notice the attacker by itself: only the hit can make it chase.
	m.camp.Kind.AggroRange = 0.5
	a := joinNow(t, z, 1, 32.4, 30.5) // 1.9 tiles: in slash range with the slack, out of its attack range
	v := joinNow(t, z, 2, 30.5, 36.5) // a viewer
	a.take()
	v.take()
	attackNow(t, z, 1, 9, m.id)
	envs := a.take()
	ri, wi := attackRespAt(envs, 9), -1
	for i, e := range envs {
		if e.GetWorldTick() != nil {
			wi = i
		}
	}
	if ri < 0 || wi < 0 || ri > wi {
		t.Fatalf("AttackResp at %d, WorldTick at %d: the reply must come first", ri, wi)
	}
	// Level-1 slash (phys attack 20) on a level-1 monster (defense 8): 20*(1-8/28) = 14.
	if m.hp != 56-14 {
		t.Fatalf("monster hp %d, want 42", m.hp)
	}
	want := &pb.Hit{AttackerId: 1, TargetId: m.id, Damage: 14, HpAfter: 42}
	for name, got := range map[string][]*pb.Hit{"attacker": hitsIn(envs), "viewer": hitsIn(v.take())} {
		if len(got) != 1 || got[0].String() != want.String() {
			t.Fatalf("%s hits %v, want %v", name, got, want)
		}
	}
	if m.state != chase || m.target != 1 {
		t.Fatalf("state %v target %d, want an idle monster to chase its attacker", m.state, m.target)
	}
	if _, ok := m.attackers[1]; !ok {
		t.Fatal("the attacker must be in the monster's attackers")
	}

	// The cooldown is 8 ticks: an attack 8 ticks after the last is accepted.
	for i := 0; i < 7; i++ {
		z.advance(time.Now())
	}
	a.take()
	attackNow(t, z, 1, 10, m.id)
	if envs := a.take(); attackRespAt(envs, 10) < 0 {
		t.Fatalf("attack 8 ticks later rejected: %v", errorFor(envs, 10))
	}
}

func TestRangeSlack(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 32.49, 30.5)
	attackNow(t, z, 1, 3, m.id)
	if envs := a.take(); attackRespAt(envs, 3) < 0 {
		t.Fatalf("an attack at 1.99 tiles must be accepted, got %v", errorFor(envs, 3))
	}
}

func TestNewerAttackReplacesPending(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 31.5, 30.5)
	z.Attack(1, 5, m.id)
	attackNow(t, z, 1, 6, m.id)
	envs := a.take()
	if er := errorFor(envs, 5); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_ON_COOLDOWN {
		t.Fatalf("the replaced attack got %v, want ON_COOLDOWN", er)
	}
	if attackRespAt(envs, 6) < 0 || m.hp != 56-14 {
		t.Fatalf("the newer attack must land once: hp %d", m.hp)
	}
}

func TestLineBlocked(t *testing.T) {
	g := gridFrom(
		"......",
		"..#...",
		"......",
	)
	for _, c := range []struct {
		a, b Vec2
		want bool
	}{
		{Vec2{0.5, 1.5}, Vec2{4.5, 1.5}, true},
		{Vec2{0.5, 0.5}, Vec2{4.5, 0.5}, false},
		{Vec2{1.5, 1.5}, Vec2{2.9, 1.5}, false}, // an endpoint's own tile does not count
		{Vec2{1.5, 1.5}, Vec2{1.5, 1.5}, false},
	} {
		if got := lineBlocked(g, c.a, c.b); got != c.want {
			t.Errorf("lineBlocked(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestKillGivesExpAndRespawns(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 31.5, 30.5)
	v := joinNow(t, z, 2, 30.5, 36.5)
	camp := z.camps[0]
	now := time.Now()
	var diedAt uint32
	for i := 0; i < 4; i++ {
		if i > 0 {
			for j := 0; j < 7; j++ {
				z.advance(now)
			}
		}
		a.take()
		attackNow(t, z, 1, uint32(10+i), m.id)
		envs := a.take()
		if attackRespAt(envs, uint32(10+i)) < 0 {
			t.Fatalf("slash %d rejected: %v", i+1, errorFor(envs, uint32(10+i)))
		}
		if m.hp != 56-14*(i+1) && !(i == 3 && m.hp == 0) {
			t.Fatalf("after slash %d hp %d", i+1, m.hp)
		}
		if i < 3 {
			continue
		}
		diedAt = z.tickNo
		if !m.dead || !slices.Equal(deathsIn(envs), []uint64{m.id}) || !slices.Equal(deathsIn(v.take()), []uint64{m.id}) {
			t.Fatalf("dead=%v deaths %v, want the monster dead and a Death for everyone", m.dead, deathsIn(envs))
		}
		p := z.players[1]
		if p.exp != 10 || p.level != 1 {
			t.Fatalf("killer level %d exp %d, want level 1 with 10 EXP", p.level, p.exp)
		}
		ss := selfStatsIn(envs)
		if len(ss) != 1 || ss[0].Exp != 10 || envs[len(envs)-1].GetSelfStats() == nil || envs[len(envs)-1].Seq != 0 {
			t.Fatalf("SelfStats %v, want one push with 10 EXP after the WorldTick", ss)
		}
	}
	if z.camps[0] != camp || camp.alive != 1 {
		t.Fatalf("a corpse still counts as alive until removed, alive %d", camp.alive)
	}
	z.advance(now)
	if ss := selfStatsIn(a.take()); len(ss) != 0 {
		t.Fatalf("SelfStats pushed again without a change: %v", ss)
	}

	old := m.id
	var leftAt, newAt uint32
	var newID uint64
	for z.tickNo < diedAt+100 && newID == 0 {
		z.advance(now)
		envs := v.take()
		if slices.Contains(leaves(envs), old) {
			leftAt = z.tickNo
		}
		for _, id := range enters(envs) {
			if id != old && id != 1 {
				newID, newAt = id, z.tickNo
			}
		}
	}
	if leftAt != diedAt+10 {
		t.Fatalf("EntityLeave at tick %d, want %d (1 s after death)", leftAt, diedAt+10)
	}
	if _, still := z.monsters[old]; still {
		t.Fatal("the corpse must be removed from the zone")
	}
	if newAt != diedAt+80 || newID == 0 {
		t.Fatalf("new monster %d at tick %d, want a new id at %d (8 s after death)", newID, newAt, diedAt+80)
	}
	if nm := z.monsters[newID]; nm == nil || nm.hp != 56 || nm.dead || camp.alive != 1 || len(camp.pending) != 0 {
		t.Fatalf("respawned monster %+v alive %d pending %v, want a full one", nm, camp.alive, camp.pending)
	}
}

// ______________________TEST PROGRESSION_____________________

func TestLevelUp(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	in := info(1, 31.5, 30.5)
	in.Exp, in.HP, in.MP = 125, 100, 20
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	v := joinNow(t, z, 2, 30.5, 36.5)
	m.hp = 1
	sv := z.saver.(*fakeSaver)
	attackNow(t, z, 1, 1, m.id)
	p := z.players[1]
	// 125 + 10 = 135 of 130 needed: level 2 with 5 EXP left over.
	if p.level != 2 || p.exp != 5 || p.freePoints != 10 {
		t.Fatalf("level %d exp %d free %d, want 2, 5, 10", p.level, p.exp, p.freePoints)
	}
	if p.maxHP != 560 || p.hp != 560 || p.mp != 120 || p.stats.MaxMP != 120 || p.stats.PhysAttack != 25 {
		t.Fatalf("hp %d/%d mp %d stats %+v, want full at level-2 stats", p.hp, p.maxHP, p.mp, p.stats)
	}
	envs := a.take()
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		var lv []*pb.LevelUp
		for _, wt := range ticksIn(es) {
			lv = append(lv, wt.LevelUps...)
		}
		if len(lv) != 1 || lv[0].Id != 1 || lv[0].Level != 2 {
			t.Fatalf("%s level-ups %v, want 1 to level 2", name, lv)
		}
		var hc *pb.HpChange
		for _, c := range hpChangesIn(es) {
			if c.Id == 1 {
				hc = c
			}
		}
		if hc == nil || hc.Hp != 560 || hc.MaxHp != 560 {
			t.Fatalf("%s HpChange %v, want 560/560", name, hc)
		}
	}
	if ss := selfStatsIn(envs); len(ss) != 1 || ss[0].Level != 2 || ss[0].FreePoints != 10 || ss[0].MaxHp != 560 {
		t.Fatalf("SelfStats %v, want level 2 with 10 free points", ss)
	}
	saves := sv.all()
	if len(saves) != 1 || saves[0].Level != 2 || saves[0].Exp != 5 || saves[0].FreePoints != 10 || saves[0].HP != 560 || saves[0].MP != 120 {
		t.Fatalf("saves %+v, want one immediate save at level 2", saves)
	}
}

func TestJoinDeadPlayerRespawns(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{30.5, 30.5})
	in := info(1, 40.5, 40.5)
	in.HP, in.MP = 0, 3
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	p := z.players[1]
	if p.pos != z.area.Respawn || p.cell != z.cellOf(z.area.Respawn) || p.dead {
		t.Fatalf("at %v cell %v dead %v, want alive at the respawn point %v", p.pos, p.cell, p.dead, z.area.Respawn)
	}
	if p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("hp %d/%d mp %d/%d, want full", p.hp, p.maxHP, p.mp, p.stats.MaxMP)
	}
	r := out.take()[0].GetEnterWorldResp()
	if r.X != z.area.Respawn.X || r.Y != z.area.Respawn.Y {
		t.Fatalf("EnterWorldResp at %v,%v, want the respawn point", r.X, r.Y)
	}
}

func TestJoinClampsHP(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	for _, c := range []struct {
		hp, mp         int
		wantHP, wantMP int
	}{{-1, -1, 500, 100}, {9999, 9999, 500, 100}, {300, 40, 300, 40}} {
		in := info(1, 5.5, 5.5)
		in.HP, in.MP = c.hp, c.mp
		if err := z.join(in, &fakeOut{}); err != nil {
			t.Fatal(err)
		}
		p := z.players[1]
		if p.hp != c.wantHP || p.mp != c.wantMP {
			t.Fatalf("join with hp %d mp %d: got %d %d, want %d %d", c.hp, c.mp, p.hp, p.mp, c.wantHP, c.wantMP)
		}
		<-z.leave(1)
	}
}

func TestEnterWorldRespHasStats(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	in := info(1, 5.5, 5.5)
	in.Class = pb.CharClass_CHAR_CLASS_THUY
	in.Level, in.Exp, in.FreePoints, in.HP = 3, 50, 4, 200
	in.Points = combat.Points{Str: 1, Agi: 2, Vit: 3, Int: 4}
	out := &fakeOut{}
	if err := z.join(in, out); err != nil {
		t.Fatal(err)
	}
	got := out.take()[0].GetEnterWorldResp().GetStats()
	s := combat.Compute(true, 3, in.Points)
	want := &pb.SelfStats{Level: 3, Exp: 50, ExpNext: combat.ExpToNext(3), FreePoints: 4,
		Strength: 1, Agility: 2, Vitality: 3, Intellect: 4, Hp: 200, MaxHp: int32(s.MaxHP), Mp: int32(s.MaxMP), MaxMp: int32(s.MaxMP),
		PhysAttack: int32(s.PhysAttack), MagicAttack: int32(s.MagicAttack), PhysDefense: int32(s.PhysDefense), MagicDefense: int32(s.MagicDefense),
		Accuracy: int32(s.Accuracy), Evasion: int32(s.Evasion), CritRateBp: int32(s.CritRateBP)}
	if got.String() != want.String() {
		t.Fatalf("stats %v, want %v", got, want)
	}
	if s.MaxHP != 500+50*2+16*3 {
		t.Fatalf("magic class max HP %d, want the magic growth", s.MaxHP)
	}
}

func TestStateSavesProgress(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	in := info(1, 5.5, 5.5)
	in.Points, in.FreePoints = combat.Points{Str: 1}, 9
	if err := z.join(in, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	p := z.players[1]
	p.points, p.freePoints, p.hp, p.mp, p.exp = combat.Points{Str: 2, Agi: 3, Vit: 4, Int: 5}, 1, 77, 33, 88
	t0 := time.Now()
	z.advance(t0)
	z.advance(t0.Add(61 * time.Second))
	saves := sv.all()
	want := store.CharState{ID: 100, MapID: "m1", X: 5.5, Y: 5.5, Level: 1, Exp: 88,
		Points: combat.Points{Str: 2, Agi: 3, Vit: 4, Int: 5}, FreePoints: 1, HP: 77, MP: 33}
	if len(saves) != 1 || saves[0] != want {
		t.Fatalf("saves %+v, want %+v", saves, want)
	}
}
