// `package obs`: Sentry istemcisine sahte bir taşıyıcı vermek gerekiyor ve o
// alanı üretim API'sinde dışa açmak, olayların yanlışlıkla hiçbir yere
// gitmemesinin yolunu açardı.
package obs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

/* ═══════════════════════ Metrikler ═══════════════════════ */

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics durum kodu = %d", rec.Code)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestMetricsHandlerServesRegisteredMetrics(t *testing.T) {
	m := NewMetrics()
	m.ObserveHTTP("get", "/api/v1/catalog/services", 200, 12*time.Millisecond)
	m.ObserveProviderCall("herosms", "purchase", "", 800*time.Millisecond)
	m.ObserveProviderCall("herosms", "purchase", "OUT_OF_STOCK", 300*time.Millisecond)
	m.SetQueueDepth("webhook", 7)

	out := scrape(t, m)
	for _, want := range []string{
		"sms_http_request_duration_seconds_count",
		"sms_provider_call_duration_seconds_count",
		`sms_provider_call_errors_total{op="purchase",provider="herosms",kind="OUT_OF_STOCK"}`,
		`sms_queue_depth{queue="webhook"} 7`,
	} {
		// Etiket sırası kütüphaneye bağlı; sıraya duyarlı olmayan kontrol.
		if !containsMetric(out, want) {
			t.Fatalf("beklenen metrik yok: %s\n---\n%s", want, out)
		}
	}
}

// containsMetric etiket SIRASINDAN bağımsız arama yapar.
func containsMetric(out, want string) bool {
	i := strings.IndexByte(want, '{')
	if i < 0 {
		return strings.Contains(out, want)
	}
	name := want[:i]
	rest := strings.TrimSuffix(want[i+1:], "}")
	labels := strings.Split(rest, ",")
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, name+"{") {
			continue
		}
		ok := true
		for _, l := range labels {
			if !strings.Contains(line, strings.TrimSpace(l)) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// 🔴 Etiketlere kimlik/telefon/sipariş numarası girmemeli: hem seri patlaması
// hem de /metrics ucundan kişisel veri sızıntısı olur.
func TestObserveHTTPNormalizesRouteLabel(t *testing.T) {
	m := NewMetrics()

	const (
		orderID = "9f1c2d3e-4b5a-6789-abcd-ef0123456789"
		phone   = "905321234567"
	)
	m.ObserveHTTP("GET", "/api/v1/orders/"+orderID, 200, time.Millisecond)
	m.ObserveHTTP("GET", "/api/v1/orders/"+orderID+"/messages", 200, time.Millisecond)
	m.ObserveHTTP("GET", "/api/v1/lookup/"+phone, 200, time.Millisecond)
	m.ObserveHTTP("POST", "/api/v1/auth/password/reset?token=COK-GIZLI", 204, time.Millisecond)

	out := scrape(t, m)
	for _, leak := range []string{orderID, phone, "COK-GIZLI", "token="} {
		if strings.Contains(out, leak) {
			t.Fatalf("etikete sızdı: %q\n---\n%s", leak, out)
		}
	}
	if !containsMetric(out, `sms_http_request_duration_seconds_count{route="/api/v1/orders/:id"`) {
		t.Fatalf("kimlik segmenti :id'ye indirgenmemiş\n---\n%s", out)
	}
	// Sürüm segmenti rakam taşır ama kimlik değildir; korunmalı.
	if strings.Contains(out, `route="/api/:id/`) {
		t.Fatalf("sürüm segmenti yanlışlıkla :id yapılmış\n---\n%s", out)
	}
}

func TestRouteLabelCardinalityIsCapped(t *testing.T) {
	m := NewMetrics()
	for i := 0; i < maxRouteLabels+50; i++ {
		// Rakamsız, benzersiz segmentler: normalizasyondan geçerler ve
		// yalnız üst sınır onları durdurabilir.
		m.ObserveHTTP("GET", "/api/v1/"+strings.Repeat("a", 1+i%30)+letters(i), 200, time.Millisecond)
	}

	out := scrape(t, m)
	re := regexp.MustCompile(`sms_http_request_duration_seconds_count\{[^}]*route="([^"]+)"`)
	seen := map[string]struct{}{}
	for _, mm := range re.FindAllStringSubmatch(out, -1) {
		seen[mm[1]] = struct{}{}
	}
	if len(seen) > maxRouteLabels+1 {
		t.Fatalf("etiket sayısı sınırı aştı: %d", len(seen))
	}
	if _, ok := seen["other"]; !ok {
		t.Fatalf("sınır aşılmasına rağmen 'other' toplama etiketi yok (%d etiket)", len(seen))
	}
}

// letters i'yi rakamsız benzersiz bir dizeye çevirir.
func letters(i int) string {
	var b strings.Builder
	for i >= 0 {
		b.WriteByte(byte('a' + i%26))
		i = i/26 - 1
	}
	return b.String()
}

func TestNormalizeRouteEdgeCases(t *testing.T) {
	cases := map[string]string{
		"":                         "unknown",
		"/api/v1/catalog/services": "/api/v1/catalog/services",
		"/api/v1/orders/:id":       "/api/v1/orders/:id",
		"/api/v1/orders/12345":     "/api/v1/orders/:id",
		"/healthz":                 "/healthz",
		"/api/v2/x":                "/api/v2/x",
	}
	for in, want := range cases {
		if got := normalizeRoute(in); got != want {
			t.Errorf("normalizeRoute(%q) = %q, beklenen %q", in, got, want)
		}
	}
}

func TestStartSamplerRunsAndSurvivesPanic(t *testing.T) {
	m := NewMetrics()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	calls := 0
	done := make(chan struct{})
	m.StartSampler(ctx, 10*time.Millisecond, func(context.Context) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			panic("örnekleyici patladı")
		}
		if n == 3 {
			select {
			case <-done:
			default:
				close(done)
			}
		}
		m.SetQueueDepth("webhook", float64(n))
	})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("örnekleyici panikten sonra devam etmedi")
	}
}

