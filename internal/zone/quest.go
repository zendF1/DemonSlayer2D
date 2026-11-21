package zone

import (
	"maps"
	"slices"
	"time"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/quest"
	"demonslayer2d/internal/store"
)

type questAcceptCmd struct {
	id         uint64
	seq        uint32
	npc, quest string
}

type questTurnInCmd struct {
	id         uint64
	seq        uint32
	npc, quest string
}

// QuestAccept queues taking quest from the quest NPC npc. The zone answers
// seq with QuestAcceptResp or ErrorResp on the player's outbox. It never
// blocks and reports false if the inbox is full.
func (z *Zone) QuestAccept(id uint64, seq uint32, npc, quest string) bool {
	return z.offer(questAcceptCmd{id: id, seq: seq, npc: npc, quest: quest})
}

// QuestTurnIn queues turning quest in to the quest NPC npc. The zone
// answers seq with QuestTurnInResp or ErrorResp on the player's outbox. It
// never blocks and reports false if the inbox is full.
func (z *Zone) QuestTurnIn(id uint64, seq uint32, npc, quest string) bool {
	return z.offer(questTurnInCmd{id: id, seq: seq, npc: npc, quest: quest})
}

// questByID is the quest id of the area's data, nil when there is none.
func (z *Zone) questByID(id string) *gamedata.Quest {
	if z.area.Quests == nil {
		return nil
	}
	return z.area.Quests.QuestByID[id]
}

// here reports whether ref names the NPC npc of this zone's map.
func (z *Zone) here(ref gamedata.NPCRef, npc string) bool {
	return ref.Map == z.cfg.MapID && ref.NPC == npc
}

// questPlayer is the player a quest command from id is for, once DEAD and
// the quest NPC's reach are checked (spec 4.2 and 4.4 steps 1-3), or nil
// when there is nothing more to do.
func (z *Zone) questPlayer(id uint64, seq uint32, npc string) *player {
	p, ok := z.players[id]
	if !ok {
		return nil // left or evicted; see queueAttack
	}
	if p.dead {
		z.replyError(p, seq, pb.ErrorCode_ERROR_CODE_DEAD)
		return nil
	}
	if _, code := z.nearNPC(p, npc, mapdata.RoleQuest); code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, seq, code)
		return nil
	}
	return p
}

// questAccept takes a quest (phase 6b spec 4.2). Checks run in the spec's
// order and the first failure is answered and changes nothing. A talk quest
// starts complete at progress 1. The new entry is saved at once. A waiting
// refine result is kept: the bag does not change.
func (z *Zone) questAccept(c questAcceptCmd) {
	p := z.questPlayer(c.id, c.seq, c.npc)
	if p == nil {
		return
	}
	q := z.questByID(c.quest)
	today := quest.Day(time.Now()) // the zone clock comes with the 6d zone task
	var code pb.ErrorCode
	switch {
	case q == nil || !z.here(q.Giver, c.npc):
		code = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	// At q's own level Available checks only the entry and the required
	// quest, which come before the level (step 5).
	case !quest.Available(q, p.quests, q.MinLevel, today):
		code = pb.ErrorCode_ERROR_CODE_QUEST_NOT_AVAILABLE
	case p.level < q.MinLevel:
		code = pb.ErrorCode_ERROR_CODE_LEVEL_TOO_LOW
	}
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, c.seq, code)
		return
	}
	e := quest.Entry{State: quest.Active}
	if q.Objective.Type == gamedata.ObjTalk {
		e.Progress = 1
	}
	if q.Line == gamedata.LineDaily {
		e.Day = today
	}
	p.quests[q.ID] = e
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_QuestAcceptResp{QuestAcceptResp: &pb.QuestAcceptResp{
		Quest: questEntry(q.ID, e)}}})
}

