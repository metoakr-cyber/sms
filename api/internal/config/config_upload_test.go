package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/config"
)

// production üretim için geçerli bir minimum ortam kurar.
func production(t *testing.T) {
	t.Helper()
	valid(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_BASE_URL", "https://ornek.test")
	t.Setenv("RECAPTCHA_SITE_KEY", "x")
	t.Setenv("RECAPTCHA_SECRET_KEY", "x")
	t.Setenv("WEBHOOK_HEROSMS_SECRET", "x")
	t.Setenv("WEBHOOK_HEROSMS_ALLOWED_IPS", "1.2.3.4")
	t.Setenv("SENTRY_DSN", "https://x@o1.ingest.sentry.io/1")
	t.Setenv("MAIL_PROVIDER", "smtp")
	t.Setenv("SMTP_HOST", "smtp.ornek.test")
	t.Setenv("SMTP_USERNAME", "u")
	t.Setenv("SMTP_PASSWORD", "p")
	t.Setenv("TRUSTED_PROXIES", "172.16.0.0/12")
	t.Setenv("UPLOAD_DIR", "/var/lib/onay360/uploads")
}

// TestProductionRequiresUploadDir
//
// Geliştirme varsayılanı GEÇİCİ dizindir. Üretimde sessizce kullanılırsa
// yüklenen dekontlar ilk yeniden başlatmada kaybolur: kullanıcı "yükledim"
// der, yöneticinin elinde hiçbir kanıt kalmaz ve kimse sebebini aramaz.
func TestProductionRequiresUploadDir(t *testing.T) {
	t.Run("tanımsız", func(t *testing.T) {
		production(t)
		t.Setenv("UPLOAD_DIR", "")
		_, err := config.Load()
		if err == nil || !strings.Contains(err.Error(), "UPLOAD_DIR") {
			t.Fatalf("🔴 üretimde tanımsız UPLOAD_DIR kabul edildi: %v", err)
		}
	})

	t.Run("göreli yol", func(t *testing.T) {
		production(t)
		t.Setenv("UPLOAD_DIR", "uploads")
		_, err := config.Load()
		if err == nil || !strings.Contains(err.Error(), "UPLOAD_DIR") {
			t.Fatalf("🔴 üretimde göreli UPLOAD_DIR kabul edildi: %v", err)
		}
	})

	t.Run("mutlak yol geçerli", func(t *testing.T) {
		production(t)
		c, err := config.Load()
		if err != nil {
			t.Fatalf("geçerli üretim ortamı reddedildi: %v", err)
		}
		if c.UploadDir != "/var/lib/onay360/uploads" {
			t.Fatalf("UploadDir = %q", c.UploadDir)
		}
	})
}

// TestUploadDirDefaultsOutsideRepo geliştirme varsayılanının depoya dosya
// bırakmadığını doğrular: yanlışlıkla commit edilen bir dekont, kişisel
// finansal veriyi sürüm geçmişine kalıcı olarak yazar.
func TestUploadDirDefaultsOutsideRepo(t *testing.T) {
	valid(t)
	t.Setenv("UPLOAD_DIR", "")
	c, err := config.Load()
	if err != nil {
		t.Fatalf("geliştirme ortamı yüklenemedi: %v", err)
	}
	if !filepath.IsAbs(c.UploadDir) {
		t.Fatalf("🔴 varsayılan UPLOAD_DIR göreli: %q — çalışma dizinine göre değişir", c.UploadDir)
	}
	if !strings.HasPrefix(c.UploadDir, os.TempDir()) {
		t.Fatalf("🔴 varsayılan UPLOAD_DIR geçici dizinin dışında: %q", c.UploadDir)
	}
}
