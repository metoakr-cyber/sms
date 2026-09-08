// Package obs gözlemlenebilirlik adaptörlerini içerir: hata izleme (Sentry) ve
// metrikler (Prometheus).
//
// NEDEN BAĞIMLILIK EKLENDİ: `config.go` üretimde `SENTRY_DSN`'i ZORUNLU
// kılıyordu ama karşılığında hiçbir kod yoktu. İki seçenek vardı — zorunluluğu
// kaldırıp dürüst olmak ya da adaptörü yazmak. Zorunluluğu kaldırmak, bu
// sistemin ihtiyacını ortadan kaldırmıyor: para hareketi olan bir serviste
// üretimdeki bir panik veya iade hatasının fark edilmesi log dosyasına göz
// atmaya bırakılamaz. `github.com/getsentry/sentry-go` tek amaçlı ve küçük bir
// bağımlılıktır; yazıldı.
package obs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

// SentryConfig Sentry istemcisinin ayarları.
type SentryConfig struct {
	DSN         string
	Environment string
	Release     string
	// transport yalnız testte doldurulur; nil ise SDK'nın HTTP taşıyıcısı kullanılır.
	transport sentry.Transport
}

// Sentry hata izleme adaptörü.
type Sentry struct {
	hub *sentry.Hub
}

// NewSentry istemciyi kurar.
//
// DSN boşsa hata döner: "boş DSN = sessizce devre dışı" davranışı, tam olarak
// config'in üretimde engellemeye çalıştığı şeydir. Çağıran (main) DSN'in olup
// olmadığına kendisi karar verir.
func NewSentry(cfg SentryConfig) (*Sentry, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, fmt.Errorf("obs: SENTRY_DSN boş")
	}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:         cfg.DSN,
		Environment: cfg.Environment,
		Release:     cfg.Release,
		Transport:   cfg.transport,
		// KVKK: SDK'nın IP adresi/çerez gibi alanları kendiliğinden
		// toplamasını istemiyoruz (docs/design.md §12).
		SendDefaultPII:   false,
		AttachStacktrace: true,
		EnableTracing:    false,
	})
	if err != nil {
		return nil, fmt.Errorf("obs: sentry istemcisi kurulamadı: %w", err)
	}
	return &Sentry{hub: sentry.NewHub(client, sentry.NewScope())}, nil
}

// Flush kapanışta bekleyen olayları gönderir.
func (s *Sentry) Flush(timeout time.Duration) bool {
	if s == nil || s.hub == nil {
		return true
	}
	return s.hub.Flush(timeout)
}

// SlogHandler slog kayıtlarını hem asıl işleyiciye hem Sentry'ye yönlendirir.
//
// NEDEN slog ÜZERİNDEN: kod tabanı hataları zaten `slog.Error` ile
// bildiriyor (worker/worker.go, service/auth/emails.go, …). Ayrı bir
// `sentry.CaptureException` çağrısı eklemek, her yeni hata yolunda
// unutulabilecek ikinci bir adım demekti.
func (s *Sentry) SlogHandler(inner slog.Handler) slog.Handler {
	return &sentryHandler{inner: inner, hub: s.hub}
}

type sentryHandler struct {
	inner slog.Handler
	hub   *sentry.Hub
	attrs []slog.Attr
	group string
}

func (h *sentryHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *sentryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &sentryHandler{
		inner: h.inner.WithAttrs(attrs),
		hub:   h.hub,
		attrs: append(append([]slog.Attr{}, h.attrs...), attrs...),
		group: h.group,
	}
}

func (h *sentryHandler) WithGroup(name string) slog.Handler {
	return &sentryHandler{
		inner: h.inner.WithGroup(name), hub: h.hub,
		attrs: h.attrs, group: name,
	}
}

// Handle kaydı asıl işleyiciye geçirir; ERROR ve üstünü ayrıca Sentry'ye yollar.
//
// Sentry'ye gönderim asıl işleyiciden SONRA yapılır: Sentry'de bir sorun
// olsa bile satır stdout'a yazılmış olur.
func (h *sentryHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.inner.Handle(ctx, r)
	if r.Level < slog.LevelError || h.hub == nil {
		return err
	}

	ev := sentry.NewEvent()
	ev.Level = sentry.LevelError
	ev.Message = r.Message
	ev.Timestamp = r.Time
	ev.Logger = "slog"

	fields := map[string]any{}
	for _, a := range h.attrs {
		putAttr(fields, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		putAttr(fields, a)
		return true
	})
	if len(fields) > 0 {
		ev.Contexts["log"] = fields
	}
	h.hub.CaptureEvent(ev)
	return err
}

// sensitiveKeys değeri Sentry'ye taşınmayacak alan adlarının parçaları.
//
// Bir dış servise gönderilen her alan, log dosyasından daha geniş bir
// kitleye açılır. Bugün kod tabanında bu adlarla sır log'lanmıyor; bu liste
// yarın birinin log'ladığında sırrın Sentry'ye çıkmasını engeller.
// test: obs_test.go#TestSentryHandlerMasksSensitiveAttributes
var sensitiveKeys = []string{
	"password", "parola", "secret", "sir", "token", "api_key", "apikey",
	"authorization", "session", "otp", "code", "phone", "msisdn", "dsn",
}

// putAttr bir slog alanını olay sözlüğüne yazar; hassas adları maskeler.
func putAttr(dst map[string]any, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		for _, g := range v.Group() {
			putAttr(dst, slog.Attr{Key: a.Key + "." + g.Key, Value: g.Value})
		}
		return
	}
	if isSensitiveKey(a.Key) {
		dst[a.Key] = "[GİZLENDİ]"
		return
	}
	dst[a.Key] = v.Any()
}

func isSensitiveKey(k string) bool {
	lk := strings.ToLower(k)
	for _, s := range sensitiveKeys {
		if strings.Contains(lk, s) {
			return true
		}
	}
	return false
}
