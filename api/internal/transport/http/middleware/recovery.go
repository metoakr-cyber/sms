package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// Recovery panic'i yakalar, yığın izini LOG'a yazar ve istemciye genel bir
// hata döner. Yığın izi asla istemciye gitmez.
// test: middleware_test.go#TestPanicDoesNotLeakStack
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic yakalandı",
					"request_id", RequestIDFrom(c.Request.Context()),
					"panic", rec,
					"path", c.FullPath(),
					"stack", string(debug.Stack()),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"code":      "INTERNAL",
						"message":   "Beklenmeyen bir hata oluştu. Lütfen tekrar deneyin.",
						"requestId": RequestIDFrom(c.Request.Context()),
					},
				})
			}
		}()
		c.Next()
	}
}
