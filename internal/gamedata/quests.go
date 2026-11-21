package gamedata

import (
	"fmt"
	"slices"
	"strings"

	"demonslayer2d/internal/mapdata"
)

// Quest lines and objective types (phase 6b spec 2.1). 6d adds the daily
// line and its objective types (6d spec 2).
const (
	LineMain  = "main"
	LineDaily = "daily"

	ObjKill    = "kill"
	ObjCollect = "collect"
	ObjTalk    = "talk"

	ObjKillAny    = "kill_any"
	ObjCollectAny = "collect_any"
	ObjEnhance    = "enhance"
)

// Limits of quests.json (spec 2.6).
const (
	maxQuestLevel = 90
	maxQuestCount = 999
	maxSkillBook  = 5
	maxExpPct     = 100
	// BookPrefix starts the code of every skill book: book_<skill id>.
	BookPrefix = "book_"
)

// NPCRef names an NPC of a map.
type NPCRef struct {
	Map string `json:"map"`
	NPC string `json:"npc"`
}

// Objective is what a quest asks for. kill and collect name a monster, a
// count and the region (of Map) to walk to; collect adds the chance of a
// quest item per kill and its name; talk has no other field. The daily types
// kill_any, collect_any and enhance name no monster, map or region; collect_any
// has the chance and item name of collect.
type Objective struct {
	Type     string `json:"type"`
	Monster  string `json:"monster,omitempty"`
	Count    int    `json:"count,omitempty"`
	ChanceBP int    `json:"chance_bp,omitempty"`
	ItemName string `json:"item_name,omitempty"`
	Map      string `json:"map,omitempty"`
	Region   string `json:"region,omitempty"`
}

// Need is the progress that completes the objective: Count, or 1 for talk.
func (o Objective) Need() int {
	if o.Type == ObjTalk {
		return 1
	}
	return o.Count
}

// QuestText is the three lines of the quest card: on offer, in progress, done.
type QuestText struct {
	Offer    string `json:"offer"`
	Progress string `json:"progress"`
	Done     string `json:"done"`
}

// StackReward is a stackable reward; EquipReward a white +0 piece of Tier in
// Slot (a weapon is the turn-in player's class weapon).
type StackReward struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

// EquipReward is an equipment reward.
type EquipReward struct {
	Tier int    `json:"tier"`
	Slot string `json:"slot"`
}

// QuestReward is what turning a quest in pays. SkillBook is 0 for none or
// the class skill slot 1-5 whose book is paid (book_<class>_<n>). ExpPct and
// XuPerLevel scale a daily reward with the level at turn-in (6d spec 2); a
// main quest has them 0, a daily quest has Exp, Xu and SkillBook 0.
type QuestReward struct {
	Exp        int64         `json:"exp"`
	Xu         int64         `json:"xu"`
	ExpPct     int           `json:"exp_pct"`
	XuPerLevel int64         `json:"xu_per_level"`
	Items      []StackReward `json:"items"`
	Equipment  []EquipReward `json:"equipment"`
	SkillBook  int           `json:"skill_book"`
}

// Quest is one entry of quests.json.
type Quest struct {
	ID        string      `json:"id"`
	Line      string      `json:"line"`
	Name      string      `json:"name"`
	MinLevel  int         `json:"min_level"`
	Requires  string      `json:"requires"`
	Giver     NPCRef      `json:"giver"`
	TurnIn    NPCRef      `json:"turn_in"`
	Objective Objective   `json:"objective"`
	Text      QuestText   `json:"text"`
	Reward    QuestReward `json:"reward"`
}

type questsFile struct {
	Quests []Quest `json:"quests"`
}

// ParseQuests decodes and checks quests.json against the monsters and items
// already in d (spec 2.6); what needs the maps is checked by Validate. It
// sets d.Quests in file order and d.QuestByID.
func (d *Data) ParseQuests(b []byte) error {
	var f questsFile
	if err := decodeStrict(b, &f); err != nil {
		return fmt.Errorf("gamedata: quests: decode: %w", err)
	}
	byID := make(map[string]*Quest, len(f.Quests))
	for i := range f.Quests {
		q := &f.Quests[i]
		if q.ID == "" {
			return fmt.Errorf("gamedata: quest with empty id")
		}
		if _, dup := byID[q.ID]; dup {
			return fmt.Errorf("gamedata: quest %q: duplicate id", q.ID)
		}
		if err := d.checkQuest(q, byID); err != nil {
			return fmt.Errorf("gamedata: quest %q: %w", q.ID, err)
		}
		byID[q.ID] = q
	}
	d.Quests, d.QuestByID = f.Quests, byID
	return nil
}

