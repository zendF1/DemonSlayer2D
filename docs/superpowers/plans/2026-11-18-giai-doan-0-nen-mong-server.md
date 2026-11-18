# Phase 0 Server Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go game server where two clients can register, log in, create a character, enter one map and see each other move in real time, backed by Postgres and verified by automated tests.

**Architecture:** One Go process. A WebSocket gateway speaks Protobuf envelopes. Each map instance ("zone") is a single goroutine that owns all of its entities, ticks 10 times per second, simulates movement on a tile collision grid and sends each player what it can see (3×3 grid of 10-tile cells). A separate saver goroutine writes character state to Postgres so zones never block on the database.

**Tech Stack:** Go 1.25, `github.com/coder/websocket`, `github.com/jackc/pgx/v5`, `google.golang.org/protobuf` (code generated with `buf` + `protoc-gen-go` as Go tools), `github.com/pressly/goose/v3`, `golang.org/x/crypto/argon2`, `golang.org/x/text/unicode/norm`, `golang.org/x/time/rate`, Postgres 17 in Docker.

**Spec:** [docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md](../specs/2026-11-18-giai-doan-0-nen-mong-server-design.md)

**Commit rules (from the user):** commit and push after every task; clear commit messages; **no** `Co-Authored-By` or other signature trailer.

**Deliberate refinements of the spec** (Task 24 writes them back into the spec):
- Collision uses an axis-aligned box of half-size 0.3 tiles (not a circle), and movement is split into sub-steps of at most 0.25 tiles so fast movement can never tunnel through a wall.
- Character-name uniqueness uses a `name_key` column (lower-cased NFC name computed in Go) instead of an index on `lower(name)`, so it does not depend on the database collation.
- Oversized messages (> 4 KB) close the WebSocket with status 1009 (message too big) instead of sending `Kicked`.
- `EnterWorldResp` is sent by the zone itself, so it is always the first in-world message the client receives.
- `botclient` commands are `register`, `chars`, `create`, `delete`, `play` (enter + move + print) and `load`.

---

## File structure

```
go.mod / go.sum                  module demonslayer2d; tool deps buf + protoc-gen-go
Makefile                         proto, db-up, db-down, run, vet, test, test-db
docker-compose.yml               Postgres 17 on localhost:5433 (db ds, test db ds_test)
deploy/initdb/01-create-test-db.sql
buf.yaml / buf.gen.yaml          protobuf build + Go codegen config
proto/ds/v1/protocol.proto       the wire protocol (source of truth for the client repo)
maps/kumotori_1.json             64×48 test map
migrations/embed.go              embeds *.sql for goose
migrations/00001_init.sql        accounts, characters
internal/pb/protocol.pb.go       generated
internal/classes/                db class id <-> protocol enum
internal/config/                 env config
internal/mapdata/                map JSON parsing + validation
internal/auth/password.go        argon2id hash/verify
internal/auth/names.go           username / password / character-name rules
internal/auth/limiter.go         sliding-window attempt limiter
internal/auth/service.go         register / login
internal/store/store.go          pool, migrate
internal/store/accounts.go
internal/store/characters.go
internal/store/storetest/        per-test schema helper
internal/saver/                  async coalescing character-state writer
internal/zone/move.go            Vec2, ClampDir, Step (collision)
internal/zone/zone.go            zone goroutine: join/leave/move/tick/AOI
internal/zone/stats.go           tick duration stats
internal/world/                  maps -> zones, zone selection, online sessions, idle janitor
internal/gateway/outbox.go       per-connection bounded send queue
internal/gateway/conn.go         read/write loops, rate limits, timers, kick, cleanup
internal/gateway/handlers.go     request handlers
internal/gateway/errors.go       error -> protocol code
internal/gateway/server.go       HTTP handler, connection tracking, shutdown
internal/app/                    wiring, /healthz, stats log, graceful shutdown
internal/client/                 Go protocol client (used by botclient and e2e)
e2e/                             end-to-end tests
cmd/gameserver/main.go
cmd/botclient/main.go
README.md
```

Dependency direction (no cycles): `pb` ← `classes` ← `zone` ← `world` ← `gateway` ← `app`; `store` ← `auth`, `saver`, `zone`; `client` → `pb`, `gateway` (for `ProtocolVersion`).

---

### Task 1: Module scaffold, tooling and Postgres

**Files:**
- Create: `go.mod`, `.gitignore`, `Makefile`, `docker-compose.yml`, `deploy/initdb/01-create-test-db.sql`

- [ ] **Step 1: Initialise the module and add the code generators as Go tools**

Run from the repo root:

```bash
go mod init demonslayer2d
go get -tool github.com/bufbuild/buf/cmd/buf@v1.75.0
go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
go tool buf --version
```

Expected: last command prints `1.75.0`.

- [ ] **Step 2: Write `.gitignore`**

```gitignore
/bin/
*.test
*.out
.DS_Store
```

- [ ] **Step 3: Write `docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: ds
      POSTGRES_PASSWORD: ds
      POSTGRES_DB: ds
    ports:
      - "5433:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./deploy/initdb:/docker-entrypoint-initdb.d:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ds -d ds"]
      interval: 2s
      timeout: 3s
      retries: 30

volumes:
  pgdata:
```

- [ ] **Step 4: Write `deploy/initdb/01-create-test-db.sql`**

```sql
CREATE DATABASE ds_test;
```

- [ ] **Step 5: Write `Makefile`** (recipe lines start with a TAB)

```make
DATABASE_URL ?= postgres://ds:ds@localhost:5433/ds?sslmode=disable
TEST_DATABASE_URL ?= postgres://ds:ds@localhost:5433/ds_test?sslmode=disable

.PHONY: proto db-up db-down run vet test test-db

proto:
	go tool buf generate

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

run:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/gameserver

vet:
	go vet ./...

test:
	go test -race ./...

test-db:
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -race ./...
```

- [ ] **Step 6: Start Postgres and check both databases exist**

```bash
make db-up
docker compose exec -T postgres psql -U ds -d ds -c '\l' | grep -E ' ds(_test)? '
```

Expected: two lines, one for `ds` and one for `ds_test`.

- [ ] **Step 7: Commit and push**

```bash
git add go.mod go.sum .gitignore Makefile docker-compose.yml deploy/initdb/01-create-test-db.sql
git commit -m "Scaffold Go module, Makefile and Postgres dev database

Adds buf and protoc-gen-go as Go tool dependencies, a docker-compose
Postgres 17 on port 5433 with a ds_test database for tests, and make
targets for codegen, running and testing."
git push
```

---

### Task 2: Wire protocol and code generation

**Files:**
- Create: `proto/ds/v1/protocol.proto`, `buf.yaml`, `buf.gen.yaml`
- Generated: `internal/pb/protocol.pb.go`

- [ ] **Step 1: Write `buf.yaml`**

```yaml
version: v2
modules:
  - path: proto
lint:
  use:
    - STANDARD
breaking:
  use:
    - FILE
```

- [ ] **Step 2: Write `buf.gen.yaml`**

```yaml
version: v2
plugins:
  - local: ["go", "tool", "protoc-gen-go"]
    out: internal/pb
    opt:
      - module=demonslayer2d/internal/pb
```

- [ ] **Step 3: Write `proto/ds/v1/protocol.proto`**

```proto
syntax = "proto3";

package ds.v1;

option go_package = "demonslayer2d/internal/pb;pb";

// Every WebSocket binary message carries exactly one Envelope.
// Requests use seq >= 1; the response reuses the same seq.
// Server-initiated messages use seq = 0.
message Envelope {
  uint32 seq = 1;
  oneof payload {
    Error error = 2;

    RegisterReq register_req = 10;
    RegisterResp register_resp = 11;
    LoginReq login_req = 12;
    LoginResp login_resp = 13;
    CharListReq char_list_req = 14;
    CharListResp char_list_resp = 15;
    CreateCharReq create_char_req = 16;
    CreateCharResp create_char_resp = 17;
    DeleteCharReq delete_char_req = 18;
    DeleteCharResp delete_char_resp = 19;
    EnterWorldReq enter_world_req = 20;
    EnterWorldResp enter_world_resp = 21;
    LeaveWorldReq leave_world_req = 22;
    LeaveWorldResp leave_world_resp = 23;

    MoveInput move_input = 30;
    EntityEnter entity_enter = 31;
    EntityLeave entity_leave = 32;
    WorldTick world_tick = 33;

    Ping ping = 40;
    Pong pong = 41;
    Kicked kicked = 42;
  }
}

enum ErrorCode {
  ERROR_CODE_UNSPECIFIED = 0;
  ERROR_CODE_BAD_REQUEST = 1;
  ERROR_CODE_VERSION_MISMATCH = 2;
  ERROR_CODE_RATE_LIMITED = 3;
  ERROR_CODE_AUTH_FAILED = 4;
  ERROR_CODE_USERNAME_TAKEN = 5;
  ERROR_CODE_USERNAME_INVALID = 6;
  ERROR_CODE_PASSWORD_INVALID = 7;
  ERROR_CODE_NAME_TAKEN = 8;
  ERROR_CODE_NAME_INVALID = 9;
  ERROR_CODE_SLOT_OCCUPIED = 10;
  ERROR_CODE_CHAR_NOT_FOUND = 11;
  ERROR_CODE_NOT_IN_WORLD = 12;
  ERROR_CODE_ALREADY_IN_WORLD = 13;
  ERROR_CODE_ZONE_FULL = 14;
  ERROR_CODE_INTERNAL = 15;
}

enum KickReason {
  KICK_REASON_UNSPECIFIED = 0;
  KICK_REASON_DUPLICATE_LOGIN = 1;
  KICK_REASON_SERVER_SHUTDOWN = 2;
  KICK_REASON_ZONE_RESTART = 3;
  KICK_REASON_RATE_LIMITED = 4;
  KICK_REASON_IDLE_TIMEOUT = 5;
  KICK_REASON_SLOW_CONSUMER = 6;
  KICK_REASON_PROTOCOL_ERROR = 7;
}

enum CharClass {
  CHAR_CLASS_UNSPECIFIED = 0;
  CHAR_CLASS_VIEM = 1;
  CHAR_CLASS_THUY = 2;
  CHAR_CLASS_MINH = 3;
  CHAR_CLASS_NHAM = 4;
  CHAR_CLASS_PHONG = 5;
}

enum EntityKind {
  ENTITY_KIND_UNSPECIFIED = 0;
  ENTITY_KIND_PLAYER = 1;
}

message Error {
  ErrorCode code = 1;
  string message = 2;
}

message RegisterReq {
  string username = 1;
  string password = 2;
}

message RegisterResp {}

message LoginReq {
  string username = 1;
  string password = 2;
  uint32 protocol_version = 3;
}

message LoginResp {
  int64 account_id = 1;
}

message CharSummary {
  int64 id = 1;
  int32 slot = 2;
  string name = 3;
  CharClass class = 4;
  int32 level = 5;
  string map_id = 6;
}

message CharListReq {}

message CharListResp {
  repeated CharSummary chars = 1;
}

message CreateCharReq {
  int32 slot = 1;
  string name = 2;
  CharClass class = 3;
}

message CreateCharResp {
  CharSummary char = 1;
}

message DeleteCharReq {
  int64 char_id = 1;
  string confirm_name = 2;
}

message DeleteCharResp {}

message EnterWorldReq {
  int64 char_id = 1;
}

message EnterWorldResp {
  uint64 self_entity_id = 1;
  string map_id = 2;
  int32 zone = 3;
  float x = 4;
  float y = 5;
  uint32 server_tick = 6;
  // Tiles per second.
  float move_speed = 7;
}

message LeaveWorldReq {}

message LeaveWorldResp {}

// Direction from a joystick or WASD; length is clamped to 1, (0,0) stops.
message MoveInput {
  uint32 input_seq = 1;
  float dir_x = 2;
  float dir_y = 3;
}

message EntityEnter {
  uint64 id = 1;
  EntityKind kind = 2;
  string name = 3;
  CharClass class = 4;
  int32 level = 5;
  float x = 6;
  float y = 7;
  float dir_x = 8;
  float dir_y = 9;
}

message EntityLeave {
  uint64 id = 1;
}

message EntityMove {
  uint64 id = 1;
  float x = 2;
  float y = 3;
  float dir_x = 4;
  float dir_y = 5;
}

// Sent every tick, even when moves is empty.
message WorldTick {
  uint32 tick = 1;
  // Highest MoveInput.input_seq the server has applied for the receiver.
  uint32 ack_input_seq = 2;
  repeated EntityMove moves = 3;
}

message Ping {
  int64 client_time = 1;
}

message Pong {
  int64 client_time = 1;
  int64 server_time = 2;
}

message Kicked {
  KickReason reason = 1;
}
```

- [ ] **Step 4: Lint and generate**

```bash
go tool buf lint
make proto
go get google.golang.org/protobuf@v1.36.12
go build ./internal/pb
```

Expected: `buf lint` prints nothing; `internal/pb/protocol.pb.go` exists; build succeeds.

- [ ] **Step 5: Commit and push**

```bash
git add buf.yaml buf.gen.yaml proto internal/pb go.mod go.sum
git commit -m "Define the Phase 0 wire protocol in Protobuf

Envelope with request/response seq, auth and character messages,
in-world movement messages (MoveInput, EntityEnter/Leave, WorldTick),
Ping/Pong, Kicked, and the error/kick/class/entity enums. Go code is
generated into internal/pb with buf."
git push
```

---

### Task 3: Class id mapping

**Files:**
- Create: `internal/classes/classes.go`
- Test: `internal/classes/classes_test.go`

- [ ] **Step 1: Write the failing test**

```go
package classes

import (
	"testing"

	"demonslayer2d/internal/pb"
)

func TestRoundTrip(t *testing.T) {
	for _, id := range []string{"viem", "thuy", "minh", "nham", "phong"} {
		c := ToPB(id)
		if c == pb.CharClass_CHAR_CLASS_UNSPECIFIED {
			t.Fatalf("ToPB(%q) = UNSPECIFIED", id)
		}
		back, ok := FromPB(c)
		if !ok || back != id {
			t.Fatalf("FromPB(ToPB(%q)) = %q, %v", id, back, ok)
		}
	}
}

func TestUnknown(t *testing.T) {
	if got := ToPB("lua"); got != pb.CharClass_CHAR_CLASS_UNSPECIFIED {
		t.Fatalf("ToPB(unknown) = %v", got)
	}
	if _, ok := FromPB(pb.CharClass_CHAR_CLASS_UNSPECIFIED); ok {
		t.Fatal("FromPB(UNSPECIFIED) should fail")
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/classes/`
Expected: FAIL, `undefined: ToPB`.

- [ ] **Step 3: Implement**

```go
// Package classes maps character class ids between the database
// (lower-case text) and the protocol enum.
package classes

import "demonslayer2d/internal/pb"

var toPB = map[string]pb.CharClass{
	"viem":  pb.CharClass_CHAR_CLASS_VIEM,
	"thuy":  pb.CharClass_CHAR_CLASS_THUY,
	"minh":  pb.CharClass_CHAR_CLASS_MINH,
	"nham":  pb.CharClass_CHAR_CLASS_NHAM,
	"phong": pb.CharClass_CHAR_CLASS_PHONG,
}

// ToPB returns the protocol enum for a database class id, or UNSPECIFIED.
func ToPB(id string) pb.CharClass { return toPB[id] }

// FromPB returns the database class id for a protocol enum.
func FromPB(c pb.CharClass) (string, bool) {
	for id, v := range toPB {
		if v == c {
			return id, true
		}
	}
	return "", false
}
```

- [ ] **Step 4: Run it and see it pass**

Run: `go test ./internal/classes/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/classes
git commit -m "Add mapping between database class ids and protocol enum"
git push
```

---

### Task 4: Configuration from environment

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/config/`
Expected: FAIL, `undefined: Load`.

- [ ] **Step 3: Implement**

```go
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
		ListenAddr:     ":8080",
		MapsDir:        "./maps",
		StartMap:       "kumotori_1",
		TickRate:       10,
		MoveSpeedTiles: 4,
		AOICellSize:    10,
		ZoneCapacity:   50,
		SaveInterval:   60 * time.Second,
		LogLevel:       slog.LevelInfo,
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
```

- [ ] **Step 4: Run it and see it pass**

Run: `go test ./internal/config/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/config
git commit -m "Load server configuration from environment variables

Defaults follow the Phase 0 spec (10 ticks/s, 4 tiles/s, 10-tile AOI
cells, 50 players per zone, 60s save interval, 5 registrations per IP
per hour); DATABASE_URL is required and invalid values fail fast with
the variable name."
git push
```

---

### Task 5: Map data and the test map

**Files:**
- Create: `internal/mapdata/mapdata.go`, `maps/kumotori_1.json`
- Test: `internal/mapdata/mapdata_test.go`

- [ ] **Step 1: Write the failing test**

```go
package mapdata

import (
	"encoding/json"
	"strings"
	"testing"
)

