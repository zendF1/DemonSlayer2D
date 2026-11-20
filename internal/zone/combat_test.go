package zone

import (
	"maps"
	"math"
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
			t.Fatalf("player out of detect range: target %d state %v, want idle with no target", m.target, m.state)
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
	v := joinNow(t, z, 2, 30.5, 36.5) // in view, out of detect range
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
		t.Fatalf("farthest from home %.3f, want just beyond the chase range 10", far)
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
	m.dmgBy[1], m.expPaid[1] = 46, 8
	z.updateCell(&m.entity)
	v := joinNow(t, z, 1, 36.5, 36.5) // in view, out of detect range of home
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
	if m.dmgBy == nil || m.expPaid == nil || len(m.dmgBy) != 0 || len(m.expPaid) != 0 {
		t.Fatalf("dmgBy %v expPaid %v, want EXP tracking reset and still allocated", m.dmgBy, m.expPaid)
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

// A monster in reach but across a wall corner must not hit; it walks round
// the corner and hits once it has a clear line, as a player's slash needs.
// This is the 4x4 block at (45-48, 30-33) on kumotori_1 near camp c4.
func TestMonsterNeedsLineOfSight(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{52.5, 40.5})
	rows := make([]string, 60)
	for y := range rows {
		r := []byte(strings.Repeat(".", 60))
		if y >= 30 && y <= 33 {
			copy(r[45:49], "####")
		}
		rows[y] = string(r)
	}
	z.area.Grid = gridFrom(rows...)
	z.mgrid = z.area.monsterGrid()
	joinNow(t, z, 1, 49.3, 33.5)
	m.pos, m.state, m.target = Vec2{48.5, 34.3}, chase, 1
	z.updateCell(&m.entity)
	p := z.players[1]
	if d := distance(m.pos, p.pos); d > float32(testKind.AttackRange) || !lineBlocked(z.area.Grid, m.pos, p.pos) {
		t.Fatalf("setup: %.2f tiles apart, blocked %v; want in reach behind the corner", d, lineBlocked(z.area.Grid, m.pos, p.pos))
	}
	start := m.pos
	now := time.Now()
	hitAt := 0
	for i := 1; i <= 20 && hitAt == 0; i++ {
		z.advance(now)
		if len(z.events.hits) == 0 {
			continue
		}
		hitAt = i
		if lineBlocked(z.area.Grid, m.pos, p.pos) {
			t.Fatalf("tick %d: hit from %v through the wall", i, m.pos)
		}
	}
	if hitAt == 0 {
		t.Fatalf("never hit: %v at %v, %.2f tiles away", m.state, m.pos, distance(m.pos, p.pos))
	}
	if hitAt == 1 || m.pos == start {
		t.Fatalf("hit on tick %d from %v, want it to walk round the corner from %v first", hitAt, m.pos, start)
	}
	if m.state != attack || p.hp != 480 {
		t.Fatalf("state %v hp %d, want attacking with one hit landed", m.state, p.hp)
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
		near     bool // C (2.1 tiles away) stands within detect range
		target   uint64
		state    monsterState
	}{
		{"attacker before nearer player", true, true, 2, chase},
		{"nearest in detect range", false, true, 3, chase},
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
		// Several failures at once: the first check in spec 5.1 order wins.
		{"dead in a safe zone with a bad target", func(z *Zone, m *monster) uint64 { z.players[1].dead = true; return 999999 },
			Vec2{5.5, 5.5}, pb.ErrorCode_ERROR_CODE_DEAD},
		{"in a safe zone with a bad target", func(z *Zone, m *monster) uint64 { return 999999 },
			Vec2{5.5, 5.5}, pb.ErrorCode_ERROR_CODE_IN_SAFE_ZONE},
		{"far and on cooldown", func(z *Zone, m *monster) uint64 {
			attackNow(t, z, 1, 41, m.id)
			p := z.players[1]
			p.pos = Vec2{m.pos.X + 3, m.pos.Y}
			z.updateCell(&p.entity)
			return m.id
		}, Vec2{31.5, 30.5}, pb.ErrorCode_ERROR_CODE_OUT_OF_RANGE},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, m := newFightZone(t, 1, home)
			a := joinNow(t, z, 1, c.player.X, c.player.Y)
			target := c.setup(z, m)
			hp, last := m.hp, z.players[1].lastAttack
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
			if got := z.players[1].lastAttack; got != last {
				t.Fatalf("lastAttack %d, want %d: a rejected attack must not start the cooldown", got, last)
			}
		})
	}
}

