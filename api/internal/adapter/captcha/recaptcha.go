// Package captcha robot doğrulama adaptörlerini içerir.
package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

const verifyURL = "https://www.google.com/recaptcha/api/siteverify"

// ReCaptcha Google reCAPTCHA v2/v3 doğrulaması.
type ReCaptcha struct {
	secret string
	client *http.Client
}

func NewReCaptcha(secret string) *ReCaptcha {
	return &ReCaptcha{secret: secret, client: &http.Client{Timeout: 8 * time.Second}}
}

var _ port.Captcha = (*ReCaptcha)(nil)

func (r *ReCaptcha) Enabled() bool { return r.secret != "" }

func (r *ReCaptcha) Verify(ctx context.Context, token, remoteIP string) error {
	// Eski prototipin hatası: anahtar tanımsızken Google'a secret=undefined
	// gidiyordu, yanıt her zaman success=false oluyordu ve GİRİŞ SESSİZCE
	// ÇALIŞMIYORDU (docs/memory.md §3.10). Artık yapılandırma eksikse config
	// paketi üretimde süreci başlatmıyor; burada da açık hata olarak raporlanıyor.
	if r.secret == "" {
		return apperr.Internal(fmt.Errorf("captcha: gizli anahtar tanımsız"))
	}
	if token == "" {
		return apperr.ErrCaptchaFailed
	}

	form := url.Values{"secret": {r.secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return apperr.Internal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.client.Do(req)
	if err != nil {
		// Google'a ulaşılamıyorsa bu bizim altyapı sorunumuzdur; kullanıcıya
		// "robot doğrulaması başarısız" demek yanıltıcı olur.
		return apperr.ErrProviderUnavailable.Wrap(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return apperr.ErrProviderUnavailable.Wrap(err)
	}
	if !out.Success {
		return apperr.ErrCaptchaFailed.Wrap(fmt.Errorf("recaptcha reddetti: %v", out.ErrorCodes))
	}
	return nil
}

// Disabled captcha'yı devre dışı bırakır. YALNIZ geliştirme ve test içindir;
// config paketi üretimde reCAPTCHA anahtarlarını zorunlu kılar.
type Disabled struct{}

var _ port.Captcha = Disabled{}

func (Disabled) Enabled() bool { return false }

func (Disabled) Verify(context.Context, string, string) error { return nil }
