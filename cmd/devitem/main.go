// Command devitem gives an account's first character an item or Xu, for
// development only (phase 3a spec 7):
//
//	go run ./cmd/devitem -u USER [-code CODE] [-count N] [-quality Q] [-enhance E] [-holes H] [-xu N]
//
// It reads the database from DATABASE_URL and the item data from DATA_DIR
// (default ./data). An equipment code adds one piece of quality -quality
// (0 Trắng to 4 Đỏ, default 0) at enhance level -enhance (0 to 15, default
// 0, phase 3b spec 7) with -holes empty holes (0 to sockets.max, default 0,
// phase 3c spec 7) and its lines and element rolled from a random seed into
// the first empty bag slot; a stackable code (stones, charm_protect, gems
// such as gem_hp_3) adds -count (default 1) by the bag rules, topping up
// existing stacks first. A full bag is an error and nothing is written. -xu
// adds Xu and may be used alone.
//
// The character must be offline: the server does not record who is online,
// so a running server's next save of the character overwrites the change.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"demonslayer2d/internal/gamedata"
	"demonslayer2d/internal/items"
	"demonslayer2d/internal/store"
)

const usage = "usage: devitem -u USER [-code CODE] [-count N] [-quality Q] [-enhance E] [-holes H] [-xu N]"

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "devitem:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, out io.Writer) error {
	fs := flag.NewFlagSet("devitem", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the error is returned and printed once by main
	user := fs.String("u", "", "account username")
	code := fs.String("code", "", "item code from items.json")
	count := fs.Int("count", 1, "how many of a stackable to add")
	quality := fs.Int("quality", 0, "quality of a piece of equipment, 0 (Trắng) to 4 (Đỏ)")
	enhance := fs.Int("enhance", 0, "enhance level of a piece of equipment, 0 to 15")
	holes := fs.Int("holes", 0, "empty holes punched in a piece of equipment, 0 to 3")
	xu := fs.Int64("xu", 0, "Xu to add")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w; %s", err, usage)
	}
	if *user == "" || fs.NArg() > 0 || (*code == "" && *xu == 0) {
		return errors.New(usage)
	}
	if *xu < 0 {
		return fmt.Errorf("-xu %d is negative", *xu)
	}
	if *count < 1 {
		return fmt.Errorf("-count %d is below 1", *count)
	}

	var item *items.Item
	if *code != "" {
		dir := getenv("DATA_DIR")
		if dir == "" {
			dir = "./data"
		}
		b, err := os.ReadFile(filepath.Join(dir, "items.json"))
		if err != nil {
			return err
		}
		d, err := gamedata.ParseItems(b)
		if err != nil {
			return err
		}
		if item, err = newItem(d, *code, *count, *quality, *enhance, *holes); err != nil {
			return err
		}
		return give(getenv, out, *user, d, item, *xu)
	}
	return give(getenv, out, *user, nil, nil, *xu)
}

// newItem checks the flags against the item data and makes the item: a
// freshly rolled piece of equipment at the given enhance level with holes
// empty holes, or a stack.
func newItem(d *gamedata.Items, code string, count, quality, enhance, holes int) (*items.Item, error) {
	if d.IsEquipment(code) {
		if quality < 0 || quality >= len(d.Qualities) {
			return nil, fmt.Errorf("-quality %d is outside 0-%d", quality, len(d.Qualities)-1)
		}
		if enhance < 0 || enhance > items.MaxEnhance {
			return nil, fmt.Errorf("-enhance %d is outside 0-%d", enhance, items.MaxEnhance)
		}
		if holes < 0 || holes > d.Sockets.Max {
			return nil, fmt.Errorf("-holes %d is outside 0-%d", holes, d.Sockets.Max)
		}
		if count != 1 {
			return nil, fmt.Errorf("-count %d: equipment is added one piece at a time", count)
		}
		rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), rand.Uint64()))
		it := items.NewEquipment(rng, d, code, quality)
		it.Enhance = enhance
		if holes > 0 {
			it.Gems = make([]string, holes)
		}
		return &it, nil
	}
	if _, ok := d.Stackables[code]; !ok {
		return nil, fmt.Errorf("unknown item code %q", code)
	}
	if quality != 0 {
		return nil, fmt.Errorf("-quality %d: %s is not equipment", quality, code)
	}
	if enhance != 0 {
		return nil, fmt.Errorf("-enhance %d: %s is not equipment", enhance, code)
	}
	if holes != 0 {
		return nil, fmt.Errorf("-holes %d: %s is not equipment", holes, code)
	}
	return &items.Item{Code: code, Count: count}, nil
}