func TestAttackHitsAndAggro(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	// Too short-sighted to notice the attacker by itself: only the hit can make it chase.
	m.camp.Kind.DetectRange = 0.5
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
		{Vec2{0.5, 2.5}, Vec2{5.5, 0.5}, true}, // a shallow slope through the wall tile
		{Vec2{0.5, 0.5}, Vec2{1.9, 2.9}, false},
	} {
		if got := lineBlocked(g, c.a, c.b); got != c.want {
			t.Errorf("lineBlocked(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}

	// A diagonal through the corner between two walls is blocked, even with
	// one of them, but not with both side tiles open.
	for _, c := range []struct {
		rows []string
		want bool
	}{
		{[]string{".#.", "#..", "..."}, true},
		{[]string{".#.", "...", "..."}, true},
		{[]string{"...", "#..", "..."}, true},
		{[]string{"...", "...", "..."}, false},
	} {
		for _, ab := range [][2]Vec2{{{0.5, 0.5}, {1.5, 1.5}}, {{1.5, 1.5}, {0.5, 0.5}}, {{0.5, 0.5}, {2.5, 2.5}}} {
			if got := lineBlocked(gridFrom(c.rows...), ab[0], ab[1]); got != c.want {
				t.Errorf("grid %v: lineBlocked(%v, %v) = %v, want %v", c.rows, ab[0], ab[1], got, c.want)
			}
		}
	}
}

// Each slash pays its EXP share at once: 14 of 56 HP of a 10-EXP monster
// pays 2, 3, 2, 3 (spec 3.4), with a SelfStats push after each.
func TestKillPaysExpPerHitAndRespawns(t *testing.T) {
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
		want := []int64{2, 5, 7, 10}[i]
		if p := z.players[1]; p.exp != want || p.level != 1 {
			t.Fatalf("after slash %d level %d exp %d, want level 1 with %d EXP", i+1, p.level, p.exp, want)
		}
		ss := selfStatsIn(envs)
		if len(ss) != 1 || ss[0].Exp != want || envs[len(envs)-1].GetSelfStats() == nil || envs[len(envs)-1].Seq != 0 {
			t.Fatalf("after slash %d SelfStats %v, want one push with %d EXP after the WorldTick", i+1, ss, want)
		}
		if saves := z.saver.(*fakeSaver).all(); len(saves) != 0 {
			t.Fatalf("after slash %d saves %+v, want none: EXP without a level-up waits for the periodic save", i+1, saves)
		}
		if i < 3 {
			continue
		}
		diedAt = z.tickNo
		if !m.dead || !slices.Equal(deathsIn(envs), []uint64{m.id}) || !slices.Equal(deathsIn(v.take()), []uint64{m.id}) {
			t.Fatalf("dead=%v deaths %v, want the monster dead and a Death for everyone", m.dead, deathsIn(envs))
		}
	}
	if z.camps[0] != camp || camp.present != 1 {
		t.Fatalf("a corpse still counts as present until removed, present %d", camp.present)
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
	if nm := z.monsters[newID]; nm == nil || nm.hp != 56 || nm.dead || camp.present != 1 || len(camp.pending) != 0 {
		t.Fatalf("respawned monster %+v present %d pending %v, want a full one", nm, camp.present, camp.pending)
	}
}

// ______________________TEST PROGRESSION_____________________

// A hit's EXP share can level a player up mid-fight, with no kill.
func TestLevelUp(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	in := info(1, 31.5, 30.5)
	in.Exp, in.HP, in.MP = 129, 100, 20
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	v := joinNow(t, z, 2, 30.5, 36.5)
	sv := z.saver.(*fakeSaver)
	m.nextAttack = 1 << 30 // it survives the slash; keep it from hitting back
	attackNow(t, z, 1, 1, m.id)
	p := z.players[1]
	if m.dead || m.hp != 42 {
		t.Fatalf("monster dead=%v hp %d, want it alive at 42", m.dead, m.hp)
	}
	// 129 + 2 (14 of 56 HP of 10 EXP) = 131 of 130 needed: level 2 with 1 left over.
	if p.level != 2 || p.exp != 1 || p.freePoints != 10 {
		t.Fatalf("level %d exp %d free %d, want 2, 1, 10", p.level, p.exp, p.freePoints)
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
	if len(saves) != 1 || saves[0].Level != 2 || saves[0].Exp != 1 || saves[0].FreePoints != 10 || saves[0].HP != 560 || saves[0].MP != 120 {
		t.Fatalf("saves %+v, want one immediate save at level 2", saves)
	}
}

// HpChange carries end-of-tick HP: a level-up heal at step 2 followed by a
// monster hit at step 4 must leave viewers with the real HP.
func TestLevelUpThenHitSameTick(t *testing.T) {
	home := Vec2{30.5, 30.5}
	area := Area{
		Grid:    openGrid(60, 60),
		Respawn: Vec2{5.5, 5.5},
		Camps:   []Camp{{ID: "c1", Tiles: []Vec2{home}, Count: 2, MinLevel: 1, MaxLevel: 1, Kind: testKind}},
		Levels:  testLevels,
	}
	z := New(testConfig(10), area, &fakeSaver{}, quietLog())
	ids := slices.Sorted(maps.Keys(z.monsters))
	victim := z.monsters[ids[0]]
	in := info(1, 31.5, 30.5)
	in.Exp, in.HP = 128, 100 // the slash's 2 EXP share reaches level 2
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	v := joinNow(t, z, 2, 30.5, 36.5)
	attackNow(t, z, 1, 1, victim.id)
	p := z.players[1]
	if p.level != 2 || p.hp == p.maxHP {
		t.Fatalf("level %d hp %d/%d, want a level-up and then a hit in the same tick", p.level, p.hp, p.maxHP)
	}
	for name, envs := range map[string][]*pb.Envelope{"player": a.take(), "viewer": v.take()} {
		// A client applies hits, then hp_changes.
		hp := int32(-1)
		for _, wt := range ticksIn(envs) {
			for _, h := range wt.Hits {
				if h.TargetId == 1 {
					hp = h.HpAfter
				}
			}
			for _, c := range wt.HpChanges {
				if c.Id == 1 {
					hp = c.Hp
					if c.MaxHp != int32(p.maxHP) {
						t.Fatalf("%s: HpChange max %d, want %d", name, c.MaxHp, p.maxHP)
					}
				}
			}
		}
		if hp != int32(p.hp) {
			t.Fatalf("%s sees HP %d, real HP %d", name, hp, p.hp)
		}
	}
}

// Two players on one monster each earn the share of their own damage, the
// one who does not land the kill included, and together never more than it
// is worth.
func TestExpSharedByDamage(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	joinNow(t, z, 1, 31.5, 30.5) // slashes for 14
	in := info(2, 29.5, 30.5)
	in.Points = combat.Points{Str: 5} // phys attack 30: slashes for 21
	if err := z.join(in, &fakeOut{}); err != nil {
		t.Fatal(err)
	}
	p1, p2 := z.players[1], z.players[2]
	now := time.Now()
	// Tick 1: 1 hits for 14 (42 left, 2 EXP), then 2 for 21 (21 left, 3 EXP).
	z.Attack(1, 1, m.id)
	attackNow(t, z, 2, 1, m.id)
	if m.hp != 21 || p1.exp != 2 || p2.exp != 3 {
		t.Fatalf("hp %d exp %d/%d, want 21 and 2/3", m.hp, p1.exp, p2.exp)
	}
	// 1 hits twice more: 14 (7 left) and the kill, which counts only 7.
	for seq := uint32(2); seq <= 3; seq++ {
		for range 7 {
			z.advance(now)
		}
		attackNow(t, z, 1, seq, m.id)
	}
	if !m.dead {
		t.Fatalf("monster hp %d, want it dead", m.hp)
	}
	// 1 counted 35 of 56 HP: floor(6.25) = 6. 2 counted 21: floor(3.75) = 3.
	if p1.exp != 6 || p2.exp != 3 {
		t.Fatalf("exp %d/%d, want 6 for the killer and 3 for the other", p1.exp, p2.exp)
	}
	if total := p1.exp + p2.exp; total > combat.MonsterExp(1) {
		t.Fatalf("paid %d in all, more than the monster's %d", total, combat.MonsterExp(1))
	}
}

// Damage past the monster's remaining HP earns nothing, though the Hit still
// shows all of it.
func TestOverkillEarnsNoExp(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 31.5, 30.5)
	m.hp = 11
	a.take()
	attackNow(t, z, 1, 1, m.id)
	hs := hitsIn(a.take())
	if len(hs) != 1 || hs[0].Damage != 14 || hs[0].HpAfter != 0 || !m.dead {
		t.Fatalf("hits %v dead %v, want a 14-damage kill", hs, m.dead)
	}
	// 11 of 56 HP counts: floor(110/56) = 1, not the 2 that 14 would earn.
	if p := z.players[1]; p.exp != 1 || m.dmgBy[1] != 11 {
		t.Fatalf("exp %d counted %d, want 1 EXP for 11 damage", p.exp, m.dmgBy[1])
	}
}

// A monster that heals at home starts its damage tracking again; players
// keep the EXP it paid.
func TestLeashHealResetsExpTracking(t *testing.T) {
	home := Vec2{30.5, 30.5}
	z, m := newFightZone(t, 1, home)
	joinNow(t, z, 1, 31.5, 30.5)
	p := z.players[1]
	attackNow(t, z, 1, 1, m.id)
	if p.exp != 2 || m.dmgBy[1] != 14 || m.expPaid[1] != 2 {
		t.Fatalf("exp %d dmgBy %v expPaid %v, want 2, 14, 2", p.exp, m.dmgBy, m.expPaid)
	}
	// Drag it past the chase range: it walks home and heals.
	m.pos = Vec2{home.X + 11, home.Y}
	z.updateCell(&m.entity)
	now := time.Now()
	for i := 0; i < 100 && !(m.state == idle && m.pos == home); i++ {
		z.advance(now)
	}
	if m.pos != home || m.hp != m.maxHP {
		t.Fatalf("at %v hp %d, want home with full HP", m.pos, m.hp)
	}
	if p.exp != 2 {
		t.Fatalf("exp %d, want the 2 already paid kept", p.exp)
	}
	if m.dmgBy == nil || m.expPaid == nil || len(m.dmgBy) != 0 || len(m.expPaid) != 0 {
		t.Fatalf("dmgBy %v expPaid %v, want empty and still allocated", m.dmgBy, m.expPaid)
	}
	// A fresh 14 pays 2 again, not the 3 that 28 summed would.
	attackNow(t, z, 1, 2, m.id)
	if p.exp != 4 {
		t.Fatalf("exp %d, want 4", p.exp)
	}
}

// A player at the level cap gains nothing and gets no SelfStats push.
func TestNoExpAtMaxLevel(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	in := info(1, 31.5, 30.5)
	in.Level = combat.MaxLevel
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	a.take()
	attackNow(t, z, 1, 1, m.id)
	p := z.players[1]
	if p.level != combat.MaxLevel || p.exp != 0 {
		t.Fatalf("level %d exp %d, want the cap with 0 EXP", p.level, p.exp)
	}
	if ss := selfStatsIn(a.take()); len(ss) != 0 {
		t.Fatalf("SelfStats %v, want none", ss)
	}
	if n := len(z.saver.(*fakeSaver).all()); n != 0 {
		t.Fatalf("%d saves, want none", n)
	}
}

func TestHpChangeOncePerEntityPerTick(t *testing.T) {
	z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
	a := joinNow(t, z, 1, 30.5, 36.5)
	z.advance(time.Now())
	a.take()
	z.hpChanged(&m.entity)
	m.hp = 9
	z.hpChanged(&m.entity)
	wt := &pb.WorldTick{}
	z.addEvents(z.players[1], wt)
	if len(wt.HpChanges) != 1 || wt.HpChanges[0].Hp != 9 {
		t.Fatalf("hp changes %v, want one with the latest HP 9", wt.HpChanges)
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

// ______________________TEST ALLOCATE_____________________

// allocateNow queues an allocation by id with seq and runs one tick.
func allocateNow(t *testing.T, z *Zone, id uint64, seq uint32, p combat.Points) {
	t.Helper()
	if !z.Allocate(id, seq, p) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
}

func allocateRespFor(envs []*pb.Envelope, seq uint32) *pb.AllocatePointsResp {
	for _, e := range envs {
		if r := e.GetAllocatePointsResp(); r != nil && e.Seq == seq {
			return r
		}
	}
	return nil
}

func TestAllocate(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
	in := info(1, 20.5, 20.5)
	in.Class = pb.CharClass_CHAR_CLASS_VIEM
	in.Level, in.FreePoints, in.HP, in.MP = 2, 10, 300, 50
	a := &fakeOut{}
	if err := z.join(in, a); err != nil {
		t.Fatal(err)
	}
	v := joinNow(t, z, 2, 22.5, 20.5)
	z.advance(time.Now())
	a.take()
	v.take()
	p := z.players[1]
	if p.maxHP != 560 || p.stats.MaxMP != 120 {
		t.Fatalf("level-2 Viêm max HP %d MP %d, want 560 and 120", p.maxHP, p.stats.MaxMP)
	}
	sv := z.saver.(*fakeSaver)

	allocateNow(t, z, 1, 9, combat.Points{Vit: 4, Int: 2})
	if p.maxHP != 640 || p.hp != 380 {
		t.Fatalf("hp %d/%d, want 380/640: +4 VIT adds 80 to both", p.hp, p.maxHP)
	}
	if p.stats.MaxMP != 130 || p.mp != 60 {
		t.Fatalf("mp %d/%d, want 60/130: +2 INT adds 10 to both", p.mp, p.stats.MaxMP)
	}
	if p.freePoints != 4 || p.points != (combat.Points{Vit: 4, Int: 2}) {
		t.Fatalf("free %d points %+v, want 4 free and VIT 4 INT 2", p.freePoints, p.points)
	}
	saves := sv.all()
	if len(saves) != 1 || saves[0].Points != p.points || saves[0].FreePoints != 4 || saves[0].HP != 380 || saves[0].MP != 60 {
		t.Fatalf("saves %+v, want one immediate save of the new points", saves)
	}
	envs := a.take()
	r := allocateRespFor(envs, 9)
	if r == nil || r.Stats.String() != selfStats(p).String() {
		t.Fatalf("AllocatePointsResp %v, want the new stats %v", r, selfStats(p))
	}
	if r.Stats.Vitality != 4 || r.Stats.FreePoints != 4 || r.Stats.MaxHp != 640 {
		t.Fatalf("reply stats %v, want VIT 4, 4 free, max HP 640", r.Stats)
	}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		if cs := hpChangesIn(es); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != 380 || cs[0].MaxHp != 640 {
			t.Fatalf("%s HpChanges %v, want 380/640 for 1", name, cs)
		}
	}

	// STR leaves max HP alone, so no HpChange.
	allocateNow(t, z, 1, 10, combat.Points{Str: 4})
	envs = a.take()
	if allocateRespFor(envs, 10) == nil || p.freePoints != 0 || p.stats.PhysAttack != 25+8 {
		t.Fatalf("STR allocation: free %d attack %d, errors %v", p.freePoints, p.stats.PhysAttack, errorFor(envs, 10))
	}
	if cs := hpChangesIn(envs); len(cs) != 0 {
		t.Fatalf("HpChanges %v, want none when max HP is unchanged", cs)
	}
}

func TestAllocateErrors(t *testing.T) {
	for _, c := range []struct {
		name string
		pts  combat.Points
		dead bool
		code pb.ErrorCode
	}{
		{"zero total", combat.Points{}, false, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"negative", combat.Points{Str: 3, Agi: -1}, false, pb.ErrorCode_ERROR_CODE_BAD_REQUEST},
		{"too many", combat.Points{Str: 3, Vit: 3}, false, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_POINTS},
		{"dead", combat.Points{Str: 1}, true, pb.ErrorCode_ERROR_CODE_DEAD},
		{"dead with zero total", combat.Points{}, true, pb.ErrorCode_ERROR_CODE_DEAD},
		{"sum overflows", combat.Points{Str: math.MaxInt, Agi: 2}, false, pb.ErrorCode_ERROR_CODE_NOT_ENOUGH_POINTS},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
			in := info(1, 20.5, 20.5)
			in.FreePoints = 5
			a := &fakeOut{}
			if err := z.join(in, a); err != nil {
				t.Fatal(err)
			}
			p := z.players[1]
			if c.dead {
				z.damagePlayer(p, 0, 10000)
			}
			before := p.stats
			allocateNow(t, z, 1, 3, c.pts)
			envs := a.take()
			if er := errorFor(envs, 3); er == nil || er.Code != c.code {
				t.Fatalf("error %v, want %v", er, c.code)
			}
			if allocateRespFor(envs, 3) != nil || p.freePoints != 5 || p.points != (combat.Points{}) || p.stats != before {
				t.Fatalf("a rejected allocation changed the player: free %d points %+v", p.freePoints, p.points)
			}
			if saves := z.saver.(*fakeSaver).all(); len(saves) != 0 {
				t.Fatalf("saves %+v, want none", saves)
			}
		})
	}
}

// ______________________TEST DEATH_____________________

func respawnRespFor(envs []*pb.Envelope, seq uint32) (*pb.RespawnResp, int) {
	for i, e := range envs {
		if r := e.GetRespawnResp(); r != nil && e.Seq == seq {
			return r, i
		}
	}
	return nil, -1
}

func indexOfTick(envs []*pb.Envelope) int {
	for i, e := range envs {
		if e.GetWorldTick() != nil {
			return i
		}
	}
	return -1
}

func TestRespawnRequest(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
	a := joinNow(t, z, 1, 15.5, 5.5) // cell (1,0); the respawn point is in (0,0)
	v := joinNow(t, z, 2, 12.5, 5.5) // sees both cells
	p := z.players[1]
	p.mp = 7
	z.advance(time.Now())
	a.take()
	v.take()

	if !z.Respawn(1, 4) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
	if er := errorFor(a.take(), 4); er == nil || er.Code != pb.ErrorCode_ERROR_CODE_NOT_DEAD {
		t.Fatalf("respawn while alive: error %v, want NOT_DEAD", er)
	}

	z.damagePlayer(p, 0, 10000)
	z.advance(time.Now())
	a.take()
	v.take()
	if !z.Respawn(1, 5) {
		t.Fatal("inbox full")
	}
	z.advance(time.Now())
	if p.dead || p.pos != z.area.Respawn || p.cell != z.cellOf(z.area.Respawn) || p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("after respawn dead %v at %v cell %v hp %d/%d mp %d/%d, want alive and full at %v",
			p.dead, p.pos, p.cell, p.hp, p.maxHP, p.mp, p.stats.MaxMP, z.area.Respawn)
	}
	envs := a.take()
	r, ri := respawnRespFor(envs, 5)
	if r == nil || r.X != z.area.Respawn.X || r.Y != z.area.Respawn.Y {
		t.Fatalf("RespawnResp %v, want the respawn point %v", r, z.area.Respawn)
	}
	if ti := indexOfTick(envs); ti < ri {
		t.Fatalf("RespawnResp at %d after the WorldTick at %d", ri, ti)
	}
	if ss := selfStatsIn(envs); len(ss) != 1 || ss[0].Hp != int32(p.maxHP) || ss[0].Mp != int32(p.stats.MaxMP) {
		t.Fatalf("SelfStats %v, want one push with full HP and MP", ss)
	}
	ev := v.take()
	wt := lastTick(ev)
	mv := moveFor(wt, 1)
	if mv == nil || !mv.Snap || mv.X != z.area.Respawn.X || mv.Y != z.area.Respawn.Y {
		t.Fatalf("viewer move %v, want a snap to the respawn point", mv)
	}
	if cs := hpChangesIn(ev); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != int32(p.maxHP) {
		t.Fatalf("viewer HpChanges %v, want full HP for 1", cs)
	}

	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{0, 1}})
	z.advance(time.Now())
	if p.pos == z.area.Respawn {
		t.Fatal("a respawned player must be able to move")
	}
	if mv := moveFor(lastTick(v.take()), 1); mv == nil || mv.Snap {
		t.Fatalf("viewer move %v, want an ordinary move after the snap", mv)
	}
}

