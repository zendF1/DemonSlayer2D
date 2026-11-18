package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		ListenAddr: ":8080", DatabaseURL: "postgres://x", MapsDir: "./maps", StartMap: "kumotori_1",
		TickRate: 10, MoveSpeedTiles: 4, AOICellSize: 10, ZoneCapacity: 50,
		SaveInterval: 60 * time.Second, LogLevel: slog.LevelInfo, RegisterPerHour: 5,
	}
	if c != want {
		t.Fatalf("got %+v\nwant %+v", c, want)
	}
}

func TestOverrides(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://x", "LISTEN_ADDR": ":9000", "MAPS_DIR": "/m", "START_MAP": "town",
		"TICK_RATE": "20", "MOVE_SPEED_TILES": "5.5", "AOI_CELL_SIZE": "8", "ZONE_CAPACITY": "30",
		"SAVE_INTERVAL": "10s", "LOG_LEVEL": "debug", "REGISTER_PER_HOUR": "1000",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":9000" || c.MapsDir != "/m" || c.StartMap != "town" || c.TickRate != 20 ||
		c.MoveSpeedTiles != 5.5 || c.AOICellSize != 8 || c.ZoneCapacity != 30 ||
		c.SaveInterval != 10*time.Second || c.LogLevel != slog.LevelDebug || c.RegisterPerHour != 1000 {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func TestErrors(t *testing.T) {
	cases := map[string]map[string]string{
		"DATABASE_URL":     {},
		"TICK_RATE":        {"DATABASE_URL": "x", "TICK_RATE": "0"},
		"MOVE_SPEED_TILES": {"DATABASE_URL": "x", "MOVE_SPEED_TILES": "fast"},
		"SAVE_INTERVAL":    {"DATABASE_URL": "x", "SAVE_INTERVAL": "-1s"},
		"LOG_LEVEL":        {"DATABASE_URL": "x", "LOG_LEVEL": "loud"},
	}
	for key, kv := range cases {
		_, err := Load(env(kv))
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s: want error mentioning %s, got %v", key, key, err)
		}
	}
}
