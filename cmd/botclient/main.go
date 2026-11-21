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
  partner  [-for 60s]
  -fight N [-timeout 2m] [-u USER -p PASS] [-id CHAR_ID] [-class viem] [-seek 22.5,10.5]

partner registers a new account, creates a character, enters the world and
prints "PARTNER <name>". It accepts the first party invite it receives, prints
"PARTY OK" once the party has two members, and stands still until -for ends.

Every command accepts -url (default ws://localhost:8080/ws).

-fight enters the world and kills N monsters, walking to the nearest one it
sees (or towards -seek, the nest of region r1 on kumotori_1, when it sees
none). Without -u it registers a new account; without -id it plays the first
character, creating one if the account has none. It exits 0 after N kills and 1 on timeout.
`

var classByName = map[string]pb.CharClass{
	"viem": pb.CharClass_CHAR_CLASS_VIEM, "thuy": pb.CharClass_CHAR_CLASS_THUY, "minh": pb.CharClass_CHAR_CLASS_MINH,
	"nham": pb.CharClass_CHAR_CLASS_NHAM, "phong": pb.CharClass_CHAR_CLASS_PHONG,
}

func main() {
	cmds := map[string]func([]string) error{
		"register": cmdRegister, "chars": cmdChars, "create": cmdCreate,
		"delete": cmdDelete, "play": cmdPlay, "load": cmdLoad, "fight": cmdFight, "partner": cmdPartner,
	}
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	name, args := os.Args[1], os.Args[2:]
	if strings.HasPrefix(name, "-fight") || strings.HasPrefix(name, "--fight") {
		name, args = "fight", os.Args[1:] // fight mode is selected by its -fight flag
	}
	run, ok := cmds[name]
	if !ok {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err := run(args); err != nil {
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
		fmt.Printf("id=%d slot=%d name=%s class=%s level=%d map=%s\n", ch.Id, ch.Slot, ch.Name, ch.CharClass, ch.Level, ch.MapId)
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
		fmt.Printf("enter  %d %s %s at (%.2f, %.2f)\n", e.Id, who, e.CharClass, e.X, e.Y)
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

func cmdFight(args []string) error {
	fs, url, user, pass := flags("fight")
	n := fs.Int("fight", 1, "number of monsters to kill")
	limit := fs.Duration("timeout", 2*time.Minute, "give up after this long")
	id := fs.Int64("id", 0, "character id (default: the first character, created if needed)")
	className := fs.String("class", "viem", "class of a newly created character")
	seek := fs.String("seek", "22.5,10.5", "where to walk when no monster is in view, x,y")
	_ = fs.Parse(args)
	class, ok := classByName[*className]
	if !ok {
		return fmt.Errorf("unknown class %q", *className)
	}
	sx, sy, err := parseDir(*seek)
	if err != nil {
		return fmt.Errorf("-seek: %w", err)
	}
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(sigCtx, *limit)
	defer cancel()

	c, err := client.Dial(ctx, *url)
	if err != nil {
		return err
	}
	defer c.Close()
	if *user == "" {
		*user, *pass = fmt.Sprintf("fighter_%06d", rand.IntN(1_000_000)), "fighterpass1"
		if err := c.Register(ctx, *user, *pass); err != nil {
			return err
		}
		fmt.Println("registered", *user)
	}
	if _, err := c.Login(ctx, *user, *pass); err != nil {
		return err
	}
	charID := *id
	if charID == 0 {
		if charID, err = firstChar(ctx, c, class); err != nil {
			return err
		}
	}
	ew, err := c.EnterWorld(ctx, charID)
	if err != nil {
		return err
	}
	fmt.Printf("entered map=%s entity=%d at (%.2f, %.2f) level=%d exp=%d\n",
		ew.MapId, ew.SelfEntityId, ew.X, ew.Y, ew.Stats.GetLevel(), ew.Stats.GetExp())

	start := time.Now()
	f := client.NewFighter(c, ew)
	f.Seek = [2]float32{sx, sy}
	kills := 0
	err = f.Fight(ctx, *n, func(k client.Kill) {
		kills++
		fmt.Printf("kill %d/%d monster=%d level=%d exp=%d after %v\n",
			kills, *n, k.ID, k.Level, k.Exp, time.Since(start).Round(100*time.Millisecond))
	})
	if err != nil {
		return fmt.Errorf("after %d of %d kills: %w", kills, *n, err)
	}
	return leave(c)
}

// cmdPartner is the dev partner for the party smoke: it enters the world as a
// fresh character, accepts the first party invite and stands still.
func cmdPartner(args []string) error {
	fs, url, _, _ := flags("partner")
	dur := fs.Duration("for", 60*time.Second, "how long to stay in the world")
	_ = fs.Parse(args)
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(sigCtx, *dur)
	defer cancel()

	c, err := client.Dial(ctx, *url)
	if err != nil {
		return err
	}
	defer c.Close()
	user, pass := fmt.Sprintf("partner_%06d", rand.IntN(1_000_000)), "partnerpass1"
	if err := c.Register(ctx, user, pass); err != nil {
		return err
	}
	if _, err := c.Login(ctx, user, pass); err != nil {
		return err
	}
	name := fmt.Sprintf("Partner%06d", rand.IntN(1_000_000))
	ch, err := c.CreateChar(ctx, 1, name, pb.CharClass_CHAR_CLASS_VIEM)
	if err != nil {
		return err
	}
	if _, err := c.EnterWorld(ctx, ch.Id); err != nil {
		return err
	}
	fmt.Println("PARTNER", name)
	answered, ok := false, false
	for {
		select {
		case env := <-c.Pushes():
			if inv := env.GetPartyInvite(); inv != nil && !answered {
				answered = true
				if err := c.PartyAnswer(ctx, inv.FromId, true); err != nil {
					return fmt.Errorf("accept invite: %w", err)
				}
			}
			if st := env.GetPartyState(); st != nil && !ok && len(st.Members) >= 2 {
				ok = true
				fmt.Println("PARTY OK")
			}
			if k := env.GetKicked(); k != nil {
				return fmt.Errorf("kicked: %v", k.Reason)
			}
		case <-c.Closed():
			return fmt.Errorf("connection closed: %v", c.Err())
		case <-ctx.Done():
			return leave(c)
		}
	}
}

// firstChar returns the account's first character, creating one if it has none.
func firstChar(ctx context.Context, c *client.Client, class pb.CharClass) (int64, error) {
	chars, err := c.ListChars(ctx)
	if err != nil {
		return 0, err
	}
	if len(chars) > 0 {
		return chars[0].Id, nil
	}
	ch, err := c.CreateChar(ctx, 1, fmt.Sprintf("Fighter%06d", rand.IntN(1_000_000)), class)
	if err != nil {
		return 0, err
	}
	fmt.Printf("created id=%d name=%s\n", ch.Id, ch.Name)
	return ch.Id, nil
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
	// -p comes from flags(); for load it is the password shared by every bot.
	fs, url, _, pass := flags("load")
	n := fs.Int("n", 100, "number of bots")
	prefix := fs.String("prefix", "bot", "username/name prefix (letters only)")
	dur := fs.Duration("for", 60*time.Second, "test duration")
	ramp := fs.Duration("ramp", 20*time.Millisecond, "delay between bot starts")
	_ = fs.Parse(args)
	if *pass == "" {
		*pass = "botpass123"
	}

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
