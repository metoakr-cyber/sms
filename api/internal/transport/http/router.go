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
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Deps yönlendiricinin ihtiyaç duyduğu bağımlılıklar.
type Deps struct {
	Config *config.Config
	Pool   *pgxpool.Pool
	Redis  *goredis.Client
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

	r.Use(
		middleware.RequestID(),
		middleware.Recovery(),
		middleware.Logger(),
	)

	// Sağlık uçları kimlik doğrulaması gerektirmez ve log'u kirletmemesi için
	// yönlendirici üstünde tutulur.
	r.GET("/healthz", healthz)
	r.GET("/readyz", readyz(d))

	v1 := r.Group("/api/v1")
	registerRoutes(v1, d)

	r.NoRoute(func(c *gin.Context) {
		Fail(c, notFound)
	})
	return r
}

// registerRoutes v1 uç noktalarını kaydeder. Modüller eklendikçe burada büyür.
func registerRoutes(rg *gin.RouterGroup, d Deps) {
	_ = d
	// M1: /auth/*  · M2: /wallet/*  · M4: /catalog/*  · M5: /orders/*
}

// healthz CANLILIK: süreç ayakta mı. Bağımlılıklara BAKMAZ —
// veritabanı düştüğünde konteynerin yeniden başlatılması işe yaramaz.
func healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// readyz HAZIRLIK: istek alabilir miyiz. Bağımlılıkları kontrol eder.
func readyz(d Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		checks := gin.H{}
		ready := true

		if err := d.Pool.Ping(ctx); err != nil {
			checks["postgres"] = "down"
			ready = false
		} else {
			checks["postgres"] = "up"
		}

		if err := d.Redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = "down"
			ready = false
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