func TestAutoRespawnAfter60s(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
	a := joinNow(t, z, 1, 15.5, 5.5)
	v := joinNow(t, z, 2, 12.5, 5.5)
	p := z.players[1]
	z.advance(time.Now())
	z.damagePlayer(p, 0, 10000)
	diedAt := z.tickNo
	now := time.Now()
	for z.tickNo < diedAt+599 {
		z.advance(now)
	}
	if !p.dead {
		t.Fatalf("respawned at tick %d, before 60 s had passed", z.tickNo)
	}
	a.take()
	v.take()
	z.advance(now)
	if p.dead || p.pos != z.area.Respawn || p.hp != p.maxHP {
		t.Fatalf("tick %d: dead %v at %v hp %d, want respawned 60 s after death", z.tickNo, p.dead, p.pos, p.hp)
	}
	envs := a.take()
	for _, e := range envs {
		if e.GetRespawnResp() != nil {
			t.Fatal("an automatic respawn must not send RespawnResp")
		}
	}
	if len(selfStatsIn(envs)) != 1 {
		t.Fatal("want a SelfStats push after the automatic respawn")
	}
	if mv := moveFor(lastTick(v.take()), 1); mv == nil || !mv.Snap {
		t.Fatalf("viewer move %v, want a snap", mv)
	}
}

