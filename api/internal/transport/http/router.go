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
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Deps yönlendiricinin ihtiyaç duyduğu bağımlılıklar.
type Deps struct {
	Config   *config.Config
	Pool     *pgxpool.Pool
	Redis    *goredis.Client
	Queries  *db.Queries
	Sessions port.SessionStore
	Limiter  port.RateLimiter
	AuthSvc  *authsvc.Service
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

	// ─── Oturum gerektiren ───
	auth := rg.Group("", requireAuth)
	{
		auth.POST("/auth/logout", authH.Logout)
		auth.GET("/me", authH.Me)
		auth.GET("/me/sessions", authH.ListSessions)
		auth.DELETE("/me/sessions/:id", authH.RevokeSession)
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
