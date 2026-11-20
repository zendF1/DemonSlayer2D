package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
)

const mapsDir = "../../maps"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// ______________________TEST FLAGS_____________________

// Bad flags, unknown maps and blocked tiles fail before the database is
// opened, so no DATABASE_URL is needed except for the last cases.
func TestRunRefusesBadFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no user", []string{"-level", "5"}, "usage"},
		{"nothing to set", []string{"-u", "a"}, "usage"},
		{"extra argument", []string{"-u", "a", "-level", "5", "x"}, "usage"},
		{"level too low", []string{"-u", "a", "-level", "0"}, "-level 0 is outside 1-90"},
		{"level too high", []string{"-u", "a", "-level", "91"}, "-level 91 is outside 1-90"},
		{"map without x", []string{"-u", "a", "-map", "ubuyashiki", "-y", "16.5"}, "-map needs -x and -y"},
		{"map without y", []string{"-u", "a", "-map", "ubuyashiki", "-x", "8.5"}, "-map needs -x and -y"},
		{"x without map", []string{"-u", "a", "-level", "5", "-x", "8.5"}, "-x and -y need -map"},
		{"unknown map", []string{"-u", "a", "-map", "nowhere", "-x", "8.5", "-y", "16.5"}, `unknown map "nowhere"`},
		{"wall tile", []string{"-u", "a", "-map", "ubuyashiki", "-x", "0.5", "-y", "16.5"}, "(0.5, 16.5) on ubuyashiki is not walkable"},
		{"outside the map", []string{"-u", "a", "-map", "ubuyashiki", "-x", "8.5", "-y", "40"}, "is not walkable"},
		{"no database for level", []string{"-u", "a", "-level", "5"}, "DATABASE_URL"},
		{"no database for map", []string{"-u", "a", "-map", "ubuyashiki", "-x", "8.5", "-y", "16.5"}, "DATABASE_URL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(c.args, env(map[string]string{"MAPS_DIR": mapsDir}), &out)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want one mentioning %q", err, c.want)
			}
			if out.Len() != 0 {
				t.Fatalf("printed %q before failing", out.String())
			}
		})
	}
}

// ______________________TEST DEV CHAR_____________________

// devDB returns a migrated test schema and a DATABASE_URL that reaches it
// (pgx passes the unknown search_path parameter on to the server).
func devDB(t *testing.T) (*store.Store, string) {
	t.Helper()
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
	return st, url + sep + "search_path=" + schema
}

func TestRunSetsLevelAndPlace(t *testing.T) {
	st, url := devDB(t)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	c, err := st.CreateCharacter(ctx, store.NewCharacter{AccountID: acc, Slot: 1, Name: "Tanjiro", NameKey: "tanjiro",
		Class: "viem", MapID: "kumotori_1", X: 5.5, Y: 5.5})
	if err != nil {
		t.Fatal(err)
	}
	devchar := func(vars map[string]string, args ...string) (string, error) {
		var out bytes.Buffer
		err := run(append([]string{"-u", "owner"}, args...), env(vars), &out)
		if strings.Contains(out.String(), url) || strings.Contains(out.String(), "ds:ds") {
			t.Fatalf("printed the database URL: %q", out.String())
		}
		return out.String(), err
	}
	load := func() store.Character {
		t.Helper()
		got, err := st.GetCharacter(ctx, acc, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	vars := map[string]string{"DATABASE_URL": url, "MAPS_DIR": mapsDir}

	// -level alone keeps its phase 2 behaviour.
	out, err := devchar(vars, "-level", "12")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "warning:") || !strings.Contains(out, "Tanjiro: level 12") {
		t.Fatalf("output %q", out)
	}
	if got := load(); got.Level != 12 || got.MapID != "kumotori_1" || got.X != 5.5 {
		t.Fatalf("after -level: %+v", got)
	}

	// -map alone moves the character and leaves its level.
	if out, err = devchar(vars, "-map", "ubuyashiki", "-x", "8.5", "-y", "16.5"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "warning:") || !strings.Contains(out, "Tanjiro: ubuyashiki (8.5, 16.5)") {
		t.Fatalf("output %q", out)
	}
	if got := load(); got.Level != 12 || got.MapID != "ubuyashiki" || got.X != 8.5 || got.Y != 16.5 {
		t.Fatalf("after -map: %+v", got)
	}

	// Both together; MAPS_DIR defaults to ./maps, so an unset MAPS_DIR finds
	// no maps from this directory.
	if _, err = devchar(vars, "-level", "3", "-map", "kumotori_1", "-x", "61.5", "-y", "24"); err != nil {
		t.Fatal(err)
	}
	if got := load(); got.Level != 3 || got.MapID != "kumotori_1" || got.X != 61.5 || got.Y != 24 {
		t.Fatalf("after -level and -map: %+v", got)
	}
	if _, err = devchar(map[string]string{"DATABASE_URL": url}, "-map", "kumotori_1", "-x", "5.5", "-y", "5.5"); err == nil {
		t.Fatal("no maps in ./maps: no error")
	}

	// A wall refuses -level too: nothing is written.
	if _, err = devchar(vars, "-level", "20", "-map", "kumotori_1", "-x", "0.5", "-y", "0.5"); err == nil {
		t.Fatal("wall tile: no error")
	}
	if got := load(); got.Level != 3 || got.MapID != "kumotori_1" || got.X != 61.5 {
		t.Fatalf("after a refused wall tile: %+v", got)
	}

	// A missing account is an error from the store.
	if err := run([]string{"-u", "nobody", "-map", "ubuyashiki", "-x", "8.5", "-y", "16.5"}, env(vars), &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "nobody") {
		t.Fatalf("missing account: %v", err)
	}
}
