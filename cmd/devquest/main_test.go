package main

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

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
