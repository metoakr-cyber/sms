// Command worker arka plan işlerini AYRI bir süreçte koşturur.
//
// NEDEN AYRI BİR SÜREÇ SEÇENEĞİ VAR
// ─────────────────────────────────
// İşler bugün varsayılan olarak API sürecinin içinde çalışıyor (tek VPS'te
// ikinci bir süreci yönetmek kazandırdığından fazla yük getiriyordu). Ama
// dağıtım büyüdüğünde iki şey ayrışıyor: API'yi yeniden başlatmak (dağıtım,
// ölçekleme) işleri de kesiyor, ve API sürecini yatayda çoğaltmak işleri
// N kez koşturuyor. Bu ikili, işleri kendi sürecine taşıyarak çözülür.
//
// ÇİFT KOŞMA NASIL ENGELLENİYOR
// ─────────────────────────────
// TEK ANAHTAR: `WORKERS_IN_PROCESS`.
//
//	true  (varsayılan) → cmd/server işleri koşturur, cmd/worker AÇILMAZ.
//	false              → cmd/server işleri koşturmaz, cmd/worker koşturur.
//
// İki sürecin de işleri koşturduğu bir kombinasyon YOKTUR; anahtar ortak
// ortam dosyasından okunduğu için tek yerden yönetilir.
//
// Çift koşmanın bedeli ölçüldü: bakiye BOZULMAZDI — `Orders.Expire`
// `SELECT ... FOR UPDATE` altında durumu tekrar kontrol ediyor ve iade
// `order:{uuid}:refund` deterministik anahtarıyla yazılıyor (idempotent).
// Bozulan şey SAĞLAYICI TARAFI olurdu: iki `Cancel`/`Finish` çağrısı, iki kat
// TCMB isteği ve `rental-sync` turunda 810 yerine 1620 istek. Yani "sadece
// idempotent, çalışır" demek yeterli değildi.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/fx"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/obs"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/herosms"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/redis"
	"github.com/ikmetrik/sms-platform/api/internal/config"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	catalogsvc "github.com/ikmetrik/sms-platform/api/internal/service/catalog"
	ordersvc "github.com/ikmetrik/sms-platform/api/internal/service/order"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
	"github.com/ikmetrik/sms-platform/api/internal/worker"
)

// shutdownGrace kapanışta devam eden bir turun bitmesi için tanınan süre.
//
// `webhook-ingest` kuyruktan 2 saniye bloklayarak okur; `order-expirer` bir
// turda 100 siparişe kadar sağlayıcıya gidebilir. Turun ortasında kesmek,
// iptal edilmiş ama sağlayıcıda kapatılmamış siparişler bırakır —
// `activation-reaper` bunu toplar ama gereksiz yere.
const shutdownGrace = 30 * time.Second

// errWorkersInProcess çift koşma kapısının hata mesajı.
//
// Operatöre ne yapacağını SÖYLER: "yanlış yapılandırma" demek, saat 03:00'te
// dağıtım yapan kişiye yardımcı olmaz.
var errWorkersInProcess = errors.New(
	"worker: WORKERS_IN_PROCESS=true — işler zaten API sürecinde koşuyor.\n" +
		"  İşleri ayrı bir sürece taşımak için ortak ortam dosyasında\n" +
		"  WORKERS_IN_PROCESS=false yapın ve API sürecini yeniden başlatın;\n" +
		"  aksi hâlde aynı iş iki süreçte birden koşar (sağlayıcıya çift istek).")

func main() {
	if err := run(); err != nil {
		slog.Error("işçi başlatılamadı", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// 1) Yapılandırma — sunucudakiyle AYNI doğrulamadan geçer. Yarım
	//    yapılandırılmış bir işçi, sessizce iade yazmayan bir işçidir.
	config.LoadDotEnv()
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// 🔴 ÇİFT KOŞMA KAPISI. Anahtar true iken cmd/server işleri zaten
	// koşturuyor; bu süreç açılırsa aynı iş iki kez çalışır.
	if cfg.WorkersInProcess {
		return errWorkersInProcess
	}

	sentryAdapter, err := setupSentry(cfg)
	if err != nil {
		return err
	}
	if sentryAdapter != nil {
		defer sentryAdapter.Flush(5 * time.Second)
	}
	setupLogger(cfg, sentryAdapter)

	slog.Info("işçi başlatılıyor", "env", cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2) Bağımlılıklar
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

	// 3) Bağımlılık grafiği — cmd/server ile AYNI kablolama, yalnız HTTP
	//    tarafı (oturum deposu, hız limiti, captcha, mailer) yok.
	txRunner := postgres.NewTxRunner(pool)
	walletService := walletsvc.New(txRunner)
	orderBus := redis.NewOrderBus(rdb)
	webhookQueue := redis.NewWebhookQueue(rdb)

	secrets, err := crypto.New(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	registry := provider.NewRegistry()
	registry.Register(fake.New(port.RealClock{}))
	registry.Register(herosms.New())

	var fxProvider port.FXProvider = fx.NewTCMB()
	fxService := pricingsvc.NewFXService(txRunner, fxProvider, port.RealClock{}, cfg.FXMaxAge)

	catalogService := catalogsvc.New(catalogsvc.Deps{
		TxRunner: txRunner, Registry: registry, Secrets: secrets, Clock: port.RealClock{},
	})

	// Sipariş servisi olay yayınlamayı SÜRDÜRÜR: SSE akışı API sürecinde
	// dinleniyor ama olaylar Redis üzerinden taşınıyor. Yayıncıyı buradan
	// çıkarmak, iadeyi gören ekranın hiç güncellenmemesi demekti.
	orderService := ordersvc.New(ordersvc.Deps{
		TxRunner: txRunner, Registry: registry, Secrets: secrets,
		Wallet: walletService, Publisher: ordersvc.NewBusPublisher(orderBus),
		Clock: port.RealClock{},
	})

	// 4) İşler
	jobs := worker.New(worker.AllWithWebhook(worker.Deps{
		TxRunner: txRunner, Orders: orderService, FX: fxService,
		Wallet: walletService, Catalog: catalogService, Clock: port.RealClock{},
	}, webhookQueue, "herosms")...)
	jobs.Start(ctx)
	slog.Info("işler başlatıldı")

	<-ctx.Done()
	slog.Info("kapatma sinyali alındı — devam eden tur bekleniyor")

	// 5) Zarif kapanış: Runner ctx iptaliyle duruyor, Wait turların bitmesini
	//    bekliyor. Süre aşılırsa yine de çıkarız — takılı bir tur yüzünden
	//    süreç sonsuza kadar ayakta kalmamalı.
	done := make(chan struct{})
	go func() {
		jobs.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("işçi kapatıldı")
	case <-time.After(shutdownGrace):
		slog.Warn("kapanış süresi doldu — devam eden tur yarıda kesildi",
			"grace", shutdownGrace)
	}
	return nil
}

func setupSentry(cfg *config.Config) (*obs.Sentry, error) {
	if cfg.SentryDSN == "" {
		return nil, nil
	}
	return obs.NewSentry(obs.SentryConfig{
		DSN:         cfg.SentryDSN,
		Environment: string(cfg.Env),
	})
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
	var h slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	if cfg.Env.IsDevelopment() {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	// İşçi sürecinde bu ÖZELLİKLE önemli: burada bir hata olduğunda ekranda
	// bekleyen bir kullanıcı yok, yani kimse fark etmiyor.
	if sen != nil {
		h = sen.SlogHandler(h)
	}
	slog.SetDefault(slog.New(h))
}
