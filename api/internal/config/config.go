// Package config ortam değişkenlerini okur ve DOĞRULAR.
//
// Kural: eksik veya bozuk bir yapılandırmada süreç BAŞLAMAZ.
// Eski prototipte RECAPTCHA_SECRET_KEY tanımsızdı ve giriş sessizce hep başarısız
// oluyordu — hata mesajı yoktu, log yoktu, sadece çalışmıyordu.
// Bkz. docs/memory.md §3.10, docs/trd.md NFR-802.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
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
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string

	// WorkersInProcess arka plan işleri API süreci içinde mi koşsun.
	//
	// TEK ANAHTAR, İKİ SÜREÇ: `cmd/server` işleri yalnız bu değer true iken
	// başlatır, `cmd/worker` ise yalnız false iken açılır. Böylece iki sürecin
	// aynı işi aynı anda koşturduğu bir yapılandırma kombinasyonu kalmaz.
	// Çift koşmak bakiyeyi bozmazdı (iade idempotency anahtarı deterministik)
	// ama sağlayıcıya iki kat istek giderdi — bkz. cmd/worker/main.go.
	WorkersInProcess bool

	WebhookHeroSMSSecret     string
	WebhookHeroSMSAllowedIPs []string

	// TrustedProxies ters vekilin ağ aralıkları (CIDR).
	//
	// 🔴 BU LİSTE BOŞSA VEKİL BAŞLIKLARINA HİÇ BAKILMAZ ve ters vekil
	// arkasında gerçek istemci IP'si GÖRÜLEMEZ: webhook izin listesi
	// sağlayıcının her bildirimini eler, IP hız limiti tüm kullanıcıları
	// tek kovaya koyar. Docker'da Caddy ayrı bir konteynerdir, yani
	// 127.0.0.1 varsayılanı üretimde HİÇBİR ZAMAN eşleşmez.
	//
	// test: config_test.go#TestProductionRequiresTrustedProxies
	// test: ../transport/http/handler/webhook_test.go#TestTrustedProxyHeaderIsHonored
	TrustedProxies []string

	// UploadDir yüklenen dekontların saklandığı dizin (FR-500).
	//
	// 🔴 WEB KÖKÜNÜN DIŞINDA olmalıdır ve hiçbir statik dosya sunucusuna
	// bağlanmamalıdır: dekont doğrudan bir URL ile servis edilemez (KK-500).
	// Erişim yalnız yetkili HTTP uçlarındandır.
	//
	// Yazılabilirliği AÇILIŞTA doğrulanır (adapter/storage.NewLocal, cmd/server):
	// ilk yükleme anında keşfedilen bir izin hatası kullanıcıya "beklenmeyen
	// hata" olarak döner ve sebebi günlerce fark edilmez.
	//
	// test: config_upload_test.go#TestProductionRequiresUploadDir
	UploadDir string

	SentryDSN      string
	MetricsEnabled bool
}

// LoadDotEnv .env dosyasını ortama yükler. Yalnız GELİŞTİRME kolaylığıdır;
// mevcut ortam değişkenlerini EZMEZ ve dosya yoksa sessizce geçer.
//
// Load() bunu KENDİSİ çağırmaz: bir yapılandırma okuyucusunun diskten dosya
// okuması onu ortama bağımlı kılar ve testleri kırılgan yapar. Çağıran (main)
// ne zaman yükleyeceğine kendi karar verir.
func LoadDotEnv(paths ...string) {
	if len(paths) == 0 {
		paths = []string{".env", "../.env"}
	}
	// Her yol AYRI AYRI denenir.
	//
	// godotenv.Load(a, b) dosyaları sırayla açar ve İLK HATADA döner. `api/`
	// dizininden çalıştırıldığında (Makefile'daki `dev` hedefi tam da bunu
	// yapar) `.env` yoktur, çağrı orada durur ve kökteki `../.env` HİÇ
	// OKUNMAZ. Sonuç: "DATABASE_URL: zorunlu ama tanımsız" — dosya oracıkta
	// dururken.
	//
	// Ayrıca sıra ÖNEMLİDİR: godotenv önce tanımlanan değeri korur, bu yüzden
	// yakındaki .env uzaktakini ezer.
	for _, p := range paths {
		_ = godotenv.Load(p)
	}
}