// A monster whose target is killed by its own hits picks another player, or
// goes home when there is none (spec 4.2).
func TestMonsterDropsDeadTarget(t *testing.T) {
	for _, other := range []bool{true, false} {
		z, m := newFightZone(t, 1, Vec2{30.5, 30.5})
		joinHP(t, z, 1, 31.5, 30.5, 20)
		if other {
			joinNow(t, z, 2, 30.5, 33.5)
			m.attackers[2] = struct{}{}
		}
		now := time.Now()
		for i := 0; i < 30 && !z.players[1].dead; i++ {
			z.advance(now)
		}
		if !z.players[1].dead {
			t.Fatal("the monster should have killed player 1")
		}
		z.advance(now)
		if other && (m.target != 2 || m.state == returning) {
			t.Fatalf("target %d state %v, want the monster on player 2", m.target, m.state)
		}
		if !other && (m.target != 0 || (m.state != returning && m.state != idle)) {
			t.Fatalf("target %d state %v, want the monster going home", m.target, m.state)
		}
	}
}

// A respawned player is not chased by the monster that killed it (spec 3.6):
// even on a map without safe zones, the monster forgets it at death.
func TestRespawnedPlayerIsNotChased(t *testing.T) {
	home := Vec2{30.5, 30.5}
	area := Area{
		Grid:    openGrid(60, 60),
		Respawn: Vec2{24.5, 30.5}, // 6 tiles from home: outside aggro, inside leash
		Camps:   []Camp{{ID: "c1", Tiles: []Vec2{home}, Count: 1, MinLevel: 1, MaxLevel: 1, Kind: testKind}},
		Levels:  testLevels,
	}
	z := New(testConfig(10), area, &fakeSaver{}, quietLog())
	var m *monster
	for _, mm := range z.monsters {
		m = mm
	}
	joinHP(t, z, 1, 31.5, 30.5, 20)
	z.advance(time.Now())
	p := z.players[1]
	if !p.dead {
		t.Fatal("the monster should have killed player 1")
	}
	if m.target == 1 {
		t.Fatalf("target %d, want the dead player forgotten at death", m.target)
	}
	if !z.Respawn(1, 1) {
		t.Fatal("inbox full")
	}
	now := time.Now()
	for i := 0; i < 20; i++ {
		z.advance(now)
		if m.target == 1 {
			t.Fatalf("tick %d: the monster chases the respawned player", i)
		}
	}
}