// questTurnIn turns a complete quest in (phase 6b spec 4.4). Checks run in
// the spec's order and the first failure is answered and changes nothing;
// the rewards are tried on a copy of the bag first, so a bag that cannot
// hold them all is BAG_FULL with nothing paid. Then the entry is done, the
// items go into the bag, the Xu into the wallet and the EXP through
// gainExp (a level-up as from a kill), and the player is saved at once. A
// waiting refine result is kept: items are only added, as by a purchase.
func (z *Zone) questTurnIn(c questTurnInCmd) {
	p := z.questPlayer(c.id, c.seq, c.npc)
	if p == nil {
		return
	}
	q := z.questByID(c.quest)
	e := p.quests[c.quest]
	var code pb.ErrorCode
	switch {
	case q == nil || z.area.Items == nil:
		code = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	case e.State != quest.Active:
		code = pb.ErrorCode_ERROR_CODE_QUEST_NOT_ACTIVE
	case !z.here(q.TurnIn, c.npc):
		code = pb.ErrorCode_ERROR_CODE_BAD_REQUEST
	case !quest.Complete(q, e):
		code = pb.ErrorCode_ERROR_CODE_QUEST_NOT_COMPLETE
	}
	if code != pb.ErrorCode_ERROR_CODE_UNSPECIFIED {
		z.replyError(p, c.seq, code)
		return
	}
	rewards := z.questItems(p, q)
	if _, err := addAll(z.area.Items, p.inv.Clone(), rewards); err != nil {
		z.replyError(p, c.seq, itemErrorCode(err))
		return
	}
	e.State = quest.Done
	p.quests[q.ID] = e
	changed, _ := addAll(z.area.Items, p.inv, rewards) // fits: tried on the copy
	p.xu += q.Reward.Xu
	z.gainExp(p, q.Reward.Exp)
	p.statsDirty = true
	z.saver.Save(z.state(p), nil)
	p.out.Send(&pb.Envelope{Seq: c.seq, Payload: &pb.Envelope_QuestTurnInResp{QuestTurnInResp: &pb.QuestTurnInResp{
		QuestId: q.ID, Exp: q.Reward.Exp, Xu: q.Reward.Xu,
		Update: inventoryUpdate(p.inv, changed, nil), Stats: z.selfStats(p)}}})
}

// questItems is what turning q in puts in p's bag: the stack rewards, the
// white +0 equipment and the skill book when p needs it (quest.Grant).
func (z *Zone) questItems(p *player, q *gamedata.Quest) []items.Item {
	class, _ := classes.FromPB(p.class)
	stacks, equipment, book := quest.Grant(z.area.Quests, q, class, p.skills, p.bookWaiver)
	out := stacks
	for _, code := range equipment {
		out = append(out, items.Item{Code: code, Count: 1})
	}
	if book != "" {
		out = append(out, items.Item{Code: book, Count: 1})
	}
	return out
}

// addAll puts every item of its into inv's bag, in order, and returns the
// changed bag slots, ascending and without repeats. On an error the bag may
// hold some of them, so callers try a copy first.
func addAll(d *gamedata.Items, inv *items.Inventory, its []items.Item) ([]int, error) {
	var changed []int
	for _, it := range its {
		c, err := inv.Add(d, it)
		if err != nil {
			return nil, err
		}
		changed = append(changed, c...)
	}
	slices.Sort(changed)
	return slices.Compact(changed), nil
}

// creditQuests counts m's death towards killer's active quests (phase 6b
// spec 4.3): only the killer, dead or alive, never a teammate or a player
// who only did damage; nil (the killer has left the zone) gets nothing.
// Collect rolls use the zone's random source. The changed quests go out in
// this tick's syncView; they are saved with the next save.
func (z *Zone) creditQuests(m *monster, killer *player) {
	if killer == nil || z.area.Quests == nil || len(killer.quests) == 0 {
		return
	}
	roll := func(bp int) bool { return z.rng.IntN(10000) < bp }
	killer.questChanged = append(killer.questChanged, quest.Credit(z.area.Quests, killer.quests, m.region.Kind.ID, killer.level, m.level, roll)...)
}

// questEntry is the entry of quest id on the wire.
func questEntry(id string, e quest.Entry) *pb.QuestEntry {
	return &pb.QuestEntry{QuestId: id, State: pb.QuestState(e.State), Progress: int32(e.Progress)}
}

// questEntries is p's entries of ids on the wire, in the order given.
func questEntries(p *player, ids []string) []*pb.QuestEntry {
	out := make([]*pb.QuestEntry, 0, len(ids))
	for _, id := range ids {
		out = append(out, questEntry(id, p.quests[id]))
	}
	return out
}

// questRows is log as store rows ordered by quest id; nil for an empty log.
func questRows(log quest.Log) []store.QuestRow {
	var rows []store.QuestRow
	for _, id := range slices.Sorted(maps.Keys(log)) {
		e := log[id]
		rows = append(rows, store.QuestRow{QuestID: id, State: int16(e.State), Progress: e.Progress, Day: e.Day})
	}
	return rows
}
