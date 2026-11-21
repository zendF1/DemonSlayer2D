// Package quest holds the pure quest rules (phase 6b spec 4): availability,
// kill and collect credit, rewards and whether a skill book is needed. It
// knows nothing of zones, worlds or the store.
package quest

import (
	"fmt"
	"maps"
	"slices"

	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
)

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
// quest done, and the level reached (spec 4.1).
func Available(q *gamedata.Quest, log Log, level int) bool {
	if _, ok := log[q.ID]; ok {
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

// Credit applies one monster death of monsterKind to the active entries in
// log (spec 4.3), in quest id order, and returns the ids that changed.
// roll(bp) is a chance out of 10000, used only for collect objectives.
func Credit(d *gamedata.Data, log Log, monsterKind string, roll func(bp int) bool) []string {
	ids := make([]string, 0, len(log))
	for id := range log {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var changed []string
	for _, id := range ids {
		e := log[id]
		q := d.QuestByID[id]
		if e.State != Active || q == nil || e.Progress >= q.Objective.Need() {
			continue
		}
		o := q.Objective
		if o.Monster != monsterKind {
			continue
		}
		switch o.Type {
		case gamedata.ObjKill:
		case gamedata.ObjCollect:
			if !roll(o.ChanceBP) {
				continue
			}
		default:
			continue
		}
		e.Progress++
		log[id] = e
		changed = append(changed, id)
	}
	return changed
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
