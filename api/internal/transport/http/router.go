// Package http HTTP taşıma katmanıdır: DTO bağlama, doğrulama, servis çağrısı,
// yanıt yazma. İş kuralı, veritabanı erişimi ve dış API çağrısı BURADA OLMAZ.
package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/config"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	authsvc "github.com/ikmetrik/sms-platform/api/internal/service/auth"
	depositsvc "github.com/ikmetrik/sms-platform/api/internal/service/deposit"
	ordersvc "github.com/ikmetrik/sms-platform/api/internal/service/order"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	reviewsvc "github.com/ikmetrik/sms-platform/api/internal/service/review"
	ticketsvc "github.com/ikmetrik/sms-platform/api/internal/service/ticket"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Deps yönlendiricinin ihtiyaç duyduğu bağımlılıklar.
type Deps struct {
	Config       *config.Config
	Pool         *pgxpool.Pool
	Redis        *goredis.Client
	Queries      *db.Queries
	Sessions     port.SessionStore
	Limiter      port.RateLimiter
	Secrets      *crypto.SecretBox
	AuthSvc      *authsvc.Service
	WebhookQueue handler.WebhookQueue
	WalletSvc    *walletsvc.Service
	QuoteSvc     *pricingsvc.QuoteService
	OrderSvc     *ordersvc.Service
	OrderBus     handler.OrderStream
	DepositSvc   *depositsvc.Service

	// TicketSvc destek talepleri (FR-600).
	TicketSvc *ticketsvc.Service

	// ReviewSvc müşteri yorumları.
	ReviewSvc *reviewsvc.Service

	// RuleSvc fiyat kuralı yönetimi (FR-703). nil ise /admin/pricing-rules
	// uçları 500 döner.
	RuleSvc *pricingsvc.RuleService

	// CatalogSync katalog senkronunu arka planda çalıştırır.
	//
	// nil ise POST /admin/providers/:id/sync ucu KURULUR ama 503 döner ve
	// açılışta uyarı basılır. Uç hiç kurulmasaydı yönetici 404 görür ve
	// sebebini arardı; sessiz eksiklik yerine konuşan bir eksiklik.
	CatalogSync handler.CatalogSyncer

	// Metrics Prometheus toplayıcısının HTTP işleyicisi. nil ise /metrics
	// ucu HİÇ tanımlanmaz (METRICS_ENABLED=false).
	Metrics http.Handler
}

// NewRouter uygulamanın HTTP yönlendiricisini kurar.
func NewRouter(d Deps) *gin.Engine {
	if d.Config.Env.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.RedirectTrailingSlash = false

	// Vekil güveni YAPILANDIRMADAN gelir, koda gömülü DEĞİLDİR.
	//
	// Gömülü "127.0.0.1, ::1" listesi geliştirmede doğru, üretimde SESSİZCE
	// YANLIŞTI: Docker'da Caddy ayrı bir konteynerdir, peer 172.x.x.x olur ve
	// hiçbir vekil başlığı okunmaz. Sonuç: IP hız limiti bütün kullanıcıları
	// tek kovaya koyar, webhook izin listesi sağlayıcının her bildirimini eler.
	// Varsayılan "tüm vekillere güven" ise hız limitini tümüyle atlatılabilir
	// kılardı — config.cidrs() bu yüzden 0.0.0.0/0'ı reddediyor.
	if err := r.SetTrustedProxies(d.Config.TrustedProxies); err != nil {
		// Yapılandırma açılışta doğrulandı; buraya düşmek programlama hatasıdır.
		panic(fmt.Sprintf("güvenilen vekil listesi geçersiz: %v", err))
	}

	r.Use(middleware.RequestID(), middleware.Recovery(), middleware.Logger())

	r.GET("/healthz", healthz)
	r.GET("/readyz", readyz(d))

	// /metrics — YALNIZ İÇ AĞ.
	//
	// 🔴 Uç kimlik doğrulaması İSTEMEZ; korunması ters vekildedir
	// (deploy/Caddyfile bu yolu dışarıdan 404 yapar). Metrikler sistemin
	// hacmini, hata oranını ve kuyruk derinliğini açık eder — rakibe de,
	// saldırgana da. Vekil olmadan doğrudan internete açılırsa bu bilgi
	// herkese açıktır.
	if d.Metrics != nil {
		r.GET("/metrics", gin.WrapH(d.Metrics))
	}

	registerV1(r.Group("/api/v1"), d)

	r.NoRoute(func(c *gin.Context) { Fail(c, notFound) })
	return r
}

