// Command botclient is a terminal client for manual testing and load testing.
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"demonslayer2d/internal/client"
	"demonslayer2d/internal/pb"
)

const usage = `botclient: test client for the game server

Commands:
  register -u USER -p PASS
  chars    -u USER -p PASS
  create   -u USER -p PASS -slot N -name NAME -class viem|thuy|minh|nham|phong
  delete   -u USER -p PASS -id CHAR_ID -name NAME
  play     -u USER -p PASS -id CHAR_ID [-dir 1,0] [-for 10s]
  load     [-n 100] [-prefix bot] [-p botpass123] [-for 60s] [-ramp 20ms]

Every command accepts -url (default ws://localhost:8080/ws).
`

var classByName = map[string]pb.CharClass{
	"viem": pb.CharClass_CHAR_CLASS_VIEM, "thuy": pb.CharClass_CHAR_CLASS_THUY, "minh": pb.CharClass_CHAR_CLASS_MINH,
	"nham": pb.CharClass_CHAR_CLASS_NHAM, "phong": pb.CharClass_CHAR_CLASS_PHONG,
}

func main() {
	cmds := map[string]func([]string) error{
		"register": cmdRegister, "chars": cmdChars, "create": cmdCreate,
		"delete": cmdDelete, "play": cmdPlay, "load": cmdLoad,
	}
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	run, ok := cmds[os.Args[1]]
	if !ok {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func flags(name string) (fs *flag.FlagSet, url, user, pass *string) {
	fs = flag.NewFlagSet(name, flag.ExitOnError)
	url = fs.String("url", "ws://localhost:8080/ws", "server WebSocket URL")
	user = fs.String("u", "", "username")
	pass = fs.String("p", "", "password")
	return
}

func timeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

func session(ctx context.Context, url, user, pass string) (*client.Client, error) {
	c, err := client.Dial(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := c.Login(ctx, user, pass); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func cmdRegister(args []string) error {
	fs, url, user, pass := flags("register")
	_ = fs.Parse(args)
	ctx, cancel := timeout()
	defer cancel()
	c, err := client.Dial(ctx, *url)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Register(ctx, *user, *pass); err != nil {
		return err
	}
	fmt.Println("registered", *user)
	return nil
}

func cmdChars(args []string) error {
	fs, url, user, pass := flags("chars")
	_ = fs.Parse(args)
	ctx, cancel := timeout()
	defer cancel()
	c, err := session(ctx, *url, *user, *pass)
	if err != nil {
		return err
	}
	defer c.Close()
	chars, err := c.ListChars(ctx)
	if err != nil {
		return err
	}
	if len(chars) == 0 {
		fmt.Println("no characters")
	}
	for _, ch := range chars {
		fmt.Printf("id=%d slot=%d name=%s class=%s level=%d map=%s\n", ch.Id, ch.Slot, ch.Name, ch.Class, ch.Level, ch.MapId)
	}
	return nil
}

func cmdCreate(args []string) error {
	fs, url, user, pass := flags("create")
	slot := fs.Int("slot", 1, "slot 1-3")
	name := fs.String("name", "", "character name")
	className := fs.String("class", "viem", "viem|thuy|minh|nham|phong")
	_ = fs.Parse(args)
	class, ok := classByName[*className]
	if !ok {
		return fmt.Errorf("unknown class %q", *className)
	}
	ctx, cancel := timeout()
	defer cancel()
	c, err := session(ctx, *url, *user, *pass)
	if err != nil {
		return err
	}
	defer c.Close()
	ch, err := c.CreateChar(ctx, int32(*slot), *name, class)
	if err != nil {
		return err
	}
	fmt.Printf("created id=%d name=%s map=%s\n", ch.Id, ch.Name, ch.MapId)
	return nil
}

func cmdDelete(args []string) error {
	fs, url, user, pass := flags("delete")
	id := fs.Int64("id", 0, "character id")
	name := fs.String("name", "", "character name, to confirm")
	_ = fs.Parse(args)
	ctx, cancel := timeout()
	defer cancel()
	c, err := session(ctx, *url, *user, *pass)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.DeleteChar(ctx, *id, *name); err != nil {
		return err
	}
	fmt.Println("deleted", *id)
	return nil
}

func parseDir(s string) (float32, float32, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("direction must be x,y, got %q", s)
	}
	x, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 32)
	y, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 32)
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("direction must be two numbers, got %q", s)
	}
	return float32(x), float32(y), nil
}

