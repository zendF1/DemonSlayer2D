// Package config reads server settings from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr     string
	DatabaseURL    string
	MapsDir        string
	StartMap       string
	TickRate       int
	MoveSpeedTiles float64
	AOICellSize    int
	ZoneCapacity   int
	SaveInterval   time.Duration
	LogLevel       slog.Level
	// Registrations allowed per IP per hour; raise it for load tests.
	RegisterPerHour int
	// Reverse proxies (IPs or CIDRs) whose X-Forwarded-For header gives the player's IP.
	TrustedProxies []netip.Prefix
}

// Run-time limits. A tick rate above MaxTickRate is pointless for a 2D RPG and one
// above 1e9 makes the tick interval zero, which panics time.NewTicker. Movement is
// simulated in sub-steps of at most 0.25 tiles, so the speed cap also bounds the
// per-tick work for each moving player.
const (
	MaxTickRate       = 100
	MaxMoveSpeedTiles = 100
)

// Load builds a Config from getenv (normally os.Getenv). DATABASE_URL is required.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		ListenAddr:      ":8080",
		MapsDir:         "./maps",
		StartMap:        "kumotori_1",
		TickRate:        10,
		MoveSpeedTiles:  4,
		AOICellSize:     10,
		ZoneCapacity:    50,
		SaveInterval:    60 * time.Second,
		LogLevel:        slog.LevelInfo,
		RegisterPerHour: 5,
	}
	c.DatabaseURL = getenv("DATABASE_URL")
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	for key, dst := range map[string]*string{
		"LISTEN_ADDR": &c.ListenAddr, "MAPS_DIR": &c.MapsDir, "START_MAP": &c.StartMap,
	} {
		if v := getenv(key); v != "" {
			*dst = v
		}
	}
	for key, dst := range map[string]*int{
		"TICK_RATE": &c.TickRate, "AOI_CELL_SIZE": &c.AOICellSize, "ZONE_CAPACITY": &c.ZoneCapacity,
		"REGISTER_PER_HOUR": &c.RegisterPerHour,
	} {
		v := getenv(key)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("config: %s must be a positive integer, got %q", key, v)
		}
		*dst = n
	}
	if c.TickRate > MaxTickRate {
		return Config{}, fmt.Errorf("config: TICK_RATE must be at most %d, got %d", MaxTickRate, c.TickRate)
	}
	if v := getenv("MOVE_SPEED_TILES"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		// The negated comparison also rejects NaN; +Inf and values too big for the
		// float32 the zones use fail the upper bound.
		if err != nil || !(f > 0 && f <= MaxMoveSpeedTiles) {
			return Config{}, fmt.Errorf("config: MOVE_SPEED_TILES must be a number in (0, %d], got %q", MaxMoveSpeedTiles, v)
		}
		c.MoveSpeedTiles = f
	}
	if v := getenv("SAVE_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("config: SAVE_INTERVAL must be a positive duration, got %q", v)
		}
		c.SaveInterval = d
	}
	if v := getenv("TRUSTED_PROXIES"); v != "" {
		for _, f := range strings.Split(v, ",") {
			p, err := parsePrefix(strings.TrimSpace(f))
			if err != nil {
				return Config{}, fmt.Errorf("config: TRUSTED_PROXIES entry %q is not an IP or CIDR", f)
			}
			c.TrustedProxies = append(c.TrustedProxies, p)
		}
	}
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return Config{}, fmt.Errorf("config: LOG_LEVEL %q: %w", v, err)
		}
	}
	return c, nil
}

// parsePrefix accepts a CIDR ("10.0.0.0/8") or a single address ("127.0.0.1").
func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		return p.Masked(), err
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}
