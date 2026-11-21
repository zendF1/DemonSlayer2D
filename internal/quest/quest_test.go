package quest

import (
	"reflect"
	"testing"

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
	if !Available(q["m01"], Log{}, 1) {
		t.Error("m01 at level 1")
	}
	if Available(q["m02"], Log{}, 5) {
		t.Error("m02 before m01 done")
	}
	if Available(q["m02"], Log{"m01": {State: Active}}, 5) {
		t.Error("m02 with m01 active")
	}
	done := Log{"m01": {State: Done}}
	if Available(q["m02"], done, 1) {
		t.Error("m02 at level 1")
	}
	if !Available(q["m02"], done, 2) {
		t.Error("m02 at level 2 after m01")
	}
	if !Available(q["m23"], Log{}, 30) {
		t.Error("m23 at 30 with empty log")
	}
	if Available(q["m23"], Log{}, 29) {
		t.Error("m23 at 29")
	}
	for _, s := range []State{Active, Done} {
		if Available(q["m01"], Log{"m01": {State: s}}, 9) {
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
	if got := Credit(d, log, "other", yes); len(got) != 0 || log["m01"].Progress != 0 {
		t.Errorf("wrong kind changed: %v", got)
	}
	if got := Credit(d, log, "wanderer", yes); !reflect.DeepEqual(got, []string{"m01"}) || log["m01"].Progress != 1 {
		t.Errorf("kill: %v %v", got, log)
	}
	log["m01"] = Entry{State: Active, Progress: 5}
	if got := Credit(d, log, "wanderer", yes); len(got) != 0 || log["m01"].Progress != 5 {
		t.Errorf("cap: %v", got)
	}
	log["m01"] = Entry{State: Done, Progress: 5}
	if got := Credit(d, log, "wanderer", yes); len(got) != 0 {
		t.Errorf("done: %v", got)
	}

	log = Log{"m02": {State: Active}}
	if got := Credit(d, log, "wanderer", no); len(got) != 0 || log["m02"].Progress != 0 {
		t.Errorf("collect miss: %v", got)
	}
	var seen int
	roll := func(bp int) bool { seen = bp; return true }
	if got := Credit(d, log, "wanderer", roll); !reflect.DeepEqual(got, []string{"m02"}) || log["m02"].Progress != 1 || seen != 5000 {
		t.Errorf("collect hit: %v %v %d", got, log, seen)
	}

	log = Log{"m11": {State: Active, Progress: 1}}
	if got := Credit(d, log, "wanderer", yes); len(got) != 0 || log["m11"].Progress != 1 {
		t.Errorf("talk: %v", got)
	}

	// two active quests on one kind, returned in id order
	log = Log{"m02": {State: Active}, "m01": {State: Active}}
	got := Credit(d, log, "wanderer", yes)
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