func cmdPlay(args []string) error {
	fs, url, user, pass := flags("play")
	id := fs.Int64("id", 0, "character id")
	dir := fs.String("dir", "0,0", "move direction x,y")
	dur := fs.Duration("for", 10*time.Second, "how long to stay in the world")
	_ = fs.Parse(args)
	dx, dy, err := parseDir(*dir)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c, err := session(ctx, *url, *user, *pass)
	if err != nil {
		return err
	}
	defer c.Close()
	ew, err := c.EnterWorld(ctx, *id)
	if err != nil {
		return err
	}
	fmt.Printf("entered map=%s zone=%d entity=%d at (%.2f, %.2f), speed %.1f tiles/s\n",
		ew.MapId, ew.Zone, ew.SelfEntityId, ew.X, ew.Y, ew.MoveSpeed)
	if dx != 0 || dy != 0 {
		if _, err := c.Move(ctx, dx, dy); err != nil {
			return err
		}
	}
	timer := time.NewTimer(*dur)
	defer timer.Stop()
	var lastSelf time.Time
	for {
		select {
		case env := <-c.Pushes():
			printEvent(env, ew.SelfEntityId, &lastSelf)
		case <-timer.C:
			return leave(c)
		case <-ctx.Done():
			return leave(c)
		case <-c.Closed():
			return fmt.Errorf("connection closed: %v", c.Err())
		}
	}
}

func leave(c *client.Client) error {
	ctx, cancel := timeout()
	defer cancel()
	if err := c.LeaveWorld(ctx); err != nil {
		return err
	}
	fmt.Println("left world")
	return nil
}

func printEvent(env *pb.Envelope, self uint64, lastSelf *time.Time) {
	switch {
	case env.GetEntityEnter() != nil:
		e := env.GetEntityEnter()
		who := e.Name
		if e.Id == self {
			who += " (you)"
		}
		fmt.Printf("enter  %d %s %s at (%.2f, %.2f)\n", e.Id, who, e.Class, e.X, e.Y)
	case env.GetEntityLeave() != nil:
		fmt.Printf("leave  %d\n", env.GetEntityLeave().Id)
	case env.GetKicked() != nil:
		fmt.Printf("kicked %s\n", env.GetKicked().Reason)
	case env.GetWorldTick() != nil:
		for _, m := range env.GetWorldTick().Moves {
			if m.Id == self {
				if time.Since(*lastSelf) < time.Second {
					continue
				}
				*lastSelf = time.Now()
			}
			fmt.Printf("move   %d to (%.2f, %.2f) dir (%.2f, %.2f)\n", m.Id, m.X, m.Y, m.DirX, m.DirY)
		}
	}
}

type loadStats struct {
	inWorld, entered, failed atomic.Int64

	mu       sync.Mutex
	kicks    map[pb.KickReason]int
	errs     map[string]int
	rttSum   time.Duration
	rttCount int
	rttMax   time.Duration
}

func (s *loadStats) fail(stage string, err error) {
	s.failed.Add(1)
	s.mu.Lock()
	s.errs[stage+": "+err.Error()]++
	s.mu.Unlock()
}

func (s *loadStats) kick(r pb.KickReason) {
	s.mu.Lock()
	s.kicks[r]++
	s.mu.Unlock()
}

func (s *loadStats) rtt(d time.Duration) {
	s.mu.Lock()
	s.rttSum += d
	s.rttCount++
	s.rttMax = max(s.rttMax, d)
	s.mu.Unlock()
}

