// Package config ortam değişkenlerini okur ve DOĞRULAR.
//
// Kural: eksik veya bozuk bir yapılandırmada süreç BAŞLAMAZ.
// Eski prototipte RECAPTCHA_SECRET_KEY tanımsızdı ve giriş sessizce hep başarısız
// oluyordu — hata mesajı yoktu, log yoktu, sadece çalışmıyordu.
// Bkz. docs/memory.md §3.10, docs/trd.md NFR-802.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Env string

const (
	EnvDevelopment Env = "development"
	EnvStaging     Env = "staging"
	EnvProduction  Env = "production"
)

func (e Env) IsProduction() bool  { return e == EnvProduction }
func (e Env) IsDevelopment() bool { return e == EnvDevelopment }

type Config struct {
	Env           Env
	HTTPAddr      string
	LogLevel      string
	PublicBaseURL string

	DatabaseURL string
	RedisURL    string

	SessionSecret []byte
	SessionTTL    time.Duration
	EncryptionKey []byte

	FXProvider        string
	FXSafetyMarginPct string // ondalık metin; money.RateFromString ile ayrıştırılır
	FXMaxAge          time.Duration

	RecaptchaSiteKey   string
	RecaptchaSecretKey string

	MailProvider string
	MailFrom     string
	ResendAPIKey string

	WebhookHeroSMSSecret     string
	WebhookHeroSMSAllowedIPs []string

	SentryDSN      string
	MetricsEnabled bool
}

// Load ortam değişkenlerini okur ve doğrular.
// Bir hata dönerse çağıran süreci sonlandırmalıdır — kısmi yapılandırmayla çalışılmaz.
func Load() (*Config, error) {
	// .env yalnız geliştirme kolaylığıdır; yoksa sorun değil.
	// Üretimde değişkenler gerçek ortamdan gelir.
	_ = godotenv.Load(".env", "../.env")

	v := &validator{}
	c := &Config{
		Env:           Env(v.oneOf("APP_ENV", "development", "development", "staging", "production")),
		HTTPAddr:      v.str("HTTP_ADDR", ":8080"),
		LogLevel:      v.oneOf("LOG_LEVEL", "info", "debug", "info", "warn", "error"),
		PublicBaseURL: v.urlStr("PUBLIC_BASE_URL", "http://localhost:3000"),

		DatabaseURL: v.requiredURL("DATABASE_URL", "postgres", "postgresql"),
		RedisURL:    v.requiredURL("REDIS_URL", "redis", "rediss"),

		SessionSecret: v.requiredKey("SESSION_SECRET", 32),
		SessionTTL:    v.duration("SESSION_TTL", 720*time.Hour),
		EncryptionKey: v.requiredKey("ENCRYPTION_KEY", 32),

		FXProvider:        v.oneOf("FX_PROVIDER", "tcmb", "tcmb", "manual"),
		FXSafetyMarginPct: v.decimal("FX_SAFETY_MARGIN_PCT", "2.0"),
		FXMaxAge:          v.duration("FX_MAX_AGE", 30*time.Minute),

		MailProvider: v.oneOf("MAIL_PROVIDER", "console", "console", "resend", "smtp"),
		MailFrom:     v.str("MAIL_FROM", "noreply@localhost"),
		ResendAPIKey: v.str("RESEND_API_KEY", ""),

		WebhookHeroSMSSecret:     v.str("WEBHOOK_HEROSMS_SECRET", ""),
		WebhookHeroSMSAllowedIPs: v.csv("WEBHOOK_HEROSMS_ALLOWED_IPS"),

		RecaptchaSiteKey:   v.str("RECAPTCHA_SITE_KEY", ""),
		RecaptchaSecretKey: v.str("RECAPTCHA_SECRET_KEY", ""),

		SentryDSN:      v.str("SENTRY_DSN", ""),
		MetricsEnabled: v.boolean("METRICS_ENABLED", true),
	}

	// ─── Ortama bağlı zorunluluklar ───
	// Üretimde sessizce devre dışı kalabilecek hiçbir güvenlik özelliği kabul edilmez.
	if c.Env.IsProduction() {
		v.require("RECAPTCHA_SITE_KEY", c.RecaptchaSiteKey)
		v.require("RECAPTCHA_SECRET_KEY", c.RecaptchaSecretKey)
		v.require("WEBHOOK_HEROSMS_SECRET", c.WebhookHeroSMSSecret)
		v.require("SENTRY_DSN", c.SentryDSN)
		if c.MailProvider == "console" {
			v.fail("MAIL_PROVIDER", `üretimde "console" olamaz — gerçek bir sağlayıcı seçin`)
		}
		if c.MailProvider == "resend" && c.ResendAPIKey == "" {
			v.require("RESEND_API_KEY", "")
		}
		if !strings.HasPrefix(c.PublicBaseURL, "https://") {
			v.fail("PUBLIC_BASE_URL", "üretimde https:// olmalı")
		}
		if len(c.WebhookHeroSMSAllowedIPs) == 0 {
			v.fail("WEBHOOK_HEROSMS_ALLOWED_IPS", "üretimde boş olamaz (webhook imza doğrulaması yok)")
		}
	}

	if err := v.err(); err != nil {
		return nil, err
	}
	return c, nil
}