func mapJSON(t *testing.T, mutate func(m map[string]any)) []byte {
	t.Helper()
	m := map[string]any{
		"id": "t1", "name": "Test", "type": "training", "width": 4, "height": 3,
		"collision":    []string{"####", "#..#", "####"},
		"spawn_points": []map[string]any{{"id": "default", "x": 1.5, "y": 1.5}},
	}
	if mutate != nil {
		mutate(m)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseValid(t *testing.T) {
	m, err := Parse(mapJSON(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "t1" || m.Width != 4 || m.Height != 3 {
		t.Fatalf("bad header: %+v", m)
	}
	if !m.Blocked(0, 0) || m.Blocked(1, 1) || m.Blocked(2, 1) || !m.Blocked(3, 1) {
		t.Fatal("collision grid mismatch")
	}
	if got := m.Spawns["default"]; got != (Point{1.5, 1.5}) {
		t.Fatalf("default spawn = %+v", got)
	}
}

func TestBlockedOutOfBounds(t *testing.T) {
	m, err := Parse(mapJSON(t, func(m map[string]any) { m["collision"] = []string{"....", "....", "...."} }))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]int{{-1, 0}, {0, -1}, {4, 0}, {0, 3}} {
		if !m.Blocked(c[0], c[1]) {
			t.Errorf("Blocked(%d,%d) = false, want true outside the map", c[0], c[1])
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]func(m map[string]any){
		"missing id":       func(m map[string]any) { m["id"] = "" },
		"invalid type":     func(m map[string]any) { m["type"] = "castle" },
		"rows":             func(m map[string]any) { m["collision"] = []string{"####", "#..#"} },
		"row 1 has length": func(m map[string]any) { m["collision"] = []string{"####", "#...#", "####"} },
		"invalid char":     func(m map[string]any) { m["collision"] = []string{"####", "#.x#", "####"} },
		"not walkable": func(m map[string]any) {
			m["spawn_points"] = []map[string]any{{"id": "default", "x": 0.5, "y": 0.5}}
		},
		"default spawn": func(m map[string]any) {
			m["spawn_points"] = []map[string]any{{"id": "gate", "x": 1.5, "y": 1.5}}
		},
		"duplicate spawn": func(m map[string]any) {
			m["spawn_points"] = []map[string]any{
				{"id": "default", "x": 1.5, "y": 1.5}, {"id": "default", "x": 2.5, "y": 1.5},
			}
		},
		"unknown field": func(m map[string]any) { m["portals"] = []string{} },
	}
	for want, mutate := range cases {
		_, err := Parse(mapJSON(t, mutate))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got err %v", want, err)
		}
	}
}

func TestLoadDirRepoMaps(t *testing.T) {
	maps, err := LoadDir("../../maps")
	if err != nil {
		t.Fatal(err)
	}
	m, ok := maps["kumotori_1"]
	if !ok {
		t.Fatal("kumotori_1 not loaded")
	}
	if m.Width != 64 || m.Height != 48 || !m.Walkable(m.Spawns["default"]) {
		t.Fatalf("unexpected kumotori_1: %dx%d", m.Width, m.Height)
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/mapdata/`
Expected: FAIL, `undefined: Parse`.

- [ ] **Step 3: Implement `internal/mapdata/mapdata.go`**

```go
// Package mapdata loads map files: size, tile collision grid and spawn points.
package mapdata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Point is a position in tile units; (12.5, 3.5) is the centre of tile (12, 3).
type Point struct{ X, Y float32 }

type Map struct {
	ID     string
	Name   string
	Type   string
	Width  int
	Height int
	Spawns map[string]Point

	blocked []bool
}

var validTypes = map[string]bool{"town": true, "training": true, "battle": true, "instance": true}

type spawnFile struct {
	ID string  `json:"id"`
	X  float32 `json:"x"`
	Y  float32 `json:"y"`
}

type mapFile struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Collision   []string    `json:"collision"`
	SpawnPoints []spawnFile `json:"spawn_points"`
}

// Parse decodes and validates one map file.
func Parse(data []byte) (*Map, error) {
	var f mapFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("mapdata: decode (unknown field?): %w", err)
	}
	if f.ID == "" {
		return nil, errors.New("mapdata: missing id")
	}
	if !validTypes[f.Type] {
		return nil, fmt.Errorf("mapdata %s: invalid type %q", f.ID, f.Type)
	}
	if f.Width <= 0 || f.Height <= 0 {
		return nil, fmt.Errorf("mapdata %s: width and height must be positive", f.ID)
	}
	if len(f.Collision) != f.Height {
		return nil, fmt.Errorf("mapdata %s: collision has %d rows, want %d", f.ID, len(f.Collision), f.Height)
	}
	m := &Map{
		ID: f.ID, Name: f.Name, Type: f.Type, Width: f.Width, Height: f.Height,
		Spawns:  make(map[string]Point),
		blocked: make([]bool, f.Width*f.Height),
	}
	for y, row := range f.Collision {
		if len(row) != f.Width {
			return nil, fmt.Errorf("mapdata %s: collision row %d has length %d, want %d", f.ID, y, len(row), f.Width)
		}
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case '.':
			case '#':
				m.blocked[y*f.Width+x] = true
			default:
				return nil, fmt.Errorf("mapdata %s: invalid char %q at (%d,%d)", f.ID, row[x], x, y)
			}
		}
	}
	for _, sp := range f.SpawnPoints {
		if sp.ID == "" {
			return nil, fmt.Errorf("mapdata %s: spawn point without id", f.ID)
		}
		if _, dup := m.Spawns[sp.ID]; dup {
			return nil, fmt.Errorf("mapdata %s: duplicate spawn point %q", f.ID, sp.ID)
		}
		p := Point{sp.X, sp.Y}
		if !m.Walkable(p) {
			return nil, fmt.Errorf("mapdata %s: spawn point %q at (%.2f, %.2f) is not walkable", f.ID, sp.ID, sp.X, sp.Y)
		}
		m.Spawns[sp.ID] = p
	}
	if _, ok := m.Spawns["default"]; !ok {
		return nil, fmt.Errorf("mapdata %s: missing default spawn point", f.ID)
	}
	return m, nil
}

// Blocked reports whether tile (tx, ty) blocks movement. Tiles outside the map are blocked.
func (m *Map) Blocked(tx, ty int) bool {
	if tx < 0 || ty < 0 || tx >= m.Width || ty >= m.Height {
		return true
	}
	return m.blocked[ty*m.Width+tx]
}

// Walkable reports whether the tile containing p is walkable.
func (m *Map) Walkable(p Point) bool {
	x, y := float64(p.X), float64(p.Y)
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return false
	}
	return !m.Blocked(int(math.Floor(x)), int(math.Floor(y)))
}

// LoadDir parses every *.json file in dir and indexes the maps by id.
func LoadDir(dir string) (map[string]*Map, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("mapdata: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("mapdata: no map files in %s", dir)
	}
	out := make(map[string]*Map, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("mapdata: %w", err)
		}
		m, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if _, dup := out[m.ID]; dup {
			return nil, fmt.Errorf("mapdata: duplicate map id %q in %s", m.ID, p)
		}
		out[m.ID] = m
	}
	return out, nil
}
```

- [ ] **Step 4: Write `maps/kumotori_1.json`**

Border walls, a horizontal wall at row 20 (columns 10–40, gap at 25–27) and a 4×4 block at columns 45–48, rows 30–33.

```json
{
  "id": "kumotori_1",
  "name": "Kumotori – Đường núi tuyết",
  "type": "training",
  "width": 64,
  "height": 48,
  "collision": [
    "################################################################",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#.........###############...#############......................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#............................................####..............#",
    "#............................................####..............#",
    "#............................................####..............#",
    "#............................................####..............#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "#..............................................................#",
    "################################################################"
  ],
  "spawn_points": [
    { "id": "default", "x": 5.5, "y": 5.5 }
  ]
}
```

- [ ] **Step 5: Run the tests and see them pass**

Run: `go test ./internal/mapdata/`
Expected: `ok`.

- [ ] **Step 6: Commit and push**

```bash
git add internal/mapdata maps
git commit -m "Parse and validate map files; add the kumotori_1 test map

Maps are JSON with a '.'/'#' collision grid and named spawn points.
Parsing rejects unknown fields, size mismatches, invalid characters,
unwalkable spawns and maps without a default spawn. Tiles outside the
map count as blocked."
git push
```

---

### Task 6: Password hashing (argon2id)

**Files:**
- Create: `internal/auth/password.go`
- Test: `internal/auth/password_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"errors"
	"strings"
	"testing"
)

var fastParams = Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse", fastParams)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Fatalf("unexpected encoding %q", h)
	}
	ok, err := VerifyPassword("correct horse", h)
	if err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong horse", h)
	if err != nil || ok {
		t.Fatalf("verify wrong password: ok=%v err=%v", ok, err)
	}
}

func TestHashUsesRandomSalt(t *testing.T) {
	a, _ := HashPassword("same", fastParams)
	b, _ := HashPassword("same", fastParams)
	if a == b {
		t.Fatal("two hashes of the same password are identical")
	}
}

func TestVerifyMalformed(t *testing.T) {
	for _, h := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$AAAA$AAAA", "$argon2id$v=19$m=x$AAAA$AAAA", "$argon2id$v=19$m=8,t=1,p=1$!!$AAAA"} {
		if _, err := VerifyPassword("x", h); !errors.Is(err, ErrBadHash) {
			t.Errorf("VerifyPassword(%q) err = %v, want ErrBadHash", h, err)
		}
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/auth/`
Expected: FAIL, `undefined: Params`.

- [ ] **Step 3: Implement**

```go
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params are argon2id cost parameters. Memory is in KiB.
type Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

// DefaultParams: 64 MiB, 3 passes, 2 lanes, 16-byte salt, 32-byte key.
var DefaultParams = Params{Memory: 64 * 1024, Iterations: 3, Parallelism: 2, SaltLen: 16, KeyLen: 32}

var ErrBadHash = errors.New("auth: malformed password hash")

var b64 = base64.RawStdEncoding

// HashPassword returns a PHC-format argon2id hash including parameters and salt.
func HashPassword(password string, p Params) (string, error) {
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword checks password against an encoded hash in constant time.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, ErrBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, ErrBadHash
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m == 0 || t == 0 || p == 0 {
		return false, ErrBadHash
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false, ErrBadHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, ErrBadHash
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
```

- [ ] **Step 4: Fetch the dependency and run the tests**

```bash
go get golang.org/x/crypto@v0.59.0
go test ./internal/auth/
```

Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/auth/password.go internal/auth/password_test.go go.mod go.sum
git commit -m "Hash passwords with argon2id in PHC format

Default cost is 64 MiB, 3 passes, 2 lanes. Parameters and salt are
stored in the hash so they can change later without breaking existing
accounts; verification is constant time."
git push
```

---

### Task 7: Username, password and character-name rules

**Files:**
- Create: `internal/auth/names.go`
- Test: `internal/auth/names_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestNormalizeUsername(t *testing.T) {
	ok := map[string]string{"Tanjiro": "tanjiro", "abcd": "abcd", "user_01": "user_01", strings.Repeat("a", 20): strings.Repeat("a", 20)}
	for in, want := range ok {
		got, err := NormalizeUsername(in)
		if err != nil || got != want {
			t.Errorf("NormalizeUsername(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"abc", strings.Repeat("a", 21), "has space", "dấu", "a-b-c", ""} {
		if _, err := NormalizeUsername(in); !errors.Is(err, ErrUsernameInvalid) {
			t.Errorf("NormalizeUsername(%q) err = %v, want ErrUsernameInvalid", in, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("12345678"); err != nil {
		t.Errorf("8 chars: %v", err)
	}
	if err := ValidatePassword(strings.Repeat("ư", 64)); err != nil {
		t.Errorf("64 runes: %v", err)
	}
	for _, p := range []string{"1234567", strings.Repeat("x", 65)} {
		if err := ValidatePassword(p); !errors.Is(err, ErrPasswordInvalid) {
			t.Errorf("ValidatePassword(len %d) err = %v", len(p), err)
		}
	}
}

func TestNormalizeCharName(t *testing.T) {
	d, k, err := NormalizeCharName("Thủy")
	if err != nil || d != "Thủy" || k != "thủy" {
		t.Fatalf("Thủy -> %q %q %v", d, k, err)
	}
	// Decomposed input (u + combining hook + ...) normalises to the same name.
	d2, k2, err := NormalizeCharName(norm.NFD.String("Thủy"))
	if err != nil || d2 != d || k2 != k {
		t.Fatalf("NFD input -> %q %q %v", d2, k2, err)
	}
	for _, ok := range []string{"Ab", "Tanjiro2", strings.Repeat("ê", 16)} {
		if _, _, err := NormalizeCharName(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"A", strings.Repeat("a", 17), "có space", "x_y", "bad!", ""} {
		if _, _, err := NormalizeCharName(bad); !errors.Is(err, ErrNameInvalid) {
			t.Errorf("%q err = %v, want ErrNameInvalid", bad, err)
		}
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go get golang.org/x/text@v0.44.0 && go test ./internal/auth/`
Expected: FAIL, `undefined: NormalizeUsername`.

- [ ] **Step 3: Implement**

```go
package auth

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrUsernameInvalid = errors.New("auth: invalid username")
	ErrPasswordInvalid = errors.New("auth: invalid password")
	ErrNameInvalid     = errors.New("auth: invalid character name")
)

// NormalizeUsername lower-cases s and checks it is 4-20 chars of a-z, 0-9, _.
func NormalizeUsername(s string) (string, error) {
	u := strings.ToLower(s)
	if len(u) < 4 || len(u) > 20 {
		return "", ErrUsernameInvalid
	}
	for i := 0; i < len(u); i++ {
		c := u[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return "", ErrUsernameInvalid
		}
	}
	return u, nil
}

// ValidatePassword checks the password is 8-64 characters.
func ValidatePassword(p string) error {
	if n := utf8.RuneCountInString(p); n < 8 || n > 64 {
		return ErrPasswordInvalid
	}
	return nil
}

// NormalizeCharName returns the NFC display name and its lower-case uniqueness key.
// A valid name is 2-16 letters or digits (Vietnamese letters allowed), no spaces or symbols.
func NormalizeCharName(s string) (display, key string, err error) {
	d := norm.NFC.String(s)
	if n := utf8.RuneCountInString(d); n < 2 || n > 16 {
		return "", "", ErrNameInvalid
	}
	for _, r := range d {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return "", "", ErrNameInvalid
		}
	}
	return d, strings.ToLower(d), nil
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `go test ./internal/auth/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/auth/names.go internal/auth/names_test.go go.mod go.sum
git commit -m "Validate usernames, passwords and character names

Usernames are 4-20 chars of a-z0-9_ stored lower-case. Character names
are NFC-normalised so the same Vietnamese name typed two ways is one
name, 2-16 letters or digits, with a lower-case key for uniqueness."
git push
```

---

### Task 8: Attempt limiter

**Files:**
- Create: `internal/auth/limiter.go`
- Test: `internal/auth/limiter_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"testing"
	"time"
)

func TestLimiterWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	if !l.Allow("k") {
		t.Fatal("fresh key should be allowed")
	}
	l.Hit("k")
	l.Hit("k")
	if l.Allow("k") {
		t.Fatal("should be blocked after 2 hits")
	}
	if !l.Allow("other") {
		t.Fatal("keys must be independent")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("k") {
		t.Fatal("should be allowed after the window passes")
	}
	if n := len(l.hits); n != 0 {
		t.Fatalf("expired keys should be pruned, %d left", n)
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/auth/ -run TestLimiter`
Expected: FAIL, `undefined: NewLimiter`.

- [ ] **Step 3: Implement**

```go
package auth

import (
	"sync"
	"time"
)

// Limiter counts attempts per key over a sliding window.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string][]time.Time
	calls  int
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, now: time.Now, hits: make(map[string][]time.Time)}
}

// Allow reports whether key has fewer than max hits in the window. It does not record a hit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key)) < l.max
}

// Hit records one attempt for key.
func (l *Limiter) Hit(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.prune(key), l.now())
	l.calls++
	if l.calls%1024 == 0 {
		for k := range l.hits {
			l.prune(k)
		}
	}
}

func (l *Limiter) prune(key string) []time.Time {
	h := l.hits[key]
	cutoff := l.now().Add(-l.window)
	i := 0
	for i < len(h) && !h[i].After(cutoff) {
		i++
	}
	h = h[i:]
	if len(h) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = h
	return h
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `go test ./internal/auth/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/auth/limiter.go internal/auth/limiter_test.go
git commit -m "Add a sliding-window attempt limiter for login and registration"
git push
```

---

### Task 9: Store foundation, migration and test schema helper

**Files:**
- Create: `migrations/embed.go`, `migrations/00001_init.sql`, `internal/store/store.go`, `internal/store/storetest/storetest.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Write `migrations/00001_init.sql`**

```sql
-- +goose Up
CREATE TABLE accounts (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      text        NOT NULL,
    password_hash text        NOT NULL,
    status        text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'banned')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz,
    CONSTRAINT accounts_username_key UNIQUE (username)
);

CREATE TABLE characters (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint      NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    slot       smallint    NOT NULL CHECK (slot BETWEEN 1 AND 3),
    name       text        NOT NULL,
    name_key   text        NOT NULL,
    class      text        NOT NULL CHECK (class IN ('viem', 'thuy', 'minh', 'nham', 'phong')),
    level      int         NOT NULL DEFAULT 1 CHECK (level >= 1),
    exp        bigint      NOT NULL DEFAULT 0,
    map_id     text        NOT NULL,
    pos_x      real        NOT NULL,
    pos_y      real        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT characters_account_slot_key UNIQUE (account_id, slot),
    CONSTRAINT characters_name_key_key UNIQUE (name_key)
);

-- +goose Down
DROP TABLE characters;
DROP TABLE accounts;
```

- [ ] **Step 2: Write `migrations/embed.go`**

```go
// Package migrations embeds the SQL migrations applied by goose at startup.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 3: Write the failing test `internal/store/store_test.go`**

```go
package store_test

import (
	"context"
	"testing"

	"demonslayer2d/internal/store/storetest"
)

func TestMigrateIsIdempotent(t *testing.T) {
	st := storetest.New(t) // already migrated once
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if err := st.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 4: Run it and see it fail**

Run: `TEST_DATABASE_URL=postgres://ds:ds@localhost:5433/ds_test?sslmode=disable go test ./internal/store/`
Expected: FAIL, package `storetest` not found.

- [ ] **Step 5: Implement `internal/store/store.go`**

```go
// Package store persists accounts and characters in Postgres.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"demonslayer2d/migrations"
)

type Store struct {
	pool *pgxpool.Pool
}

// Open connects to the database at url.
func Open(ctx context.Context, url string) (*Store, error) {
	return OpenSchema(ctx, url, "")
}

// OpenSchema connects with search_path set to schema (used by tests). Empty schema keeps the default.
func OpenSchema(ctx context.Context, url, schema string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("store: parse url: %w", err)
	}
	if schema != "" {
		cfg.ConnConfig.RuntimeParams["search_path"] = schema
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate applies all pending migrations.
func (s *Store) Migrate(ctx context.Context) error {
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("store: migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("store: migrate up: %w", err)
	}
	return nil
}

// uniqueViolation returns the violated constraint name for a unique-violation error.
func uniqueViolation(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName, true
	}
	return "", false
}
```

- [ ] **Step 6: Implement `internal/store/storetest/storetest.go`**

```go
// Package storetest gives each test its own migrated Postgres schema.
package storetest

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"demonslayer2d/internal/store"
)

// New returns a migrated store in a fresh schema, dropped when the test ends.
// The test is skipped when TEST_DATABASE_URL is not set.
func New(t testing.TB) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("t_%d_%d", time.Now().UnixNano(), rand.IntN(1_000_000))
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("storetest: connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close(ctx)
		t.Fatalf("storetest: create schema: %v", err)
	}
	st, err := store.OpenSchema(ctx, url, schema)
	if err != nil {
		t.Fatalf("storetest: open: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("storetest: drop schema: %v", err)
		}
		admin.Close(ctx)
	})
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("storetest: migrate: %v", err)
	}
	return st
}
```

- [ ] **Step 7: Fetch dependencies and run the test**

```bash
go get github.com/jackc/pgx/v5@v5.13.0 github.com/pressly/goose/v3@v3.30.1
go mod tidy
TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test ./internal/store/
go test ./internal/store/
```

Expected: first run `ok`; second run (no env var) `ok` with the test skipped.

- [ ] **Step 8: Commit and push**

```bash
git add migrations internal/store go.mod go.sum
git commit -m "Add Postgres store with embedded goose migrations

Creates the Phase 0 accounts and characters tables. Character names
are unique on a name_key column computed in Go. storetest gives every
test its own migrated schema and skips when TEST_DATABASE_URL is unset."
git push
```

---

### Task 10: Accounts in the store

**Files:**
- Create: `internal/store/accounts.go`
- Test: `internal/store/accounts_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store_test

import (
	"context"
	"errors"
	"testing"

	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
)

func TestAccounts(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()

	id, err := st.CreateAccount(ctx, "tanjiro", "hash1")
	if err != nil || id <= 0 {
		t.Fatalf("create: id=%d err=%v", id, err)
	}
	if _, err := st.CreateAccount(ctx, "tanjiro", "hash2"); !errors.Is(err, store.ErrUsernameTaken) {
		t.Fatalf("duplicate username err = %v", err)
	}
	a, err := st.GetAccountByUsername(ctx, "tanjiro")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != id || a.PasswordHash != "hash1" || a.Status != "active" {
		t.Fatalf("got %+v", a)
	}
	if _, err := st.GetAccountByUsername(ctx, "nezuko"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing account err = %v", err)
	}
	if err := st.TouchLogin(ctx, id); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test ./internal/store/`
Expected: FAIL, `st.CreateAccount undefined`.

- [ ] **Step 3: Implement**

```go
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound      = errors.New("store: not found")
	ErrUsernameTaken = errors.New("store: username taken")
	ErrNameTaken     = errors.New("store: character name taken")
	ErrSlotOccupied  = errors.New("store: character slot occupied")
)

type Account struct {
	ID           int64
	Username     string
	PasswordHash string
	Status       string
}

func (s *Store) CreateAccount(ctx context.Context, username, passwordHash string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO accounts (username, password_hash) VALUES ($1, $2) RETURNING id`,
		username, passwordHash).Scan(&id)
	if c, ok := uniqueViolation(err); ok && c == "accounts_username_key" {
		return 0, ErrUsernameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("store: create account: %w", err)
	}
	return id, nil
}