func (d *Data) checkQuest(q *Quest, earlier map[string]*Quest) error {
	if q.Line != LineMain && q.Line != LineDaily {
		return fmt.Errorf("line %q must be %q or %q", q.Line, LineMain, LineDaily)
	}
	if q.Name == "" {
		return fmt.Errorf("name is empty")
	}
	if q.MinLevel < 1 || q.MinLevel > maxQuestLevel {
		return fmt.Errorf("min_level %d must be in 1-%d", q.MinLevel, maxQuestLevel)
	}
	if q.Requires != "" && q.Line == LineDaily {
		return fmt.Errorf("a daily quest must not require another quest")
	}
	if q.Requires != "" {
		if r, ok := earlier[q.Requires]; !ok || r.Line != LineMain {
			return fmt.Errorf("requires %q, which is not an earlier main quest", q.Requires)
		}
	}
	for name, s := range map[string]string{"offer": q.Text.Offer, "progress": q.Text.Progress, "done": q.Text.Done} {
		if s == "" {
			return fmt.Errorf("text %s is empty", name)
		}
	}
	if q.Line == LineDaily {
		if err := checkDailyObjective(q.Objective); err != nil {
			return err
		}
	} else if err := d.checkObjective(q.Objective); err != nil {
		return err
	}
	return d.checkReward(q.Line, q.Reward)
}

// checkDailyObjective checks a kill_any, collect_any or enhance objective.
func checkDailyObjective(o Objective) error {
	switch o.Type {
	case ObjKillAny, ObjCollectAny, ObjEnhance:
	default:
		return fmt.Errorf("objective type %q must be kill_any, collect_any or enhance", o.Type)
	}
	if o.Monster != "" || o.Map != "" || o.Region != "" {
		return fmt.Errorf("objective %s names no monster, map or region", o.Type)
	}
	if o.Count < 1 || o.Count > maxQuestCount {
		return fmt.Errorf("count %d must be in 1-%d", o.Count, maxQuestCount)
	}
	return checkChance(o, o.Type == ObjCollectAny)
}

// checkChance checks chance_bp and item_name: set for a collect type, unset
// otherwise.
func checkChance(o Objective, collect bool) error {
	if !collect {
		if o.ChanceBP != 0 || o.ItemName != "" {
			return fmt.Errorf("chance_bp and item_name are for collect and collect_any only")
		}
		return nil
	}
	if o.ChanceBP < 1 || o.ChanceBP > 10000 {
		return fmt.Errorf("chance_bp %d must be in 1-10000", o.ChanceBP)
	}
	if o.ItemName == "" {
		return fmt.Errorf("item_name is empty")
	}
	return nil
}

func (d *Data) checkObjective(o Objective) error {
	switch o.Type {
	case ObjTalk:
		if o != (Objective{Type: ObjTalk}) {
			return fmt.Errorf("talk objective has other fields")
		}
		return nil
	case ObjKill, ObjCollect:
	default:
		return fmt.Errorf("objective type %q must be kill, collect or talk", o.Type)
	}
	if _, ok := d.Kinds[o.Monster]; !ok {
		return fmt.Errorf("unknown monster %q", o.Monster)
	}
	if o.Count < 1 || o.Count > maxQuestCount {
		return fmt.Errorf("count %d must be in 1-%d", o.Count, maxQuestCount)
	}
	if o.Map == "" || o.Region == "" {
		return fmt.Errorf("objective needs map and region")
	}
	return checkChance(o, o.Type == ObjCollect)
}