func registerV1(rg *gin.RouterGroup, d Deps) {
	responder := handler.Responder{
		OK: OK, NoContent: NoContent, Fail: Fail, FailField: FailFields,
	}
	secureCookie := !d.Config.Env.IsDevelopment()

	authH := handler.NewAuth(d.AuthSvc, d.Queries, responder, d.Config.SessionTTL, secureCookie)
	walletH := handler.NewWallet(d.WalletSvc, d.Queries, responder)
	catalogH := handler.NewCatalog(d.Queries, d.QuoteSvc, responder)
	orderH := handler.NewOrder(d.OrderSvc, d.OrderBus, responder)
	depositH := handler.NewDeposit(d.DepositSvc, responder)
	ticketH := handler.NewTicket(d.TicketSvc, responder)
	reviewH := handler.NewReview(d.ReviewSvc, responder)

	requireAuth := middleware.RequireAuth(middleware.AuthDeps{
		Sessions: d.Sessions, Queries: d.Queries,
		SessionTTL: d.Config.SessionTTL, FailFn: Fail,
	})

	// IP bazlı limit KABA bir emniyet supabıdır, hassas savunma değildir:
	// Türkiye'de mobil operatörler binlerce aboneyi tek IP'nin (CGNAT) arkasına
	// koyar. 10/dk gibi sıkı bir değer meşru kullanıcıları kilitlerdi.
	// Kaba kuvvete karşı asıl savunma HESAP BAZLI kilittir (5 deneme / 15 dk,
	// service/auth içinde) — docs/trd.md NFR-802, KK-102.
	authLimit := middleware.RateLimit(d.Limiter, "auth", middleware.RateLimitConfig{
		Limit: 30, Window: time.Minute, KeyFn: middleware.ByIP,
	}, Fail)

	// ─── Kimlik doğrulama gerektirmeyen ───
	a := rg.Group("/auth", authLimit)
	{
		a.POST("/register", authH.Register)
		a.POST("/verify-email", authH.VerifyEmail)
		a.POST("/login", authH.Login)
		a.POST("/password/forgot", authH.ForgotPassword)
		a.POST("/password/reset", authH.ResetPassword)
	}

	// ─── Katalog: oturumsuz okunabilir ───
	// Servis ve ülke listesi genel bilgidir ve SEO açısından da oturumsuz
	// erişilebilir olmalıdır (docs/frontend-contract.md §10).
	cat := rg.Group("/catalog")
	{
		cat.GET("/services", catalogH.Services)
		cat.GET("/services-in-stock", catalogH.ServicesWithStock)
		cat.GET("/countries", catalogH.Countries)
		cat.GET("/availability", catalogH.Availability)

		// Kiralık katalog. Aktivasyondan AYRI uçlar: kiralıkta bir SÜRE
		// boyutu var ve onu aktivasyon yanıtına sıkıştırmak, her iki tarafı
		// da anlaşılmaz yapardı.
		cat.GET("/rental/services", catalogH.RentalServices)
		cat.GET("/rental/countries", catalogH.RentalCountries)
		cat.GET("/rental/durations", catalogH.RentalDurations)

		// ─── Müşteri yorumları (SİTEDE gösterilen liste) ───
		//
		// Oturumsuzdur: ziyaretçi de görür ve sunucu bileşeninden çekilir.
		// YALNIZ ONAYLI yorumlar döner; e-posta seçilmez bile.
		// Katalog grubunda durur çünkü aynı önbellek ve aynı SEO görünürlük
		// kuralına tabidir — ayrı bir grup açmak yalnız tekrar üretirdi.
		cat.GET("/reviews", reviewH.Public)
	}

	// ─── Oturum gerektiren ───
	auth := rg.Group("", requireAuth)
	{
		auth.POST("/auth/logout", authH.Logout)
		auth.GET("/me", authH.Me)
		auth.GET("/me/sessions", authH.ListSessions)
		auth.DELETE("/me/sessions/:id", authH.RevokeSession)

		// Cüzdan — sahiplik sorgunun parçasıdır, ayrı bir izin gerekmez.
		auth.GET("/wallet/balance", walletH.Balance)
		auth.GET("/wallet/entries", walletH.Statement)

		// ─── Bakiye yükleme (FR-500 … FR-503) ───
		//
		// Kullanıcı uçlarında ayrı bir izin YOKTUR: sahiplik sorgunun
		// parçasıdır ve başkasının talebi 404 döner.
		auth.GET("/wallet/deposit-methods", depositH.Methods)
		auth.GET("/wallet/deposits", depositH.List)
		auth.GET("/wallet/deposits/:id", depositH.Get)
		auth.GET("/wallet/deposits/:id/receipt", depositH.Receipt)

		// Talep açmak DOĞRULANMIŞ E-POSTA ister ve DAR bir limit taşır:
		// her talep bir yöneticiye iş üretir ve doğrulanmamış bir hesabın
		// kuyruğu doldurması operasyonu kilitler.
		auth.POST("/wallet/deposits",
			middleware.RequireVerifiedEmail(Fail),
			middleware.RateLimit(d.Limiter, "deposit", middleware.RateLimitConfig{
				Limit: 10, Window: time.Minute, KeyFn: middleware.ByUser,
			}, Fail),
			depositH.Create)

		// Dekont yükleme AYRI ve daha dar bir limit taşır: her istek diske
		// yazar. POST'tur — durum değiştirir (değişmez #8).
		auth.POST("/wallet/deposits/:id/receipt",
			middleware.RequireVerifiedEmail(Fail),
			middleware.RateLimit(d.Limiter, "receipt", middleware.RateLimitConfig{
				Limit: 10, Window: time.Minute, KeyFn: middleware.ByUser,
			}, Fail),
			depositH.UploadReceipt)

		// ─── Destek talepleri (FR-600) ───
		//
		// Kullanıcı uçlarında ayrı bir izin YOKTUR: sahiplik sorgunun
		// parçasıdır ve başkasının talebi 404 döner.
		//
		// 🔴 DOĞRULANMIŞ E-POSTA İSTENMEZ — bilerek. Bakiye yüklemenin aksine
		// destek, e-postası doğrulanmayan kullanıcının BAŞVURACAĞI yerdir:
		// "doğrulama e-postası gelmiyor" diyen kişiyi destek kanalından da
		// kilitlemek, onu tümüyle sessiz bırakırdı.
		auth.GET("/tickets", ticketH.List)
		auth.GET("/tickets/:id", ticketH.Get)

		// Talep açmak DAR bir limit taşır: her talep bir yöneticiye iş üretir.
		auth.POST("/tickets",
			middleware.RateLimit(d.Limiter, "ticket", middleware.RateLimitConfig{
				Limit: 5, Window: time.Minute, KeyFn: middleware.ByUser,
			}, Fail),
			ticketH.Create)

		// Mesaj eklemenin limiti daha geniştir: yazışma sırasında art arda
		// birkaç mesaj yazmak olağandır, yeni talep açmak değildir.
		auth.POST("/tickets/:id/messages",
			middleware.RateLimit(d.Limiter, "ticket-msg", middleware.RateLimitConfig{
				Limit: 20, Window: time.Minute, KeyFn: middleware.ByUser,
			}, Fail),
			ticketH.AddMessage)

		// ─── Müşteri yorumları (kullanıcı yolu) ───
		//
		// Kullanıcı uçlarında ayrı bir izin YOKTUR: sahiplik sorgunun
		// parçasıdır ve başkasının yorumu 404 döner.
		auth.GET("/reviews/mine", reviewH.Mine)
		auth.GET("/reviews/:id", reviewH.Get)

		// 🔴 YORUM YAZMAK DOĞRULANMIŞ E-POSTA İSTER — destek talebinin
		// AKSİNE. Gerekçe: destek, e-postası doğrulanmayan kullanıcının
		// başvuracağı yerdir; yorum ise SİTEDE YAYIMLANACAK bir içeriktir.
		// Doğrulanmamış tek kullanımlık hesaplarla üretilen yorumlar hem
		// moderasyon kuyruğunu doldurur hem de yayımlandığında sahte
		// referans hâline gelir.
		//
		// Limit DAR: bekleyen yorum sınırı zaten bir kişiyi tek yoruma
		// indiriyor, ama reddedilen yorumu art arda yeniden göndermek
		// mümkün olmamalı.
		auth.POST("/reviews",
			middleware.RequireVerifiedEmail(Fail),
			middleware.RateLimit(d.Limiter, "review", middleware.RateLimitConfig{
				Limit: 5, Window: time.Minute, KeyFn: middleware.ByUser,
			}, Fail),
			reviewH.Create)

		// Teklif: oturum + DOĞRULANMIŞ E-POSTA + hız limiti.
		//
		// E-posta doğrulaması FR-101 gereğidir: doğrulanmamış hesap satın alma
		// yapamaz. Ara katman yazılmıştı ama HİÇBİR YERE BAĞLANMAMIŞTI —
		// tasarım biliniyordu, koda bağlanmamıştı.
		//
		// test: scripts/smoke-auth.sh (doğrulanmamış kullanıcı teklif alamıyor)
		auth.GET("/catalog/quote",
			middleware.RequireVerifiedEmail(Fail),
			middleware.RateLimit(d.Limiter, "quote", middleware.RateLimitConfig{
				Limit: 60, Window: time.Minute, KeyFn: middleware.ByUser,
			}, Fail),
			catalogH.Quote)

		// ─── Siparişler ───
		//
		// Satın alma DOĞRULANMIŞ E-POSTA gerektirir (FR-101) ve ayrı bir hız
		// limiti taşır: her istek gerçek para harcar ve sağlayıcıda envanter
		// tüketir. Genel /auth limitiyle aynı kovaya koymak, bir kullanıcının
		// giriş denemeleriyle satın alma hakkını tüketmesi demekti.
		auth.POST("/orders",
			middleware.RequireVerifiedEmail(Fail),
			middleware.RateLimit(d.Limiter, "order", middleware.RateLimitConfig{
				Limit: 20, Window: time.Minute, KeyFn: middleware.ByUser,
			}, Fail),
			orderH.Create)

		auth.GET("/orders", orderH.List)
		auth.GET("/orders/:id", orderH.Get)
		// DELETE: durum DEĞİŞTİRİR, dolayısıyla GET olamaz (değişmez #8).
		auth.DELETE("/orders/:id", orderH.Cancel)

		// SSE akışı. Hız limiti YOK: uzun ömürlü tek bir bağlantıdır ve
		// limitlemek kod bekleyen kullanıcıyı akıştan düşürürdü. Koruma
		// sahiplik kontrolündedir — başkasının akışı 404 döner (KK-403).
		auth.GET("/orders/:id/stream", orderH.Stream)
	}

	// ─── Webhook: KİMLİK DOĞRULAMA YOK ───
	//
	// Sağlayıcı oturum veya çerez taşımaz. Auth ara katmanı eklemek her
	// bildirimin 401 almasına, sağlayıcının ≥7 kez yeniden denemesine ve
	// kodun HİÇ ulaşmamasına yol açardı (FR-410/6).
	//
	// Koruma üç katmanlı: tahmin edilemez yol + IP izin listesi + teyit.
	// Hız limiti de var: gizli yol sızarsa sınırsız kuyruk doldurma vektörü
	// olurdu (NFR-802).
	if d.Config.WebhookHeroSMSSecret != "" && d.WebhookQueue != nil {
		webhookH := handler.NewWebhook(d.WebhookQueue, d.Config.WebhookHeroSMSSecret,
			d.Config.WebhookHeroSMSAllowedIPs, d.Config.TrustedProxies, responder)
		rg.POST("/webhooks/herosms/:secret",
			middleware.RateLimit(d.Limiter, "webhook", middleware.RateLimitConfig{
				Limit: 300, Window: time.Minute, KeyFn: middleware.ByIP,
			}, Fail),
			webhookH.HeroSMS)
	} else {
		// SESSİZ KALMAYIZ: sır tanımsızsa webhook ucu YOKTUR ve sistem
		// yalnız yoklamaya (30 sn) düşer. Bunu fark etmemek, "kod neden geç
		// geliyor?" sorusunu aylarca cevapsız bırakır.
		slog.Warn("webhook ucu KAPALI — WEBHOOK_HEROSMS_SECRET tanımsız; " +
			"kodlar yalnız 30 saniyelik yoklamayla gelecek")
	}

	// ─── Yönetim: izin ZORUNLU ───
	// Yönetim uçları da hız limitlidir: yetkili bir hesabın ele geçirilmesi
	// veya bir betik hatası, sınırsız bakiye düzeltmesi anlamına gelmemeli.
	adminLimit := middleware.RateLimit(d.Limiter, "admin", middleware.RateLimitConfig{
		Limit: 60, Window: time.Minute, KeyFn: middleware.ByUser,
	}, Fail)

	if d.RuleSvc == nil {
		// Marj bu sunucudan DEĞİŞTİRİLEMEZ. Sessiz kalınsaydı yönetici
		// "Fiyat kuralları" ekranında 500 görür ve sebebini arardı.
		slog.Warn("fiyat kuralı uçları KAPALI — RuleSvc bağlanmamış; " +
			"marj yalnız veritabanından elle değiştirilebilir")
	}
	if d.CatalogSync == nil {
		// SESSİZ KALMAYIZ: yönetici "Senkronla" düğmesine basıp hata
		// aldığında sebebi burada yazar.
		slog.Warn("katalog senkron ucu KAPALI — CatalogSync bağlanmamış; " +
			"katalog yalnız `cli catalog:sync` ile ve periyodik işlerle tazelenir")
	}

	adminH := handler.NewAdmin(handler.AdminDeps{
		Queries: d.Queries, Secrets: d.Secrets,
		Rules: d.RuleSvc, Sync: d.CatalogSync, Responder: responder,
	})

	admin := rg.Group("/admin", requireAuth, adminLimit)
	{
		// HER UÇ KENDİ İZNİNİ İSTER. "admin ise her şeyi yapabilir" modeli,
		// tek bir hesabın ele geçirilmesini toplam kayba çevirir; ayrıca
		// destek personeline yalnız okuma verilemezdi.
		admin.GET("/users",
			middleware.RequirePermission("users:read", Fail), adminH.ListUsers)
		admin.PATCH("/users/:id/status",
			middleware.RequirePermission("users:write", Fail), adminH.SetUserStatus)
		admin.POST("/users/:id/balance",
			middleware.RequirePermission("users:write", Fail), walletH.AdjustBalance)

		admin.GET("/providers",
			middleware.RequirePermission("providers:read", Fail), adminH.ListProviders)
		// Sağlayıcı EKLEME (FR-701). Gövdesinde API anahtarı ALINMAZ.
		admin.POST("/providers",
			middleware.RequirePermission("providers:write", Fail), adminH.CreateProvider)
		admin.PATCH("/providers/:id",
			middleware.RequirePermission("providers:write", Fail), adminH.UpdateProvider)
		// API anahtarı AYRI bir uç: ayar kaydetmek anahtarı silememeli.
		admin.PUT("/providers/:id/api-key",
			middleware.RequirePermission("providers:write", Fail), adminH.SetProviderAPIKey)
		// Katalog senkronu: POST, çünkü durum değiştirir (Değişmez #8).
		// İş ARKA PLANDA koşar; uç yalnız başlatır.
		admin.POST("/providers/:id/sync",
			middleware.RequirePermission("providers:write", Fail), adminH.SyncProvider)

		// ─── Fiyat kuralları (FR-703) ───
		//
		// Okuma ile yazma AYRI izinlerdir: marj bir iş kararıdır ve destek
		// personelinin kuralları görmesi, değiştirebilmesi anlamına gelmez.
		admin.GET("/pricing-rules",
			middleware.RequirePermission("pricing:read", Fail), adminH.ListPricingRules)
		admin.POST("/pricing-rules",
			middleware.RequirePermission("pricing:write", Fail), adminH.CreatePricingRule)
		admin.DELETE("/pricing-rules/:id",
			middleware.RequirePermission("pricing:write", Fail), adminH.DeactivatePricingRule)
		// Önizleme yalnız OKUR: durum değiştirmez, teklif üretmez.
		admin.POST("/pricing-rules/preview",
			middleware.RequirePermission("pricing:read", Fail), adminH.PreviewPricing)

		// ─── Denetim kaydı (FR-705) ───
		admin.GET("/audit-logs",
			middleware.RequirePermission("audit:read", Fail), adminH.ListAuditLogs)

		admin.GET("/deposit-methods",
			middleware.RequirePermission("deposits:read", Fail), adminH.ListDepositMethods)
		admin.POST("/deposit-methods",
			middleware.RequirePermission("deposits:approve", Fail), adminH.CreateDepositMethod)
		admin.PATCH("/deposit-methods/:id",
			middleware.RequirePermission("deposits:approve", Fail), adminH.UpdateDepositMethod)
		admin.PATCH("/deposit-methods/:id/active",
			middleware.RequirePermission("deposits:approve", Fail), adminH.SetDepositMethodActive)
		admin.DELETE("/deposit-methods/:id",
			middleware.RequirePermission("deposits:approve", Fail), adminH.DeleteDepositMethod)

		// ─── Bakiye yükleme talepleri ───
		//
		// 🔴 ONAY VE RED **POST**'TUR. Eski sistemde onay bir GET'ti ve bir
		// <img src="…/approve"> etiketiyle tetiklenebiliyordu (design.md §11).
		admin.GET("/deposits",
			middleware.RequirePermission("deposits:read", Fail), depositH.AdminList)
		admin.GET("/deposits/:id/receipt",
			middleware.RequirePermission("deposits:read", Fail), depositH.AdminReceipt)
		admin.POST("/deposits/:id/approve",
			middleware.RequirePermission("deposits:approve", Fail), depositH.Approve)
		admin.POST("/deposits/:id/reject",
			middleware.RequirePermission("deposits:approve", Fail), depositH.Reject)

		// ─── Destek talepleri (FR-600) ───
		//
		// İzin kodları docs/trd.md §izinler tablosundan gelir ve
		// 00002_seed_rbac.sql'de ZATEN tanımlıdır: okuma `tickets:read`,
		// yazma `tickets:reply`. Yeni bir `tickets:write` kodu ÜRETİLMEDİ —
		// iki eş anlamlı izin, yetki matrisini okunamaz hâle getirir.
		//
		// Durum değişikliği de `tickets:reply` ister: kapatmak da yazışmaya
		// müdahaledir ve yalnız okuma yetkisi olan personelin işi değildir.
		admin.GET("/tickets",
			middleware.RequirePermission("tickets:read", Fail), ticketH.AdminList)
		admin.GET("/tickets/:id",
			middleware.RequirePermission("tickets:read", Fail), ticketH.AdminGet)
		admin.POST("/tickets/:id/messages",
			middleware.RequirePermission("tickets:reply", Fail), ticketH.AdminAddMessage)
		// PATCH: durum DEĞİŞTİRİR, dolayısıyla GET olamaz (değişmez #8).
		admin.PATCH("/tickets/:id/status",
			middleware.RequirePermission("tickets:reply", Fail), ticketH.AdminSetStatus)

		// ─── Müşteri yorumları (yönetim) ───
		//
		// İzin kodları 00013_reviews.sql'de tanımlanır ve admin rolüne
		// bağlanır. Okuma ile moderasyon AYRI izinlerdir: destek personelinin
		// kuyruğu görmesi, sitede ne yayımlanacağına karar verebilmesi
		// anlamına gelmez.
		admin.GET("/reviews",
			middleware.RequirePermission("reviews:read", Fail), reviewH.AdminList)
		// 🔴 ONAY VE RED **POST**'TUR, GET DEĞİL (değişmez #8): eski sistemde
		// onay bir GET'ti ve <img src="…/approve"> ile tetiklenebiliyordu.
		admin.POST("/reviews/:id/approve",
			middleware.RequirePermission("reviews:moderate", Fail), reviewH.Approve)
		admin.POST("/reviews/:id/reject",
			middleware.RequirePermission("reviews:moderate", Fail), reviewH.Reject)
	}

	// M2: /wallet/*  ·  M4: /catalog/*  ·  M5: /orders/*  ·  M6: /admin/*
}

// healthz CANLILIK: süreç ayakta mı. Bağımlılıklara BAKMAZ —
// veritabanı düştüğünde konteynerin yeniden başlatılması işe yaramaz.
func healthz(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }

// readyz HAZIRLIK: istek alabilir miyiz. Bağımlılıkları kontrol eder.
func readyz(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		checks := gin.H{}
		ready := true
		if err := d.Pool.Ping(ctx); err != nil {
			checks["postgres"], ready = "down", false
		} else {
			checks["postgres"] = "up"
		}
		if err := d.Redis.Ping(ctx).Err(); err != nil {
			checks["redis"], ready = "down", false
		} else {
			checks["redis"] = "up"
		}

		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"ready": ready, "checks": checks})
	}
}
