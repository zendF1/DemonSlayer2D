package main

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"demonslayer2d/internal/quest"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
)

const dataDir = "../../data"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// ______________________TEST FLAGS_____________________

func TestRunRefusesBadFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no user", []string{"-reset"}, "usage"},
		{"nothing to do", []string{"-u", "a"}, "usage"},
		{"extra argument", []string{"-u", "a", "-reset", "x"}, "usage"},
		{"progress alone", []string{"-u", "a", "-progress", "2"}, "usage"},
		{"negative progress", []string{"-u", "a", "-active", "m01", "-progress", "-1"}, "-progress"},
		{"unknown done", []string{"-u", "a", "-done", "zz"}, `unknown quest "zz"`},
		{"unknown active", []string{"-u", "a", "-active", "zz"}, `unknown quest "zz"`},
		{"no database", []string{"-u", "a", "-reset"}, "DATABASE_URL"},
		{"day offset alone", []string{"-u", "a", "-day-offset", "-1"}, "usage"},
		{"day offset with reset only", []string{"-u", "a", "-reset", "-day-offset", "1"}, "usage"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(c.args, env(map[string]string{"DATA_DIR": dataDir}), &out)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want one mentioning %q", err, c.want)
			}
			if out.Len() != 0 {
				t.Fatalf("output before the error: %q", out.String())
			}
		})
	}
}

// ______________________TEST DEV QUEST_____________________

func TestRunEditsQuests(t *testing.T) {
	url, schema := storetest.Schema(t)
	ctx := context.Background()
	st, err := store.OpenSchema(ctx, url, schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	dbURL := url + sep + "search_path=" + schema
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	c, err := st.CreateCharacter(ctx, store.NewCharacter{AccountID: acc, Slot: 1, Name: "Tanjiro", NameKey: "tanjiro",
		Class: "viem", MapID: "kumotori_1", X: 5.5, Y: 5.5})
	if err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"DATABASE_URL": dbURL, "DATA_DIR": dataDir}
	devquest := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := run(append([]string{"-u", "owner"}, args...), env(vars), &out)
		if strings.Contains(out.String(), dbURL) || strings.Contains(out.String(), "ds:ds") {
			t.Fatalf("printed the database URL: %q", out.String())
		}
		return out.String(), err
	}
	rows := func() []store.QuestRow {
		t.Helper()
		got, err := st.GetCharacter(ctx, acc, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Quests
	}
	done := func(id string, progress int) store.QuestRow {
		return store.QuestRow{QuestID: id, State: 2, Progress: progress}
	}

	out, err := devquest("-done", "m05")
	if err != nil {
		t.Fatal(err)
	}
	want := []store.QuestRow{done("m01", 5), done("m02", 6), done("m03", 8), done("m04", 6), done("m05", 1)}
	if got := rows(); !reflect.DeepEqual(got, want) {
		t.Fatalf("after -done m05: %+v, want %+v", got, want)
	}
	if !strings.HasPrefix(out, "warning:") || !strings.Contains(out, "m05") {
		t.Fatalf("output %q", out)
	}

	if _, err = devquest("-active", "m07", "-progress", "3"); err != nil {
		t.Fatal(err)
	}
	want = append(want, store.QuestRow{QuestID: "m07", State: 1, Progress: 3})
	if got := rows(); !reflect.DeepEqual(got, want) {
		t.Fatalf("after -active m07: %+v, want %+v", got, want)
	}

	// A talk quest is always at progress 1; progress clamps to the count.
	if _, err = devquest("-active", "m06"); err != nil {
		t.Fatal(err)
	}
	if _, err = devquest("-active", "m08", "-progress", "99"); err != nil {
		t.Fatal(err)
	}
	got := rows()
	if len(got) != 8 || got[5] != (store.QuestRow{QuestID: "m06", State: 1, Progress: 1}) ||
		got[7] != (store.QuestRow{QuestID: "m08", State: 1, Progress: 8}) {
		t.Fatalf("after talk and clamp: %+v", got)
	}

	// An unknown id writes nothing, even beside a valid one.
	if _, err = devquest("-done", "m10", "-active", "zz"); err == nil {
		t.Fatal("unknown id: no error")
	}
	if after := rows(); !reflect.DeepEqual(after, got) {
		t.Fatalf("unknown id changed rows: %+v", after)
	}

	// -reset runs first, then the other flags.
	if _, err = devquest("-reset", "-done", "m02"); err != nil {
		t.Fatal(err)
	}
	if got := rows(); !reflect.DeepEqual(got, []store.QuestRow{done("m01", 5), done("m02", 6)}) {
		t.Fatalf("after -reset -done m02: %+v", got)
	}
	if _, err = devquest("-reset"); err != nil {
		t.Fatal(err)
	}
	if got := rows(); len(got) != 0 {
		t.Fatalf("after -reset: %+v", got)
	}
}

// ______________________TEST DEV DAILY_____________________

// A daily quest is written on today's quest day, plus -day-offset; a main
// quest keeps day 0 whatever the offset.
func TestRunWritesDailyDay(t *testing.T) {
	url, schema := storetest.Schema(t)
	ctx := context.Background()
	st, err := store.OpenSchema(ctx, url, schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	c, err := st.CreateCharacter(ctx, store.NewCharacter{AccountID: acc, Slot: 1, Name: "Tanjiro", NameKey: "tanjiro",
		Class: "viem", MapID: "kumotori_1", X: 5.5, Y: 5.5})
	if err != nil {
		t.Fatal(err)
	}
	// 16:59:59 UTC is 23:59:59 in Vietnam: one second from the next day.
	at := time.Date(2030, 3, 14, 16, 59, 59, 0, time.UTC)
	old := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = old })
	today := quest.Day(at)
	vars := map[string]string{"DATABASE_URL": url + sep + "search_path=" + schema, "DATA_DIR": dataDir}
	devquest := func(args ...string) []store.QuestRow {
		t.Helper()
		if err := run(append([]string{"-u", "owner"}, args...), env(vars), &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		got, err := st.GetCharacter(ctx, acc, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Quests
	}

	got := devquest("-active", "d_kill", "-progress", "4")
	if want := []store.QuestRow{{QuestID: "d_kill", State: 1, Progress: 4, Day: today}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("-active d_kill: %+v, want %+v", got, want)
	}
	got = devquest("-reset", "-active", "d_kill", "-day-offset", "-1")
	if want := []store.QuestRow{{QuestID: "d_kill", State: 1, Day: today - 1}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("-day-offset -1: %+v, want %+v", got, want)
	}
	got = devquest("-done", "d_enhance", "-active", "m02", "-day-offset", "2")
	want := []store.QuestRow{
		{QuestID: "d_enhance", State: 2, Progress: 3, Day: today + 2},
		{QuestID: "d_kill", State: 1, Day: today - 1},
		{QuestID: "m02", State: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("-done d_enhance -active m02: %+v, want %+v", got, want)
	}
}