func (d *Data) checkReward(line string, r QuestReward) error {
	if r.ExpPct < 0 || r.ExpPct > maxExpPct {
		return fmt.Errorf("reward exp_pct %d must be in 0-%d", r.ExpPct, maxExpPct)
	}
	if r.XuPerLevel < 0 {
		return fmt.Errorf("reward xu_per_level %d must not be negative", r.XuPerLevel)
	}
	if line == LineMain && (r.ExpPct != 0 || r.XuPerLevel != 0) {
		return fmt.Errorf("exp_pct and xu_per_level are for daily quests only")
	}
	if line == LineDaily && (r.Exp != 0 || r.Xu != 0 || r.SkillBook != 0 || len(r.Equipment) != 0) {
		return fmt.Errorf("a daily reward has no exp, xu, equipment or skill_book")
	}
	if r.Exp < 0 {
		return fmt.Errorf("reward exp %d must not be negative", r.Exp)
	}
	if r.Xu < 0 {
		return fmt.Errorf("reward xu %d must not be negative", r.Xu)
	}
	it := d.Items
	for _, s := range r.Items {
		if _, ok := it.Stackables[s.Code]; !ok || strings.HasPrefix(s.Code, BookPrefix) {
			return fmt.Errorf("reward item %q is not a stackable item that is not a skill book", s.Code)
		}
		if s.Count < 1 || s.Count > it.StackMax {
			return fmt.Errorf("reward item %q count %d must be in 1-%d", s.Code, s.Count, it.StackMax)
		}
	}
	for _, e := range r.Equipment {
		if _, ok := it.Tiers[e.Tier]; !ok {
			return fmt.Errorf("reward equipment tier %d is not an item tier", e.Tier)
		}
		if !slices.Contains(SlotIDs[:], e.Slot) {
			return fmt.Errorf("reward equipment slot %q is not a slot", e.Slot)
		}
	}
	if r.SkillBook < 0 || r.SkillBook > maxSkillBook {
		return fmt.Errorf("skill_book %d must be in 0-%d", r.SkillBook, maxSkillBook)
	}
	return nil
}

// validateQuests checks what needs the maps: the objective's map, region and
// the region's monster, and that the giver and turn-in are quest NPCs.
func (d *Data) validateQuests(maps map[string]*mapdata.Map) error {
	npc := func(q *Quest, what string, ref NPCRef) error {
		m, ok := maps[ref.Map]
		if !ok {
			return fmt.Errorf("gamedata: quest %q: %s map %q does not exist", q.ID, what, ref.Map)
		}
		n, ok := m.NPC(ref.NPC)
		if !ok {
			return fmt.Errorf("gamedata: quest %q: %s npc %q does not exist on map %q", q.ID, what, ref.NPC, ref.Map)
		}
		if n.Role != mapdata.RoleQuest {
			return fmt.Errorf("gamedata: quest %q: %s npc %q is not a quest npc", q.ID, what, ref.NPC)
		}
		return nil
	}
	for i := range d.Quests {
		q := &d.Quests[i]
		if o := q.Objective; o.Type == ObjKill || o.Type == ObjCollect {
			m, ok := maps[o.Map]
			if !ok {
				return fmt.Errorf("gamedata: quest %q: unknown map %q", q.ID, o.Map)
			}
			idx := slices.IndexFunc(m.Regions, func(r mapdata.Region) bool { return r.ID == o.Region })
			if idx < 0 {
				return fmt.Errorf("gamedata: quest %q: map %q has no region %q", q.ID, o.Map, o.Region)
			}
			if got := m.Regions[idx].Monster; got != o.Monster {
				return fmt.Errorf("gamedata: quest %q: region %q holds %q, not %q", q.ID, o.Region, got, o.Monster)
			}
		}
		if err := npc(q, "giver", q.Giver); err != nil {
			return err
		}
		if err := npc(q, "turn_in", q.TurnIn); err != nil {
			return err
		}
	}
	return nil
}

// validateBooks checks that every skill has exactly one book, book_<id>,
// and that every book belongs to a skill (spec 2.4).
func (d *Data) validateBooks() error {
	for id := range d.Skills {
		if _, ok := d.Items.Stackables[BookPrefix+id]; !ok {
			return fmt.Errorf("gamedata: skill %q has no book %q", id, BookPrefix+id)
		}
	}
	for code := range d.Items.Stackables {
		if id, ok := strings.CutPrefix(code, BookPrefix); ok {
			if _, ok := d.Skills[id]; !ok {
				return fmt.Errorf("gamedata: book %q has no skill", code)
			}
		}
	}
	return nil
}