func (s *loadStats) line() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	avg := time.Duration(0)
	if s.rttCount > 0 {
		avg = s.rttSum / time.Duration(s.rttCount)
	}
	return fmt.Sprintf("in_world=%d entered=%d failed=%d kicks=%v ping_avg=%v ping_max=%v",
		s.inWorld.Load(), s.entered.Load(), s.failed.Load(), s.kicks, avg.Round(time.Millisecond), s.rttMax.Round(time.Millisecond))
}

func (s *loadStats) errorSummary() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for e, n := range s.errs {
		out = append(out, fmt.Sprintf("%5d  %s", n, e))
	}
	sort.Strings(out)
	return out
}

func cmdLoad(args []string) error {
	fs, url, _, _ := flags("load")
	n := fs.Int("n", 100, "number of bots")
	prefix := fs.String("prefix", "bot", "username/name prefix (letters only)")
	pass := fs.String("p", "botpass123", "password for every bot")
	dur := fs.Duration("for", 60*time.Second, "test duration")
	ramp := fs.Duration("ramp", 20*time.Millisecond, "delay between bot starts")
	_ = fs.Parse(args)

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(sigCtx, *dur)
	defer cancel()

	st := &loadStats{kicks: map[pb.KickReason]int{}, errs: map[string]int{}}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				fmt.Println(st.line())
			}
		}
	}()

	var wg sync.WaitGroup
start:
	for i := 0; i < *n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			runBot(ctx, *url, fmt.Sprintf("%s_%05d", *prefix, i), *pass, fmt.Sprintf("%s%05d", *prefix, i), st)
		}(i)
		select {
		case <-time.After(*ramp):
		case <-ctx.Done():
			break start
		}
	}
	wg.Wait()
	fmt.Println("final:", st.line())
	for _, e := range st.errorSummary() {
		fmt.Println(e)
	}
	return nil
}

var allClasses = []pb.CharClass{
	pb.CharClass_CHAR_CLASS_VIEM, pb.CharClass_CHAR_CLASS_THUY, pb.CharClass_CHAR_CLASS_MINH,
	pb.CharClass_CHAR_CLASS_NHAM, pb.CharClass_CHAR_CLASS_PHONG,
}

func runBot(ctx context.Context, url, user, pass, name string, st *loadStats) {
	c, err := client.Dial(ctx, url)
	if err != nil {
		st.fail("dial", err)
		return
	}
	defer c.Close()
	if err := c.Register(ctx, user, pass); err != nil && client.Code(err) != pb.ErrorCode_ERROR_CODE_USERNAME_TAKEN {
		st.fail("register", err)
		return
	}
	if _, err := c.Login(ctx, user, pass); err != nil {
		st.fail("login", err)
		return
	}
	chars, err := c.ListChars(ctx)
	if err != nil {
		st.fail("chars", err)
		return
	}
	var id int64
	if len(chars) > 0 {
		id = chars[0].Id
	} else {
		ch, err := c.CreateChar(ctx, 1, name, allClasses[rand.IntN(len(allClasses))])
		if err != nil {
			st.fail("create", err)
			return
		}
		id = ch.Id
	}
	if _, err := c.EnterWorld(ctx, id); err != nil {
		st.fail("enter", err)
		return
	}
	st.entered.Add(1)
	st.inWorld.Add(1)
	defer st.inWorld.Add(-1)

	go func() {
		for {
			select {
			case env := <-c.Pushes():
				if k := env.GetKicked(); k != nil {
					st.kick(k.Reason)
				}
			case <-c.Closed():
				return
			}
		}
	}()

	walk := time.NewTicker(2 * time.Second)
	defer walk.Stop()
	ping := time.NewTicker(5 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			lctx, cancel := timeout()
			_ = c.LeaveWorld(lctx)
			cancel()
			return
		case <-c.Closed():
			return
		case <-walk.C:
			a := rand.Float64() * 2 * math.Pi
			_, _ = c.Move(ctx, float32(math.Cos(a)), float32(math.Sin(a)))
		case <-ping.C:
			if d, err := c.Ping(ctx); err == nil {
				st.rtt(d)
			}
		}
	}
}
