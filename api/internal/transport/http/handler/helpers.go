package handler

import (
	"fmt"
	"net/netip"
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
// hesaplanmaz (test: scripts/smoke-auth.sh — formatted alanı sunucudan gelir).
// Böylece sunucu ile istemci gösterimi ayrışmaz
// (docs/frontend-contract.md §5.1).
func formatTRY(m money.Money) string {
	// PARA BİRİMİNİ VE ÖLÇEĞİNİ PARANIN KENDİSİNDEN OKU.
	//
	// Bu fonksiyon her değeri kuruş (10^2) sayıp "₺" ekliyordu. Yönetim
	// panelinde sağlayıcı bakiyesi USD (10^6) olarak tutuluyor ve
	// 30,3133 USD ekranda "303.133,00 ₺" görünüyordu: hem yanlış para birimi
	// hem on bin kat hata. Yönetici sağlayıcıda 303 bin lira olduğunu sanırdı.
	scale := m.Currency().Scale
	div := int64(1)
	for i := int32(0); i < scale; i++ {
		div *= 10
	}

	minor := m.Minor()
	neg := minor < 0
	if neg {
		minor = -minor
	}
	whole, frac := minor/div, minor%div

	symbol := " ₺"
	if m.Currency().Code == "USD" {
		symbol = " $"
	}

	var sb strings.Builder
	if neg {
		sb.WriteByte('-')
	}
	sb.WriteString(groupThousands(whole))
	// Kesir HER ZAMAN iki hane gösterilir: 6 haneli mikro-dolarda altı hane
	// basmak okunmaz olur ve kullanıcı için anlamsızdır.
	cents := frac
	for scale > 2 {
		cents /= 10
		scale--
	}
	fmt.Fprintf(&sb, ",%02d%s", cents, symbol)
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

// ipString veritabanından gelen INET değerini metne çevirir.
func ipString(a *netip.Addr) string {
	if a == nil || !a.IsValid() {
		return ""
	}
	return a.String()
}
