// Package quest holds the pure quest rules (phase 6b spec 4): availability,
// kill and collect credit, rewards and whether a skill book is needed; and
// the quest day and the daily rules (phase 6d spec 4). It knows nothing of
// zones, worlds or the store.
package quest

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
)

// DayOffsetSec is UTC+7 (Vietnam, no daylight saving time): the quest day
// changes at 00:00 Vietnam time (6d spec 1.2).
const DayOffsetSec = 7 * 3600

const daySec = 24 * 3600

// Day is the quest day of t: (Unix seconds + DayOffsetSec) / 86400, floored.
func Day(t time.Time) int32 {
	return int32(floorDiv(t.Unix()+DayOffsetSec, daySec))
}

// ResetIn is the time from t to the start of the next quest day, in
// (0, 24 h].
func ResetIn(t time.Time) time.Duration {
	next := time.Unix((int64(Day(t))+1)*daySec-DayOffsetSec, 0)
	return next.Sub(t)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// State is the state of a quest log entry.
type State int8

const (
	Active State = 1
	Done   State = 2
)

// Entry is one quest of a character. Day is for the daily line (6d).
type Entry struct {
	State    State
	Progress int
	Day      int32
}

// Log maps quest id to its entry; a missing key means never accepted.
type Log map[string]Entry

// Clone returns a copy of the log.
func (l Log) Clone() Log { return maps.Clone(l) }

// Available reports whether q can be accepted: no entry yet, the required
// quest done, and the level reached (spec 4.1). A daily quest needs no entry
// of today instead of none at all (6d spec 4.3).
func Available(q *gamedata.Quest, log Log, level int, today int32) bool {
	if e, ok := log[q.ID]; ok && (q.Line != gamedata.LineDaily || e.Day == today) {
		return false
	}
	if q.Requires != "" && log[q.Requires].State != Done {
		return false
	}
	return level >= q.MinLevel
}

// Complete reports whether the entry is active and meets the objective.
func Complete(q *gamedata.Quest, e Entry) bool {
	return e.State == Active && e.Progress >= q.Objective.Need()
}

// Credit applies one monster death of monsterKind at monsterLevel, killed by
// a character of killerLevel, to the active entries in log (spec 4.3), in
// quest id order, and returns the ids that changed. kill and collect need
// the monster kind; kill_any and collect_any need the levels within the EXP
// window (combat.ExpEligible, 6d spec 4.3). roll(bp) is a chance out of
// 10000, used only for the collect types.
func Credit(d *gamedata.Data, log Log, monsterKind string, killerLevel, monsterLevel int, roll func(bp int) bool) []string {
	eligible := combat.ExpEligible(killerLevel, monsterLevel)
	return credit(d, log, func(o gamedata.Objective) int {
		switch o.Type {
		case gamedata.ObjKill:
			if o.Monster == monsterKind {
				return 1
			}
		case gamedata.ObjCollect:
			if o.Monster == monsterKind && roll(o.ChanceBP) {
				return 1
			}
		case gamedata.ObjKillAny:
			if eligible {
				return 1
			}
		case gamedata.ObjCollectAny:
			if eligible && roll(o.ChanceBP) {
				return 1
			}
		}
		return 0
	})
}

// CreditEnhance adds attempts enhance attempts to the active enhance entries
// in log, capped at the count (6d spec 4.3), and returns the ids that
// changed, in id order.
func CreditEnhance(d *gamedata.Data, log Log, attempts int) []string {
	return credit(d, log, func(o gamedata.Objective) int {
		if o.Type == gamedata.ObjEnhance {
			return attempts
		}
		return 0
	})
}

// credit adds gain(objective) to every active, incomplete entry of log in
// quest id order, capped at the objective's need, and returns the ids that
// changed.
func credit(d *gamedata.Data, log Log, gain func(o gamedata.Objective) int) []string {
	var changed []string
	for _, id := range slices.Sorted(maps.Keys(log)) {
		e := log[id]
		q := d.QuestByID[id]
		if e.State != Active || q == nil || e.Progress >= q.Objective.Need() {
			continue
		}
		n := gain(q.Objective)
		if n <= 0 {
			continue
		}
		e.Progress = min(e.Progress+n, q.Objective.Need())
		log[id] = e
		changed = append(changed, id)
	}
	return changed
}

// Prune drops the entries of daily quests whose day is not today (6d spec
// 4.2), active or done, and reports whether it dropped any. Main entries and
// entries of quests no longer in d are kept.
func Prune(d *gamedata.Data, log Log, today int32) bool {
	dropped := false
	for id, e := range log {
		if q := d.QuestByID[id]; q != nil && q.Line == gamedata.LineDaily && e.Day != today {
			delete(log, id)
			dropped = true
		}
	}
	return dropped
}

// DailyReward is the EXP and Xu that turning in q pays a character of level
// with toNext EXP to the next level (6d spec 4.3): Round10(toNext, ExpPct),
// 0 at the top level where toNext is 0, and Xu + XuPerLevel x level.
func DailyReward(q *gamedata.Quest, level int, toNext int64) (exp, xu int64) {
	return Round10(toNext, q.Reward.ExpPct), q.Reward.Xu + q.Reward.XuPerLevel*int64(level)
}

// BookSkill is the skill id of class slot n, which book_<id> teaches.
func BookSkill(class string, n int) string { return fmt.Sprintf("%s_%d", class, n) }

// NeedsBook reports whether a first learn needs a book: the skill is not
// learned and its learn level is above the character's waiver.
func NeedsBook(learnLevel, current, waiver int) bool {
	return current == 0 && learnLevel > waiver
}

// Grant lists what turning q in puts in the bag: stackable rewards, equipment
// codes (white +0, the weapon of the player's class) and the skill book code,
// empty when none is paid or the player does not need it (spec 4.4).
func Grant(d *gamedata.Data, q *gamedata.Quest, class string, learned map[string]int, waiver int) (stacks []items.Item, equipment []string, book string) {
	for _, s := range q.Reward.Items {
		stacks = append(stacks, items.Item{Code: s.Code, Count: s.Count})
	}
	for _, e := range q.Reward.Equipment {
		if e.Slot == "weapon" {
			equipment = append(equipment, fmt.Sprintf("eq%d_weapon_%s", e.Tier, class))
		} else {
			equipment = append(equipment, fmt.Sprintf("eq%d_%s", e.Tier, e.Slot))
		}
	}
	if n := q.Reward.SkillBook; n > 0 {
		id := BookSkill(class, n)
		if sk, ok := d.Skills[id]; ok && NeedsBook(sk.LearnLevel, learned[id], waiver) {
			book = "book_" + id
		}
	}
	return stacks, equipment, book
}

// Round10 is toNext x pct per mille, rounded to the nearest 10.
func Round10(toNext int64, pct int) int64 {
	return (toNext*int64(pct) + 500) / 1000 * 10
}