// ______________________TEST REST_____________________

// newRestZone is a fight zone with a rest point of radius 2 at (5.5, 5.5),
// far from the monster, and player 1 standing on it with hp.
func newRestZone(t *testing.T, hp int) (*Zone, *player, *fakeOut, *fakeOut) {
	t.Helper()
	z, _ := newFightZone(t, 1, Vec2{50.5, 50.5})
	z.area.Rests = []Circle{{C: Vec2{5.5, 5.5}, R: 2}}
	a := joinHP(t, z, 1, 5.5, 5.5, hp)
	v := joinNow(t, z, 2, 8.5, 8.5)
	return z, z.players[1], a, v
}

func TestRestHealsAfterFiveSeconds(t *testing.T) {
	z, p, a, v := newRestZone(t, 100)
	p.mp = 10
	now := time.Now()
	for i := 1; i < 50; i++ {
		z.advance(now)
	}
	if p.hp != 100 || p.mp != 10 {
		t.Fatalf("hp %d mp %d after 49 ticks, want no heal yet", p.hp, p.mp)
	}
	a.take()
	v.take()
	z.advance(now)
	if p.hp != p.maxHP || p.mp != p.stats.MaxMP {
		t.Fatalf("hp %d/%d mp %d/%d after 50 ticks, want full", p.hp, p.maxHP, p.mp, p.stats.MaxMP)
	}
	envs := a.take()
	if len(selfStatsIn(envs)) != 1 {
		t.Fatal("want a SelfStats push after the rest heal")
	}
	for name, es := range map[string][]*pb.Envelope{"player": envs, "viewer": v.take()} {
		if cs := hpChangesIn(es); len(cs) != 1 || cs[0].Id != 1 || cs[0].Hp != int32(p.maxHP) {
			t.Fatalf("%s HpChanges %v, want full HP for 1", name, cs)
		}
	}

	// Moving at tick 30 restarts the count, so the heal comes 50 still ticks later.
	z, p, _, _ = newRestZone(t, 100)
	for i := 1; i < 30; i++ {
		z.advance(now)
	}
	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{1, 0}})
	z.advance(now) // tick 30, moving
	z.move(moveCmd{id: 1, seq: 2, dir: Vec2{}})
	for z.tickNo < 79 {
		z.advance(now)
	}
	if p.hp != 100 {
		t.Fatalf("hp %d at tick 79, want no heal before 50 still ticks", p.hp)
	}
	z.advance(now)
	if p.hp != p.maxHP {
		t.Fatalf("hp %d at tick 80, want healed after 50 still ticks", p.hp)
	}
}

