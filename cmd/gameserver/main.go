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
