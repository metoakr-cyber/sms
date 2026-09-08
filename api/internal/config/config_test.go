package config_test

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/config"
)

func key32(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// valid geçerli bir minimum ortam kurar.
//
// Testler .env dosyasından ETKİLENMEZ: config.Load() diskten okumaz, yalnız
// ortam değişkenlerine bakar. Yine de kalıntı değişkenler sızmasın diye
// varsayılanı test edilen alanlar burada açıkça temizlenir.
func valid(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"HTTP_ADDR", "LOG_LEVEL", "PUBLIC_BASE_URL", "SESSION_TTL",
		"FX_PROVIDER", "FX_SAFETY_MARGIN_PCT", "FX_MAX_AGE",
		"MAIL_PROVIDER", "MAIL_FROM", "RESEND_API_KEY",
		"RECAPTCHA_SITE_KEY", "RECAPTCHA_SECRET_KEY",
		"WEBHOOK_HEROSMS_SECRET", "WEBHOOK_HEROSMS_ALLOWED_IPS",
		"SENTRY_DSN", "METRICS_ENABLED",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("SESSION_SECRET", key32(t))
	t.Setenv("ENCRYPTION_KEY", key32(t))
}

func TestLoadMinimalValid(t *testing.T) {
	valid(t)
	c, err := config.Load()
	if err != nil {
		t.Fatalf("geçerli ortam yüklenemedi: %v", err)
	}
	if c.Env != config.EnvDevelopment || !c.Env.IsDevelopment() {
		t.Fatalf("Env = %q", c.Env)
	}
	if c.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr varsayılanı = %q", c.HTTPAddr)
	}
	if len(c.SessionSecret) != 32 || len(c.EncryptionKey) != 32 {
		t.Fatalf("anahtar uzunlukları: session=%d encryption=%d",
			len(c.SessionSecret), len(c.EncryptionKey))
	}
}

// EN KRİTİK TEST: eksik yapılandırmada süreç BAŞLAMAMALI.
// Eski prototipte RECAPTCHA_SECRET_KEY yoktu ve giriş sessizce hep başarısız oluyordu.
func TestMissingRequiredFailsLoudly(t *testing.T) {
	cases := map[string]string{
		"DATABASE_URL":   "DATABASE_URL",
		"REDIS_URL":      "REDIS_URL",
		"SESSION_SECRET": "SESSION_SECRET",
		"ENCRYPTION_KEY": "ENCRYPTION_KEY",
	}
	for unset, wantMention := range cases {
		t.Run(unset, func(t *testing.T) {
			valid(t)
			t.Setenv(unset, "")
			_, err := config.Load()
			if err == nil {
				t.Fatalf("%s tanımsızken Load() BAŞARILI oldu — sessizce çalışmamalı", unset)
			}
			if !strings.Contains(err.Error(), wantMention) {
				t.Fatalf("hata mesajı %q içermiyor: %v", wantMention, err)
			}
		})
	}
}

func TestAllProblemsReportedAtOnce(t *testing.T) {
	valid(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("SESSION_SECRET", "")
	_, err := config.Load()
	if err == nil {
		t.Fatal("hata bekleniyordu")
	}
	// Kullanıcı tek tek deneme yanılma yapmasın: tüm eksikler bir kerede raporlanır.
	for _, want := range []string{"DATABASE_URL", "REDIS_URL", "SESSION_SECRET"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("hata %q'yi raporlamadı:\n%v", want, err)
		}
	}
}

func TestInvalidValuesRejected(t *testing.T) {
	tests := []struct{ key, val, mention string }{
		{"APP_ENV", "prod", "APP_ENV"},                       // 'production' olmalı
		{"LOG_LEVEL", "verbose", "LOG_LEVEL"},                // izin verilenler dışında
		{"DATABASE_URL", "mysql://x/y", "DATABASE_URL"},      // yanlış şema
		{"REDIS_URL", "http://localhost", "REDIS_URL"},       // yanlış şema
		{"SESSION_SECRET", "kisa", "SESSION_SECRET"},         // 32 bayt değil
		{"SESSION_TTL", "bir-saat", "SESSION_TTL"},           // ayrıştırılamaz
		{"SESSION_TTL", "-5m", "SESSION_TTL"},                // pozitif olmalı
		{"FX_SAFETY_MARGIN_PCT", "yüzde iki", "FX_SAFETY"},   // ondalık değil
		{"METRICS_ENABLED", "belki", "METRICS_ENABLED"},      // mantıksal değil
		{"FX_PROVIDER", "yahoo", "FX_PROVIDER"},              // desteklenmiyor
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.val, func(t *testing.T) {
			valid(t)
			t.Setenv(tt.key, tt.val)
			_, err := config.Load()
			if err == nil {
				t.Fatalf("%s=%q kabul edildi, reddedilmeliydi", tt.key, tt.val)
			}
			if !strings.Contains(err.Error(), tt.mention) {
				t.Fatalf("hata %q içermiyor: %v", tt.mention, err)
			}
		})
	}
}