func (s *Store) GetAccountByUsername(ctx context.Context, username string) (Account, error) {
	var a Account
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, password_hash, status FROM accounts WHERE username = $1`,
		username).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("store: get account: %w", err)
	}
	return a, nil
}

func (s *Store) TouchLogin(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `UPDATE accounts SET last_login_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("store: touch login: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test ./internal/store/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/store/accounts.go internal/store/accounts_test.go
git commit -m "Store accounts: create, look up by username, record last login"
git push
```

---

### Task 11: Characters in the store

**Files:**
- Create: `internal/store/characters.go`
- Test: `internal/store/characters_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store_test

import (
	"context"
	"errors"
	"testing"

	"demonslayer2d/internal/store"
	"demonslayer2d/internal/store/storetest"
)

func newChar(acc int64, slot int, name string) store.NewCharacter {
	return store.NewCharacter{AccountID: acc, Slot: slot, Name: name, NameKey: name + "_key", Class: "viem", MapID: "kumotori_1", X: 5.5, Y: 5.5}
}

func TestCharacters(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	acc, _ := st.CreateAccount(ctx, "owner", "h")
	other, _ := st.CreateAccount(ctx, "other", "h")

	c2, err := st.CreateCharacter(ctx, newChar(acc, 2, "Zenitsu"))
	if err != nil {
		t.Fatal(err)
	}
	if c2.ID <= 0 || c2.Level != 1 || c2.Exp != 0 || c2.X != 5.5 || c2.Class != "viem" {
		t.Fatalf("created %+v", c2)
	}
	c1, err := st.CreateCharacter(ctx, newChar(acc, 1, "Tanjiro"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := st.CreateCharacter(ctx, newChar(other, 1, "Zenitsu")); !errors.Is(err, store.ErrNameTaken) {
		t.Fatalf("duplicate name err = %v", err)
	}
	if _, err := st.CreateCharacter(ctx, newChar(acc, 1, "Inosuke")); !errors.Is(err, store.ErrSlotOccupied) {
		t.Fatalf("occupied slot err = %v", err)
	}

	list, err := st.ListCharacters(ctx, acc)
	if err != nil || len(list) != 2 || list[0].ID != c1.ID || list[1].ID != c2.ID {
		t.Fatalf("list = %+v, %v (want ordered by slot)", list, err)
	}

	if _, err := st.GetCharacter(ctx, other, c1.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get with wrong account err = %v", err)
	}

	if err := st.SaveCharacterState(ctx, store.CharState{ID: c1.ID, MapID: "kumotori_1", X: 10.25, Y: 7.75, Level: 3, Exp: -5}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCharacter(ctx, acc, c1.ID)
	if err != nil || got.X != 10.25 || got.Y != 7.75 || got.Level != 3 || got.Exp != -5 {
		t.Fatalf("after save: %+v, %v", got, err)
	}

	if err := st.DeleteCharacter(ctx, acc, c2.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCharacter(ctx, acc, c2.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
	if err := st.SaveCharacterState(ctx, store.CharState{ID: c2.ID, MapID: "kumotori_1"}); err != nil {
		t.Fatalf("saving a deleted character must be a no-op, got %v", err)
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test ./internal/store/`
Expected: FAIL, `undefined: store.NewCharacter`.

- [ ] **Step 3: Implement**

```go
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type Character struct {
	ID        int64
	AccountID int64
	Slot      int
	Name      string
	Class     string
	Level     int
	Exp       int64
	MapID     string
	X, Y      float32
}

type NewCharacter struct {
	AccountID int64
	Slot      int
	Name      string
	NameKey   string
	Class     string
	MapID     string
	X, Y      float32
}

// CharState is the part of a character the game loop changes and saves.
type CharState struct {
	ID    int64
	MapID string
	X, Y  float32
	Level int
	Exp   int64
}

const charColumns = `id, account_id, slot, name, class, level, exp, map_id, pos_x, pos_y`

func scanCharacter(row pgx.Row) (Character, error) {
	var c Character
	err := row.Scan(&c.ID, &c.AccountID, &c.Slot, &c.Name, &c.Class, &c.Level, &c.Exp, &c.MapID, &c.X, &c.Y)
	return c, err
}

func (s *Store) ListCharacters(ctx context.Context, accountID int64) ([]Character, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+charColumns+` FROM characters WHERE account_id = $1 ORDER BY slot`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list characters: %w", err)
	}
	chars, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Character, error) { return scanCharacter(r) })
	if err != nil {
		return nil, fmt.Errorf("store: list characters: %w", err)
	}
	return chars, nil
}

func (s *Store) GetCharacter(ctx context.Context, accountID, charID int64) (Character, error) {
	c, err := scanCharacter(s.pool.QueryRow(ctx,
		`SELECT `+charColumns+` FROM characters WHERE id = $1 AND account_id = $2`, charID, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Character{}, ErrNotFound
	}
	if err != nil {
		return Character{}, fmt.Errorf("store: get character: %w", err)
	}
	return c, nil
}

func (s *Store) CreateCharacter(ctx context.Context, nc NewCharacter) (Character, error) {
	c, err := scanCharacter(s.pool.QueryRow(ctx,
		`INSERT INTO characters (account_id, slot, name, name_key, class, map_id, pos_x, pos_y)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+charColumns,
		nc.AccountID, nc.Slot, nc.Name, nc.NameKey, nc.Class, nc.MapID, nc.X, nc.Y))
	if constraint, ok := uniqueViolation(err); ok {
		switch constraint {
		case "characters_name_key_key":
			return Character{}, ErrNameTaken
		case "characters_account_slot_key":
			return Character{}, ErrSlotOccupied
		}
	}
	if err != nil {
		return Character{}, fmt.Errorf("store: create character: %w", err)
	}
	return c, nil
}

func (s *Store) DeleteCharacter(ctx context.Context, accountID, charID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM characters WHERE id = $1 AND account_id = $2`, charID, accountID)
	if err != nil {
		return fmt.Errorf("store: delete character: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveCharacterState writes position, level and EXP. A missing character is not an error.
func (s *Store) SaveCharacterState(ctx context.Context, st CharState) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE characters SET map_id = $2, pos_x = $3, pos_y = $4, level = $5, exp = $6, updated_at = now() WHERE id = $1`,
		st.ID, st.MapID, st.X, st.Y, st.Level, st.Exp)
	if err != nil {
		return fmt.Errorf("store: save character state: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `TEST_DATABASE_URL='postgres://ds:ds@localhost:5433/ds_test?sslmode=disable' go test ./internal/store/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/store/characters.go internal/store/characters_test.go
git commit -m "Store characters: create, list, get, delete and save game state

Maps unique-constraint violations to ErrNameTaken / ErrSlotOccupied,
scopes reads and deletes to the owning account, and treats saving a
deleted character as a no-op."
git push
```

---

### Task 12: Auth service (register and login)

**Files:**
- Create: `internal/auth/service.go`
- Test: `internal/auth/service_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"context"
	"errors"
	"sync"
	"testing"

	"demonslayer2d/internal/store"
)

type fakeAccounts struct {
	mu     sync.Mutex
	byName map[string]store.Account
	nextID int64
}

func newFakeAccounts() *fakeAccounts { return &fakeAccounts{byName: map[string]store.Account{}} }

func (f *fakeAccounts) CreateAccount(_ context.Context, u, h string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byName[u]; ok {
		return 0, store.ErrUsernameTaken
	}
	f.nextID++
	f.byName[u] = store.Account{ID: f.nextID, Username: u, PasswordHash: h, Status: "active"}
	return f.nextID, nil
}

func (f *fakeAccounts) GetAccountByUsername(_ context.Context, u string) (store.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.byName[u]
	if !ok {
		return store.Account{}, store.ErrNotFound
	}
	return a, nil
}

func (f *fakeAccounts) TouchLogin(context.Context, int64) error { return nil }

func TestRegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	svc, err := NewService(newFakeAccounts(), fastParams, 5)
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Register(ctx, "1.1.1.1", "Tanjiro", "kamado123")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Login(ctx, "1.1.1.1", "TANJIRO", "kamado123")
	if err != nil || got != id {
		t.Fatalf("login: %d %v", got, err)
	}
	if _, err := svc.Register(ctx, "1.1.1.1", "tanjiro", "kamado123"); !errors.Is(err, store.ErrUsernameTaken) {
		t.Fatalf("duplicate register err = %v", err)
	}
	if _, err := svc.Register(ctx, "1.1.1.1", "ab", "kamado123"); !errors.Is(err, ErrUsernameInvalid) {
		t.Fatalf("bad username err = %v", err)
	}
	if _, err := svc.Register(ctx, "1.1.1.1", "nezuko", "short"); !errors.Is(err, ErrPasswordInvalid) {
		t.Fatalf("bad password err = %v", err)
	}
}

func TestLoginFailuresLookTheSame(t *testing.T) {
	ctx := context.Background()
	svc, _ := NewService(newFakeAccounts(), fastParams, 5)
	_, _ = svc.Register(ctx, "ip", "tanjiro", "kamado123")
	if _, err := svc.Login(ctx, "ip", "tanjiro", "wrongpass"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("wrong password err = %v", err)
	}
	if _, err := svc.Login(ctx, "ip", "nobody", "whatever1"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("unknown user err = %v", err)
	}
	if _, err := svc.Login(ctx, "ip", "x", "whatever1"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("invalid username err = %v", err)
	}
}

func TestBannedAccountCannotLogin(t *testing.T) {
	ctx := context.Background()
	accs := newFakeAccounts()
	svc, _ := NewService(accs, fastParams, 5)
	_, _ = svc.Register(ctx, "ip", "banned", "kamado123")
	a := accs.byName["banned"]
	a.Status = "banned"
	accs.byName["banned"] = a
	if _, err := svc.Login(ctx, "ip", "banned", "kamado123"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("banned login err = %v", err)
	}
}

func TestLoginRateLimit(t *testing.T) {
	ctx := context.Background()
	svc, _ := NewService(newFakeAccounts(), fastParams, 5)
	_, _ = svc.Register(ctx, "ip", "tanjiro", "kamado123")
	for i := 0; i < 5; i++ {
		_, _ = svc.Login(ctx, "ip", "tanjiro", "wrongpass")
	}
	if _, err := svc.Login(ctx, "ip", "tanjiro", "kamado123"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("6th attempt err = %v, want ErrRateLimited", err)
	}
	if _, err := svc.Login(ctx, "other-ip", "tanjiro", "kamado123"); err != nil {
		t.Fatalf("other IP should not be limited: %v", err)
	}
}

func TestRegisterRateLimit(t *testing.T) {
	ctx := context.Background()
	svc, _ := NewService(newFakeAccounts(), fastParams, 5)
	for i := 0; i < 5; i++ {
		if _, err := svc.Register(ctx, "ip", "user"+string(rune('a'+i))+"xx", "kamado123"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Register(ctx, "ip", "userfxx", "kamado123"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("6th registration err = %v", err)
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/auth/`
Expected: FAIL, `undefined: NewService`.

- [ ] **Step 3: Implement**

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"demonslayer2d/internal/store"
)

var (
	ErrAuthFailed  = errors.New("auth: authentication failed")
	ErrRateLimited = errors.New("auth: rate limited")
)

type AccountStore interface {
	CreateAccount(ctx context.Context, username, passwordHash string) (int64, error)
	GetAccountByUsername(ctx context.Context, username string) (store.Account, error)
	TouchLogin(ctx context.Context, id int64) error
}

type Service struct {
	store      AccountStore
	params     Params
	sem        chan struct{} // bounds concurrent argon2 work to the CPU count
	dummyHash  string        // verified for unknown users so timing does not reveal existence
	loginFails *Limiter      // 5 failures per minute per ip|username
	registers  *Limiter      // registrations per hour per ip
}

func NewService(st AccountStore, p Params, registersPerHour int) (*Service, error) {
	dummy, err := HashPassword("dummy-password-for-timing", p)
	if err != nil {
		return nil, err
	}
	return &Service{
		store:      st,
		params:     p,
		sem:        make(chan struct{}, runtime.NumCPU()),
		dummyHash:  dummy,
		loginFails: NewLimiter(5, time.Minute),
		registers:  NewLimiter(registersPerHour, time.Hour),
	}, nil
}

func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) release() { <-s.sem }

// Register creates an account and returns its id.
func (s *Service) Register(ctx context.Context, ip, username, password string) (int64, error) {
	u, err := NormalizeUsername(username)
	if err != nil {
		return 0, err
	}
	if err := ValidatePassword(password); err != nil {
		return 0, err
	}
	if !s.registers.Allow(ip) {
		return 0, ErrRateLimited
	}
	s.registers.Hit(ip)
	if err := s.acquire(ctx); err != nil {
		return 0, err
	}
	hash, err := HashPassword(password, s.params)
	s.release()
	if err != nil {
		return 0, fmt.Errorf("auth: hash: %w", err)
	}
	return s.store.CreateAccount(ctx, u, hash)
}

// Login checks credentials and returns the account id.
// Unknown user, wrong password and banned account all return ErrAuthFailed.
func (s *Service) Login(ctx context.Context, ip, username, password string) (int64, error) {
	u, err := NormalizeUsername(username)
	if err != nil {
		return 0, ErrAuthFailed
	}
	key := ip + "|" + u
	if !s.loginFails.Allow(key) {
		return 0, ErrRateLimited
	}
	acc, err := s.store.GetAccountByUsername(ctx, u)
	notFound := errors.Is(err, store.ErrNotFound)
	if err != nil && !notFound {
		return 0, err
	}
	hash := acc.PasswordHash
	if notFound {
		hash = s.dummyHash
	}
	if err := s.acquire(ctx); err != nil {
		return 0, err
	}
	ok, err := VerifyPassword(password, hash)
	s.release()
	if err != nil {
		return 0, fmt.Errorf("auth: verify: %w", err)
	}
	if notFound || !ok || acc.Status != "active" {
		s.loginFails.Hit(key)
		return 0, ErrAuthFailed
	}
	if err := s.store.TouchLogin(ctx, acc.ID); err != nil {
		return 0, err
	}
	return acc.ID, nil
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `go test -race ./internal/auth/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/auth/service.go internal/auth/service_test.go
git commit -m "Add register/login service with rate limits

Login returns the same ErrAuthFailed for unknown users, wrong passwords
and banned accounts, and verifies a dummy hash for unknown users so
timing does not reveal which accounts exist. Argon2 work is bounded to
the CPU count. Limits: 5 failed logins/min per IP+username and a
configurable number of registrations/hour per IP."
git push
```

---

### Task 13: Saver (asynchronous character-state writer)

**Files:**
- Create: `internal/saver/saver.go`
- Test: `internal/saver/saver_test.go`

- [ ] **Step 1: Write the failing test**

```go
package saver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"demonslayer2d/internal/store"
)

type fakeWriter struct {
	mu     sync.Mutex
	writes []store.CharState
	fail   int // fail this many writes first
}

func (f *fakeWriter) SaveCharacterState(_ context.Context, st store.CharState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail > 0 {
		f.fail--
		return errors.New("db down")
	}
	f.writes = append(f.writes, st)
	return nil
}

func (f *fakeWriter) snapshot() []store.CharState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.CharState(nil), f.writes...)
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func waitErr(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for save result")
		return nil
	}
}

func TestSaveNotifiesWaiter(t *testing.T) {
	w := &fakeWriter{}
	s := New(w, quiet())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	done := make(chan error, 1)
	s.Save(store.CharState{ID: 1, X: 2}, done)
	if err := waitErr(t, done); err != nil {
		t.Fatal(err)
	}
	if got := w.snapshot(); len(got) != 1 || got[0].X != 2 {
		t.Fatalf("writes = %+v", got)
	}
}

func TestCoalescesPendingSaves(t *testing.T) {
	w := &fakeWriter{}
	s := New(w, quiet())
	d1, d2 := make(chan error, 1), make(chan error, 1)
	s.Save(store.CharState{ID: 7, X: 1}, d1)
	s.Save(store.CharState{ID: 7, X: 9}, d2) // queued before Run starts
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	if waitErr(t, d1) != nil || waitErr(t, d2) != nil {
		t.Fatal("waiters should both succeed")
	}
	got := w.snapshot()
	if len(got) != 1 || got[0].X != 9 {
		t.Fatalf("want one write with the latest state, got %+v", got)
	}
}

func TestRetriesAfterFailure(t *testing.T) {
	w := &fakeWriter{fail: 2}
	s := New(w, quiet())
	s.MinBackoff, s.MaxBackoff = time.Millisecond, 4*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	done := make(chan error, 1)
	s.Save(store.CharState{ID: 3}, done)
	if err := waitErr(t, done); err != nil {
		t.Fatal(err)
	}
	if n := len(w.snapshot()); n != 1 {
		t.Fatalf("writes = %d, want 1", n)
	}
}

func TestStopFlushesPending(t *testing.T) {
	w := &fakeWriter{}
	s := New(w, quiet())
	for i := int64(1); i <= 3; i++ {
		s.Save(store.CharState{ID: i}, nil)
	}
	s.Stop()
	go s.Run(context.Background())
	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after Stop")
	}
	if n := len(w.snapshot()); n != 3 {
		t.Fatalf("writes = %d, want 3", n)
	}
}

func TestCancelFailsWaiters(t *testing.T) {
	w := &fakeWriter{fail: 1 << 30}
	s := New(w, quiet())
	s.MinBackoff, s.MaxBackoff = time.Hour, time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)

	done := make(chan error, 1)
	s.Save(store.CharState{ID: 1}, done)
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := waitErr(t, done); err == nil {
		t.Fatal("want an error when the saver is cancelled before writing")
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/saver/`
Expected: FAIL, `undefined: New`.

- [ ] **Step 3: Implement**

```go
// Package saver writes character state to the database off the game loop.
// Saves for the same character are coalesced; failed writes are retried with backoff.
package saver

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"demonslayer2d/internal/store"
)

type Writer interface {
	SaveCharacterState(ctx context.Context, st store.CharState) error
}

type entry struct {
	st      store.CharState
	waiters []chan<- error
}

type Saver struct {
	w   Writer
	log *slog.Logger

	MinBackoff   time.Duration
	MaxBackoff   time.Duration
	WriteTimeout time.Duration

	mu       sync.Mutex
	pending  map[int64]*entry
	wake     chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

func New(w Writer, log *slog.Logger) *Saver {
	return &Saver{
		w: w, log: log,
		MinBackoff: time.Second, MaxBackoff: 30 * time.Second, WriteTimeout: 5 * time.Second,
		pending: make(map[int64]*entry),
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Save queues st and never blocks. If done is non-nil it receives exactly one value:
// nil once the state (or a newer one) is written, or an error if the saver is cancelled first.
// done must have a buffer of at least 1.
func (s *Saver) Save(st store.CharState, done chan<- error) {
	s.mu.Lock()
	e, ok := s.pending[st.ID]
	if !ok {
		e = &entry{}
		s.pending[st.ID] = e
	}
	e.st = st
	if done != nil {
		e.waiters = append(e.waiters, done)
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Stop asks Run to write everything pending and return.
func (s *Saver) Stop() { s.stopOnce.Do(func() { close(s.stop) }) }

// Done is closed when Run returns.
func (s *Saver) Done() <-chan struct{} { return s.done }

// Run writes queued states until Stop (after flushing) or ctx is cancelled (pending waiters get ctx.Err()).
func (s *Saver) Run(ctx context.Context) {
	defer close(s.done)
	var backoff time.Duration
	for {
		batch := s.take()
		if len(batch) == 0 {
			select {
			case <-s.wake:
				continue
			case <-s.stop:
				if s.empty() {
					return
				}
				continue
			case <-ctx.Done():
				s.failAll(ctx.Err())
				return
			}
		}
		failed := s.write(ctx, batch)
		if len(failed) == 0 {
			backoff = 0
			continue
		}
		s.requeue(failed)
		if backoff == 0 {
			backoff = s.MinBackoff
		} else {
			backoff = min(2*backoff, s.MaxBackoff)
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			s.failAll(ctx.Err())
			return
		}
	}
}

func (s *Saver) take() map[int64]*entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.pending
	s.pending = make(map[int64]*entry)
	return b
}

func (s *Saver) empty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending) == 0
}

func (s *Saver) write(ctx context.Context, batch map[int64]*entry) []*entry {
	var failed []*entry
	for _, e := range batch {
		wctx, cancel := context.WithTimeout(ctx, s.WriteTimeout)
		err := s.w.SaveCharacterState(wctx, e.st)
		cancel()
		if err != nil {
			s.log.Error("save character failed", "char_id", e.st.ID, "err", err)
			failed = append(failed, e)
			continue
		}
		notify(e.waiters, nil)
	}
	return failed
}

// requeue puts failed entries back. If a newer state was queued meanwhile, it wins and keeps both sets of waiters.
func (s *Saver) requeue(failed []*entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range failed {
		if cur, ok := s.pending[f.st.ID]; ok {
			cur.waiters = append(f.waiters, cur.waiters...)
		} else {
			s.pending[f.st.ID] = f
		}
	}
}

func (s *Saver) failAll(err error) {
	for _, e := range s.take() {
		notify(e.waiters, err)
	}
}

func notify(waiters []chan<- error, err error) {
	for _, w := range waiters {
		select {
		case w <- err:
		default:
		}
	}
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `go test -race ./internal/saver/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/saver
git commit -m "Add asynchronous character-state saver

Save never blocks the caller, coalesces pending saves per character,
retries failed writes with exponential backoff (1s to 30s), notifies
optional waiters once written, flushes on Stop and fails waiters if
cancelled."
git push
```

---

### Task 14: Movement and collision

**Files:**
- Create: `internal/zone/move.go`
- Test: `internal/zone/move_test.go`

- [ ] **Step 1: Write the failing test**

```go
package zone

import (
	"math"
	"strings"
	"testing"
)

type testGrid struct {
	w, h    int
	blocked map[[2]int]bool
}

func gridFrom(rows ...string) *testGrid {
	g := &testGrid{w: len(rows[0]), h: len(rows), blocked: map[[2]int]bool{}}
	for y, r := range rows {
		for x := 0; x < len(r); x++ {
			if r[x] == '#' {
				g.blocked[[2]int{x, y}] = true
			}
		}
	}
	return g
}

func openGrid(w, h int) *testGrid {
	rows := make([]string, h)
	for i := range rows {
		rows[i] = strings.Repeat(".", w)
	}
	return gridFrom(rows...)
}

func (g *testGrid) Blocked(x, y int) bool {
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return true
	}
	return g.blocked[[2]int{x, y}]
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

var room = []string{
	"#####",
	"#...#",
	"#...#",
	"#####",
}

func TestStepOpenArea(t *testing.T) {
	p := Step(openGrid(6, 3), Vec2{1.5, 1.5}, Vec2{1, 0}, 0.4)
	if !near(p.X, 1.9) || !near(p.Y, 1.5) {
		t.Fatalf("got %+v", p)
	}
}

func TestStepStopsAtWall(t *testing.T) {
	p := Step(gridFrom(room...), Vec2{1.5, 1.5}, Vec2{-1, 0}, 1)
	if !near(p.X, 1.3) || !near(p.Y, 1.5) {
		t.Fatalf("got %+v, want x=1.3 (box edge on the wall)", p)
	}
}

func TestStepSlidesAlongWall(t *testing.T) {
	p := Step(gridFrom(room...), Vec2{1.5, 1.5}, ClampDir(-1, 1), 1)
	if !near(p.X, 1.3) {
		t.Fatalf("x = %v, want 1.3", p.X)
	}
	if !near(p.Y, 1.5+0.7071) {
		t.Fatalf("y = %v, want to keep sliding down to ~2.207", p.Y)
	}
}

func TestStepNoTunnelling(t *testing.T) {
	p := Step(gridFrom(".#...."), Vec2{0.5, 0.5}, Vec2{1, 0}, 10)
	if !near(p.X, 0.7) {
		t.Fatalf("x = %v, a 1-tile wall must stop a long step at 0.7", p.X)
	}
}

func TestStepOutsideMapIsBlocked(t *testing.T) {
	p := Step(gridFrom("..."), Vec2{0.5, 0.5}, Vec2{-1, 0}, 1)
	if !near(p.X, 0.3) {
		t.Fatalf("x = %v, want 0.3", p.X)
	}
}

func TestStepNeverExceedsDistance(t *testing.T) {
	from := Vec2{10, 10}
	p := Step(openGrid(20, 20), from, ClampDir(0.6, 0.8), 0.4)
	dx, dy := float64(p.X-from.X), float64(p.Y-from.Y)
	if d := math.Hypot(dx, dy); d > 0.4+1e-4 {
		t.Fatalf("moved %v, want <= 0.4", d)
	}
}

func TestClampDir(t *testing.T) {
	if d := ClampDir(3, 4); !near(d.X, 0.6) || !near(d.Y, 0.8) {
		t.Fatalf("ClampDir(3,4) = %+v", d)
	}
	if d := ClampDir(0.5, 0); d != (Vec2{0.5, 0}) {
		t.Fatalf("short vectors are kept, got %+v", d)
	}
	if d := ClampDir(float32(math.NaN()), 1); d != (Vec2{}) {
		t.Fatalf("NaN must become zero, got %+v", d)
	}
	if d := ClampDir(float32(math.Inf(1)), 0); d != (Vec2{}) {
		t.Fatalf("Inf must become zero, got %+v", d)
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/zone/`
Expected: FAIL, `undefined: Step`.

- [ ] **Step 3: Implement**

```go
package zone

import "math"

// Vec2 is a position or direction in tile units.
type Vec2 struct{ X, Y float32 }

// Grid reports blocked tiles; *mapdata.Map implements it.
type Grid interface {
	Blocked(tx, ty int) bool
}

const (
	halfSize   = 0.3  // entities are 0.6x0.6 boxes centred on their position
	maxSubstep = 0.25 // < halfSize so a step can never skip over a tile edge
	eps        = 1e-4
)

// ClampDir limits a direction to length 1 and turns NaN/Inf into zero.
func ClampDir(x, y float32) Vec2 {
	if bad(x) || bad(y) {
		return Vec2{}
	}
	if l2 := float64(x)*float64(x) + float64(y)*float64(y); l2 > 1 {
		l := float32(math.Sqrt(l2))
		return Vec2{x / l, y / l}
	}
	return Vec2{x, y}
}

func bad(v float32) bool {
	f := float64(v)
	return math.IsNaN(f) || math.IsInf(f, 0)
}

func floor(v float32) int { return int(math.Floor(float64(v))) }

// Step moves pos by dir*dist, resolving X then Y in sub-steps so the box slides along walls.
func Step(g Grid, pos, dir Vec2, dist float32) Vec2 {
	if dist <= 0 || dir == (Vec2{}) {
		return pos
	}
	n := int(math.Ceil(float64(dist / maxSubstep)))
	sx := dir.X * dist / float32(n)
	sy := dir.Y * dist / float32(n)
	for i := 0; i < n; i++ {
		pos.X = moveX(g, pos, sx)
		pos.Y = moveY(g, pos, sy)
	}
	return pos
}

func moveX(g Grid, pos Vec2, dx float32) float32 {
	if dx == 0 {
		return pos.X
	}
	nx := pos.X + dx
	top, bottom := floor(pos.Y-halfSize+eps), floor(pos.Y+halfSize-eps)
	if dx > 0 {
		col := floor(nx + halfSize - eps)
		for row := top; row <= bottom; row++ {
			if g.Blocked(col, row) {
				return float32(col) - halfSize
			}
		}
	} else {
		col := floor(nx - halfSize + eps)
		for row := top; row <= bottom; row++ {
			if g.Blocked(col, row) {
				return float32(col+1) + halfSize
			}
		}
	}
	return nx
}

func moveY(g Grid, pos Vec2, dy float32) float32 {
	if dy == 0 {
		return pos.Y
	}
	ny := pos.Y + dy
	left, right := floor(pos.X-halfSize+eps), floor(pos.X+halfSize-eps)
	if dy > 0 {
		row := floor(ny + halfSize - eps)
		for col := left; col <= right; col++ {
			if g.Blocked(col, row) {
				return float32(row) - halfSize
			}
		}
	} else {
		row := floor(ny - halfSize + eps)
		for col := left; col <= right; col++ {
			if g.Blocked(col, row) {
				return float32(row+1) + halfSize
			}
		}
	}
	return ny
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `go test ./internal/zone/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/zone/move.go internal/zone/move_test.go
git commit -m "Add server-side movement with tile collision

Entities are 0.6x0.6 boxes. Movement resolves X then Y in sub-steps of
at most 0.25 tiles, so players slide along walls and can never tunnel
through a one-tile wall however far they move. Directions are clamped
to length 1 and NaN/Inf inputs become zero."
git push
```

---

### Task 15: Zone loop (join, leave, move, tick, visibility)

**Files:**
- Create: `internal/zone/zone.go`, `internal/zone/stats.go`
- Test: `internal/zone/zone_test.go`

- [ ] **Step 1: Write the failing test**

```go
package zone

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
)

type fakeOut struct {
	mu    sync.Mutex
	envs  []*pb.Envelope
	kicks []pb.KickReason
	full  bool
}

func (o *fakeOut) Send(e *pb.Envelope) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.full {
		return false
	}
	o.envs = append(o.envs, e)
	return true
}

func (o *fakeOut) Kick(r pb.KickReason) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.kicks = append(o.kicks, r)
}

func (o *fakeOut) take() []*pb.Envelope {
	o.mu.Lock()
	defer o.mu.Unlock()
	e := o.envs
	o.envs = nil
	return e
}

func (o *fakeOut) kicked() []pb.KickReason {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]pb.KickReason(nil), o.kicks...)
}

type fakeSaver struct {
	mu    sync.Mutex
	saves []store.CharState
}

func (s *fakeSaver) Save(st store.CharState, done chan<- error) {
	s.mu.Lock()
	s.saves = append(s.saves, st)
	s.mu.Unlock()
	if done != nil {
		done <- nil
	}
}

func (s *fakeSaver) all() []store.CharState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.CharState(nil), s.saves...)
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testConfig(capacity int) Config {
	return Config{MapID: "m1", ZoneNo: 1, TickRate: 10, MoveSpeed: 4, AOICellSize: 10, Capacity: capacity, SaveInterval: time.Minute}
}

func newTestZone(capacity int, g Grid) (*Zone, *fakeSaver) {
	sv := &fakeSaver{}
	return New(testConfig(capacity), g, sv, quietLog()), sv
}

func info(id uint64, x, y float32) PlayerInfo {
	return PlayerInfo{EntityID: id, CharID: int64(id) * 100, Name: fmt.Sprint("p", id), Level: 1, Pos: Vec2{x, y}, RespSeq: 7}
}

func joinNow(t *testing.T, z *Zone, id uint64, x, y float32) *fakeOut {
	t.Helper()
	out := &fakeOut{}
	if err := z.join(info(id, x, y), out); err != nil {
		t.Fatal(err)
	}
	return out
}

func enters(envs []*pb.Envelope) []uint64 {
	var ids []uint64
	for _, e := range envs {
		if en := e.GetEntityEnter(); en != nil {
			ids = append(ids, en.Id)
		}
	}
	return ids
}

func leaves(envs []*pb.Envelope) []uint64 {
	var ids []uint64
	for _, e := range envs {
		if l := e.GetEntityLeave(); l != nil {
			ids = append(ids, l.Id)
		}
	}
	return ids
}

func lastTick(envs []*pb.Envelope) *pb.WorldTick {
	for i := len(envs) - 1; i >= 0; i-- {
		if wt := envs[i].GetWorldTick(); wt != nil {
			return wt
		}
	}
	return nil
}

func moveFor(wt *pb.WorldTick, id uint64) *pb.EntityMove {
	for _, m := range wt.GetMoves() {
		if m.Id == id {
			return m
		}
	}
	return nil
}

func TestJoinSendsEnterWorldRespFirst(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	out := joinNow(t, z, 1, 5.5, 5.5)
	envs := out.take()
	if len(envs) != 1 || envs[0].Seq != 7 {
		t.Fatalf("want exactly the EnterWorldResp with seq 7, got %v", envs)
	}
	r := envs[0].GetEnterWorldResp()
	if r == nil || r.SelfEntityId != 1 || r.MapId != "m1" || r.Zone != 1 || r.X != 5.5 || r.Y != 5.5 || r.MoveSpeed != 4 {
		t.Fatalf("bad EnterWorldResp %v", r)
	}
	z.advance(time.Now())
	envs = out.take()
	if !slices.Equal(enters(envs), []uint64{1}) || lastTick(envs) == nil {
		t.Fatalf("after first tick want own EntityEnter then WorldTick, got %v", envs)
	}
}

func TestNearbyPlayersSeeEachOther(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	b := joinNow(t, z, 2, 8.5, 5.5)
	z.advance(time.Now())
	if got := enters(a.take()); !slices.Contains(got, 2) {
		t.Fatalf("a should see b, enters=%v", got)
	}
	if got := enters(b.take()); !slices.Contains(got, 1) {
		t.Fatalf("b should see a, enters=%v", got)
	}
}

func TestMoveIsBroadcast(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	b := joinNow(t, z, 2, 8.5, 5.5)
	z.advance(time.Now())
	a.take()
	b.take()

	z.move(moveCmd{id: 1, seq: 3, dir: Vec2{1, 0}})
	z.advance(time.Now())
	m := moveFor(lastTick(b.take()), 1)
	if m == nil || !near(m.X, 5.9) || m.DirX != 1 {
		t.Fatalf("b should get a's move to x=5.9, got %v", m)
	}
	ta := lastTick(a.take())
	if ta.AckInputSeq != 3 || moveFor(ta, 1) == nil {
		t.Fatalf("a should get its own move and ack seq 3, got %v", ta)
	}

	z.move(moveCmd{id: 1, seq: 4, dir: Vec2{}})
	z.advance(time.Now())
	if m := moveFor(lastTick(b.take()), 1); m == nil || m.DirX != 0 {
		t.Fatalf("stopping must be broadcast once, got %v", m)
	}
	z.advance(time.Now())
	if m := moveFor(lastTick(b.take()), 1); m != nil {
		t.Fatalf("an idle entity must not be resent, got %v", m)
	}
}

func TestOldInputSeqIgnored(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	z.move(moveCmd{id: 1, seq: 5, dir: Vec2{1, 0}})
	z.move(moveCmd{id: 1, seq: 4, dir: Vec2{-1, 0}})
	z.advance(time.Now())
	if m := moveFor(lastTick(a.take()), 1); m == nil || m.DirX != 1 {
		t.Fatalf("older input must not override newer, got %v", m)
	}
}

func TestFarPlayersAreInvisible(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	joinNow(t, z, 2, 35.5, 5.5)
	z.advance(time.Now())
	if slices.Contains(enters(a.take()), 2) {
		t.Fatal("b is 3 cells away and must not be visible")
	}
}

func TestWalkingIntoAndOutOfView(t *testing.T) {
	z, _ := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	joinNow(t, z, 2, 35.5, 5.5)
	z.move(moveCmd{id: 2, seq: 1, dir: Vec2{-1, 0}})
	seen := false
	for i := 0; i < 60 && !seen; i++ {
		z.advance(time.Now())
		seen = slices.Contains(enters(a.take()), 2)
	}
	if !seen {
		t.Fatal("b never came into view")
	}
	z.move(moveCmd{id: 2, seq: 2, dir: Vec2{1, 0}})
	gone := false
	for i := 0; i < 60 && !gone; i++ {
		z.advance(time.Now())
		gone = slices.Contains(leaves(a.take()), 2)
	}
	if !gone {
		t.Fatal("b never left view")
	}
}

func TestCapacity(t *testing.T) {
	z, _ := newTestZone(2, openGrid(64, 64))
	joinNow(t, z, 1, 5.5, 5.5)
	joinNow(t, z, 2, 5.5, 5.5)
	if err := z.join(info(3, 5.5, 5.5), &fakeOut{}); !errors.Is(err, ErrZoneFull) {
		t.Fatalf("err = %v, want ErrZoneFull", err)
	}
	if z.Count() != 2 {
		t.Fatalf("count = %d", z.Count())
	}
}

func TestLeaveSavesAndOthersSeeLeave(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	a := joinNow(t, z, 1, 5.5, 5.5)
	joinNow(t, z, 2, 6.5, 5.5)
	z.advance(time.Now())
	a.take()

	if err := <-z.leave(2); err != nil {
		t.Fatal(err)
	}
	if saves := sv.all(); len(saves) != 1 || saves[0].ID != 200 || saves[0].MapID != "m1" || saves[0].X != 6.5 {
		t.Fatalf("saves = %+v", saves)
	}
	z.advance(time.Now())
	if !slices.Contains(leaves(a.take()), 2) {
		t.Fatal("a should get EntityLeave for b")
	}
	if z.Count() != 1 {
		t.Fatalf("count = %d", z.Count())
	}
	if err := <-z.leave(99); err != nil {
		t.Fatalf("leaving an unknown entity must succeed, got %v", err)
	}
}

func TestSlowConsumerIsKickedAndSaved(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	joinNow(t, z, 1, 5.5, 5.5)
	b := joinNow(t, z, 2, 6.5, 5.5)
	b.full = true
	z.advance(time.Now())
	if k := b.kicked(); !slices.Equal(k, []pb.KickReason{pb.KickReason_KICK_REASON_SLOW_CONSUMER}) {
		t.Fatalf("kicks = %v", k)
	}
	if z.Count() != 1 || len(sv.all()) != 1 {
		t.Fatalf("count=%d saves=%v", z.Count(), sv.all())
	}
	if err := <-z.leave(2); err != nil {
		t.Fatalf("leave after eviction should return the eviction save result, got %v", err)
	}
}

func TestPeriodicSave(t *testing.T) {
	z, sv := newTestZone(10, openGrid(64, 64))
	joinNow(t, z, 1, 5.5, 5.5)
	t0 := time.Now()
	z.advance(t0)
	z.advance(t0.Add(30 * time.Second))
	if n := len(sv.all()); n != 0 {
		t.Fatalf("saved too early: %d", n)
	}
	z.advance(t0.Add(61 * time.Second))
	if n := len(sv.all()); n != 1 {
		t.Fatalf("want 1 periodic save, got %d", n)
	}
}

type panicGrid struct{ on bool }

func (g *panicGrid) Blocked(int, int) bool {
	if g.on {
		panic("boom")
	}
	return false
}

func TestPanicEvictsPlayers(t *testing.T) {
	g := &panicGrid{}
	z, sv := newTestZone(10, g)
	a := joinNow(t, z, 1, 5.5, 5.5)
	z.move(moveCmd{id: 1, seq: 1, dir: Vec2{1, 0}})
	g.on = true
	z.safeAdvance(time.Now())
	if k := a.kicked(); !slices.Equal(k, []pb.KickReason{pb.KickReason_KICK_REASON_ZONE_RESTART}) {
		t.Fatalf("kicks = %v", k)
	}
	if z.Count() != 0 || len(sv.all()) != 1 {
		t.Fatalf("count=%d saves=%d", z.Count(), len(sv.all()))
	}
	g.on = false
	joinNow(t, z, 2, 5.5, 5.5) // zone keeps working
	z.safeAdvance(time.Now())
}

func TestRunJoinLeaveAndShutdown(t *testing.T) {
	sv := &fakeSaver{}
	cfg := testConfig(10)
	cfg.TickRate = 50
	z := New(cfg, openGrid(64, 64), sv, quietLog())
	ctx, cancel := context.WithCancel(context.Background())
	go z.Run(ctx)

	o1, o2 := &fakeOut{}, &fakeOut{}
	if err := z.Join(ctx, info(1, 5.5, 5.5), o1); err != nil {
		t.Fatal(err)
	}
	if err := z.Join(ctx, info(2, 6.5, 5.5), o2); err != nil {
		t.Fatal(err)
	}
	z.Move(1, 1, 1, 0)
	time.Sleep(100 * time.Millisecond)
	if err := <-z.Leave(1); err != nil {
		t.Fatal(err)
	}
	if st := z.TakeStats(); st.Ticks == 0 {
		t.Fatal("no ticks recorded")
	}

	cancel()
	select {
	case <-z.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("zone did not stop")
	}
	if k := o2.kicked(); !slices.Equal(k, []pb.KickReason{pb.KickReason_KICK_REASON_SERVER_SHUTDOWN}) {
		t.Fatalf("o2 kicks = %v", k)
	}
	if n := len(sv.all()); n != 2 {
		t.Fatalf("saves = %d, want 2 (leave + shutdown)", n)
	}
	if err := z.Join(context.Background(), info(3, 5.5, 5.5), &fakeOut{}); !errors.Is(err, ErrZoneClosed) {
		t.Fatalf("join after stop err = %v", err)
	}
	if err := <-z.Leave(2); err != nil {
		t.Fatalf("leave after stop err = %v", err)
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/zone/`
Expected: FAIL, `undefined: New`.

- [ ] **Step 3: Implement `internal/zone/stats.go`**

```go
package zone

import (
	"math"
	"slices"
	"sync"
	"time"
)

// TickStats summarises tick durations since the previous TakeStats call.
type TickStats struct {
	Ticks int
	Max   time.Duration
	P99   time.Duration
}

type tickStats struct {
	mu      sync.Mutex
	samples []time.Duration
}

func (s *tickStats) record(d time.Duration) {
	s.mu.Lock()
	s.samples = append(s.samples, d)
	s.mu.Unlock()
}

func (s *tickStats) take() TickStats {
	s.mu.Lock()
	xs := s.samples
	s.samples = nil
	s.mu.Unlock()
	if len(xs) == 0 {
		return TickStats{}
	}
	slices.Sort(xs)
	i := int(math.Ceil(0.99*float64(len(xs)))) - 1
	return TickStats{Ticks: len(xs), Max: xs[len(xs)-1], P99: xs[i]}
}
```

- [ ] **Step 4: Implement `internal/zone/zone.go`**

```go
// Package zone runs one map instance ("khu"). A single goroutine owns every
// entity in it, simulates movement each tick and tells each player what it sees.
package zone

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"runtime/debug"
	"slices"
	"sync/atomic"
	"time"

	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
)

// Outbox delivers messages to one connected player. Implementations must not block.
type Outbox interface {
	// Send queues env and reports whether it was accepted.
	Send(env *pb.Envelope) bool
	// Kick tells the connection to send Kicked and close.
	Kick(reason pb.KickReason)
}

// Saver persists character state without blocking.
type Saver interface {
	Save(st store.CharState, done chan<- error)
}

type Config struct {
	MapID        string
	ZoneNo       int
	TickRate     int
	MoveSpeed    float32 // tiles per second at speed factor 1
	AOICellSize  int
	Capacity     int
	SaveInterval time.Duration
}

type PlayerInfo struct {
	EntityID uint64
	CharID   int64
	Name     string
	Class    pb.CharClass
	Level    int
	Exp      int64
	Pos      Vec2
	RespSeq  uint32 // seq of the EnterWorldReq; the zone answers it with EnterWorldResp
}

var (
	ErrZoneFull   = errors.New("zone: full")
	ErrZoneClosed = errors.New("zone: closed")
)

const slowTick = 50 * time.Millisecond

type cellKey struct{ X, Y int }

type player struct {
	info     PlayerInfo
	out      Outbox
	pos, dir Vec2
	inputSeq uint32
	cell     cellKey
	visible  map[uint64]struct{}
	changed  bool
}

type joinCmd struct {
	info  PlayerInfo
	out   Outbox
	reply chan error
}

type leaveCmd struct {
	id    uint64
	reply chan (<-chan error)
}

type moveCmd struct {
	id  uint64
	seq uint32
	dir Vec2
}

type Zone struct {
	cfg   Config
	grid  Grid
	saver Saver
	log   *slog.Logger

	inbox chan any
	done  chan struct{}

	count      atomic.Int32
	emptySince atomic.Int64 // unix nanos, 0 while occupied
	stats      tickStats

	// Owned by the zone goroutine.
	tickNo   uint32
	players  map[uint64]*player
	cells    map[cellKey]map[uint64]*player
	departed map[uint64]chan error // evicted players whose Leave has not arrived yet
	lastSave time.Time
}

func New(cfg Config, grid Grid, saver Saver, log *slog.Logger) *Zone {
	z := &Zone{
		cfg:      cfg,
		grid:     grid,
		saver:    saver,
		log:      log.With("map", cfg.MapID, "zone", cfg.ZoneNo),
		inbox:    make(chan any, 1024),
		done:     make(chan struct{}),
		players:  make(map[uint64]*player),
		cells:    make(map[cellKey]map[uint64]*player),
		departed: make(map[uint64]chan error),
	}
	z.emptySince.Store(time.Now().UnixNano())
	return z
}

func (z *Zone) Config() Config         { return z.cfg }
func (z *Zone) Count() int             { return int(z.count.Load()) }
func (z *Zone) Done() <-chan struct{}  { return z.done }
func (z *Zone) TakeStats() TickStats   { return z.stats.take() }

// EmptySince is when the zone last became empty, or zero while it has players.
func (z *Zone) EmptySince() time.Time {
	v := z.emptySince.Load()
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(0, v)
}

// Join adds a player. On success the zone has already queued EnterWorldResp on out.
func (z *Zone) Join(ctx context.Context, info PlayerInfo, out Outbox) error {
	reply := make(chan error, 1)
	select {
	case z.inbox <- joinCmd{info: info, out: out, reply: reply}:
	case <-z.done:
		return ErrZoneClosed
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-reply:
		return err
	case <-z.done:
		select {
		case err := <-reply:
			return err
		default:
			return ErrZoneClosed
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Leave removes a player. The returned channel yields the save result.
func (z *Zone) Leave(id uint64) <-chan error {
	reply := make(chan (<-chan error), 1)
	select {
	case z.inbox <- leaveCmd{id: id, reply: reply}:
	case <-z.done:
		return closedNil()
	}
	select {
	case ch := <-reply:
		return ch
	case <-z.done:
		return closedNil()
	}
}

// Move sets a player's direction. It never blocks; input is dropped if the inbox is full.
func (z *Zone) Move(id uint64, seq uint32, x, y float32) {
	select {
	case z.inbox <- moveCmd{id: id, seq: seq, dir: ClampDir(x, y)}:
	default:
		z.log.Debug("inbox full, dropping move", "entity", id)
	}
}

// Run ticks until ctx is cancelled, then saves and kicks every remaining player.
func (z *Zone) Run(ctx context.Context) {
	defer close(z.done)
	t := time.NewTicker(time.Second / time.Duration(z.cfg.TickRate))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			z.drain()
			z.evictAll(pb.KickReason_KICK_REASON_SERVER_SHUTDOWN)
			return
		case now := <-t.C:
			start := time.Now()
			z.safeAdvance(now)
			d := time.Since(start)
			z.stats.record(d)
			if d > slowTick {
				z.log.Warn("slow tick", "duration", d, "players", len(z.players))
			}
		}
	}
}

func (z *Zone) safeAdvance(now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			z.log.Error("zone panic, evicting players", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			z.evictAll(pb.KickReason_KICK_REASON_ZONE_RESTART)
		}
	}()
	z.advance(now)
}

// advance runs one tick: apply commands, move, then send each player its view.
func (z *Zone) advance(now time.Time) {
	z.tickNo++
	z.drain()

	step := z.cfg.MoveSpeed / float32(z.cfg.TickRate)
	for _, p := range z.players {
		if p.dir == (Vec2{}) {
			continue
		}
		if np := Step(z.grid, p.pos, p.dir, step); np != p.pos {
			p.pos = np
			p.changed = true
			z.updateCell(p)
		}
	}

	var slow []*player
	for _, p := range z.players {
		if !z.syncView(p) {
			slow = append(slow, p)
		}
	}
	for _, p := range z.players {
		p.changed = false
	}
	for _, p := range slow {
		p.out.Kick(pb.KickReason_KICK_REASON_SLOW_CONSUMER)
		z.departed[p.info.EntityID] = z.remove(p)
	}

	if z.lastSave.IsZero() {
		z.lastSave = now
	} else if now.Sub(z.lastSave) >= z.cfg.SaveInterval {
		for _, p := range z.players {
			z.saver.Save(z.state(p), nil)
		}
		z.lastSave = now
	}
}

// drain handles the commands queued so far (not ones that arrive meanwhile).
func (z *Zone) drain() {
	for n := len(z.inbox); n > 0; n-- {
		z.handle(<-z.inbox)
	}
}

func (z *Zone) handle(c any) {
	switch c := c.(type) {
	case joinCmd:
		c.reply <- z.join(c.info, c.out)
	case leaveCmd:
		c.reply <- z.leave(c.id)
	case moveCmd:
		z.move(c)
	}
}

func (z *Zone) join(info PlayerInfo, out Outbox) error {
	if len(z.players) >= z.cfg.Capacity {
		return ErrZoneFull
	}
	if _, dup := z.players[info.EntityID]; dup {
		return fmt.Errorf("zone: duplicate entity %d", info.EntityID)
	}
	p := &player{info: info, out: out, pos: info.Pos, visible: make(map[uint64]struct{})}
	p.cell = z.cellOf(p.pos)
	z.players[info.EntityID] = p
	z.addToCell(p)
	z.setCount()
	out.Send(&pb.Envelope{Seq: info.RespSeq, Payload: &pb.Envelope_EnterWorldResp{EnterWorldResp: &pb.EnterWorldResp{
		SelfEntityId: info.EntityID,
		MapId:        z.cfg.MapID,
		Zone:         int32(z.cfg.ZoneNo),
		X:            p.pos.X,
		Y:            p.pos.Y,
		ServerTick:   z.tickNo,
		MoveSpeed:    z.cfg.MoveSpeed,
	}}})
	return nil
}

func (z *Zone) leave(id uint64) <-chan error {
	if p, ok := z.players[id]; ok {
		return z.remove(p)
	}
	if d, ok := z.departed[id]; ok {
		delete(z.departed, id)
		return d
	}
	return closedNil()
}

func (z *Zone) move(c moveCmd) {
	p, ok := z.players[c.id]
	if !ok || c.seq < p.inputSeq {
		return
	}
	p.inputSeq = c.seq
	if p.dir != c.dir {
		p.dir = c.dir
		p.changed = true
	}
}

// remove takes p out of the zone and starts saving it.
func (z *Zone) remove(p *player) chan error {
	delete(z.players, p.info.EntityID)
	z.removeFromCell(p)
	z.setCount()
	done := make(chan error, 1)
	z.saver.Save(z.state(p), done)
	return done
}

func (z *Zone) evictAll(reason pb.KickReason) {
	for _, p := range z.players {
		p.out.Kick(reason)
		z.departed[p.info.EntityID] = z.remove(p)
	}
}

func (z *Zone) state(p *player) store.CharState {
	return store.CharState{ID: p.info.CharID, MapID: z.cfg.MapID, X: p.pos.X, Y: p.pos.Y, Level: p.info.Level, Exp: p.info.Exp}
}

// syncView sends p its EntityLeave/EntityEnter messages and WorldTick. It returns false if p's outbox is full.
func (z *Zone) syncView(p *player) bool {
	ok := true
	send := func(env *pb.Envelope) {
		if ok && !p.out.Send(env) {
			ok = false
		}
	}
	near := make(map[uint64]*player)
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for id, o := range z.cells[cellKey{p.cell.X + dx, p.cell.Y + dy}] {
				near[id] = o
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.visible)) {
		if _, still := near[id]; !still {
			delete(p.visible, id)
			send(&pb.Envelope{Payload: &pb.Envelope_EntityLeave{EntityLeave: &pb.EntityLeave{Id: id}}})
		}
	}
	var moves []*pb.EntityMove
	for _, id := range slices.Sorted(maps.Keys(near)) {
		o := near[id]
		if _, seen := p.visible[id]; !seen {
			p.visible[id] = struct{}{}
			send(enterEnv(o))
			continue
		}
		if o.changed {
			moves = append(moves, &pb.EntityMove{Id: id, X: o.pos.X, Y: o.pos.Y, DirX: o.dir.X, DirY: o.dir.Y})
		}
	}
	send(&pb.Envelope{Payload: &pb.Envelope_WorldTick{WorldTick: &pb.WorldTick{
		Tick: z.tickNo, AckInputSeq: p.inputSeq, Moves: moves,
	}}})
	return ok
}

func enterEnv(p *player) *pb.Envelope {
	return &pb.Envelope{Payload: &pb.Envelope_EntityEnter{EntityEnter: &pb.EntityEnter{
		Id:    p.info.EntityID,
		Kind:  pb.EntityKind_ENTITY_KIND_PLAYER,
		Name:  p.info.Name,
		Class: p.info.Class,
		Level: int32(p.info.Level),
		X:     p.pos.X,
		Y:     p.pos.Y,
		DirX:  p.dir.X,
		DirY:  p.dir.Y,
	}}}
}

func (z *Zone) cellOf(pos Vec2) cellKey {
	size := float64(z.cfg.AOICellSize)
	return cellKey{int(math.Floor(float64(pos.X) / size)), int(math.Floor(float64(pos.Y) / size))}
}

func (z *Zone) addToCell(p *player) {
	c := z.cells[p.cell]
	if c == nil {
		c = make(map[uint64]*player)
		z.cells[p.cell] = c
	}
	c[p.info.EntityID] = p
}

func (z *Zone) removeFromCell(p *player) {
	if c := z.cells[p.cell]; c != nil {
		delete(c, p.info.EntityID)
		if len(c) == 0 {
			delete(z.cells, p.cell)
		}
	}
}

func (z *Zone) updateCell(p *player) {
	if nc := z.cellOf(p.pos); nc != p.cell {
		z.removeFromCell(p)
		p.cell = nc
		z.addToCell(p)
	}
}

func (z *Zone) setCount() {
	n := len(z.players)
	z.count.Store(int32(n))
	if n > 0 {
		z.emptySince.Store(0)
	} else if z.emptySince.Load() == 0 {
		z.emptySince.Store(time.Now().UnixNano())
	}
}

func closedNil() <-chan error {
	ch := make(chan error, 1)
	ch <- nil
	return ch
}
```

- [ ] **Step 5: Run the tests (with the race detector) and see them pass**

Run: `go test -race ./internal/zone/`
Expected: `ok`.

- [ ] **Step 6: Commit and push**

```bash
git add internal/zone/zone.go internal/zone/stats.go internal/zone/zone_test.go
git commit -m "Add the zone game loop

One goroutine per zone owns its players. Each tick it applies queued
join/leave/move commands, moves players with collision, and sends each
player EntityLeave/EntityEnter for its 3x3 AOI cells plus a WorldTick
with changed entities and the acked input seq. EnterWorldResp is sent
by the zone so it is always the first in-world message. Full outboxes
are kicked as slow consumers, panics evict and save everyone without
stopping the zone, players are saved on leave, periodically and on
shutdown, and tick durations are recorded for stats."
git push
```

---

### Task 16: World (zones per map, placement, online sessions, idle janitor)

**Files:**
- Create: `internal/world/world.go`
- Test: `internal/world/world_test.go`

- [ ] **Step 1: Write the failing test**

```go
package world

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
)

type nopSaver struct{}

func (nopSaver) Save(_ store.CharState, done chan<- error) {
	if done != nil {
		done <- nil
	}
}

type recOut struct{ ch chan *pb.Envelope }

func newRecOut() *recOut { return &recOut{ch: make(chan *pb.Envelope, 256)} }

func (o *recOut) Send(e *pb.Envelope) bool {
	select {
	case o.ch <- e:
	default:
	}
	return true
}

func (o *recOut) Kick(pb.KickReason) {}

func (o *recOut) enterResp(t *testing.T) *pb.EnterWorldResp {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e := <-o.ch:
			if r := e.GetEnterWorldResp(); r != nil {
				return r
			}
		case <-timeout:
			t.Fatal("no EnterWorldResp")
		}
	}
}

func testMaps(t *testing.T) map[string]*mapdata.Map {
	t.Helper()
	rows := make([]string, 20)
	for i := range rows {
		rows[i] = strings.Repeat(".", 20)
	}
	rows[10] = strings.Repeat("#", 20)
	data, _ := json.Marshal(map[string]any{
		"id": "m1", "name": "M1", "type": "training", "width": 20, "height": 20, "collision": rows,
		"spawn_points": []map[string]any{{"id": "default", "x": 2.5, "y": 2.5}},
	})
	m, err := mapdata.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]*mapdata.Map{"m1": m}
}

func newTestWorld(t *testing.T, capacity int) *World {
	t.Helper()
	w, err := New(Config{
		StartMap: "m1", TickRate: 50, MoveSpeed: 4, AOICellSize: 10, ZoneCapacity: capacity,
		SaveInterval: time.Minute, IdleZoneTTL: 5 * time.Minute, JanitorEvery: time.Hour,
	}, testMaps(t), nopSaver{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() { cancel(); w.Wait() })
	return w
}

func char(id int64, mapID string, x, y float32) store.Character {
	return store.Character{ID: id, AccountID: id, Slot: 1, Name: "c", Class: "viem", Level: 1, MapID: mapID, X: x, Y: y}
}

func TestNewRejectsUnknownStartMap(t *testing.T) {
	_, err := New(Config{StartMap: "nope"}, testMaps(t), nopSaver{}, slog.Default())
	if err == nil {
		t.Fatal("want error for unknown start map")
	}
}

func TestEnterUsesSavedPosition(t *testing.T) {
	w := newTestWorld(t, 10)
	out := newRecOut()
	m, err := w.Enter(context.Background(), EnterRequest{Char: char(1, "m1", 3.5, 4.5), Out: out, RespSeq: 9})
	if err != nil {
		t.Fatal(err)
	}
	r := out.enterResp(t)
	if r.X != 3.5 || r.Y != 4.5 || m.ZoneNo != 1 || m.MapID != "m1" || r.SelfEntityId != m.EntityID {
		t.Fatalf("resp %v membership %+v", r, m)
	}
}

func TestEnterFallsBackToStartSpawn(t *testing.T) {
	w := newTestWorld(t, 10)
	for _, c := range []store.Character{char(1, "m1", 5.5, 10.5), char(2, "gone", 3.5, 3.5)} {
		out := newRecOut()
		if _, err := w.Enter(context.Background(), EnterRequest{Char: c, Out: out}); err != nil {
			t.Fatal(err)
		}
		if r := out.enterResp(t); r.MapId != "m1" || r.X != 2.5 || r.Y != 2.5 {
			t.Fatalf("char %d: want start spawn, got %v", c.ID, r)
		}
	}
}

func TestEnterOpensNewZoneWhenFull(t *testing.T) {
	w := newTestWorld(t, 2)
	var zones []int
	for i := int64(1); i <= 3; i++ {
		m, err := w.Enter(context.Background(), EnterRequest{Char: char(i, "m1", 3.5, 3.5), Out: newRecOut()})
		if err != nil {
			t.Fatal(err)
		}
		zones = append(zones, m.ZoneNo)
	}
	if zones[0] != 1 || zones[1] != 1 || zones[2] != 2 {
		t.Fatalf("zones = %v, want [1 1 2]", zones)
	}
}

func TestLeaveAndCloseIdleZone(t *testing.T) {
	w := newTestWorld(t, 1)
	m1, _ := w.Enter(context.Background(), EnterRequest{Char: char(1, "m1", 3.5, 3.5), Out: newRecOut()})
	m2, err := w.Enter(context.Background(), EnterRequest{Char: char(2, "m1", 3.5, 3.5), Out: newRecOut()})
	if err != nil || m2.ZoneNo != 2 {
		t.Fatalf("second player zone %d err %v", m2.ZoneNo, err)
	}
	if err := <-m2.Leave(); err != nil {
		t.Fatal(err)
	}
	w.closeIdle(time.Now().Add(10 * time.Minute))
	if n := len(w.Stats()); n != 1 {
		t.Fatalf("zones after janitor = %d, want 1 (zone 1 is never closed)", n)
	}
	_ = m1
}

type sess struct{ done chan struct{} }

func (s *sess) Kick(pb.KickReason)    {}
func (s *sess) Done() <-chan struct{} { return s.done }

func TestClaimRelease(t *testing.T) {
	w := newTestWorld(t, 10)
	a, b := &sess{}, &sess{}
	if prev := w.Claim(7, a); prev != nil {
		t.Fatal("first claim should have no previous session")
	}
	if prev := w.Claim(7, b); prev != a {
		t.Fatal("second claim should return the first session")
	}
	w.Release(7, a) // stale release must not remove b
	if prev := w.Claim(7, a); prev != b {
		t.Fatal("b should still be registered")
	}
	w.Release(7, a)
	if w.Online() != 0 {
		t.Fatalf("online = %d", w.Online())
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `go test ./internal/world/`
Expected: FAIL, `undefined: New`.

- [ ] **Step 3: Implement**

```go
// Package world owns every map's zones, places characters into zones and tracks
// which account is online where.
package world

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/zone"
)

// Session is an online connection that can be kicked.
type Session interface {
	Kick(reason pb.KickReason)
	// Done is closed once the session has left the world and its character is saved.
	Done() <-chan struct{}
}

type Config struct {
	StartMap     string
	TickRate     int
	MoveSpeed    float32
	AOICellSize  int
	ZoneCapacity int
	SaveInterval time.Duration
	IdleZoneTTL  time.Duration
	JanitorEvery time.Duration
}

type World struct {
	cfg   Config
	maps  map[string]*mapdata.Map
	saver zone.Saver
	log   *slog.Logger
	start mapdata.Point

	ctx context.Context
	wg  sync.WaitGroup

	mu     sync.Mutex
	zones  map[string][]*slot // index i holds zone number i+1; nil = closed
	online map[int64]Session

	nextEntity atomic.Uint64
}

type slot struct {
	z        *zone.Zone
	cancel   context.CancelFunc
	no       int
	reserved atomic.Int32 // joins in flight
}

func New(cfg Config, maps map[string]*mapdata.Map, saver zone.Saver, log *slog.Logger) (*World, error) {
	m, ok := maps[cfg.StartMap]
	if !ok {
		return nil, fmt.Errorf("world: start map %q not found", cfg.StartMap)
	}
	return &World{
		cfg: cfg, maps: maps, saver: saver, log: log,
		start:  m.Spawns["default"],
		zones:  make(map[string][]*slot),
		online: make(map[int64]Session),
	}, nil
}

// Start opens zone 1 of every map and the idle-zone janitor. Everything stops when ctx is cancelled.
// It must be called before Enter.
func (w *World) Start(ctx context.Context) {
	w.ctx = ctx
	w.mu.Lock()
	for id := range w.maps {
		w.openZoneLocked(id)
	}
	w.mu.Unlock()
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		t := time.NewTicker(w.cfg.JanitorEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				w.closeIdle(now)
			}
		}
	}()
}

// Wait blocks until every zone goroutine has exited (after the Start context is cancelled).
func (w *World) Wait() { w.wg.Wait() }

// StartPosition is where new characters appear.
func (w *World) StartPosition() (string, mapdata.Point) { return w.cfg.StartMap, w.start }

func (w *World) openZoneLocked(mapID string) *slot {
	slots := w.zones[mapID]
	idx := len(slots)
	for i, s := range slots {
		if s == nil {
			idx = i
			break
		}
	}
	z := zone.New(zone.Config{
		MapID: mapID, ZoneNo: idx + 1, TickRate: w.cfg.TickRate, MoveSpeed: w.cfg.MoveSpeed,
		AOICellSize: w.cfg.AOICellSize, Capacity: w.cfg.ZoneCapacity, SaveInterval: w.cfg.SaveInterval,
	}, w.maps[mapID], w.saver, w.log)
	zctx, cancel := context.WithCancel(w.ctx)
	s := &slot{z: z, cancel: cancel, no: idx + 1}
	if idx == len(slots) {
		w.zones[mapID] = append(slots, s)
	} else {
		slots[idx] = s
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		z.Run(zctx)
	}()
	return s
}

type EnterRequest struct {
	Char    store.Character
	Out     zone.Outbox
	RespSeq uint32
}

// Membership is a character's presence in a zone.
type Membership struct {
	z        *zone.Zone
	EntityID uint64
	MapID    string
	ZoneNo   int
}

func (m *Membership) Move(seq uint32, x, y float32) { m.z.Move(m.EntityID, seq, x, y) }

// Leave removes the character; the channel yields the save result.
func (m *Membership) Leave() <-chan error { return m.z.Leave(m.EntityID) }

// Enter puts the character into the first zone with room, opening a new zone if all are full.
// The zone sends EnterWorldResp (with req.RespSeq) on req.Out.
func (w *World) Enter(ctx context.Context, req EnterRequest) (*Membership, error) {
	mapID, pos := w.placement(req.Char)
	info := zone.PlayerInfo{
		EntityID: w.nextEntity.Add(1),
		CharID:   req.Char.ID,
		Name:     req.Char.Name,
		Class:    classes.ToPB(req.Char.Class),
		Level:    req.Char.Level,
		Exp:      req.Char.Exp,
		Pos:      pos,
		RespSeq:  req.RespSeq,
	}
	for attempt := 0; attempt < 10; attempt++ {
		s := w.reserve(mapID)
		err := s.z.Join(ctx, info, req.Out)
		s.reserved.Add(-1)
		switch {
		case err == nil:
			return &Membership{z: s.z, EntityID: info.EntityID, MapID: mapID, ZoneNo: s.no}, nil
		case errors.Is(err, zone.ErrZoneFull), errors.Is(err, zone.ErrZoneClosed):
			continue
		default:
			<-s.z.Leave(info.EntityID) // the join may have been applied after ctx expired
			return nil, err
		}
	}
	return nil, zone.ErrZoneFull
}

func (w *World) placement(c store.Character) (string, zone.Vec2) {
	if m, ok := w.maps[c.MapID]; ok && m.Walkable(mapdata.Point{X: c.X, Y: c.Y}) {
		return c.MapID, zone.Vec2{X: c.X, Y: c.Y}
	}
	return w.cfg.StartMap, zone.Vec2{X: w.start.X, Y: w.start.Y}
}

func (w *World) reserve(mapID string) *slot {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, s := range w.zones[mapID] {
		if s == nil {
			continue
		}
		select {
		case <-s.z.Done():
			continue
		default:
		}
		if s.z.Count()+int(s.reserved.Load()) < w.cfg.ZoneCapacity {
			s.reserved.Add(1)
			return s
		}
	}
	s := w.openZoneLocked(mapID)
	s.reserved.Add(1)
	return s
}

// closeIdle stops zones other than zone 1 that have been empty for IdleZoneTTL.
func (w *World) closeIdle(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for mapID, slots := range w.zones {
		for i, s := range slots {
			if i == 0 || s == nil || s.reserved.Load() > 0 || s.z.Count() > 0 {
				continue
			}
			since := s.z.EmptySince()
			if since.IsZero() || now.Sub(since) < w.cfg.IdleZoneTTL {
				continue
			}
			s.cancel()
			slots[i] = nil
			w.log.Info("closed idle zone", "map", mapID, "zone", s.no)
		}
	}
}

// Claim registers s as the online session of accountID and returns the previous one, if any.
func (w *World) Claim(accountID int64, s Session) Session {
	w.mu.Lock()
	defer w.mu.Unlock()
	prev := w.online[accountID]
	w.online[accountID] = s
	return prev
}

// Release unregisters s if it is still the online session of accountID.
func (w *World) Release(accountID int64, s Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.online[accountID] == s {
		delete(w.online, accountID)
	}
}

// Online is the number of logged-in accounts.
func (w *World) Online() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.online)
}

type ZoneStat struct {
	MapID   string
	ZoneNo  int
	Players int
	zone.TickStats
}

// Stats returns per-zone player counts and tick timings since the last call.
func (w *World) Stats() []ZoneStat {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []ZoneStat
	for mapID, slots := range w.zones {
		for _, s := range slots {
			if s == nil {
				continue
			}
			out = append(out, ZoneStat{MapID: mapID, ZoneNo: s.no, Players: s.z.Count(), TickStats: s.z.TakeStats()})
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `go test -race ./internal/world/`
Expected: `ok`.

- [ ] **Step 5: Commit and push**

```bash
git add internal/world
git commit -m "Add the world: zones per map, placement and online sessions

Enter places a character at its saved position (or the start map's
default spawn if that is invalid) in the lowest-numbered zone with
room, reserving a seat while the join is in flight and opening a new
zone when all are full. Zones other than zone 1 close after 5 minutes
empty. Claim/Release track the single online session per account."
git push
```

---

### Task 17: Gateway (WebSocket connections and request handlers)

**Files:**
- Create: `internal/gateway/outbox.go`, `internal/gateway/errors.go`, `internal/gateway/server.go`, `internal/gateway/conn.go`, `internal/gateway/handlers.go`
- Test: `internal/gateway/outbox_test.go`, `internal/gateway/errors_test.go` (the full request flow is covered by e2e tests in Task 20)

- [ ] **Step 1: Write the failing tests**

`internal/gateway/outbox_test.go`:

```go
package gateway

import (
	"testing"

	"demonslayer2d/internal/pb"
)

func TestOutbox(t *testing.T) {
	var kicked []pb.KickReason
	o := newOutbox(2, func(r pb.KickReason) { kicked = append(kicked, r) })
	e := &pb.Envelope{}
	if !o.Send(e) || !o.Send(e) {
		t.Fatal("first two sends should fit")
	}
	if o.Send(e) {
		t.Fatal("third send should report a full queue")
	}
	<-o.ch
	o.closeWith(&pb.Envelope{Seq: 9})
	o.closeWith(nil) // no-op
	if o.Send(e) {
		t.Fatal("send after close must fail")
	}
	var got []*pb.Envelope
	for env := range o.ch {
		got = append(got, env)
	}
	if len(got) != 2 || got[1].Seq != 9 {
		t.Fatalf("drained %v, want the queued envelope then the final one", got)
	}
	o.Kick(pb.KickReason_KICK_REASON_IDLE_TIMEOUT)
	if len(kicked) != 1 || kicked[0] != pb.KickReason_KICK_REASON_IDLE_TIMEOUT {
		t.Fatalf("kicked = %v", kicked)
	}
}
```

`internal/gateway/errors_test.go`:

```go
package gateway

import (
	"errors"
	"fmt"
	"testing"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/zone"
)

func TestErrorCode(t *testing.T) {
	cases := map[error]pb.ErrorCode{
		auth.ErrUsernameInvalid: pb.ErrorCode_ERROR_CODE_USERNAME_INVALID,
		auth.ErrPasswordInvalid: pb.ErrorCode_ERROR_CODE_PASSWORD_INVALID,
		auth.ErrNameInvalid:     pb.ErrorCode_ERROR_CODE_NAME_INVALID,
		auth.ErrAuthFailed:      pb.ErrorCode_ERROR_CODE_AUTH_FAILED,
		auth.ErrRateLimited:     pb.ErrorCode_ERROR_CODE_RATE_LIMITED,
		store.ErrUsernameTaken:  pb.ErrorCode_ERROR_CODE_USERNAME_TAKEN,
		store.ErrNameTaken:      pb.ErrorCode_ERROR_CODE_NAME_TAKEN,
		store.ErrSlotOccupied:   pb.ErrorCode_ERROR_CODE_SLOT_OCCUPIED,
		store.ErrNotFound:       pb.ErrorCode_ERROR_CODE_CHAR_NOT_FOUND,
		zone.ErrZoneFull:        pb.ErrorCode_ERROR_CODE_ZONE_FULL,
		errors.New("boom"):      pb.ErrorCode_ERROR_CODE_INTERNAL,
	}
	for err, want := range cases {
		if got := errorCode(fmt.Errorf("wrapped: %w", err)); got != want {
			t.Errorf("errorCode(%v) = %v, want %v", err, got, want)
		}
	}
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./internal/gateway/`
Expected: FAIL, `undefined: newOutbox`.

- [ ] **Step 3: Implement `internal/gateway/outbox.go`**

```go
package gateway

import (
	"sync"

	"demonslayer2d/internal/pb"
)

// outbox is a connection's bounded send queue. It implements zone.Outbox.
type outbox struct {
	mu     sync.Mutex
	ch     chan *pb.Envelope
	closed bool
	kick   func(pb.KickReason)
}

func newOutbox(size int, kick func(pb.KickReason)) *outbox {
	return &outbox{ch: make(chan *pb.Envelope, size), kick: kick}
}

// Send queues env without blocking; it returns false when the queue is full or closed.
func (o *outbox) Send(env *pb.Envelope) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return false
	}
	select {
	case o.ch <- env:
		return true
	default:
		return false
	}
}

func (o *outbox) Kick(reason pb.KickReason) { o.kick(reason) }

// closeWith queues final if there is room, then closes the queue. Later calls do nothing.
func (o *outbox) closeWith(final *pb.Envelope) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return
	}
	if final != nil {
		select {
		case o.ch <- final:
		default:
		}
	}
	o.closed = true
	close(o.ch)
}
```

- [ ] **Step 4: Implement `internal/gateway/errors.go`**

```go
package gateway

import (
	"errors"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/zone"
)

// errorCode maps a service error to the protocol error code; unknown errors are INTERNAL.
func errorCode(err error) pb.ErrorCode {
	switch {
	case errors.Is(err, auth.ErrUsernameInvalid):
		return pb.ErrorCode_ERROR_CODE_USERNAME_INVALID
	case errors.Is(err, auth.ErrPasswordInvalid):
		return pb.ErrorCode_ERROR_CODE_PASSWORD_INVALID
	case errors.Is(err, auth.ErrNameInvalid):
		return pb.ErrorCode_ERROR_CODE_NAME_INVALID
	case errors.Is(err, auth.ErrAuthFailed):
		return pb.ErrorCode_ERROR_CODE_AUTH_FAILED
	case errors.Is(err, auth.ErrRateLimited):
		return pb.ErrorCode_ERROR_CODE_RATE_LIMITED
	case errors.Is(err, store.ErrUsernameTaken):
		return pb.ErrorCode_ERROR_CODE_USERNAME_TAKEN
	case errors.Is(err, store.ErrNameTaken):
		return pb.ErrorCode_ERROR_CODE_NAME_TAKEN
	case errors.Is(err, store.ErrSlotOccupied):
		return pb.ErrorCode_ERROR_CODE_SLOT_OCCUPIED
	case errors.Is(err, store.ErrNotFound):
		return pb.ErrorCode_ERROR_CODE_CHAR_NOT_FOUND
	case errors.Is(err, zone.ErrZoneFull):
		return pb.ErrorCode_ERROR_CODE_ZONE_FULL
	default:
		return pb.ErrorCode_ERROR_CODE_INTERNAL
	}
}
```

- [ ] **Step 5: Implement `internal/gateway/server.go`**

```go
// Package gateway accepts WebSocket connections and turns protocol messages
// into calls on the auth service, the character store and the world.
package gateway

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/world"
)

// ProtocolVersion must match LoginReq.protocol_version.
const ProtocolVersion uint32 = 1

const (
	maxMessageSize = 4096
	outboxSize     = 256
	authTimeout    = 30 * time.Second
	idleTimeout    = 30 * time.Second
	writeTimeout   = 10 * time.Second
	requestTimeout = 10 * time.Second
	packetsPerSec  = 30
	movesPerSec    = 20
	dropStreakKick = 5 // consecutive seconds with dropped packets before kicking
)

type CharacterStore interface {
	ListCharacters(ctx context.Context, accountID int64) ([]store.Character, error)
	GetCharacter(ctx context.Context, accountID, charID int64) (store.Character, error)
	CreateCharacter(ctx context.Context, nc store.NewCharacter) (store.Character, error)
	DeleteCharacter(ctx context.Context, accountID, charID int64) error
}

type Deps struct {
	Auth  *auth.Service
	Chars CharacterStore
	World *world.World
	Log   *slog.Logger
}

type Server struct {
	deps   Deps
	log    *slog.Logger
	nextID atomic.Uint64

	mu      sync.Mutex
	conns   map[*conn]struct{}
	closing bool
	wg      sync.WaitGroup
}

func NewServer(d Deps) *Server {
	return &Server{deps: d, log: d.Log, conns: make(map[*conn]struct{})}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		http.Error(w, "server shutting down", http.StatusServiceUnavailable)
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()

	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		s.log.Debug("websocket accept failed", "err", err)
		return
	}
	ws.SetReadLimit(maxMessageSize)
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	c := s.newConn(ws, ip)
	s.track(c, true)
	defer s.track(c, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.writeLoop()
	c.readLoop(ctx)
	c.cleanup()
}

func (s *Server) track(c *conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if add {
		s.conns[c] = struct{}{}
	} else {
		delete(s.conns, c)
	}
}

// ConnCount is the number of open connections.
func (s *Server) ConnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// Shutdown refuses new connections, kicks every open one with SERVER_SHUTDOWN
// and waits until they have all left the world.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.closing = true
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	for _, c := range conns {
		c.Kick(pb.KickReason_KICK_REASON_SERVER_SHUTDOWN)
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

- [ ] **Step 6: Implement `internal/gateway/conn.go`**

```go
package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/world"
)

type connState int

const (
	stateConnected connState = iota
	stateAuthed
	stateInWorld
)

type conn struct {
	srv *Server
	ws  *websocket.Conn
	id  uint64
	ip  string
	log *slog.Logger // never reassigned: timers log from other goroutines
	out *outbox

	kicked   atomic.Bool
	kickOnce sync.Once
	done     chan struct{} // closed after cleanup (left world, character saved)

	idle      *time.Timer
	authTimer *time.Timer

	// Owned by the read goroutine.
	state       connState
	accountID   int64
	member      *world.Membership
	packets     *rate.Limiter
	moves       *rate.Limiter
	lastDropSec int64
	dropStreak  int
}

func (s *Server) newConn(ws *websocket.Conn, ip string) *conn {
	c := &conn{
		srv:     s,
		ws:      ws,
		id:      s.nextID.Add(1),
		ip:      ip,
		done:    make(chan struct{}),
		packets: rate.NewLimiter(packetsPerSec, packetsPerSec),
		moves:   rate.NewLimiter(movesPerSec, movesPerSec),
	}
	c.log = s.log.With("conn_id", c.id)
	c.out = newOutbox(outboxSize, c.Kick)
	c.idle = time.AfterFunc(idleTimeout, func() { c.Kick(pb.KickReason_KICK_REASON_IDLE_TIMEOUT) })
	c.authTimer = time.AfterFunc(authTimeout, func() { c.Kick(pb.KickReason_KICK_REASON_IDLE_TIMEOUT) })
	return c
}

// Kick sends Kicked and closes the connection. Safe from any goroutine; only the first call counts.
func (c *conn) Kick(reason pb.KickReason) {
	c.kickOnce.Do(func() {
		c.kicked.Store(true)
		c.log.Info("kick", "reason", reason.String())
		c.out.closeWith(&pb.Envelope{Payload: &pb.Envelope_Kicked{Kicked: &pb.Kicked{Reason: reason}}})
	})
}

func (c *conn) Done() <-chan struct{} { return c.done }

func (c *conn) writeLoop() {
	for env := range c.out.ch {
		data, err := proto.Marshal(env)
		if err != nil {
			c.log.Error("marshal envelope", "err", err)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		err = c.ws.Write(ctx, websocket.MessageBinary, data)
		cancel()
		if err != nil {
			c.out.closeWith(nil)
			c.ws.CloseNow()
			return
		}
	}
	c.ws.Close(websocket.StatusNormalClosure, "")
}

func (c *conn) readLoop(ctx context.Context) {
	for {
		typ, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		c.idle.Reset(idleTimeout)
		if c.kicked.Load() {
			continue // keep reading until the close handshake finishes
		}
		if typ != websocket.MessageBinary {
			c.Kick(pb.KickReason_KICK_REASON_PROTOCOL_ERROR)
			continue
		}
		if !c.allowPacket(time.Now()) {
			continue
		}
		env := &pb.Envelope{}
		if err := proto.Unmarshal(data, env); err != nil {
			c.Kick(pb.KickReason_KICK_REASON_PROTOCOL_ERROR)
			continue
		}
		c.safeDispatch(ctx, env)
	}
}

// safeDispatch handles one request; a panic is logged and closes only this connection.
func (c *conn) safeDispatch(ctx context.Context, env *pb.Envelope) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error("panic handling request", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			c.Kick(pb.KickReason_KICK_REASON_PROTOCOL_ERROR)
		}
	}()
	c.dispatch(ctx, env)
}

// allowPacket applies the per-connection packet limit; dropping in 5 consecutive seconds kicks.
func (c *conn) allowPacket(now time.Time) bool {
	if c.packets.AllowN(now, 1) {
		return true
	}
	sec := now.Unix()
	switch {
	case sec == c.lastDropSec:
	case sec == c.lastDropSec+1:
		c.dropStreak++
	default:
		c.dropStreak = 1
	}
	c.lastDropSec = sec
	if c.dropStreak >= dropStreakKick {
		c.Kick(pb.KickReason_KICK_REASON_RATE_LIMITED)
	}
	return false
}

func (c *conn) cleanup() {
	c.idle.Stop()
	c.authTimer.Stop()
	if c.member != nil {
		select {
		case err := <-c.member.Leave():
			if err != nil {
				c.log.Error("save on disconnect failed", "err", err)
			}
		case <-time.After(requestTimeout):
			c.log.Warn("leave world timed out on disconnect")
		}
		c.member = nil
	}
	if c.accountID != 0 {
		c.srv.deps.World.Release(c.accountID, c)
	}
	c.out.closeWith(nil)
	close(c.done)
}

// send queues env; a full queue means the client is not reading, so it is kicked.
func (c *conn) send(env *pb.Envelope) {
	if !c.out.Send(env) {
		c.Kick(pb.KickReason_KICK_REASON_SLOW_CONSUMER)
	}
}

func (c *conn) fail(seq uint32, code pb.ErrorCode, msg string) {
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_Error{Error: &pb.Error{Code: code, Message: msg}}})
}

func (c *conn) failErr(seq uint32, err error) {
	code := errorCode(err)
	if code == pb.ErrorCode_ERROR_CODE_INTERNAL {
		c.log.Error("request failed", "account_id", c.accountID, "err", err)
	}
	c.fail(seq, code, code.String())
}
```

- [ ] **Step 7: Implement `internal/gateway/handlers.go`**

```go
package gateway

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/text/unicode/norm"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/classes"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/world"
)

func (c *conn) dispatch(ctx context.Context, env *pb.Envelope) {
	seq := env.Seq
	switch p := env.Payload.(type) {
	case *pb.Envelope_Ping:
		c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_Pong{Pong: &pb.Pong{
			ClientTime: p.Ping.ClientTime, ServerTime: time.Now().UnixMilli(),
		}}})
	case *pb.Envelope_MoveInput:
		if c.state != stateInWorld || !c.moves.Allow() {
			return
		}
		c.member.Move(p.MoveInput.InputSeq, p.MoveInput.DirX, p.MoveInput.DirY)
	case *pb.Envelope_RegisterReq:
		if c.expect(seq, stateConnected) {
			c.handleRegister(ctx, seq, p.RegisterReq)
		}
	case *pb.Envelope_LoginReq:
		if c.expect(seq, stateConnected) {
			c.handleLogin(ctx, seq, p.LoginReq)
		}
	case *pb.Envelope_CharListReq:
		if c.expect(seq, stateAuthed) {
			c.handleCharList(ctx, seq)
		}
	case *pb.Envelope_CreateCharReq:
		if c.expect(seq, stateAuthed) {
			c.handleCreateChar(ctx, seq, p.CreateCharReq)
		}
	case *pb.Envelope_DeleteCharReq:
		if c.expect(seq, stateAuthed) {
			c.handleDeleteChar(ctx, seq, p.DeleteCharReq)
		}
	case *pb.Envelope_EnterWorldReq:
		if c.state == stateInWorld {
			c.fail(seq, pb.ErrorCode_ERROR_CODE_ALREADY_IN_WORLD, "already in world")
			return
		}
		if c.expect(seq, stateAuthed) {
			c.handleEnterWorld(ctx, seq, p.EnterWorldReq)
		}
	case *pb.Envelope_LeaveWorldReq:
		if c.state != stateInWorld {
			c.fail(seq, pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD, "not in world")
			return
		}
		c.handleLeaveWorld(seq)
	default:
		c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "unexpected message")
	}
}

func (c *conn) expect(seq uint32, want connState) bool {
	if c.state == want {
		return true
	}
	c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "not allowed in the current state")
	return false
}

func (c *conn) handleRegister(ctx context.Context, seq uint32, r *pb.RegisterReq) {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if _, err := c.srv.deps.Auth.Register(rctx, c.ip, r.Username, r.Password); err != nil {
		c.failErr(seq, err)
		return
	}
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_RegisterResp{RegisterResp: &pb.RegisterResp{}}})
}

func (c *conn) handleLogin(ctx context.Context, seq uint32, r *pb.LoginReq) {
	if r.ProtocolVersion != ProtocolVersion {
		c.fail(seq, pb.ErrorCode_ERROR_CODE_VERSION_MISMATCH, fmt.Sprintf("server speaks protocol %d", ProtocolVersion))
		return
	}
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	id, err := c.srv.deps.Auth.Login(rctx, c.ip, r.Username, r.Password)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	if prev := c.srv.deps.World.Claim(id, c); prev != nil {
		prev.Kick(pb.KickReason_KICK_REASON_DUPLICATE_LOGIN)
		select {
		case <-prev.Done(): // previous session has left the world and saved its character
		case <-time.After(requestTimeout):
			c.log.Warn("previous session did not close in time", "account_id", id)
		}
	}
	c.authTimer.Stop()
	c.accountID = id
	c.state = stateAuthed
	c.log.Info("login", "account_id", id)
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_LoginResp{LoginResp: &pb.LoginResp{AccountId: id}}})
}

func summary(ch store.Character) *pb.CharSummary {
	return &pb.CharSummary{
		Id: ch.ID, Slot: int32(ch.Slot), Name: ch.Name, Class: classes.ToPB(ch.Class),
		Level: int32(ch.Level), MapId: ch.MapID,
	}
}

func (c *conn) handleCharList(ctx context.Context, seq uint32) {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	chars, err := c.srv.deps.Chars.ListCharacters(rctx, c.accountID)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	resp := &pb.CharListResp{}
	for _, ch := range chars {
		resp.Chars = append(resp.Chars, summary(ch))
	}
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_CharListResp{CharListResp: resp}})
}

func (c *conn) handleCreateChar(ctx context.Context, seq uint32, r *pb.CreateCharReq) {
	if r.Slot < 1 || r.Slot > 3 {
		c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "slot must be 1-3")
		return
	}
	class, ok := classes.FromPB(r.Class)
	if !ok {
		c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "invalid class")
		return
	}
	name, key, err := auth.NormalizeCharName(r.Name)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	mapID, pos := c.srv.deps.World.StartPosition()
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ch, err := c.srv.deps.Chars.CreateCharacter(rctx, store.NewCharacter{
		AccountID: c.accountID, Slot: int(r.Slot), Name: name, NameKey: key, Class: class,
		MapID: mapID, X: pos.X, Y: pos.Y,
	})
	if err != nil {
		c.failErr(seq, err)
		return
	}
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_CreateCharResp{CreateCharResp: &pb.CreateCharResp{Char: summary(ch)}}})
}

func (c *conn) handleDeleteChar(ctx context.Context, seq uint32, r *pb.DeleteCharReq) {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ch, err := c.srv.deps.Chars.GetCharacter(rctx, c.accountID, r.CharId)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	if norm.NFC.String(r.ConfirmName) != ch.Name {
		c.fail(seq, pb.ErrorCode_ERROR_CODE_BAD_REQUEST, "confirm_name does not match the character name")
		return
	}
	if err := c.srv.deps.Chars.DeleteCharacter(rctx, c.accountID, r.CharId); err != nil {
		c.failErr(seq, err)
		return
	}
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_DeleteCharResp{DeleteCharResp: &pb.DeleteCharResp{}}})
}

func (c *conn) handleEnterWorld(ctx context.Context, seq uint32, r *pb.EnterWorldReq) {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ch, err := c.srv.deps.Chars.GetCharacter(rctx, c.accountID, r.CharId)
	if err != nil {
		c.failErr(seq, err)
		return
	}
	m, err := c.srv.deps.World.Enter(rctx, world.EnterRequest{Char: ch, Out: c.out, RespSeq: seq})
	if err != nil {
		c.failErr(seq, err)
		return
	}
	// The zone has already queued EnterWorldResp for this seq.
	c.member = m
	c.state = stateInWorld
	c.log.Info("enter world", "account_id", c.accountID, "char_id", ch.ID, "map", m.MapID, "zone", m.ZoneNo)
}

func (c *conn) handleLeaveWorld(seq uint32) {
	select {
	case err := <-c.member.Leave():
		if err != nil {
			c.log.Error("save on leave failed", "err", err)
		}
	case <-time.After(requestTimeout):
		c.log.Warn("leave world timed out")
	}
	c.member = nil
	c.state = stateAuthed
	c.send(&pb.Envelope{Seq: seq, Payload: &pb.Envelope_LeaveWorldResp{LeaveWorldResp: &pb.LeaveWorldResp{}}})
}
```

- [ ] **Step 8: Fetch dependencies, build and test**

```bash
go get github.com/coder/websocket@v1.8.15 golang.org/x/time@v0.16.0
go mod tidy
go vet ./internal/gateway/
go test -race ./internal/gateway/
```

Expected: vet prints nothing; tests `ok`.

- [ ] **Step 9: Commit and push**

```bash
git add internal/gateway go.mod go.sum
git commit -m "Add the WebSocket gateway

Each connection has a read loop and a write loop joined by a bounded
outbox; a full outbox kicks the client as a slow consumer. Enforces the
connection state machine (connected -> authed -> in world), the
4 KB message limit, 30 packets/s (kick after 5 seconds of drops),
20 moves/s, a 30s login deadline and a 30s idle timeout. Duplicate
logins kick the old session and wait until its character is saved.
A panic while handling a request closes only that connection.
Shutdown kicks every connection and waits for them to leave."
git push
```

---

### Task 18: App wiring, health check and graceful shutdown

**Files:**
- Create: `internal/app/app.go`

(Covered by the e2e tests in Task 20.)

- [ ] **Step 1: Implement**

```go
// Package app wires the server together: store, maps, saver, world, auth and gateway.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/config"
	"demonslayer2d/internal/gateway"
	"demonslayer2d/internal/mapdata"
	"demonslayer2d/internal/saver"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/world"
)

type Options struct {
	Store      *store.Store // use this store instead of opening cfg.DatabaseURL (tests); the app does not close it
	AuthParams *auth.Params // override password hashing cost (tests)
}

type App struct {
	cfg      config.Config
	log      *slog.Logger
	store    *store.Store
	ownStore bool
	saver    *saver.Saver
	world    *world.World
	gw       *gateway.Server
	mux      *http.ServeMux

	cancelSaver, cancelWorld, cancelStats context.CancelFunc
	statsWG                               sync.WaitGroup
}

func New(ctx context.Context, cfg config.Config, log *slog.Logger, opts Options) (*App, error) {
	a := &App{cfg: cfg, log: log, store: opts.Store}
	if a.store == nil {
		st, err := store.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		a.store, a.ownStore = st, true
	}
	fail := func(err error) (*App, error) {
		a.closeStore()
		return nil, err
	}
	if err := a.store.Migrate(ctx); err != nil {
		return fail(err)
	}
	maps, err := mapdata.LoadDir(cfg.MapsDir)
	if err != nil {
		return fail(err)
	}
	a.saver = saver.New(a.store, log)
	a.world, err = world.New(world.Config{
		StartMap:     cfg.StartMap,
		TickRate:     cfg.TickRate,
		MoveSpeed:    float32(cfg.MoveSpeedTiles),
		AOICellSize:  cfg.AOICellSize,
		ZoneCapacity: cfg.ZoneCapacity,
		SaveInterval: cfg.SaveInterval,
		IdleZoneTTL:  5 * time.Minute,
		JanitorEvery: 30 * time.Second,
	}, maps, a.saver, log)
	if err != nil {
		return fail(err)
	}
	params := auth.DefaultParams
	if opts.AuthParams != nil {
		params = *opts.AuthParams
	}
	authSvc, err := auth.NewService(a.store, params, cfg.RegisterPerHour)
	if err != nil {
		return fail(err)
	}
	a.gw = gateway.NewServer(gateway.Deps{Auth: authSvc, Chars: a.store, World: a.world, Log: log})
	a.mux = http.NewServeMux()
	a.mux.Handle("/ws", a.gw)
	a.mux.HandleFunc("/healthz", a.healthz)
	return a, nil
}

func (a *App) Handler() http.Handler { return a.mux }

// Start launches the saver, the world's zones and the per-minute stats log.
func (a *App) Start() {
	var sctx, wctx, stctx context.Context
	sctx, a.cancelSaver = context.WithCancel(context.Background())
	wctx, a.cancelWorld = context.WithCancel(context.Background())
	stctx, a.cancelStats = context.WithCancel(context.Background())
	go a.saver.Run(sctx)
	a.world.Start(wctx)
	a.statsWG.Add(1)
	go func() {
		defer a.statsWG.Done()
		a.logStats(stctx)
	}()
}

// Shutdown kicks every player, stops the zones, flushes pending saves and closes the store.
func (a *App) Shutdown(ctx context.Context) error {
	var errs []error
	if err := a.gw.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("gateway shutdown: %w", err))
	}
	a.cancelStats()
	a.statsWG.Wait()
	a.cancelWorld()
	a.world.Wait()
	a.saver.Stop()
	select {
	case <-a.saver.Done():
	case <-ctx.Done():
		a.cancelSaver()
		<-a.saver.Done()
		errs = append(errs, errors.New("saver: flush timed out, some saves were lost"))
	}
	a.cancelSaver()
	a.closeStore()
	return errors.Join(errs...)
}

func (a *App) closeStore() {
	if a.ownStore {
		a.store.Close()
	}
}

func (a *App) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.store.Ping(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (a *App) logStats(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			stats := a.world.Stats()
			var players int
			var tickMax, tickP99 time.Duration
			for _, s := range stats {
				players += s.Players
				tickMax = max(tickMax, s.Max)
				tickP99 = max(tickP99, s.P99)
			}
			a.log.Info("stats",
				"connections", a.gw.ConnCount(),
				"online_accounts", a.world.Online(),
				"in_world", players,
				"zones", len(stats),
				"tick_max", tickMax,
				"tick_p99", tickP99)
		}
	}
}
```

- [ ] **Step 2: Build**

Run: `go build ./... && go vet ./internal/app/`
Expected: no output.

- [ ] **Step 3: Commit and push**

```bash
git add internal/app
git commit -m "Wire the server together and add graceful shutdown

app.New opens the store, migrates, loads maps and builds the saver,
world, auth service and gateway; /ws serves the protocol and /healthz
checks the database. Shutdown kicks players, stops zones, flushes the
saver (bounded by the context) and closes the store. A stats line is
logged every minute."
git push
```

---

### Task 19: Go protocol client

**Files:**
- Create: `internal/client/client.go`

(Exercised by the e2e tests in Task 20 and by `botclient`.)

- [ ] **Step 1: Implement**

```go
// Package client is a Go implementation of the game protocol, used by botclient and e2e tests.
package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"demonslayer2d/internal/gateway"
	"demonslayer2d/internal/pb"
)

// ServerError is an Error envelope returned for a request.
type ServerError struct {
	Code    pb.ErrorCode
	Message string
}

func (e *ServerError) Error() string { return fmt.Sprintf("server error %s: %s", e.Code, e.Message) }

// Code returns the server error code inside err, or UNSPECIFIED.
func Code(err error) pb.ErrorCode {
	var se *ServerError
	if errors.As(err, &se) {
		return se.Code
	}
	return pb.ErrorCode_ERROR_CODE_UNSPECIFIED
}

type Client struct {
	ws       *websocket.Conn
	seq      atomic.Uint32
	inputSeq atomic.Uint32

	mu      sync.Mutex
	pending map[uint32]chan *pb.Envelope
	readErr error

	pushes chan *pb.Envelope
	closed chan struct{}
}

func Dial(ctx context.Context, url string) (*Client, error) {
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(1 << 20)
	c := &Client{
		ws:      ws,
		pending: make(map[uint32]chan *pb.Envelope),
		pushes:  make(chan *pb.Envelope, 4096),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// Pushes yields server-initiated messages (seq 0). Messages are dropped if it is not drained.
func (c *Client) Pushes() <-chan *pb.Envelope { return c.pushes }

// Closed is closed when the connection ends.
func (c *Client) Closed() <-chan struct{} { return c.closed }

// Err is the read error that ended the connection.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readErr
}

func (c *Client) Close() { _ = c.ws.Close(websocket.StatusNormalClosure, "") }

func (c *Client) readLoop() {
	defer close(c.closed)
	for {
		_, data, err := c.ws.Read(context.Background())
		if err != nil {
			c.mu.Lock()
			c.readErr = err
			c.mu.Unlock()
			return
		}
		env := &pb.Envelope{}
		if err := proto.Unmarshal(data, env); err != nil {
			continue
		}
		if env.Seq != 0 {
			c.mu.Lock()
			ch, ok := c.pending[env.Seq]
			delete(c.pending, env.Seq)
			c.mu.Unlock()
			if ok {
				ch <- env
				continue
			}
		}
		select {
		case c.pushes <- env:
		default:
		}
	}
}

func (c *Client) write(ctx context.Context, env *pb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return err
	}
	return c.ws.Write(ctx, websocket.MessageBinary, data)
}

// Request sends env with a fresh seq and waits for its response. Error envelopes become *ServerError.
func (c *Client) Request(ctx context.Context, env *pb.Envelope) (*pb.Envelope, error) {
	env.Seq = c.seq.Add(1)
	ch := make(chan *pb.Envelope, 1)
	c.mu.Lock()
	c.pending[env.Seq] = ch
	c.mu.Unlock()
	forget := func() {
		c.mu.Lock()
		delete(c.pending, env.Seq)
		c.mu.Unlock()
	}
	if err := c.write(ctx, env); err != nil {
		forget()
		return nil, err
	}
	select {
	case resp := <-ch:
		if e := resp.GetError(); e != nil {
			return nil, &ServerError{Code: e.Code, Message: e.Message}
		}
		return resp, nil
	case <-c.closed:
		forget()
		return nil, fmt.Errorf("client: connection closed: %w", c.Err())
	case <-ctx.Done():
		forget()
		return nil, ctx.Err()
	}
}

func (c *Client) Register(ctx context.Context, username, password string) error {
	_, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_RegisterReq{RegisterReq: &pb.RegisterReq{Username: username, Password: password}}})
	return err
}

func (c *Client) Login(ctx context.Context, username, password string) (int64, error) {
	return c.LoginVersion(ctx, username, password, gateway.ProtocolVersion)
}

func (c *Client) LoginVersion(ctx context.Context, username, password string, version uint32) (int64, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_LoginReq{LoginReq: &pb.LoginReq{
		Username: username, Password: password, ProtocolVersion: version,
	}}})
	if err != nil {
		return 0, err
	}
	return resp.GetLoginResp().GetAccountId(), nil
}

func (c *Client) ListChars(ctx context.Context) ([]*pb.CharSummary, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_CharListReq{CharListReq: &pb.CharListReq{}}})
	if err != nil {
		return nil, err
	}
	return resp.GetCharListResp().GetChars(), nil
}

func (c *Client) CreateChar(ctx context.Context, slot int32, name string, class pb.CharClass) (*pb.CharSummary, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_CreateCharReq{CreateCharReq: &pb.CreateCharReq{
		Slot: slot, Name: name, Class: class,
	}}})
	if err != nil {
		return nil, err
	}
	return resp.GetCreateCharResp().GetChar(), nil
}

func (c *Client) DeleteChar(ctx context.Context, charID int64, confirmName string) error {
	_, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_DeleteCharReq{DeleteCharReq: &pb.DeleteCharReq{
		CharId: charID, ConfirmName: confirmName,
	}}})
	return err
}

func (c *Client) EnterWorld(ctx context.Context, charID int64) (*pb.EnterWorldResp, error) {
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_EnterWorldReq{EnterWorldReq: &pb.EnterWorldReq{CharId: charID}}})
	if err != nil {
		return nil, err
	}
	r := resp.GetEnterWorldResp()
	if r == nil {
		return nil, fmt.Errorf("client: unexpected response to EnterWorldReq: %v", resp)
	}
	return r, nil
}

func (c *Client) LeaveWorld(ctx context.Context) error {
	_, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_LeaveWorldReq{LeaveWorldReq: &pb.LeaveWorldReq{}}})
	return err
}

// Move sends a direction (no response). It returns the input seq used.
func (c *Client) Move(ctx context.Context, dx, dy float32) (uint32, error) {
	seq := c.inputSeq.Add(1)
	return seq, c.write(ctx, &pb.Envelope{Payload: &pb.Envelope_MoveInput{MoveInput: &pb.MoveInput{InputSeq: seq, DirX: dx, DirY: dy}}})
}

// Ping measures the round trip time.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	resp, err := c.Request(ctx, &pb.Envelope{Payload: &pb.Envelope_Ping{Ping: &pb.Ping{ClientTime: start.UnixMilli()}}})
	if err != nil {
		return 0, err
	}
	if resp.GetPong() == nil {
		return 0, fmt.Errorf("client: unexpected response to Ping: %v", resp)
	}
	return time.Since(start), nil
}

// SendRaw writes arbitrary bytes as one binary message (for protocol tests).
func (c *Client) SendRaw(ctx context.Context, data []byte) error {
	return c.ws.Write(ctx, websocket.MessageBinary, data)
}
```

- [ ] **Step 2: Build**

Run: `go vet ./internal/client/`
Expected: no output.

- [ ] **Step 3: Commit and push**

```bash
git add internal/client
git commit -m "Add a Go protocol client for bots and end-to-end tests

Matches responses to requests by seq, turns Error envelopes into
*ServerError, and exposes server pushes (EntityEnter/Leave, WorldTick,
Kicked) on a channel."
git push
```

---

### Task 20: End-to-end tests

**Files:**
- Create: `e2e/e2e_test.go`

- [ ] **Step 1: Write the tests**

```go
package e2e

import (
	"context"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"demonslayer2d/internal/app"
	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/client"
	"demonslayer2d/internal/config"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store/storetest"
)

type server struct{ ws, http string }

func startServer(t *testing.T) server {
	t.Helper()
	st := storetest.New(t)
	cfg := config.Config{
		MapsDir: "../maps", StartMap: "kumotori_1", TickRate: 10, MoveSpeedTiles: 4,
		AOICellSize: 10, ZoneCapacity: 50, SaveInterval: time.Minute, LogLevel: slog.LevelInfo,
		RegisterPerHour: 100,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := app.New(context.Background(), cfg, log, app.Options{
		Store:      st,
		AuthParams: &auth.Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32},
	})
	if err != nil {
		t.Fatal(err)
	}
	a.Start()
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
		srv.Close()
	})
	return server{ws: "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws", http: srv.URL}
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func dial(t *testing.T, s server) *client.Client {
	t.Helper()
	c, err := client.Dial(ctxT(t), s.ws)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

type player struct {
	c      *client.Client
	charID int64
	enter  *pb.EnterWorldResp
}

func newPlayer(t *testing.T, s server, username, charName string, class pb.CharClass) player {
	t.Helper()
	ctx := ctxT(t)
	c := dial(t, s)
	if err := c.Register(ctx, username, "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Login(ctx, username, "password123"); err != nil {
		t.Fatal(err)
	}
	ch, err := c.CreateChar(ctx, 1, charName, class)
	if err != nil {
		t.Fatal(err)
	}
	ew, err := c.EnterWorld(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	return player{c: c, charID: ch.Id, enter: ew}
}

func waitFor(t *testing.T, c *client.Client, d time.Duration, match func(*pb.Envelope) bool) *pb.Envelope {
	t.Helper()
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for {
		select {
		case env := <-c.Pushes():
			if match(env) {
				return env
			}
		case <-c.Closed():
			for {
				select {
				case env := <-c.Pushes():
					if match(env) {
						return env
					}
				default:
					t.Fatalf("connection closed before a matching message: %v", c.Err())
					return nil
				}
			}
		case <-deadline.C:
			t.Fatalf("no matching message within %v", d)
			return nil
		}
	}
}

func enterOf(id uint64) func(*pb.Envelope) bool {
	return func(e *pb.Envelope) bool { return e.GetEntityEnter().GetId() == id }
}

func isKicked(e *pb.Envelope) bool { return e.GetKicked() != nil }

func TestTwoPlayersSeeEachOtherMove(t *testing.T) {
	s := startServer(t)
	alice := newPlayer(t, s, "alice01", "Alice", pb.CharClass_CHAR_CLASS_VIEM)
	bob := newPlayer(t, s, "bob01", "Bob", pb.CharClass_CHAR_CLASS_THUY)

	waitFor(t, bob.c, 2*time.Second, enterOf(alice.enter.SelfEntityId))
	waitFor(t, alice.c, 2*time.Second, enterOf(bob.enter.SelfEntityId))

	start := time.Now()
	if _, err := alice.c.Move(ctxT(t), 1, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, bob.c, 2*time.Second, func(e *pb.Envelope) bool {
		for _, m := range e.GetWorldTick().GetMoves() {
			if m.Id == alice.enter.SelfEntityId && m.X > alice.enter.X {
				return true
			}
		}
		return false
	})
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("alice's move reached bob after %v, want <= 200ms", elapsed)
	}
}

func TestDuplicateLoginKicksOldSession(t *testing.T) {
	s := startServer(t)
	first := newPlayer(t, s, "carol01", "Carol", pb.CharClass_CHAR_CLASS_MINH)

	second := dial(t, s)
	if _, err := second.Login(ctxT(t), "carol01", "password123"); err != nil {
		t.Fatal(err)
	}
	k := waitFor(t, first.c, 2*time.Second, isKicked)
	if k.GetKicked().Reason != pb.KickReason_KICK_REASON_DUPLICATE_LOGIN {
		t.Fatalf("reason = %v", k.GetKicked().Reason)
	}
	select {
	case <-first.c.Closed():
	case <-time.After(5 * time.Second):
		t.Fatal("old connection was not closed")
	}
	chars, err := second.ListChars(ctxT(t))
	if err != nil || len(chars) != 1 {
		t.Fatalf("chars = %v, %v", chars, err)
	}
}

func TestLeaveAndReenterRestoresPosition(t *testing.T) {
	s := startServer(t)
	p := newPlayer(t, s, "dave01", "Dave", pb.CharClass_CHAR_CLASS_NHAM)
	self := p.enter.SelfEntityId
	ctx := ctxT(t)

	if _, err := p.c.Move(ctx, 1, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, p.c, 3*time.Second, func(e *pb.Envelope) bool {
		for _, m := range e.GetWorldTick().GetMoves() {
			if m.Id == self && m.X >= p.enter.X+1 {
				return true
			}
		}
		return false
	})
	if _, err := p.c.Move(ctx, 0, 0); err != nil {
		t.Fatal(err)
	}
	var last float32
	waitFor(t, p.c, 2*time.Second, func(e *pb.Envelope) bool {
		for _, m := range e.GetWorldTick().GetMoves() {
			if m.Id == self && m.DirX == 0 && m.DirY == 0 {
				last = m.X
				return true
			}
		}
		return false
	})

	if err := p.c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := p.c.EnterWorld(ctx, p.charID)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(again.X-last)) > 1e-3 || again.Y != p.enter.Y {
		t.Fatalf("re-entered at (%v, %v), want (%v, %v)", again.X, again.Y, last, p.enter.Y)
	}
}

func TestMalformedPacketKicks(t *testing.T) {
	s := startServer(t)
	c := dial(t, s)
	if err := c.SendRaw(ctxT(t), []byte{0xff, 0xff, 0xff}); err != nil {
		t.Fatal(err)
	}
	k := waitFor(t, c, 2*time.Second, isKicked)
	if k.GetKicked().Reason != pb.KickReason_KICK_REASON_PROTOCOL_ERROR {
		t.Fatalf("reason = %v", k.GetKicked().Reason)
	}
}

func TestRequestErrors(t *testing.T) {
	s := startServer(t)
	ctx := ctxT(t)
	expect := func(err error, want pb.ErrorCode) {
		t.Helper()
		if got := client.Code(err); got != want {
			t.Fatalf("error code = %v (%v), want %v", got, err, want)
		}
	}

	c := dial(t, s)
	_, err := c.ListChars(ctx)
	expect(err, pb.ErrorCode_ERROR_CODE_BAD_REQUEST) // not logged in
	if err := c.Register(ctx, "erin01", "password123"); err != nil {
		t.Fatal(err)
	}
	expect(c.Register(ctx, "ERIN01", "password123"), pb.ErrorCode_ERROR_CODE_USERNAME_TAKEN)
	expect(c.Register(ctx, "x", "password123"), pb.ErrorCode_ERROR_CODE_USERNAME_INVALID)
	_, err = c.LoginVersion(ctx, "erin01", "password123", 999)
	expect(err, pb.ErrorCode_ERROR_CODE_VERSION_MISMATCH)
	_, err = c.Login(ctx, "erin01", "wrongpass")
	expect(err, pb.ErrorCode_ERROR_CODE_AUTH_FAILED)
	if _, err := c.Login(ctx, "erin01", "password123"); err != nil {
		t.Fatal(err)
	}
	ch, err := c.CreateChar(ctx, 1, "Erin", pb.CharClass_CHAR_CLASS_PHONG)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateChar(ctx, 1, "Other", pb.CharClass_CHAR_CLASS_VIEM)
	expect(err, pb.ErrorCode_ERROR_CODE_SLOT_OCCUPIED)
	_, err = c.CreateChar(ctx, 2, "có space", pb.CharClass_CHAR_CLASS_VIEM)
	expect(err, pb.ErrorCode_ERROR_CODE_NAME_INVALID)
	_, err = c.CreateChar(ctx, 4, "Fine", pb.CharClass_CHAR_CLASS_VIEM)
	expect(err, pb.ErrorCode_ERROR_CODE_BAD_REQUEST)
	_, err = c.EnterWorld(ctx, 999999)
	expect(err, pb.ErrorCode_ERROR_CODE_CHAR_NOT_FOUND)
	expect(c.LeaveWorld(ctx), pb.ErrorCode_ERROR_CODE_NOT_IN_WORLD)
	expect(c.DeleteChar(ctx, ch.Id, "Wrong"), pb.ErrorCode_ERROR_CODE_BAD_REQUEST)

	other := dial(t, s)
	if err := other.Register(ctx, "frank01", "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Login(ctx, "frank01", "password123"); err != nil {
		t.Fatal(err)
	}
	_, err = other.CreateChar(ctx, 1, "ERIN", pb.CharClass_CHAR_CLASS_THUY)
	expect(err, pb.ErrorCode_ERROR_CODE_NAME_TAKEN)

	if _, err := c.EnterWorld(ctx, ch.Id); err != nil {
		t.Fatal(err)
	}
	_, err = c.EnterWorld(ctx, ch.Id)
	expect(err, pb.ErrorCode_ERROR_CODE_ALREADY_IN_WORLD)
	if _, err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.LeaveWorld(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteChar(ctx, ch.Id, "Erin"); err != nil {
		t.Fatal(err)
	}
}

func TestHealthz(t *testing.T) {
	s := startServer(t)
	resp, err := http.Get(s.http + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run the e2e tests against the dev Postgres**

Run: `make test-db`
Expected: every package `ok`, including `demonslayer2d/e2e`. If a test fails, fix the code it exercises (not the test), rerun, and only then continue.

- [ ] **Step 3: Commit and push**

```bash
git add e2e
git commit -m "Add end-to-end tests for the Phase 0 server

Runs the full app on a random port against a per-test Postgres schema
and checks: two players see each other and a move reaches the other
player within 200ms, duplicate login kicks the old session, leaving and
re-entering restores the saved position, malformed packets are kicked,
every request error code, and /healthz."
git push
```

---

### Task 21: Game server binary

**Files:**
- Create: `cmd/gameserver/main.go`

- [ ] **Step 1: Implement**

```go
// Command gameserver runs the game server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"demonslayer2d/internal/app"
	"demonslayer2d/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gameserver:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	a, err := app.New(initCtx, cfg, log, app.Options{})
	cancel()
	if err != nil {
		return err
	}
	a.Start()

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.ListenAddr)
		errc <- srv.ListenAndServe()
	}()

	var serveErr error
	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	}
	stop()

	sctx, scancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer scancel()
	shutdownErr := a.Shutdown(sctx)
	_ = srv.Shutdown(sctx)
	log.Info("stopped")
	return errors.Join(serveErr, shutdownErr)
}
```

- [ ] **Step 2: Run it and check health**

```bash
make run &
sleep 3
curl -s localhost:8080/healthz
kill %1
```

Expected: `ok`, then JSON log lines `"msg":"shutting down"` and `"msg":"stopped"`.

- [ ] **Step 3: Commit and push**

```bash
git add cmd/gameserver
git commit -m "Add the gameserver command with signal-driven graceful shutdown"
git push
```

---

### Task 22: Bot client

**Files:**
- Create: `cmd/botclient/main.go`

- [ ] **Step 1: Implement**

```go
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
```

- [ ] **Step 2: Try it against the running server**

```bash
make run &
sleep 3
go run ./cmd/botclient register -u tanjiro -p password123
go run ./cmd/botclient create -u tanjiro -p password123 -slot 1 -name Tanjiro -class viem
go run ./cmd/botclient chars -u tanjiro -p password123
go run ./cmd/botclient register -u nezuko -p password123
go run ./cmd/botclient create -u nezuko -p password123 -slot 1 -name Nezuko -class thuy
go run ./cmd/botclient chars -u nezuko -p password123
```

Note the two character ids, then in two terminals:

```bash
go run ./cmd/botclient play -u tanjiro -p password123 -id <TANJIRO_ID> -for 15s
go run ./cmd/botclient play -u nezuko -p password123 -id <NEZUKO_ID> -dir 1,0 -for 5s
```

Expected: the first terminal prints `enter` for Nezuko and a stream of `move` lines for her entity id. Stop the server with `kill %1`.

- [ ] **Step 3: Commit and push**

```bash
git add cmd/botclient
git commit -m "Add botclient for manual testing and load testing

