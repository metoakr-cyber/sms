package handler

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

// bindJSON gövdeyi çözer. Bozuk JSON'da tek biçim hata döner.
func bindJSON(c *gin.Context, r Responder, out any) bool {
	if err := c.ShouldBindJSON(out); err != nil {
		r.Fail(c, apperr.ErrValidation.
			WithMessage("İstek gövdesi okunamadı.").Wrap(err))
		return false
	}
	return true
}

// formatTRY tutarı Türkçe biçimde yazar: 1234567 kuruş → "12.345,67 ₺"
//
// Biçimleme SUNUCUDA yapılır ve yanıta konur; istemci tarafında yeniden
// hesaplanmaz. Böylece sunucu ile istemci gösterimi asla ayrışmaz
// (docs/frontend-contract.md §5.1).
func formatTRY(m money.Money) string {
	minor := m.Minor()
	neg := minor < 0
	if neg {
		minor = -minor
	}
	whole, frac := minor/100, minor%100

	var sb strings.Builder
	if neg {
		sb.WriteByte('-')
	}
	sb.WriteString(groupThousands(whole))
	fmt.Fprintf(&sb, ",%02d ₺", frac)
	return sb.String()
}

// groupThousands binlik ayırıcı olarak nokta koyar (Türkçe biçim).
func groupThousands(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var sb strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		sb.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(s[i : i+3])
	}
	return sb.String()
}
