// Command devquest changes the quest log of an account's first character,
// for development only (phase 6b spec 7):
//
//	go run ./cmd/devquest -u USER [-reset] [-done ID] [-active ID [-progress N]] [-day-offset N]
//
// -reset deletes every quest row and runs first. -done sets the quest and
// every quest of its requires chain before it to done. -active sets the
// quest in progress at -progress (default 0; a talk quest is always 1),
// clamped to 0 up to the objective's count. A daily quest set by -done or
// -active is written on today's quest day (quest.Day of the wall clock) plus
// -day-offset days (default 0; -1 makes an entry of yesterday, which the
// server drops at login). An unknown id is an error and nothing is written.
// It prints the quest log afterwards.
//
// It reads the database from DATABASE_URL and quests.json (with the files it
// is checked against) from DATA_DIR (default ./data). The character must be
// offline: the server does not record who is online, so a running server's
// next save of the character overwrites the change.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/quest"
	"demonslayer2d/internal/store"
)

const usage = "usage: devquest -u USER [-reset] [-done ID] [-active ID [-progress N]] [-day-offset N]"

// now is the clock of the quest day; tests replace it.
var now = time.Now

const (
	stateActive = 1
	stateDone   = 2
)

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "devquest:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, out io.Writer) error {
	fs := flag.NewFlagSet("devquest", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the error is returned and printed once by main
	user := fs.String("u", "", "account username")
	reset := fs.Bool("reset", false, "delete every quest row first")
	done := fs.String("done", "", "quest id to set done, with its requires chain")
	active := fs.String("active", "", "quest id to set in progress")
	progress := fs.Int("progress", 0, "progress of -active")
	dayOffset := fs.Int("day-offset", 0, "days added to today for a daily quest")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w; %s", err, usage)
	}
	set := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if *user == "" || fs.NArg() > 0 || (!*reset && *done == "" && *active == "") || (set["progress"] && *active == "") ||
		(set["day-offset"] && *done == "" && *active == "") {
		return errors.New(usage)
	}
	if *progress < 0 {
		return fmt.Errorf("-progress %d is negative", *progress)
	}

	dir := getenv("DATA_DIR")
	if dir == "" {
		dir = "./data"
	}
	d, err := gamedata.Load(dir)
	if err != nil {
		return err
	}
	for _, id := range []string{*done, *active} {
		if id != "" && d.QuestByID[id] == nil {
			return fmt.Errorf("unknown quest %q", id)
		}
	}

	url := getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := store.Open(ctx, url)
	if err != nil {
		return err
	}
	defer st.Close()

	today := quest.Day(now()) + int32(*dayOffset)
	day := func(q *gamedata.Quest) int32 {
		if q.Line == gamedata.LineDaily {
			return today
		}
		return 0
	}
	fmt.Fprintf(out, "warning: %s's character must be offline; if it is in game, the server's next save overwrites this change\n", *user)
	res, err := st.DevEditQuests(ctx, *user, func(rows []store.QuestRow) ([]store.QuestRow, error) {
		byID := make(map[string]store.QuestRow, len(rows))
		if !*reset {
			for _, r := range rows {
				byID[r.QuestID] = r
			}
		}
		if *done != "" {
			for q := d.QuestByID[*done]; q != nil; q = d.QuestByID[q.Requires] {
				byID[q.ID] = store.QuestRow{QuestID: q.ID, State: stateDone, Progress: q.Objective.Need(), Day: day(q)}
			}
		}
		if *active != "" {
			need := d.QuestByID[*active].Objective.Need()
			p := min(*progress, need)
			if d.QuestByID[*active].Objective.Type == gamedata.ObjTalk {
				p = 1
			}
			byID[*active] = store.QuestRow{QuestID: *active, State: stateActive, Progress: p, Day: day(d.QuestByID[*active])}
		}
		merged := make([]store.QuestRow, 0, len(byID))
		for _, r := range byID {
			merged = append(merged, r)
		}
		slices.SortFunc(merged, func(a, b store.QuestRow) int { return strings.Compare(a.QuestID, b.QuestID) })
		return merged, nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %d quests\n", res.Name, len(res.Rows))
	for _, r := range res.Rows {
		state := "active"
		if r.State == stateDone {
			state = "done"
		}
		if r.Day != 0 {
			fmt.Fprintf(out, "  %s %s %d day %d\n", r.QuestID, state, r.Progress, r.Day)
			continue
		}
		fmt.Fprintf(out, "  %s %s %d\n", r.QuestID, state, r.Progress)
	}
	return nil
}