// Load ortam değişkenlerini okur ve doğrular.
// Bir hata dönerse çağıran süreci sonlandırmalıdır — kısmi yapılandırmayla çalışılmaz.
func Load() (*Config, error) {
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
		SMTPHost:     v.str("SMTP_HOST", ""),
		SMTPPort:     v.port("SMTP_PORT", 587),
		SMTPUsername: v.str("SMTP_USERNAME", ""),
		SMTPPassword: v.str("SMTP_PASSWORD", ""),

		WorkersInProcess: v.boolean("WORKERS_IN_PROCESS", true),

		WebhookHeroSMSSecret:     v.str("WEBHOOK_HEROSMS_SECRET", ""),
		WebhookHeroSMSAllowedIPs: v.csv("WEBHOOK_HEROSMS_ALLOWED_IPS"),
		TrustedProxies:           v.cidrs("TRUSTED_PROXIES", "127.0.0.1/32", "::1/128"),

		RecaptchaSiteKey:   v.str("RECAPTCHA_SITE_KEY", ""),
		RecaptchaSecretKey: v.str("RECAPTCHA_SECRET_KEY", ""),

		// Geliştirme varsayılanı geçici dizindir: depoya dosya bırakmaz.
		// Üretimde AÇIKÇA verilmesi zorunludur (aşağıdaki blok).
		UploadDir: v.str("UPLOAD_DIR", filepath.Join(os.TempDir(), "onay360-uploads")),

		SentryDSN:      v.str("SENTRY_DSN", ""),
		MetricsEnabled: v.boolean("METRICS_ENABLED", true),
	}

	// ─── Sağlayıcıya bağlı zorunluluklar ───
	// SMTP her ortamda tam bağlantı bilgisi ister: eksik bir alanla açılan
	// süreç, e-postaları çalışma anında ve sessizce düşürür. Adaptör aynı
	// kontrolü kendi içinde de yapar (adapter/mailer/smtp.go NewSMTP) — burada
	// olması, arızanın kullanıcı kaydı sırasında değil AÇILIŞTA görünmesini
	// sağlar.
	if c.MailProvider == "smtp" {
		v.require("SMTP_HOST", c.SMTPHost)
		v.require("SMTP_USERNAME", c.SMTPUsername)
		v.require("SMTP_PASSWORD", c.SMTPPassword)
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
		// Üretimde ters vekil VARDIR (Caddy, ayrı konteyner).
		//
		// 🔴 BURADA "len(c.TrustedProxies) == 0" KONTROLÜ YETMEZ: değişken
		// tanımsızken cidrs() geliştirme varsayılanını (loopback) döndürür,
		// yani liste hiçbir zaman boş olmaz ve kontrol hiç tetiklenmezdi.
		// Değişkenin AÇIKÇA VERİLMİŞ olması aranır — çünkü sessizce yanlış
		// bir varsayılan, eksik bir ayardan daha kötüdür.
		//
		// test: config_test.go#TestProductionRequiresTrustedProxies
		// UPLOAD_DIR de AÇIKÇA aranır: geliştirme varsayılanı geçici dizindir ve
		// üretimde sessizce kullanılırsa yüklenen dekontlar ilk yeniden
		// başlatmada kaybolur — kullanıcı "yükledim" der, ortada kanıt olmaz.
		//
		// test: config_upload_test.go#TestProductionRequiresUploadDir
		if raw := strings.TrimSpace(os.Getenv("UPLOAD_DIR")); raw == "" {
			v.fail("UPLOAD_DIR",
				"üretimde açıkça verilmelidir — dekontların saklanacağı, WEB KÖKÜ DIŞINDA "+
					"kalıcı bir dizin (örn. /var/lib/onay360/uploads)")
		} else if !filepath.IsAbs(raw) {
			v.fail("UPLOAD_DIR",
				"üretimde mutlak yol olmalıdır: göreli yol, sürecin çalışma dizinine "+
					"göre değişir ve yeniden başlatmada başka bir dizine düşebilir")
		}

		if strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")) == "" {
			v.fail("TRUSTED_PROXIES",
				"üretimde açıkça verilmelidir — ters vekilin ağ aralığı (örn. 172.16.0.0/12). "+
					"Varsayılan loopback listesi ters vekil arkasında ASLA eşleşmez")
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

// port bir TCP port numarasını okur ve aralığını doğrular.
func (v *validator) port(key string, def int) int {
	s := v.str(key, "")
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		v.fail(key, fmt.Sprintf("port numarası değil: %q", s))
		return def
	}
	if n < 1 || n > 65535 {
		v.fail(key, fmt.Sprintf("port 1–65535 aralığında olmalı: %d", n))
		return def
	}
	return n
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

// cidrs vekil ağ aralıklarını okur ve DOĞRULAR.
//
// 🔴 HERKESE AÇIK ARALIK REDDEDİLİR. "0.0.0.0/0" yazmak "her istemciye vekil
// gibi güven" demektir: o anda X-Forwarded-For'u uyduran herkes izin
// listesini ve hız limitini atlar. Yapılandırmayla açılabilen bir güvenlik
// açığı, olmayan bir savunmadan kötüdür — çünkü var sanılır.
//
// test: config_test.go#TestTrustedProxiesRejectsOpenRange
func (v *validator) cidrs(key string, def ...string) []string {
	raw := v.csv(key)
	if len(raw) == 0 {
		raw = def
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		// Çıplak IP de kabul edilir; tek adreslik maskeye çevrilir.
		if ip := net.ParseIP(s); ip != nil {
			if ip4 := ip.To4(); ip4 != nil {
				s = ip4.String() + "/32"
			} else {
				s = ip.String() + "/128"
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			v.fail(key, fmt.Sprintf("geçersiz CIDR: %q", s))
			continue
		}
		if ones, bits := n.Mask.Size(); ones == 0 && bits > 0 {
			v.fail(key, fmt.Sprintf("%q tüm interneti kapsıyor — vekil başlıkları "+
				"uyduran herkes izin listesini atlardı", s))
			continue
		}
		out = append(out, n.String())
	}
	return out
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
