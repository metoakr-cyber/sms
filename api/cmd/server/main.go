// Command server HTTP API sunucusudur.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/config"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/redis"
	httptransport "github.com/ikmetrik/sms-platform/api/internal/transport/http"
)

func main() {
	if err := run(); err != nil {
		slog.Error("sunucu başlatılamadı", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// 1) Yapılandırma — eksikse BURADA dururuz, yarı çalışır bir sunucu başlatmayız.
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	setupLogger(cfg)

	slog.Info("başlatılıyor", "env", cfg.Env, "addr", cfg.HTTPAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2) Bağımlılıklar — erişilemiyorsa başlamayız.
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	slog.Info("postgres bağlandı")

	rdb, err := redis.New(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()
	slog.Info("redis bağlandı")

	// 3) Sunucu
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httptransport.NewRouter(httptransport.Deps{Config: cfg, Pool: pool, Redis: rdb}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// SSE akışları uzun sürer; WriteTimeout bilinçli olarak kapalıdır.
		// Zaman aşımı istek bazında context ile yönetilir (docs/design.md §8.3).
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("dinleniyor", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("kapatma sinyali alındı")
	}

	// 4) Zarif kapanış: devam eden istekler tamamlansın.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	slog.Info("kapatıldı")
	return nil
}

func setupLogger(cfg *config.Config) {
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	// Üretimde JSON (makine okunur), geliştirmede metin (insan okunur).
	var h slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	if cfg.Env.IsDevelopment() {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(h))
}
