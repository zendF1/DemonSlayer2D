// Command devchar changes an account's first character, for development only:
//
//	go run ./cmd/devchar -u USER [-level L] [-map ID -x X -y Y] [-waive-books]
//
// -level (phase 2 spec 2.3) sets the level, EXP to 0, refunds every
// potential point, recomputes the skill points (dropping every skill when
// they no longer fit) and fills HP and MP. -map with -x and -y (phase 4a
// spec 6) moves the character to that map and position; the map is read
// from MAPS_DIR (default ./maps) and an unknown map or a tile that is not
// walkable is refused before anything is written. The two may be used
// alone or together. -waive-books (phase 6b spec 7) sets the skill book
// waiver to the level after any -level change, so every skill the character
// can reach is learned without a book; without it the waiver is unchanged.
//
// It reads the database from DATABASE_URL. The character must be offline:
// the server does not record who is online, so a running server's next
// save of the character overwrites the change.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/store"
)

const usage = "usage: devchar -u USER [-level L] [-map ID -x X -y Y] [-waive-books]"

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "devchar:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, out io.Writer) error {
	fs := flag.NewFlagSet("devchar", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the error is returned and printed once by main
	user := fs.String("u", "", "account username")
	level := fs.Int("level", 0, fmt.Sprintf("level to set, 1-%d", combat.MaxLevel))
	mapID := fs.String("map", "", "map id to move the character to; needs -x and -y")
	x := fs.Float64("x", 0, "x position on -map, in tiles")
	y := fs.Float64("y", 0, "y position on -map, in tiles")
	waive := fs.Bool("waive-books", false, "set the skill book waiver to the level (after -level)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w; %s", err, usage)
	}
	set := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if *user == "" || fs.NArg() > 0 || (!set["level"] && !set["map"] && !*waive) {
		return errors.New(usage)
	}
	if set["level"] && (*level < 1 || *level > combat.MaxLevel) {
		return fmt.Errorf("-level %d is outside 1-%d", *level, combat.MaxLevel)
	}
	if set["map"] && (!set["x"] || !set["y"]) {
		return fmt.Errorf("-map needs -x and -y; %s", usage)
	}
	if !set["map"] && (set["x"] || set["y"]) {
		return fmt.Errorf("-x and -y need -map; %s", usage)
	}
	pos := mapdata.Point{X: float32(*x), Y: float32(*y)}
	if set["map"] {
		if err := checkPlace(getenv, *mapID, pos); err != nil {
			return err
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

	fmt.Fprintf(out, "warning: %s's character must be offline; if it is in game, the server's next save overwrites this change\n", *user)
	if set["level"] {
		res, err := st.DevSetLevel(ctx, *user, *level)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: level %d, %d free potential points, %d skill points\n", res.Name, res.Level, res.FreePoints, res.SkillPoints)
	}
	if *waive {
		res, err := st.DevWaiveBooks(ctx, *user)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: skill book waiver %d\n", res.Name, res.Level)
	}
	if set["map"] {
		res, err := st.DevSetPlace(ctx, *user, *mapID, pos.X, pos.Y)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: %s (%g, %g)\n", res.Name, *mapID, pos.X, pos.Y)
	}
	return nil
}

// checkPlace loads the maps from MAPS_DIR (default ./maps) and checks that
// mapID is one of them and p is a walkable tile on it.
func checkPlace(getenv func(string) string, mapID string, p mapdata.Point) error {
	dir := getenv("MAPS_DIR")
	if dir == "" {
		dir = "./maps"
	}
	maps, err := mapdata.LoadDir(dir)
	if err != nil {
		return err
	}
	m, ok := maps[mapID]
	if !ok {
		return fmt.Errorf("unknown map %q", mapID)
	}
	if !m.Walkable(p) {
		return fmt.Errorf("(%g, %g) on %s is not walkable", p.X, p.Y, mapID)
	}
	return nil
}
