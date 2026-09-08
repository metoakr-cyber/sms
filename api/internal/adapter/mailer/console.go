// Package mailer e-posta gönderim adaptörlerini içerir.
package mailer

import (
	"context"
	"log/slog"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// Console e-postaları göndermez, log'a yazar. YALNIZ geliştirme içindir;
// config paketi üretimde bu sağlayıcıyı reddeder.
type Console struct{ From string }

func NewConsole(from string) *Console { return &Console{From: from} }

var _ port.Mailer = (*Console)(nil)

func (c *Console) Send(_ context.Context, m port.Mail) error {
	slog.Info("📧 e-posta (konsol sağlayıcısı — GÖNDERİLMEDİ)",
		"from", c.From, "to", m.To, "subject", m.Subject, "body", m.Text)
	return nil
}
