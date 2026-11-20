// Command devchar sets the level of an account's first character, for
// development only (phase 2 spec 2.3):
//
//	go run ./cmd/devchar -u USER -level L
//
// It reads the database from DATABASE_URL and takes no other settings. It
// sets the level, EXP to 0, refunds every potential point, recomputes the
// skill points (dropping every skill when they no longer fit) and fills HP
// and MP. The character must be offline: the server does not record who is
// online, so a running server's next save of the character overwrites the
// change.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"demonslayer2d/internal/combat"
	"demonslayer2d/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "devchar:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("devchar", flag.ContinueOnError)
	user := fs.String("u", "", "account username")
	level := fs.Int("level", 0, fmt.Sprintf("level to set, 1-%d", combat.MaxLevel))
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *user == "" || fs.NArg() > 0 {
		return errors.New("usage: devchar -u USER -level L")
	}
	if *level < 1 || *level > combat.MaxLevel {
		return fmt.Errorf("-level %d is outside 1-%d", *level, combat.MaxLevel)
	}
	url := os.Getenv("DATABASE_URL")
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

	fmt.Printf("warning: %s's character must be offline; if it is in game, the server's next save overwrites this change\n", *user)
	res, err := st.DevSetLevel(ctx, *user, *level)
	if err != nil {
		return err
	}
	fmt.Printf("%s: level %d, %d free potential points, %d skill points\n", res.Name, res.Level, res.FreePoints, res.SkillPoints)
	return nil
}