func give(getenv func(string) string, out io.Writer, user string, d *gamedata.Items, item *items.Item, xu int64) error {
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

	fmt.Fprintf(out, "warning: %s's character must be offline; if it is in game, the server's next save overwrites this change\n", user)
	var (
		place   func([]store.ItemRow) ([]store.ItemRow, error)
		changed []int
	)
	if item != nil {
		place = func(rows []store.ItemRow) ([]store.ItemRow, error) {
			var err error
			rows, changed, err = addItem(d, rows, *item)
			return rows, err
		}
	}
	res, err := st.DevGiveItem(ctx, user, place, xu)
	if err != nil {
		return err
	}
	if item != nil {
		fmt.Fprintf(out, "%s: added %s to bag slot %s\n", res.Name, describe(d, item), joinInts(changed))
	}
	fmt.Fprintf(out, "%s: %d Xu\n", res.Name, res.Xu)
	return nil
}

// addItem puts it into the bag held by rows with Inventory.Add and returns
// the new rows and the bag slots that changed. Rows it does not touch,
// including worn rows and bag rows outside the bag (which the world drops on
// entering), are kept as they were.
func addItem(d *gamedata.Items, rows []store.ItemRow, it items.Item) ([]store.ItemRow, []int, error) {
	inv := items.NewInventory(d.BagSize)
	for _, r := range rows {
		if !r.Equipped && r.Slot >= 0 && r.Slot < d.BagSize {
			inv.Bag[r.Slot] = &items.Item{Code: r.Code, Count: r.Count}
		}
	}
	changed, err := inv.Add(d, it)
	if err != nil {
		if errors.Is(err, items.ErrBagFull) {
			return nil, nil, errors.New("the bag has no room for it; nothing was written")
		}
		return nil, nil, err
	}
	for _, slot := range changed {
		b := inv.Bag[slot]
		r := store.ItemRow{Slot: slot, Code: b.Code, Count: b.Count, Quality: b.Quality, Element: b.Element,
			Enhance: b.Enhance, Lines: storeLines(b.Lines), Hidden: storeLines(b.Hidden), Gems: b.Gems}
		if i := bagRow(rows, slot); i >= 0 {
			// Only a stack's count changes in an occupied slot; keep the rest of its row.
			rows[i].Count = r.Count
		} else {
			rows = append(rows, r)
		}
	}
	return rows, changed, nil
}

func bagRow(rows []store.ItemRow, slot int) int {
	for i, r := range rows {
		if !r.Equipped && r.Slot == slot {
			return i
		}
	}
	return -1
}

func storeLines(ls []items.Line) []store.ItemLine {
	out := make([]store.ItemLine, len(ls))
	for i, l := range ls {
		out[i] = store.ItemLine{Stat: l.Stat, Value: l.Value}
	}
	return out
}

// describe names it for the summary: "Kiếm Lữ Hành +9 (eq10_weapon_viem),
// Vàng, lines hp+43 phys_attack+12, 2 holes" (no "+0", no holes part
// without holes) or "5 × Đá cường hóa (stone_enhance)".
func describe(d *gamedata.Items, it *items.Item) string {
	if e, ok := d.Equipment[it.Code]; ok {
		name := e.Name
		if it.Enhance > 0 {
			name = fmt.Sprintf("%s +%d", name, it.Enhance)
		}
		s := fmt.Sprintf("%s (%s), %s", name, it.Code, d.Qualities[it.Quality].Name)
		if it.Element != "" {
			s += ", element " + it.Element
		}
		var ls []string
		for _, l := range it.Lines {
			ls = append(ls, fmt.Sprintf("%s+%d", l.Stat, l.Value))
		}
		if len(ls) > 0 {
			s += ", lines " + strings.Join(ls, " ")
		}
		switch n := len(it.Gems); n {
		case 0:
		case 1:
			s += ", 1 hole"
		default:
			s += fmt.Sprintf(", %d holes", n)
		}
		return s
	}
	return fmt.Sprintf("%d × %s (%s)", it.Count, d.Stackables[it.Code].Name, it.Code)
}

func joinInts(ns []int) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = fmt.Sprint(n)
	}
	return strings.Join(s, ", ")
}
