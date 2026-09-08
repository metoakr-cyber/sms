package middleware

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// RateLimitConfig bir uç nokta grubunun hız limiti.
type RateLimitConfig struct {
	Limit  int
	Window time.Duration
	// KeyFn limitin neye göre sayılacağını belirler (IP, kullanıcı, ikisi).
	KeyFn func(*gin.Context) string
}

// RateLimit hız limiti uygular.
func RateLimit(limiter port.RateLimiter, name string, cfg RateLimitConfig, fail func(*gin.Context, error)) gin.HandlerFunc {
	if cfg.KeyFn == nil {
		cfg.KeyFn = ByIP
	}
	return func(c *gin.Context) {
		key := fmt.Sprintf("%s:%s", name, cfg.KeyFn(c))
		allowed, retryAfter, err := limiter.Allow(c.Request.Context(), key, cfg.Limit, cfg.Window)
		if err != nil {
			// Limitleyici çökerse isteği REDDETMEYİZ: Redis arızası tüm siteyi
			// kapatmamalı (fail-open). Ama SESSİZ KALMAYIZ — bu bir güvenlik
			// korumasının devre dışı kalmasıdır ve alarm üretmelidir.
			//
			// Not: oturum deposu AYNI arızada fail-CLOSED davranır (401).
			// Fark bilinçlidir: hız limiti bir koruma, oturum bir kimliktir.
			// docs/design.md §12'de tablo halinde kayıtlı.
			//
			// test: ratelimit_test.go#TestFailOpenIsLogged
			slog.Error("HIZ LİMİTİ ÇALIŞMIYOR — koruma devre dışı",
				"limiter", name, "path", c.FullPath(), "err", err)
			c.Next()
			return
		}
		if !allowed {
			c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			fail(c, apperr.ErrRateLimited)
			return
		}
		c.Next()
	}
}

// ByIP istemci IP'sine göre sayar.
func ByIP(c *gin.Context) string { return c.ClientIP() }

// ByUser giriş yapmış kullanıcıya göre sayar; yoksa IP'ye düşer.
func ByUser(c *gin.Context) string {
	if id, ok := UserIDFrom(c); ok {
		return "u" + strconv.FormatInt(id, 10)
	}
	return c.ClientIP()
}
