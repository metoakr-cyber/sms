package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ctxKey struct{ name string }

var requestIDKey = ctxKey{"request_id"}

// GinRequestIDKey gin.Context içindeki anahtar adı.
const GinRequestIDKey = "request_id"

// HeaderRequestID istemciye dönen ve log'da aranabilen korelasyon kimliği.
const HeaderRequestID = "X-Request-Id"

// RequestID her isteğe benzersiz bir kimlik atar ve yanıt başlığında döner.
//
// 🔴 YALNIZ LOG KORELASYONU İÇİNDİR. Para yolunda idempotency anahtarı olarak
// ASLA kullanılmaz (test: scripts/smoke-auth.sh — idempotencyKey gövdeden gelir):
// istemci başlık göndermediğinde her istekte yeniden üretilir,
// dolayısıyla tekrarlanan bir isteği tanıyamaz. İdempotency anahtarı isteğin
// İÇERİĞİNDEN gelir (bkz. handler/wallet.go AdjustBalance).
// Destek süreci için kritiktir: kullanıcı bu kimliği söyler, log'da tam olarak
// o istek bulunur (docs/design.md §11).
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// İstemciden gelen kimliğe güvenilir ama sınırlandırılır: log enjeksiyonu
		// ve aşırı uzun değerleri engellemek için biçim ve uzunluk doğrulanır.
		id := c.GetHeader(HeaderRequestID)
		if !validRequestID(id) {
			id = uuid.NewString()
		}
		c.Set(GinRequestIDKey, id)
		c.Request = c.Request.WithContext(
			context.WithValue(c.Request.Context(), requestIDKey, id))
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

func validRequestID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

// RequestIDFrom context'ten istek kimliğini okur.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}