// A player killed on the tick its rest count completes stays dead at 0 HP.
func TestRestDoesNotHealPlayerKilledThatTick(t *testing.T) {
	z, _ := newFightZone(t, 1, Vec2{30.5, 30.5})
	z.area.Rests = []Circle{{C: Vec2{31.5, 30.5}, R: 2}} // outside the safe zone, as a bad map might have it
	joinHP(t, z, 1, 31.5, 30.5, 20)
	p := z.players[1]
	p.restTicks = z.ticks(restSec) - 1 // step 3 completes the count; the monster kills at step 4
	z.advance(time.Now())
	if !p.dead || p.hp != 0 || p.restTicks != 0 {
		t.Fatalf("dead %v hp %d restTicks %d, want dead at 0 HP with the count reset", p.dead, p.hp, p.restTicks)
	}
	if st := z.state(p); st.HP != 0 {
		t.Fatalf("saved HP %d, want 0", st.HP)
	}
}

func TestRestOnlyInsideRadiusAndAlive(t *testing.T) {
	z, p, _, _ := newRestZone(t, 100)
	p.pos = Vec2{7.6, 5.5} // 2.1 tiles from the centre
	now := time.Now()
	for i := 0; i < 60; i++ {
		z.advance(now)
	}
	if p.hp != 100 {
		t.Fatalf("hp %d, want no heal outside the radius", p.hp)
	}

	z, p, _, _ = newRestZone(t, 100)
	z.damagePlayer(p, 0, 10000)
	for i := 0; i < 60; i++ {
		z.advance(now)
	}
	if p.hp != 0 || !p.dead {
		t.Fatalf("hp %d dead %v, want a dead player left dead", p.hp, p.dead)
	}
}

func TestRestDoesNothingWhenFull(t *testing.T) {
	z, p, a, v := newRestZone(t, -1)
	now := time.Now()
	for i := 0; i < 120; i++ {
		z.advance(now)
	}
	envs := a.take()
	if cs := append(hpChangesIn(envs), hpChangesIn(v.take())...); len(cs) != 0 {
		t.Fatalf("HpChanges %v, want none for a full player", cs)
	}
	if ss := selfStatsIn(envs); len(ss) != 0 {
		t.Fatalf("SelfStats %v, want none for a full player", ss)
	}

	// Full HP but not full MP: MP is restored, with SelfStats but no HpChange.
	p.mp = 1
	for i := 0; i < 50; i++ {
		z.advance(now)
	}
	envs = a.take()
	if p.mp != p.stats.MaxMP || len(selfStatsIn(envs)) != 1 || len(hpChangesIn(envs)) != 0 {
		t.Fatalf("mp %d/%d SelfStats %d HpChanges %v, want MP restored with only a SelfStats push",
			p.mp, p.stats.MaxMP, len(selfStatsIn(envs)), hpChangesIn(envs))
	}
}
