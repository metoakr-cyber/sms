// Command server HTTP API sunucusudur.
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

	"github.com/ikmetrik/sms-platform/api/internal/adapter/captcha"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/fx"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/mailer"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/herosms"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/redis"
	"github.com/ikmetrik/sms-platform/api/internal/config"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	authsvc "github.com/ikmetrik/sms-platform/api/internal/service/auth"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
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
	//    .env yalnız geliştirme kolaylığıdır ve mevcut ortamı ezmez.
	config.LoadDotEnv()
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

	// 3) Bağımlılık grafiği — kablolama TEK YERDE, açıkça.
	//    Servisler somut adaptörleri değil port arayüzlerini görür; bu sayede
	//    testte sahte adaptörlerle aynı grafiği kurabiliriz.
	queries := db.New(pool)
	txRunner := postgres.NewTxRunner(pool)
	sessions := redis.NewSessionStore(rdb)
	limiter := redis.NewRateLimiter(rdb)

	// Mailer YAPILANDIRMAYA göre seçilir. Koşulsuz console kablolaması,
	// config'in üretimde koyduğu yasağı ETKİSİZ kılardı: süreç ayağa kalkar,
	// hiçbir kullanıcı e-posta almaz ve kimse fark etmez.
	var mail port.Mailer
	switch cfg.MailProvider {
	case "console":
		mail = mailer.NewConsole(cfg.MailFrom)
	default:
		// Sessizce console'a DÜŞMEYİZ. Desteklenmeyen bir sağlayıcı
		// yapılandırıldıysa süreç başlamaz.
		return fmt.Errorf("main: MAIL_PROVIDER=%q için adaptör yok — "+
			"süreç başlatılmıyor (e-postasız çalışmak sessiz arızadır)", cfg.MailProvider)
	}

	var cap port.Captcha = captcha.Disabled{}
	if cfg.RecaptchaSecretKey != "" {
		cap = captcha.NewReCaptcha(cfg.RecaptchaSecretKey)
	}
	if !cap.Enabled() {
		// Üretimde config paketi anahtarı zorunlu kılıyor; buraya yalnız
		// geliştirmede düşeriz. Yine de sessiz kalmayız.
		slog.Warn("reCAPTCHA devre dışı — yalnız geliştirme için kabul edilebilir")
	}

	walletService := walletsvc.New(txRunner)

	secrets, err := crypto.New(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	// Sağlayıcı adaptörleri protokole göre kaydedilir.
	registry := provider.NewRegistry()
	registry.Register(fake.New(port.RealClock{}))
	registry.Register(herosms.New())
	// HeroSMS adaptörü hazır olduğunda buraya eklenecek.

	var fxProvider port.FXProvider = fx.NewTCMB()
	fxService := pricingsvc.NewFXService(txRunner, fxProvider, port.RealClock{}, cfg.FXMaxAge)

	safety, err := money.MarginRate(cfg.FXSafetyMarginPct)
	if err != nil {
		return err
	}
	quoteService := pricingsvc.NewQuoteService(pricingsvc.QuoteDeps{
		TxRunner: txRunner, Registry: registry, Secrets: secrets,
		FX: fxService, Clock: port.RealClock{}, FXSafetyMargin: safety,
	})

	authService := authsvc.New(authsvc.Deps{
		TxRunner: txRunner, Sessions: sessions, Mailer: mail,
		Captcha: cap, Limiter: limiter, Clock: port.RealClock{},
		SessionTTL: cfg.SessionTTL, BaseURL: cfg.PublicBaseURL,
	})

	// 4) Sunucu
	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httptransport.NewRouter(httptransport.Deps{
			Config: cfg, Pool: pool, Redis: rdb, Queries: queries,
			Sessions: sessions, Limiter: limiter,
			AuthSvc: authService, WalletSvc: walletService, QuoteSvc: quoteService,
		}),
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

	// 5) Zarif kapanış: devam eden istekler tamamlansın.
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
