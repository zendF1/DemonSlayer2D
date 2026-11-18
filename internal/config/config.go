// Package config reads server settings from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"strconv"
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
}

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
	if v := getenv("MOVE_SPEED_TILES"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return Config{}, fmt.Errorf("config: MOVE_SPEED_TILES must be a positive number, got %q", v)
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
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return Config{}, fmt.Errorf("config: LOG_LEVEL %q: %w", v, err)
		}
	}
	return c, nil
}
