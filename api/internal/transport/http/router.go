// Package http HTTP taşıma katmanıdır: DTO bağlama, doğrulama, servis çağrısı,
// yanıt yazma. İş kuralı, veritabanı erişimi ve dış API çağrısı BURADA OLMAZ.
package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/ikmetrik/sms-platform/api/internal/config"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	authsvc "github.com/ikmetrik/sms-platform/api/internal/service/auth"
	ordersvc "github.com/ikmetrik/sms-platform/api/internal/service/order"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Deps yönlendiricinin ihtiyaç duyduğu bağımlılıklar.
type Deps struct {
	Config    *config.Config
	Pool      *pgxpool.Pool
	Redis     *goredis.Client
	Queries   *db.Queries
	Sessions  port.SessionStore
	Limiter   port.RateLimiter
	AuthSvc   *authsvc.Service
	WalletSvc *walletsvc.Service
	QuoteSvc  *pricingsvc.QuoteService
	OrderSvc  *ordersvc.Service
	OrderBus  handler.OrderStream
}

// NewRouter uygulamanın HTTP yönlendiricisini kurar.
func NewRouter(d Deps) *gin.Engine {
	if d.Config.Env.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.RedirectTrailingSlash = false

	// Vekil güveni: yalnız ters vekilimizden gelen X-Forwarded-For'a güveniriz.
	// Varsayılan "tüm vekillere güven" IP tabanlı hız limitini işe yaramaz hale getirir.
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})

	r.Use(middleware.RequestID(), middleware.Recovery(), middleware.Logger())

	r.GET("/healthz", healthz)
	r.GET("/readyz", readyz(d))

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

	// ─── Yönetim: izin ZORUNLU ───
	// Yönetim uçları da hız limitlidir: yetkili bir hesabın ele geçirilmesi
	// veya bir betik hatası, sınırsız bakiye düzeltmesi anlamına gelmemeli.
	adminLimit := middleware.RateLimit(d.Limiter, "admin", middleware.RateLimitConfig{
		Limit: 60, Window: time.Minute, KeyFn: middleware.ByUser,
	}, Fail)

	admin := rg.Group("/admin", requireAuth, adminLimit)
	{
		admin.POST("/users/:id/balance",
			middleware.RequirePermission("users:write", Fail), walletH.AdjustBalance)
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