Commands: register, chars, create, delete, play (enter the world, move
in a direction and print what the character sees) and load (N bots that
register, create, enter and random-walk while reporting in-world count,
kicks and ping)."
git push
```

---

### Task 23: Load test

**Files:** none (verification only)

- [ ] **Step 1: Run the server with the registration limit lifted**

```bash
DATABASE_URL='postgres://ds:ds@localhost:5433/ds?sslmode=disable' REGISTER_PER_HOUR=100000 go run ./cmd/gameserver 2>&1 | tee /tmp/ds-load.log &
sleep 3
```

- [ ] **Step 2: Run 1,000 bots for 3 minutes**

Run: `go run ./cmd/botclient load -n 1000 -for 3m -ramp 10ms`
Expected: the 5-second report reaches `in_world=1000`; `failed=0`; `kicks=map[]` (in particular no `KICK_REASON_SLOW_CONSUMER`).

- [ ] **Step 3: Check tick timing in the server's per-minute stats line**

Run: `grep '"msg":"stats"' /tmp/ds-load.log`
Expected: lines with `"in_world":1000` and `"tick_p99"` below `50ms`. Then stop the server (`kill %1`).

- [ ] **Step 4: If the criteria are not met**

Profile with `go tool pprof` on a CPU profile (add `net/http/pprof` temporarily), fix the hot spot, rerun Steps 1–3, and commit the fix with a message that states the measured before/after numbers. Do not relax the criteria.

---

### Task 24: README, spec alignment and final verification

**Files:**
- Create: `README.md`
- Modify: `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`

- [ ] **Step 1: Write `README.md`**

````markdown
# DemonSlayer2D server

Go game server for DemonSlayer2D. The Godot client lives in a separate repository and generates its protocol code from [`proto/`](proto/) and reads maps from [`maps/`](maps/).

Design documents: [game design](docs/game-design.md), [Phase 0 server spec](docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md).

## Requirements

- Go 1.25+
- Docker (for Postgres)

Code generators (`buf`, `protoc-gen-go`) are Go tool dependencies; nothing else to install.

## Run

```bash
make db-up   # Postgres 17 on localhost:5433 (databases ds and ds_test)
make run     # server on :8080 — WebSocket at /ws, health at /healthz
```

Configuration is read from environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `DATABASE_URL` | (required; `make run` sets it) | Postgres connection string |
| `LISTEN_ADDR` | `:8080` | HTTP/WebSocket address |
| `MAPS_DIR` | `./maps` | Map files |
| `START_MAP` | `kumotori_1` | Map for new characters |
| `TICK_RATE` | `10` | Zone ticks per second |
| `MOVE_SPEED_TILES` | `4` | Tiles per second at speed factor 1 |
| `AOI_CELL_SIZE` | `10` | Visibility grid cell size in tiles |
| `ZONE_CAPACITY` | `50` | Players per zone |
| `SAVE_INTERVAL` | `60s` | Periodic position save |
| `REGISTER_PER_HOUR` | `5` | Registrations per IP per hour |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## Try it with the bot client

```bash
go run ./cmd/botclient register -u tanjiro -p password123
go run ./cmd/botclient create   -u tanjiro -p password123 -slot 1 -name Tanjiro -class viem
go run ./cmd/botclient chars    -u tanjiro -p password123
go run ./cmd/botclient play     -u tanjiro -p password123 -id 1 -dir 1,0 -for 10s
```

Load test (lift the registration limit on the server first, e.g. `REGISTER_PER_HOUR=100000`):

```bash
go run ./cmd/botclient load -n 1000 -for 3m
```

## Develop

```bash
make proto    # regenerate internal/pb after editing proto/
make vet
make test     # unit tests; database tests are skipped
make test-db  # all tests including Postgres and end-to-end (needs make db-up)
```
````

- [ ] **Step 2: Align the spec with what was built**

In `docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md`:

1. Section 3.2, `characters.name` row: replace "chỉ mục duy nhất trên `lower(name)`" with "tính duy nhất kiểm bằng cột `name_key`". Add a row after it:

   `| name_key | text not null | Tên chữ thường sau chuẩn hóa NFC, tính trong Go; unique |`

2. Section 4.5, the first row of the limits table: replace "Đóng kết nối, `PROTOCOL_ERROR`" with "Đóng WebSocket với mã 1009 (message too big)".

3. Section 5.4: replace "Nhân vật là hình tròn bán kính 0,3 ô." with "Nhân vật là hộp vuông cạnh 0,6 ô (nửa cạnh 0,3 ô) đặt giữa vị trí." Replace the bullet "Bước di chuyển mỗi tick nhỏ hơn bán kính, nên không thể xuyên qua tường mỏng một ô." with "Mỗi bước được chia nhỏ thành các bước con tối đa 0,25 ô, nên dù đi nhanh cũng không thể xuyên qua tường mỏng một ô."

4. Section 5.8 table: add the row `| REGISTER_PER_HOUR | 5 | Số lần đăng ký tối đa mỗi giờ theo IP (nâng lên khi thử tải) |`, and in section 4.5 change the registration row's value to "`REGISTER_PER_HOUR` lần/giờ theo IP (mặc định 5)".

5. Section 9, criterion 2: replace the command list with "`register`, `chars`, `create`, `delete`, `play` (vào game, di chuyển và in những gì nhìn thấy), `load`".

6. At the top, change "Trạng thái: chờ duyệt." to "Trạng thái: đã duyệt, đã triển khai."

- [ ] **Step 3: Full verification**

```bash
gofmt -l .
go vet ./...
make test-db
```

Expected: `gofmt -l` prints nothing; `go vet` prints nothing; every package `ok` with no skipped database tests.

- [ ] **Step 4: Commit and push**

```bash
git add README.md docs/superpowers/specs/2026-11-18-giai-doan-0-nen-mong-server-design.md
git commit -m "Document how to run the server and align the Phase 0 spec

README covers setup, configuration, the bot client and tests. The spec
now records the box collider with sub-stepping, the name_key column,
the 1009 close for oversized messages, REGISTER_PER_HOUR and the final
botclient commands."
git push
```
