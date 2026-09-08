package mailer_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/mailer"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// Yorumun iddiası: "Gövde KAYDEDİLMEZ."
//
// E-postalar doğrulama ve şifre sıfırlama token'ı taşır. Gövdeyi log'lamak,
// log erişimi olan herkesin herhangi bir hesabı ele geçirebilmesi demektir.
func TestConsoleDoesNotLogBody(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)

	const token = "COK-GIZLI-SIFIRLAMA-TOKENI-42"
	err := mailer.NewConsole("noreply@test").Send(context.Background(), port.Mail{
		To:      "kurban@ornek.com",
		Subject: "Şifre sıfırlama",
		Text: "Merhaba,\n\nŞifrenizi sıfırlamak için tıklayın:\n\n" +
			"https://ornek.com/sifre-sifirla?token=" + token +
			"\n\nBu bağlantı 1 saat geçerlidir.",
	})
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "Merhaba") || strings.Contains(out, "1 saat geçerlidir") {
		t.Fatalf("e-posta GÖVDESİ log'a yazıldı:\n%s", out)
	}
	if !strings.Contains(out, "kurban@ornek.com") {
		// Adres maskeli olmalı
		if !strings.Contains(out, "***") {
			t.Fatalf("alıcı adresi ne tam ne maskeli — beklenmeyen biçim:\n%s", out)
		}
	} else {
		t.Fatalf("alıcı adresi MASKESİZ log'landı (KVKK):\n%s", out)
	}
	// Geliştirme kolaylığı: bağlantı ayrı ve bilinçli olarak verilir.
	if !strings.Contains(out, "token="+token) {
		t.Fatalf("geliştirme bağlantısı verilmemiş — token'a erişilemez:\n%s", out)
	}
}