// ─────────────────────────── doğrulayıcı ───────────────────────────

type validator struct{ problems []string }

func (v *validator) fail(key, why string) {
	v.problems = append(v.problems, fmt.Sprintf("  %s: %s", key, why))
}

func (v *validator) err() error {
	if len(v.problems) == 0 {
		return nil
	}
	return fmt.Errorf("yapılandırma geçersiz:\n%s\n\n.env.example dosyasına bakın",
		strings.Join(v.problems, "\n"))
}

func (v *validator) str(key, def string) string {
	if s := strings.TrimSpace(os.Getenv(key)); s != "" {
		return s
	}
	return def
}

func (v *validator) require(key, val string) {
	if strings.TrimSpace(val) == "" {
		v.fail(key, "zorunlu ama tanımsız")
	}
}

func (v *validator) oneOf(key, def string, allowed ...string) string {
	s := v.str(key, def)
	for _, a := range allowed {
		if s == a {
			return s
		}
	}
	v.fail(key, fmt.Sprintf("%q geçersiz; izin verilenler: %s", s, strings.Join(allowed, ", ")))
	return def
}

func (v *validator) requiredURL(key string, schemes ...string) string {
	s := v.str(key, "")
	if s == "" {
		v.fail(key, "zorunlu ama tanımsız")
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		v.fail(key, "geçerli bir URL değil")
		return s
	}
	for _, sc := range schemes {
		if u.Scheme == sc {
			return s
		}
	}
	v.fail(key, fmt.Sprintf("şema %q geçersiz; beklenen: %s", u.Scheme, strings.Join(schemes, "/")))
	return s
}

func (v *validator) urlStr(key, def string) string {
	s := v.str(key, def)
	if _, err := url.Parse(s); err != nil {
		v.fail(key, "geçerli bir URL değil")
	}
	return strings.TrimRight(s, "/")
}

// requiredKey base64 kodlanmış bir anahtarı çözer ve uzunluğunu doğrular.
func (v *validator) requiredKey(key string, wantBytes int) []byte {
	s := v.str(key, "")
	if s == "" {
		v.fail(key, fmt.Sprintf("zorunlu — üretmek için: openssl rand -base64 %d", wantBytes))
		return nil
	}
	b, err := decodeKey(s, wantBytes)
	if err != nil {
		v.fail(key, fmt.Sprintf("%d baytlık bir anahtar olmalı (%v) — üretmek için: openssl rand -base64 %d",
			wantBytes, err, wantBytes))
		return nil
	}
	return b
}

func (v *validator) duration(key string, def time.Duration) time.Duration {
	s := v.str(key, "")
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		v.fail(key, fmt.Sprintf("süre ayrıştırılamadı (%q) — örn. 30m, 720h", s))
		return def
	}
	if d <= 0 {
		v.fail(key, "pozitif olmalı")
		return def
	}
	return d
}

func (v *validator) decimal(key, def string) string {
	s := v.str(key, def)
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		// Yalnız BİÇİM doğrulaması; değer money.RateFromString ile tam olarak ayrıştırılır.
		v.fail(key, fmt.Sprintf("ondalık sayı değil: %q", s))
	}
	return s
}

func (v *validator) boolean(key string, def bool) bool {
	s := v.str(key, "")
	if s == "" {
		return def
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		v.fail(key, fmt.Sprintf("mantıksal değer değil: %q (true/false)", s))
		return def
	}
	return b
}

func (v *validator) csv(key string) []string {
	s := v.str(key, "")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
