package mailer

// Resend adaptörü — https://resend.com HTTP API'si.
//
// NEDEN RESEND SDK'sı DEĞİL: tek bir POST isteği için bir SDK, kendi HTTP
// istemcisi ve yeniden deneme politikasıyla gelir. Yeniden deneme burada
// istenmeyen bir davranıştır: aynı doğrulama e-postasını iki kez göndermek
// kullanıcıyı, sessizce yutulan bir hata ise bizi yanıltır.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

const (
	resendBaseURL = "https://api.resend.com"

	// resendTimeout tek bir gönderim için üst sınır.
	//
	// Zaman aşımı YOK demek, sağlayıcı asılı kaldığında goroutine'lerin
	// birikmesi demektir; kayıt akışı e-postayı beklemediği için bu birikim
	// hiçbir kullanıcıya görünmez ve fark edilmeden büyür.
	resendTimeout = 10 * time.Second

	// resendMaxErrBody hata gövdesinden okunacak azami bayt.
	// Sınırsız okumak, kötü davranan bir uçtan gelen dev yanıtı log'a taşırdı.
	resendMaxErrBody = 2 << 10
)

// Resend HTTPS üzerinden e-posta gönderir.
type Resend struct {
	apiKey  string
	from    string
	baseURL string
	client  *http.Client
}

var _ port.Mailer = (*Resend)(nil)

// NewResend adaptörü kurar. Anahtar veya gönderen eksikse HATA döner:
// anahtarsız bir istemci her çağrıda 401 alır ve e-posta sessizce hiç gitmez.
func NewResend(apiKey, from string) (*Resend, error) {
	var missing []string
	if strings.TrimSpace(apiKey) == "" {
		missing = append(missing, "RESEND_API_KEY")
	}
	if strings.TrimSpace(from) == "" {
		missing = append(missing, "MAIL_FROM")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("resend: eksik yapılandırma: %s", strings.Join(missing, ", "))
	}
	return &Resend{
		apiKey:  apiKey,
		from:    from,
		baseURL: resendBaseURL,
		client:  &http.Client{Timeout: resendTimeout},
	}, nil
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text,omitempty"`
	HTML    string   `json:"html,omitempty"`
}

// Send mesajı Resend'e verir.
//
// Dönen hatanın metninde API anahtarı bulunmaz ve sağlayıcının ham yanıtı
// kullanıcıya değil yalnız log'a gider (değişmez #12).
// test: resend_test.go#TestResendErrorNeverLeaksAPIKey
func (r *Resend) Send(ctx context.Context, m port.Mail) error {
	if strings.TrimSpace(m.To) == "" {
		return apperr.ErrValidation.Wrap(errors.New("resend: alıcı boş"))
	}
	body, err := json.Marshal(resendRequest{
		From: r.from, To: []string{m.To}, Subject: m.Subject, Text: m.Text, HTML: m.HTML,
	})
	if err != nil {
		return apperr.Internal(fmt.Errorf("resend: istek kodlanamadı: %w", err))
	}

	// İstemci zaman aşımına ek olarak context de sınırlanır: çağıran bir
	// context.Background() verdiyse tek savunma hattı istemci zaman aşımıdır.
	ctx, cancel := context.WithTimeout(ctx, resendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/emails",
		bytes.NewReader(body))
	if err != nil {
		return apperr.Internal(r.scrub(fmt.Errorf("resend: istek kurulamadı: %w", err)))
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		// url.Error, isteğin URL'ini hata metnine koyar; anahtar başlıkta
		// taşındığı için orada görünmez, yine de temizlikten geçiriyoruz.
		return apperr.ErrProviderUnavailable.Wrap(r.scrub(fmt.Errorf("resend: istek başarısız: %w", err)))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, resendMaxErrBody))
	detail := fmt.Errorf("resend: %d — %s", resp.StatusCode, strings.TrimSpace(string(snippet)))

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		// Bu bizim yapılandırma hatamızdır, geçici bir kesinti değil:
		// yeniden denemek anlamsız, görünür olmak şart.
		return apperr.Internal(r.scrub(detail))
	default:
		return apperr.ErrProviderUnavailable.Wrap(r.scrub(detail))
	}
}

// scrub hata metninden API anahtarını temizler.
//
// Sağlayıcının hata gövdesi gönderdiğimiz anahtarı (veya bir ön ekini) geri
// yansıtabilir ve o metin doğrudan slog.Error'a gider.
// test: resend_test.go#TestResendErrorNeverLeaksAPIKey
func (r *Resend) scrub(err error) error {
	if err == nil {
		return nil
	}
	msg := redact(err.Error(), r.apiKey)
	if msg == err.Error() {
		return err
	}
	// Sarmalama zinciri BİLİNÇLİ olarak kesiliyor: iç hatanın metni anahtarı
	// içeriyordu ve %w ile taşınırsa temizlik işe yaramazdı.
	return errors.New(msg)
}
