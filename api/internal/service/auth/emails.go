package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// E-posta gönderimi kullanıcı akışını BLOKLAMAZ: gönderim başarısız olsa bile
// kayıt/sıfırlama işlemi tamamlanmıştır. Hata log'lanır ve kullanıcı
// "tekrar gönder" diyebilir.
func (s *Service) sendVerificationEmail(ctx context.Context, to, token string) {
	link := fmt.Sprintf("%s/dogrula?token=%s", s.baseURL, url.QueryEscape(token))
	err := s.mailer.Send(ctx, port.Mail{
		To:      to,
		Subject: "E-posta adresinizi doğrulayın",
		Text: "Merhaba,\n\n" +
			"Hesabınızı etkinleştirmek için aşağıdaki bağlantıya tıklayın:\n\n" +
			link + "\n\n" +
			"Bu bağlantı 24 saat geçerlidir.\n\n" +
			"Bu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz.",
	})
	if err != nil {
		slog.Error("doğrulama e-postası gönderilemedi", "to", maskEmail(to), "err", err)
	}
}

func (s *Service) sendPasswordResetEmail(ctx context.Context, to, token string) {
	link := fmt.Sprintf("%s/sifre-sifirla?token=%s", s.baseURL, url.QueryEscape(token))
	err := s.mailer.Send(ctx, port.Mail{
		To:      to,
		Subject: "Şifre sıfırlama talebi",
		Text: "Merhaba,\n\n" +
			"Şifrenizi sıfırlamak için aşağıdaki bağlantıya tıklayın:\n\n" +
			link + "\n\n" +
			"Bu bağlantı 1 saat geçerlidir ve yalnız bir kez kullanılabilir.\n\n" +
			"Bu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz; " +
			"şifreniz değişmeyecektir.",
	})
	if err != nil {
		slog.Error("şifre sıfırlama e-postası gönderilemedi", "to", maskEmail(to), "err", err)
	}
}

// maskEmail log'a tam adres yazmamak için maskeler (KVKK — docs/design.md §12).
func maskEmail(e string) string {
	for i := 0; i < len(e); i++ {
		if e[i] == '@' {
			if i <= 2 {
				return "*" + e[i:]
			}
			return e[:2] + "***" + e[i:]
		}
	}
	return "***"
}