// Üretimde sessizce devre dışı kalabilecek güvenlik özellikleri engellenmelidir.
func TestProductionRequiresSecurityConfig(t *testing.T) {
	base := func(t *testing.T) {
		valid(t)
		t.Setenv("APP_ENV", "production")
		t.Setenv("PUBLIC_BASE_URL", "https://ornek.com")
		t.Setenv("RECAPTCHA_SITE_KEY", "site")
		t.Setenv("RECAPTCHA_SECRET_KEY", "secret")
		t.Setenv("WEBHOOK_HEROSMS_SECRET", "abcdef")
		t.Setenv("WEBHOOK_HEROSMS_ALLOWED_IPS", "84.32.223.53")
		t.Setenv("SENTRY_DSN", "https://x@sentry.io/1")
		t.Setenv("MAIL_PROVIDER", "resend")
		t.Setenv("RESEND_API_KEY", "re_x")
	}

	t.Run("tam yapılandırma geçerli", func(t *testing.T) {
		base(t)
		if _, err := config.Load(); err != nil {
			t.Fatalf("geçerli üretim yapılandırması reddedildi: %v", err)
		}
	})

	for _, tt := range []struct{ name, key, val, mention string }{
		{"recaptcha yok", "RECAPTCHA_SECRET_KEY", "", "RECAPTCHA_SECRET_KEY"},
		{"webhook sırrı yok", "WEBHOOK_HEROSMS_SECRET", "", "WEBHOOK_HEROSMS_SECRET"},
		{"sentry yok", "SENTRY_DSN", "", "SENTRY_DSN"},
		{"IP listesi boş", "WEBHOOK_HEROSMS_ALLOWED_IPS", "", "ALLOWED_IPS"},
		{"console mail", "MAIL_PROVIDER", "console", "MAIL_PROVIDER"},
		{"http base url", "PUBLIC_BASE_URL", "http://ornek.com", "PUBLIC_BASE_URL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base(t)
			t.Setenv(tt.key, tt.val)
			_, err := config.Load()
			if err == nil {
				t.Fatalf("üretimde %s=%q kabul edildi", tt.key, tt.val)
			}
			if !strings.Contains(err.Error(), tt.mention) {
				t.Fatalf("hata %q içermiyor: %v", tt.mention, err)
			}
		})
	}
}

func TestCSVParsing(t *testing.T) {
	valid(t)
	t.Setenv("WEBHOOK_HEROSMS_ALLOWED_IPS", " 84.32.223.53 , 185.138.88.87 ,")
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"84.32.223.53", "185.138.88.87"}
	if len(c.WebhookHeroSMSAllowedIPs) != len(want) {
		t.Fatalf("IP sayısı = %d, beklenen %d: %v", len(c.WebhookHeroSMSAllowedIPs), len(want), c.WebhookHeroSMSAllowedIPs)
	}
	for i := range want {
		if c.WebhookHeroSMSAllowedIPs[i] != want[i] {
			t.Errorf("IP[%d] = %q, beklenen %q", i, c.WebhookHeroSMSAllowedIPs[i], want[i])
		}
	}
}

// decodeKey belirsizliği: aynı dize hem hex hem base64 olarak çözülebilir ama
// farklı uzunluk verir. Beklenen uzunluk hangisiyse o kodlama doğrudur.
func TestKeyEncodingAmbiguityResolvedByLength(t *testing.T) {
	valid(t)
	hex64 := strings.Repeat("ab", 32) // 64 karakter: hex→32 bayt, base64→48 bayt
	t.Setenv("ENCRYPTION_KEY", hex64)
	c, err := config.Load()
	if err != nil {
		t.Fatalf("hex anahtar reddedildi: %v", err)
	}
	if len(c.EncryptionKey) != 32 {
		t.Fatalf("uzunluk = %d, beklenen 32 (hex olarak çözülmeliydi)", len(c.EncryptionKey))
	}
	if c.EncryptionKey[0] != 0xab {
		t.Fatalf("ilk bayt = %#x, beklenen 0xab — base64 olarak çözülmüş olabilir", c.EncryptionKey[0])
	}
}

func TestKeyWrongLengthRejected(t *testing.T) {
	valid(t)
	t.Setenv("SESSION_SECRET", base64.StdEncoding.EncodeToString(make([]byte, 16))) // 16 bayt
	_, err := config.Load()
	if err == nil {
		t.Fatal("16 baytlık anahtar kabul edildi, 32 olmalıydı")
	}
	if !strings.Contains(err.Error(), "SESSION_SECRET") {
		t.Fatalf("hata: %v", err)
	}
}