/* ═══════════════════════ Sentry ═══════════════════════ */

type fakeTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (f *fakeTransport) Configure(sentry.ClientOptions) {}
func (f *fakeTransport) SendEvent(e *sentry.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
}
func (f *fakeTransport) Flush(time.Duration) bool              { return true }
func (f *fakeTransport) FlushWithContext(context.Context) bool { return true }
func (f *fakeTransport) Close()                                {}

func (f *fakeTransport) all() []*sentry.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*sentry.Event{}, f.events...)
}

func newTestSentry(t *testing.T) (*Sentry, *fakeTransport) {
	t.Helper()
	tr := &fakeTransport{}
	s, err := NewSentry(SentryConfig{
		DSN:         "https://publickey@o0.ingest.example.com/1",
		Environment: "test",
		transport:   tr,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, tr
}

func TestNewSentryRejectsEmptyDSN(t *testing.T) {
	if _, err := NewSentry(SentryConfig{DSN: "   "}); err == nil {
		t.Fatal("boş DSN kabul edildi — sessizce devre dışı kalırdı")
	}
}

// Hata kaydı hem stdout'a hem Sentry'ye gider; INFO yalnız stdout'a.
func TestSentryHandlerForwardsOnlyErrors(t *testing.T) {
	s, tr := newTestSentry(t)
	var buf bytes.Buffer
	log := slog.New(s.SlogHandler(slog.NewTextHandler(&buf, nil)))

	log.Info("bilgi satırı")
	log.Error("sipariş süresi işlenemedi", "order", "abc")
	s.Flush(2 * time.Second)

	if !strings.Contains(buf.String(), "bilgi satırı") ||
		!strings.Contains(buf.String(), "sipariş süresi işlenemedi") {
		t.Fatalf("asıl işleyiciye satırlar ulaşmadı:\n%s", buf.String())
	}
	ev := tr.all()
	if len(ev) != 1 {
		t.Fatalf("Sentry olay sayısı = %d, beklenen 1", len(ev))
	}
	if ev[0].Message != "sipariş süresi işlenemedi" {
		t.Fatalf("olay mesajı = %q", ev[0].Message)
	}
	if ev[0].Level != sentry.LevelError {
		t.Fatalf("olay seviyesi = %q", ev[0].Level)
	}
}

// 🔴 Sır Sentry'ye ÇIKMAZ: dış bir servise gönderilen alanlar log dosyasından
// daha geniş bir kitleye açılır.
func TestSentryHandlerMasksSensitiveAttributes(t *testing.T) {
	s, tr := newTestSentry(t)
	log := slog.New(s.SlogHandler(slog.NewTextHandler(io.Discard, nil)))

	const (
		secret = "re_COK_GIZLI_ANAHTAR"
		otp    = "483920"
		phone  = "905321234567"
	)
	log.With("api_key", secret).Error("sağlayıcı çağrısı başarısız",
		"otp_code", otp, "phone_number", phone, "order", "ok-gorunur")
	s.Flush(2 * time.Second)

	ev := tr.all()
	if len(ev) != 1 {
		t.Fatalf("Sentry olay sayısı = %d", len(ev))
	}
	raw := fmt.Sprintf("%#v", ev[0].Contexts)
	for _, leak := range []string{secret, otp, phone} {
		if strings.Contains(raw, leak) {
			t.Fatalf("sır Sentry olayına sızdı (%q):\n%s", leak, raw)
		}
	}
	// Maskeleme her şeyi silmemeli; hata ayıklanabilir kalmalı.
	if !strings.Contains(raw, "ok-gorunur") {
		t.Fatalf("hassas olmayan alan da düşürülmüş:\n%s", raw)
	}
}

// Sentry'ye gönderim asıl log satırını YUTMAZ.
func TestSentryHandlerDoesNotSwallowLogLine(t *testing.T) {
	s, _ := newTestSentry(t)
	var buf bytes.Buffer
	log := slog.New(s.SlogHandler(slog.NewTextHandler(&buf, nil)))
	log.Error("iade yazılamadı", "quote", "q-1")
	if !strings.Contains(buf.String(), "iade yazılamadı") ||
		!strings.Contains(buf.String(), "q-1") {
		t.Fatalf("satır stdout'a yazılmadı:\n%s", buf.String())
	}
}
