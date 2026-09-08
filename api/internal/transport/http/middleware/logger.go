package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger her isteği yapılandırılmış (JSON) olarak kaydeder.
//
// Sorgu dizesi KASITLI olarak kaydedilmez: sağlayıcı API anahtarları ve
// token'lar sorgu parametresinde taşınabiliyor (docs/design.md §12).
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"request_id", RequestIDFrom(c.Request.Context()),
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		}
		if uid, ok := c.Get("user_id"); ok {
			attrs = append(attrs, "user_id", uid)
		}

		switch {
		case c.Writer.Status() >= 500:
			slog.Error("istek", attrs...)
		case c.Writer.Status() >= 400:
			slog.Info("istek", attrs...)
		default:
			slog.Info("istek", attrs...)
		}
	}
}
