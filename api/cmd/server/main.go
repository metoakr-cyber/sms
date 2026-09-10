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
	"github.com/ikmetrik/sms-platform/api/internal/adapter/obs"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/herosms"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/redis"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/storage"
	"github.com/ikmetrik/sms-platform/api/internal/config"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	authsvc "github.com/ikmetrik/sms-platform/api/internal/service/auth"
	catalogsvc "github.com/ikmetrik/sms-platform/api/internal/service/catalog"
	depositsvc "github.com/ikmetrik/sms-platform/api/internal/service/deposit"
	ordersvc "github.com/ikmetrik/sms-platform/api/internal/service/order"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	reviewsvc "github.com/ikmetrik/sms-platform/api/internal/service/review"
	ticketsvc "github.com/ikmetrik/sms-platform/api/internal/service/ticket"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
	httptransport "github.com/ikmetrik/sms-platform/api/internal/transport/http"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
	"github.com/ikmetrik/sms-platform/api/internal/worker"
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
	// Sentry log'lamadan ÖNCE kurulur: açılışta oluşan bir hata da izlenebilsin.
	// config üretimde SENTRY_DSN'i zorunlu kılıyor; burada gerçekten bir istemci
	// kurulmazsa o zorunluluk boş bir söz olurdu.
	sentryAdapter, err := setupSentry(cfg)
	if err != nil {
		return err
	}
	if sentryAdapter != nil {
		defer sentryAdapter.Flush(5 * time.Second)
	}
	setupLogger(cfg, sentryAdapter)

	metrics := setupMetrics(cfg)

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
	mail, err := buildMailer(cfg)
	if err != nil {
		return err
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
	orderBus := redis.NewOrderBus(rdb)
	webhookQueue := redis.NewWebhookQueue(rdb)

	secrets, err := crypto.New(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	// Sağlayıcı adaptörleri protokole göre kaydedilir.
	registry := provider.NewRegistry()
	registry.Register(fake.New(port.RealClock{}))
	registry.Register(herosms.New())

	// FX_PROVIDER yapılandırması BUGÜN OKUNMUYOR: tek kaynak TCMB.
	// İkinci bir kaynak eklendiğinde seçim buraya gelir; o zamana kadar
	// yapılandırmanın seçim sunduğunu ima etmemek için sabit bırakıldı.
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

	catalogService := catalogsvc.New(catalogsvc.Deps{
		TxRunner: txRunner, Registry: registry, Secrets: secrets, Clock: port.RealClock{},
	})

	orderService := ordersvc.New(ordersvc.Deps{
		TxRunner: txRunner, Registry: registry, Secrets: secrets,
		Wallet: walletService, Publisher: ordersvc.NewBusPublisher(orderBus),
		Clock: port.RealClock{},
	})

	// Dekont deposu AÇILIŞTA kurulur ve yazılabilirliği burada doğrulanır.
	//
	// Hata dönerse süreç BAŞLAMAZ: ilk yükleme anında keşfedilen bir izin
	// hatası kullanıcıya "beklenmeyen hata" olarak döner, dekont kaybolur ve
	// sebebi günlerce aranmaz. Yapılandırma üretimde UPLOAD_DIR'i zorunlu
	// kılıyor; burada gerçekten bir depo kurulmazsa o zorunluluk boş bir söz
	// olurdu.
	receiptStore, err := storage.NewLocal(cfg.UploadDir)
	if err != nil {
		return err
	}
	slog.Info("dekont deposu hazır", "dir", receiptStore.Root())

	depositService := depositsvc.New(depositsvc.Deps{
		TxRunner: txRunner, Wallet: walletService,
		Clock: port.RealClock{}, Receipts: receiptStore,
	})

	// Destek talepleri (FR-600). Para hareketi üretmez; dış çağrı yapmaz.
	ticketService := ticketsvc.New(ticketsvc.Deps{
		TxRunner: txRunner, Clock: port.RealClock{},
	})

	// Müşteri yorumları. Para hareketi üretmez; dış çağrı yapmaz.
	reviewService := reviewsvc.New(reviewsvc.Deps{
		TxRunner: txRunner, Clock: port.RealClock{},
	})

	authService := authsvc.New(authsvc.Deps{
		TxRunner: txRunner, Sessions: sessions, Mailer: mail,
		Captcha: cap, Limiter: limiter, Clock: port.RealClock{},
		SessionTTL: cfg.SessionTTL, BaseURL: cfg.PublicBaseURL,
	})

	// 4) Arka plan işleri
	//
	// VARSAYILAN olarak SUNUCU SÜRECİ İÇİNDE çalışırlar: ayrı bir işçi süreci,
	// tek VPS'te kazandırdığından fazla operasyon yükü getiriyordu.
	// WORKERS_IN_PROCESS=false verildiğinde işler burada başlatılmaz ve
	// `cmd/worker` süreci devralır — aynı işin iki süreçte birden koşmaması
	// bu tek anahtara bağlıdır (bkz. cmd/worker/main.go).
	//
	// İkinci bir SUNUCU örneği eklendiğinde bu işler Redis kilidiyle tek
	// örneğe indirilmelidir — aksi hâlde iki poller aynı siparişi işler.
	if cfg.WorkersInProcess {
		jobs := worker.New(worker.AllWithWebhook(worker.Deps{
			TxRunner: txRunner, Orders: orderService, FX: fxService,
			Wallet: walletService, Catalog: catalogService, Clock: port.RealClock{},
		}, webhookQueue, "herosms")...)
		jobs.Start(ctx)
	} else {
		// Sessiz kalmayız: işçi süreci unutulduysa iadeler hiç işlenmez ve
		// bu, haftalar sonra "para asılı kalmış" olarak fark edilir.
		slog.Warn("arka plan işleri bu süreçte KOŞMUYOR (WORKERS_IN_PROCESS=false) — " +
			"ayrı bir `worker` süreci çalışıyor olmalı")
	}

	// Kuyruk derinliği yalnız SORULARAK öğrenilir; periyodik örnekleme
	// sunucuda kalır çünkü LLEN maliyeti sabittir ve işçi süreci HTTP açmaz.
	if metrics != nil {
		metrics.StartSampler(ctx, 15*time.Second, func(ctx context.Context) {
			n, err := webhookQueue.Len(ctx)
			if err != nil {
				slog.Warn("kuyruk derinliği okunamadı", "err", err)
				return
			}
			metrics.SetQueueDepth("webhook", float64(n))
		})
	}

	// 5) Sunucu
	// Fiyat kuralı yönetimi (FR-703) ve katalog senkron tetikleyicisi.
	//
	// 🔴 BUNLAR BAĞLANMAZSA uçlar DERLENİR ama üretimde 500/503 döner:
	// router.go nil gördüğünde uçları kapatıp açılışta uyarı basar.
	// "Derleniyor" ile "çalışıyor" aynı şey değildir.
	ruleService := pricingsvc.NewRuleService(pricingsvc.RuleDeps{
		TxRunner: txRunner, FX: fxService, Clock: port.RealClock{},
		FXSafetyMargin: safety,
	})
	syncRunner := handler.NewSyncRunner(func(ctx context.Context, providerID int64) error {
		// Sıra ÖNEMLİ: boyutlar (ülke/servis eşleştirmeleri) olmadan teklif
		// senkronu ürünü bulamaz; bakiye en sona bırakılır çünkü diğer ikisi
		// başarısızsa bakiyeyi tazelemenin bir anlamı yok.
		if _, err := catalogService.SyncDimensions(ctx, providerID); err != nil {
			return err
		}
		if _, err := catalogService.SyncOffers(ctx, providerID); err != nil {
			return err
		}
		return catalogService.SyncProviderBalance(ctx, providerID)
	})

	router := httptransport.NewRouter(httptransport.Deps{
		Config: cfg, Pool: pool, Redis: rdb, Queries: queries,
		Sessions: sessions, Limiter: limiter, RateLimitFactor: cfg.RateLimitFactor,
		Secrets: secrets,
		AuthSvc: authService, WalletSvc: walletService, QuoteSvc: quoteService,
		OrderSvc: orderService, OrderBus: orderBus,
		DepositSvc:   depositService,
		TicketSvc:    ticketService,
		ReviewSvc:    reviewService,
		RuleSvc:      ruleService,
		CatalogSync:  syncRunner,
		Metrics:      metricsHandler(metrics),
		WebhookQueue: webhookQueue,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
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

// buildMailer MAIL_PROVIDER'a göre adaptörü seçer.
//
// Sessizce console'a DÜŞÜLMEZ: desteklenmeyen ya da yarım yapılandırılmış bir
// sağlayıcıda süreç hata verip çıkar. Koşulsuz console kablolaması, config'in
// üretimde koyduğu yasağı ETKİSİZ kılardı: süreç ayağa kalkar, hiçbir kullanıcı
// e-posta almaz ve kimse fark etmez.
func buildMailer(cfg *config.Config) (port.Mailer, error) {
	switch cfg.MailProvider {
	case "console":
		return mailer.NewConsole(cfg.MailFrom), nil
	case "smtp":
		return mailer.NewSMTP(mailer.SMTPConfig{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort,
			Username: cfg.SMTPUsername, Password: cfg.SMTPPassword,
			From: cfg.MailFrom,
		})
	case "resend":
		return mailer.NewResend(cfg.ResendAPIKey, cfg.MailFrom)
	default:
		return nil, fmt.Errorf("main: MAIL_PROVIDER=%q için adaptör yok — "+
			"süreç başlatılmıyor (e-postasız çalışmak sessiz arızadır)", cfg.MailProvider)
	}
}

// setupSentry DSN varsa hata izlemeyi kurar.
//
// DSN yoksa nil döner ve süreç devam eder: config üretimde DSN'i zaten zorunlu
// kılıyor, geliştirmede ise Sentry'siz çalışmak normaldir.
func setupSentry(cfg *config.Config) (*obs.Sentry, error) {
	if cfg.SentryDSN == "" {
		return nil, nil
	}
	s, err := obs.NewSentry(obs.SentryConfig{
		DSN:         cfg.SentryDSN,
		Environment: string(cfg.Env),
	})
	if err != nil {
		// Yapılandırılmış ama kurulamayan bir izleme, izleme yokluğundan
		// daha tehlikelidir: kurulduğu sanılır.
		return nil, err
	}
	return s, nil
}

func setupMetrics(cfg *config.Config) *obs.Metrics {
	if !cfg.MetricsEnabled {
		return nil
	}
	return obs.NewMetrics()
}

// metricsHandler metrik işleyicisini döner; metrikler kapalıysa nil.
//
// nil dönmek bilinçlidir: router.go bunu görüp ucu HİÇ tanımlamaz.
// "Kapalıyken boş yanıt veren bir uç" açık bir uçtur ve varlığını sızdırır.
func metricsHandler(m *obs.Metrics) http.Handler {
	if m == nil {
		return nil
	}
	return m.Handler()
}

func setupLogger(cfg *config.Config, sen *obs.Sentry) {
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
	// Sentry slog'un ÜSTÜNE takılır: kod tabanı hataları zaten slog.Error ile
	// bildiriyor, ayrı bir çağrı eklemek her yeni hata yolunda unutulabilecek
	// ikinci bir adım olurdu.
	if sen != nil {
		h = sen.SlogHandler(h)
	}
	slog.SetDefault(slog.New(h))
}
