// Package mailer e-posta gönderim adaptörlerini içerir.
package mailer

import (
	"context"
	"log/slog"
	"strings"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// Console e-postaları göndermez, log'a yazar. YALNIZ geliştirme içindir.
//
// Gövde KAYDEDİLMEZ. E-postalar doğrulama ve şifre sıfırlama token'ı taşır;
// bunları log'a yazmak, log erişimi olan herkesin herhangi bir hesabı ele
// geçirebilmesi demektir. Token'a geliştirme sırasında erişmek gerekiyorsa
// Link alanı ayrıca ve BİLİNÇLİ olarak verilir.
//
// test: console_test.go#TestConsoleDoesNotLogBody
type Console struct{ From string }

func NewConsole(from string) *Console { return &Console{From: from} }

var _ port.Mailer = (*Console)(nil)

func (c *Console) Send(_ context.Context, m port.Mail) error {
	// Gövde BİLİNÇLİ olarak kaydedilmiyor — token taşır.
	slog.Info("📧 e-posta (konsol sağlayıcısı — GÖNDERİLMEDİ)",
		"from", c.From, "to", maskEmail(m.To), "subject", m.Subject,
		"body_bytes", len(m.Text))

	// Geliştirmede token'a erişmek gerekiyor; yalnız BAĞLANTIYI ayıklayıp
	// ayrı bir satırda veriyoruz. Böylece "log'a ne yazılıyor" kararı tek
	// yerde ve açıkça duruyor.
	if link := extractLink(m.Text); link != "" {
		slog.Info("📧 bağlantı (yalnız geliştirme)", "link", link)
	}
	return nil
}

// extractLink metindeki ilk http(s) bağlantısını döner.
func extractLink(s string) string {
	i := strings.Index(s, "http")
	if i < 0 {
		return ""
	}
	rest := s[i:]
	if j := strings.IndexAny(rest, " \n\t\r"); j >= 0 {
		return rest[:j]
	}
	return rest
}

// maskEmail log'a tam adres yazmamak için maskeler (KVKK).
func maskEmail(e string) string {
	i := strings.IndexByte(e, '@')
	switch {
	case i < 0:
		return "***"
	case i <= 2:
		return "*" + e[i:]
	default:
		return e[:2] + "***" + e[i:]
	}
}
