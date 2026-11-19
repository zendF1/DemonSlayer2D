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
	shutdownOnce                          sync.Once
}

// flushTimeout bounds the final save flush on shutdown. It is separate from the
// Shutdown context because the gateway may already have used that one up.
const flushTimeout = 30 * time.Second

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
	a.gw = gateway.NewServer(gateway.Deps{
		Auth: authSvc, Chars: a.store, World: a.world, Log: log, TrustedProxies: cfg.TrustedProxies,
	})
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
// ctx bounds waiting for connections to close; the save flush gets its own flushTimeout so a
// slow or stuck client can never cause saves to be thrown away. Calls after the first return nil.
func (a *App) Shutdown(ctx context.Context) error {
	err := errAlreadyShutDown
	a.shutdownOnce.Do(func() { err = a.shutdown(ctx) })
	if err == errAlreadyShutDown {
		return nil
	}
	return err
}

var errAlreadyShutDown = errors.New("app: already shut down")

func (a *App) shutdown(ctx context.Context) error {
	var errs []error
	if err := a.gw.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("gateway shutdown: %w", err))
	}
	a.cancelStats()
	a.statsWG.Wait()
	a.cancelWorld()
	a.world.Wait() // zones have now queued a save for every player still inside
	a.saver.Stop()
	fctx, fcancel := context.WithTimeout(context.Background(), flushTimeout)
	defer fcancel()
	select {
	case <-a.saver.Done():
	case <-fctx.Done():
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
