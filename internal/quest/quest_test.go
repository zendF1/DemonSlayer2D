package quest

import (
	"reflect"
	"testing"
	"time"

	"demonslayer2d/internal/gamedata"
)

func load(t *testing.T) *gamedata.Data {
	t.Helper()
	d, err := gamedata.Load("../../data")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// ______________________TEST RULES_____________________

func TestAvailable(t *testing.T) {
	d := load(t)
	q := d.QuestByID
	if !Available(q["m01"], Log{}, 1, 0) {
		t.Error("m01 at level 1")
	}
	if Available(q["m02"], Log{}, 5, 0) {
		t.Error("m02 before m01 done")
	}
	if Available(q["m02"], Log{"m01": {State: Active}}, 5, 0) {
		t.Error("m02 with m01 active")
	}
	done := Log{"m01": {State: Done}}
	if Available(q["m02"], done, 1, 0) {
		t.Error("m02 at level 1")
	}
	if !Available(q["m02"], done, 2, 0) {
		t.Error("m02 at level 2 after m01")
	}
	if !Available(q["m23"], Log{}, 30, 0) {
		t.Error("m23 at 30 with empty log")
	}
	if Available(q["m23"], Log{}, 29, 0) {
		t.Error("m23 at 29")
	}
	for _, s := range []State{Active, Done} {
		if Available(q["m01"], Log{"m01": {State: s}}, 9, 0) {
			t.Errorf("m01 with entry state %d", s)
		}
	}
}

func TestComplete(t *testing.T) {
	d := load(t)
	if Complete(d.QuestByID["m01"], Entry{State: Active, Progress: 4}) {
		t.Error("4/5")
	}
	if !Complete(d.QuestByID["m01"], Entry{State: Active, Progress: 5}) {
		t.Error("5/5")
	}
	if Complete(d.QuestByID["m01"], Entry{State: Done, Progress: 5}) {
		t.Error("done is not completable")
	}
	if !Complete(d.QuestByID["m11"], Entry{State: Active, Progress: 1}) {
		t.Error("talk")
	}
}

func TestLogClone(t *testing.T) {
	l := Log{"a": {State: Active}}
	c := l.Clone()
	c["a"] = Entry{State: Done}
	if l["a"].State != Active {
		t.Error("clone shares")
	}
}

// ______________________TEST CREDIT_____________________

func TestCredit(t *testing.T) {
	d := load(t)
	yes := func(int) bool { return true }
	no := func(int) bool { return false }

	log := Log{"m01": {State: Active}}
	if got := Credit(d, log, "other", 1, 1, yes); len(got) != 0 || log["m01"].Progress != 0 {
		t.Errorf("wrong kind changed: %v", got)
	}
	if got := Credit(d, log, "wanderer", 1, 1, yes); !reflect.DeepEqual(got, []string{"m01"}) || log["m01"].Progress != 1 {
		t.Errorf("kill: %v %v", got, log)
	}
	log["m01"] = Entry{State: Active, Progress: 5}
	if got := Credit(d, log, "wanderer", 1, 1, yes); len(got) != 0 || log["m01"].Progress != 5 {
		t.Errorf("cap: %v", got)
	}
	log["m01"] = Entry{State: Done, Progress: 5}
	if got := Credit(d, log, "wanderer", 1, 1, yes); len(got) != 0 {
		t.Errorf("done: %v", got)
	}

	log = Log{"m02": {State: Active}}
	if got := Credit(d, log, "wanderer", 1, 1, no); len(got) != 0 || log["m02"].Progress != 0 {
		t.Errorf("collect miss: %v", got)
	}
	var seen int
	roll := func(bp int) bool { seen = bp; return true }
	if got := Credit(d, log, "wanderer", 1, 1, roll); !reflect.DeepEqual(got, []string{"m02"}) || log["m02"].Progress != 1 || seen != 5000 {
		t.Errorf("collect hit: %v %v %d", got, log, seen)
	}

	log = Log{"m11": {State: Active, Progress: 1}}
	if got := Credit(d, log, "wanderer", 1, 1, yes); len(got) != 0 || log["m11"].Progress != 1 {
		t.Errorf("talk: %v", got)
	}

	// two active quests on one kind, returned in id order
	log = Log{"m02": {State: Active}, "m01": {State: Active}}
	got := Credit(d, log, "wanderer", 1, 1, yes)
	if !reflect.DeepEqual(got, []string{"m01", "m02"}) || log["m01"].Progress != 1 || log["m02"].Progress != 1 {
		t.Errorf("two: %v %v", got, log)
	}
}

// ______________________TEST REWARDS_____________________

func TestBookRules(t *testing.T) {
	if BookSkill("viem", 1) != "viem_1" {
		t.Error("BookSkill")
	}
	if !NeedsBook(10, 0, 0) || NeedsBook(10, 1, 0) || NeedsBook(10, 0, 10) || !NeedsBook(10, 0, 9) {
		t.Error("NeedsBook")
	}
}

func TestGrant(t *testing.T) {
	d := load(t)
	m11 := d.QuestByID["m11"]
	stacks, eq, book := Grant(d, m11, "viem", map[string]int{}, 0)
	if !reflect.DeepEqual(eq, []string{"eq10_weapon_viem"}) || book != "book_viem_1" || len(stacks) != 2 {
		t.Fatalf("viem: %v %v %v", stacks, eq, book)
	}
	if stacks[0].Code != "potion_hp_10" || stacks[0].Count != 10 || stacks[1].Code != "potion_mp_10" || stacks[1].Count != 10 {
		t.Errorf("stacks %v", stacks)
	}
	if _, _, b := Grant(d, m11, "viem", map[string]int{"viem_1": 1}, 0); b != "" {
		t.Error("learned still gets a book")
	}
	if s, e, b := Grant(d, m11, "viem", map[string]int{}, 10); b != "" || len(s) != 2 || len(e) != 1 {
		t.Error("waiver 10")
	}
	if _, e, _ := Grant(d, m11, "nham", nil, 0); !reflect.DeepEqual(e, []string{"eq10_weapon_nham"}) {
		t.Errorf("nham %v", e)
	}
	if _, e, b := Grant(d, d.QuestByID["m01"], "viem", nil, 0); len(e) != 0 || b != "" {
		t.Error("m01")
	}
}

func TestRound10(t *testing.T) {
	for _, c := range []struct {
		n   int64
		pct int
		w   int64
	}{{80, 20, 20}, {91520, 10, 9150}, {2065500, 10, 206550}} {
		if g := Round10(c.n, c.pct); g != c.w {
			t.Errorf("Round10(%d,%d)=%d want %d", c.n, c.pct, g, c.w)
		}
	}
}

// ______________________TEST DAILY_____________________

func TestDay(t *testing.T) {
	before := time.Date(2026, 11, 20, 16, 59, 59, 0, time.UTC)
	at := time.Date(2026, 11, 20, 17, 0, 0, 0, time.UTC)
	if Day(at) != Day(before)+1 {
		t.Errorf("Day(16:59:59) = %d, Day(17:00:00) = %d", Day(before), Day(at))
	}
	// 2026-11-21 00:00 UTC+7 is day (unix + 25200) / 86400.
	if want := int32((at.Unix() + DayOffsetSec) / 86400); Day(at) != want {
		t.Errorf("Day = %d, want %d", Day(at), want)
	}
	if Day(at.In(time.FixedZone("x", -5*3600))) != Day(at) {
		t.Error("Day depends on the location")
	}
	if got := ResetIn(at); got != 24*time.Hour {
		t.Errorf("ResetIn(17:00 UTC) = %v", got)
	}
	if got := ResetIn(time.Date(2026, 11, 20, 16, 0, 0, 0, time.UTC)); got != time.Hour {
		t.Errorf("ResetIn(16:00 UTC) = %v", got)
	}
	if got := ResetIn(before); got != time.Second {
		t.Errorf("ResetIn(16:59:59 UTC) = %v", got)
	}
	if got := ResetIn(before.Add(500 * time.Millisecond)); got != 500*time.Millisecond {
		t.Errorf("ResetIn(16:59:59.5 UTC) = %v", got)
	}
}

func TestAvailableDaily(t *testing.T) {
	d := load(t)
	q := d.QuestByID["d_kill"]
	const today = 20000
	if Available(q, Log{}, 9, today) {
		t.Error("level 9")
	}
	if !Available(q, Log{}, 10, today) {
		t.Error("level 10")
	}
	for _, s := range []State{Active, Done} {
		if Available(q, Log{"d_kill": {State: s, Day: today}}, 10, today) {
			t.Errorf("state %d of today", s)
		}
		if !Available(q, Log{"d_kill": {State: s, Day: today - 1}}, 10, today) {
			t.Errorf("state %d of yesterday", s)
		}
	}
	if !Available(q, Log{"d_collect": {State: Active, Day: today}}, 10, today) {
		t.Error("another daily of today")
	}
	// The main line ignores the day.
	if Available(d.QuestByID["m01"], Log{"m01": {State: Done, Day: 1}}, 1, today) {
		t.Error("main done on another day")
	}
}

func TestCreditDaily(t *testing.T) {
	d := load(t)
	yes := func(int) bool { return true }
	no := func(int) bool { return false }

	log := Log{"d_kill": {State: Active}}
	for _, c := range []struct {
		killer, monster int
		ok              bool
	}{{20, 1, false}, {20, 9, false}, {20, 10, true}, {20, 30, true}, {20, 31, false}, {10, 1, true}} {
		before := log["d_kill"].Progress
		got := Credit(d, log, "wanderer", c.killer, c.monster, yes)
		if ok := len(got) == 1 && got[0] == "d_kill" && log["d_kill"].Progress == before+1; ok != c.ok {
			t.Errorf("kill_any killer %d monster %d: %v, progress %d", c.killer, c.monster, got, log["d_kill"].Progress)
		}
	}
	log["d_kill"] = Entry{State: Active, Progress: 40}
	if got := Credit(d, log, "wanderer", 20, 20, yes); len(got) != 0 {
		t.Errorf("kill_any cap: %v", got)
	}

	log = Log{"d_collect": {State: Active}}
	if got := Credit(d, log, "wanderer", 20, 20, no); len(got) != 0 || log["d_collect"].Progress != 0 {
		t.Errorf("collect_any miss: %v", got)
	}
	var seen int
	roll := func(bp int) bool { seen = bp; return true }
	if got := Credit(d, log, "snow_ghoul", 20, 25, roll); !reflect.DeepEqual(got, []string{"d_collect"}) || log["d_collect"].Progress != 1 || seen != 4000 {
		t.Errorf("collect_any hit: %v %v %d", got, log, seen)
	}
	seen = 0
	if got := Credit(d, log, "snow_ghoul", 20, 31, roll); len(got) != 0 || seen != 0 {
		t.Errorf("collect_any out of the window rolled or counted: %v %d", got, seen)
	}

	// A kill_any and a kill quest both count, in id order; enhance does not.
	log = Log{"m01": {State: Active}, "d_kill": {State: Active}, "d_enhance": {State: Active}}
	if got := Credit(d, log, "wanderer", 1, 1, yes); !reflect.DeepEqual(got, []string{"d_kill", "m01"}) {
		t.Errorf("mixed: %v", got)
	}
	// The killer and monster levels do not gate a main kill quest.
	log = Log{"m01": {State: Active}}
	if got := Credit(d, log, "wanderer", 40, 1, yes); !reflect.DeepEqual(got, []string{"m01"}) {
		t.Errorf("main out of the window: %v", got)
	}
}

func TestCreditEnhance(t *testing.T) {
	d := load(t)
	log := Log{"d_enhance": {State: Active}, "d_kill": {State: Active}, "m01": {State: Active}}
	if got := CreditEnhance(d, log, 0); len(got) != 0 {
		t.Errorf("0 attempts: %v", got)
	}
	if got := CreditEnhance(d, log, 1); !reflect.DeepEqual(got, []string{"d_enhance"}) || log["d_enhance"].Progress != 1 {
		t.Errorf("1 attempt: %v %v", got, log)
	}
	if got := CreditEnhance(d, log, 5); !reflect.DeepEqual(got, []string{"d_enhance"}) || log["d_enhance"].Progress != 3 {
		t.Errorf("5 attempts: %v %v", got, log)
	}
	if got := CreditEnhance(d, log, 1); len(got) != 0 || log["d_enhance"].Progress != 3 {
		t.Errorf("complete: %v %v", got, log)
	}
	if log["d_kill"].Progress != 0 || log["m01"].Progress != 0 {
		t.Errorf("other quests changed: %v", log)
	}
	log = Log{"d_enhance": {State: Done, Progress: 3}}
	if got := CreditEnhance(d, log, 1); len(got) != 0 {
		t.Errorf("done: %v", got)
	}
}

func TestPrune(t *testing.T) {
	d := load(t)
	const today = 20000
	log := Log{
		"m01":       {State: Done, Progress: 5},
		"m02":       {State: Active, Day: today - 3}, // a main entry never has a day, but keep it anyway
		"d_kill":    {State: Active, Progress: 12, Day: today},
		"d_collect": {State: Active, Progress: 3, Day: today - 1},
		"d_enhance": {State: Done, Progress: 3, Day: today - 1},
		"gone":      {State: Active}, // no longer in the data: not a daily, kept
	}
	if !Prune(d, log, today) {
		t.Error("Prune reported nothing dropped")
	}
	want := Log{
		"m01":    {State: Done, Progress: 5},
		"m02":    {State: Active, Day: today - 3},
		"d_kill": {State: Active, Progress: 12, Day: today},
		"gone":   {State: Active},
	}
	if !reflect.DeepEqual(log, want) {
		t.Errorf("log = %v", log)
	}
	if Prune(d, log, today) {
		t.Error("second Prune dropped something")
	}
}

func TestDailyReward(t *testing.T) {
	d := load(t)
	kill := d.QuestByID["d_kill"]
	if exp, xu := DailyReward(kill, 30, d.Exp.ToNext(30)); exp != 545080 || xu != 2400 {
		t.Errorf("d_kill at 30: %d EXP, %d Xu", exp, xu)
	}
	if exp, xu := DailyReward(kill, 10, d.Exp.ToNext(10)); exp != 4580 || xu != 800 {
		t.Errorf("d_kill at 10: %d EXP, %d Xu", exp, xu)
	}
	if exp, xu := DailyReward(kill, 90, d.Exp.ToNext(90)); exp != 0 || xu != 7200 {
		t.Errorf("d_kill at 90: %d EXP, %d Xu", exp, xu)
	}
	if exp, xu := DailyReward(d.QuestByID["d_enhance"], 12, d.Exp.ToNext(12)); exp != 0 || xu != 480 {
		t.Errorf("d_enhance at 12: %d EXP, %d Xu", exp, xu)
	}
}
