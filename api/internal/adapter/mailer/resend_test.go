// `package mailer` (bkz. smtp_test.go): sahte bir uca yönlendirmek için
// adaptörün taban URL'ini değiştirmek gerekiyor ve bu ayarı üretim API'sinde
// dışa açmak, e-postaların yanlışlıkla üçüncü bir yere gitmesinin yolunu açardı.
package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

const testAPIKey = "re_COK_GIZLI_ANAHTAR_1234567890"

func newTestResend(t *testing.T, h http.HandlerFunc) *Resend {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	r, err := NewResend(testAPIKey, "SMS Platform <noreply@ornek.com>")
	if err != nil {
		t.Fatal(err)
	}
	r.baseURL = srv.URL
	return r
}

// Başarı yolu: doğru uç, doğru başlık, doğru gövde.
func TestResendSendsEmail(t *testing.T) {
	type capture struct {
		path, auth, ctype string
		body              resendRequest
	}
	got := make(chan capture, 1)

	r := newTestResend(t, func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var body resendRequest
		_ = json.Unmarshal(raw, &body)
		got <- capture{
			path: req.URL.Path, auth: req.Header.Get("Authorization"),
			ctype: req.Header.Get("Content-Type"), body: body,
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"abc-123"}`))
	})

	err := r.Send(context.Background(), port.Mail{
		To: "kullanici@ornek.com", Subject: "E-posta adresinizi doğrulayın",
		Text: "Merhaba", HTML: "<p>Merhaba</p>",
	})
	if err != nil {
		t.Fatalf("gönderim başarısız: %v", err)
	}

	c := <-got
	if c.path != "/emails" {
		t.Fatalf("uç yolu = %q, beklenen /emails", c.path)
	}
	if c.auth != "Bearer "+testAPIKey {
		t.Fatalf("Authorization başlığı hatalı: %q", c.auth)
	}
	if !strings.HasPrefix(c.ctype, "application/json") {
		t.Fatalf("Content-Type = %q", c.ctype)
	}
	if len(c.body.To) != 1 || c.body.To[0] != "kullanici@ornek.com" {
		t.Fatalf("alıcı = %v", c.body.To)
	}
	if c.body.Subject == "" || c.body.Text == "" || c.body.HTML == "" {
		t.Fatalf("gövde eksik: %+v", c.body)
	}
}

// Hata yolu: 5xx geçici kesintidir; tipli hata altyapı kategorisinde olmalı.
func TestResendMapsServerErrorToProviderUnavailable(t *testing.T) {
	r := newTestResend(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message":"upstream down"}`))
	})

	err := r.Send(context.Background(), port.Mail{To: "a@b.com", Subject: "x", Text: "y"})
	if err == nil {
		t.Fatal("502 yanıtı başarı sayıldı")
	}
	if e, ok := apperr.As(err); !ok || e.Code != "PROVIDER_UNAVAILABLE" {
		t.Fatalf("hata tipi = %v", err)
	}
}

// 401 bizim yapılandırma hatamızdır; geçici kesintiyle karıştırılmamalı.
func TestResendMapsUnauthorizedToInternal(t *testing.T) {
	r := newTestResend(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid api key"}`))
	})

	err := r.Send(context.Background(), port.Mail{To: "a@b.com", Subject: "x", Text: "y"})
	if err == nil {
		t.Fatal("401 yanıtı başarı sayıldı")
	}
	if e, ok := apperr.As(err); !ok || e.Code != "INTERNAL" {
		t.Fatalf("hata tipi = %v", err)
	}
}

// 🔴 EN KRİTİK TEST: sağlayıcı anahtarı geri yansıtsa bile ne hata metnine
// ne log'a düşer. service/auth bu hatayı slog.Error ile kaydediyor.
func TestResendErrorNeverLeaksAPIKey(t *testing.T) {
	// Konuşkan bir uç: hata gövdesinde gönderdiğimiz Authorization başlığını
	// olduğu gibi geri veriyor. Gerçek hayatta 4xx gövdesinde isteğin
	// yankılanması yaygın bir hata ayıklama kolaylığıdır.
	r := newTestResend(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid key","sent":"` +
			req.Header.Get("Authorization") + `"}`))
	})

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)

	err := r.Send(context.Background(), port.Mail{To: "a@b.com", Subject: "x", Text: "y"})
	if err == nil {
		t.Fatal("401 yanıtı başarı sayıldı")
	}

	// service/auth'un yaptığının aynısı.
	slog.Error("doğrulama e-postası gönderilemedi", "err", err)

	if strings.Contains(err.Error(), testAPIKey) {
		t.Fatalf("API ANAHTARI hata metnine sızdı:\n%v", err)
	}
	if strings.Contains(buf.String(), testAPIKey) {
		t.Fatalf("API ANAHTARI log'a sızdı:\n%s", buf.String())
	}
	if !strings.Contains(err.Error(), "[GİZLENDİ]") {
		t.Fatalf("temizlik izi yok — sır hiç eşleşmemiş olabilir:\n%v", err)
	}
}

// Eksik yapılandırma açılışta yakalanır.
func TestNewResendRequiresKeyAndFrom(t *testing.T) {
	if _, err := NewResend("", "a@b.com"); err == nil {
		t.Fatal("anahtarsız adaptör kuruldu")
	}
	if _, err := NewResend("re_x", ""); err == nil {
		t.Fatal("göndericisiz adaptör kuruldu")
	}
	if _, err := NewResend("re_x", "a@b.com"); err != nil {
		t.Fatalf("geçerli yapılandırma reddedildi: %v", err)
	}
}
