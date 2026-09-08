package obs

// Prometheus metrikleri.
//
// 🔴 ETİKET KARDİNALİTESİ BU DOSYANIN ASIL KONUSUDUR. Prometheus'ta her etiket
// kombinasyonu ayrı bir zaman serisidir. Etikete kullanıcı kimliği, telefon
// numarası veya sipariş kimliği koymak iki şeyi birden yapar: veritabanı
// büyüklüğünde bir seri patlaması ve /metrics ucundan kişisel veri sızıntısı.
// Bu yüzden yol etiketi normalize edilir ve ayrıca bir üst sınırla korunur.

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// maxRouteLabels izlenecek azami farklı yol etiketi.
//
// Normalize edilmiş yollar bugün 40 civarında. Sınır, normalize etme
// mantığında ileride açılacak bir deliğin metrik deposunu şişirmesini
// önlemek için var: sınır aşılınca yeni yollar "other" altında toplanır.
const maxRouteLabels = 200

// Metrics uygulama metriklerini toplar.
type Metrics struct {
	reg *prometheus.Registry

	httpDuration     *prometheus.HistogramVec
	providerDuration *prometheus.HistogramVec
	providerErrors   *prometheus.CounterVec
	queueDepth       *prometheus.GaugeVec

	mu     sync.Mutex
	routes map[string]struct{}
}

// NewMetrics yeni bir metrik kümesi ve kendi kayıt defterini kurar.
//
// KENDİ DEFTERİ: prometheus.DefaultRegisterer süreç genelinde paylaşılan bir
// global. Testte iki kez kurmak "duplicate metrics collector" paniğine yol
// açar ve bir kütüphanenin kendi metriğini kaydetmesi bizim çıktımızı
// kirletir.
func NewMetrics() *Metrics {
	m := &Metrics{
		reg:    prometheus.NewRegistry(),
		routes: make(map[string]struct{}),

		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "sms_http_request_duration_seconds",
			Help: "HTTP istek süresi. İstek SAYISI için ayrı bir sayaç yok: " +
				"histogram zaten _count serisini yayınlıyor.",
			// Kovalar bu uygulamaya göre: katalog uçları milisaniyeler,
			// satın alma sağlayıcıyı beklediği için saniyeler sürer.
			Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"method", "route", "status"}),

		providerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "sms_provider_call_duration_seconds",
			Help:    "Üst sağlayıcıya yapılan çağrının süresi.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"provider", "op"}),

		providerErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sms_provider_call_errors_total",
			Help: "Sağlayıcı çağrısı hataları. `kind` tipli hata kodudur, " +
				"ham mesaj DEĞİLDİR — ham mesaj sınırsız kardinalite demektir.",
		}, []string{"provider", "op", "kind"}),

		queueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sms_queue_depth",
			Help: "Kuyrukta bekleyen öğe sayısı.",
		}, []string{"queue"}),
	}

	m.reg.MustRegister(
		m.httpDuration, m.providerDuration, m.providerErrors, m.queueDepth,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// Handler /metrics ucunun işleyicisidir.
//
// Rotaya BURADA bağlanmaz: router.go bu paketin sahibi değil ve taşıma
// katmanı adaptör paketlerini import etmemeli. Kablolama main/router
// tarafında yapılır.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{
		// Toplama sırasında bir hata olursa metrik yerine 500 dönsün;
		// yarım bir çıktı, izleme sisteminde sessiz yanlış veri demektir.
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
}

// ObserveHTTP bir isteği kaydeder. `route` gin YOL ŞABLONUDUR
// (`/api/v1/orders/:id`), ham yol değil.
//
// Ham yol verilse bile etiketler normalize edilir: sipariş kimliği taşıyan bir
// segment `:id`'ye indirgenir.
// test: obs_test.go#TestObserveHTTPNormalizesRouteLabel
func (m *Metrics) ObserveHTTP(method, route string, status int, d time.Duration) {
	m.httpDuration.WithLabelValues(
		strings.ToUpper(method),
		m.routeLabel(route),
		strconv.Itoa(status),
	).Observe(d.Seconds())
}

// ObserveProviderCall bir sağlayıcı çağrısını kaydeder.
// kind: tipli hata kodu ("OUT_OF_STOCK", "PROVIDER_TIMEOUT", …) veya başarıda "".
func (m *Metrics) ObserveProviderCall(provider, op, kind string, d time.Duration) {
	m.providerDuration.WithLabelValues(provider, op).Observe(d.Seconds())
	if kind != "" {
		m.providerErrors.WithLabelValues(provider, op, kind).Inc()
	}
}

// SetQueueDepth kuyruk uzunluğu göstergesini günceller.
func (m *Metrics) SetQueueDepth(queue string, n float64) {
	m.queueDepth.WithLabelValues(queue).Set(n)
}

// StartSampler örnekleyiciyi periyodik çalıştırır (kuyruk uzunluğu gibi
// yalnız SORULARAK öğrenilen değerler için).
//
// Panik yakalanır: bir örnekleyici hatası süreci düşürmemeli — worker.Runner
// ile aynı gerekçe (internal/worker/worker.go).
func (m *Metrics) StartSampler(ctx context.Context, every time.Duration, sample func(context.Context)) {
	if every <= 0 || sample == nil {
		return
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		run := func() {
			defer func() {
				if p := recover(); p != nil {
					slog.Error("metrik örnekleyici panik verdi",
						"panic", p, "stack", string(debug.Stack()))
				}
			}()
			sample(ctx)
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}

/* ═══════════════════════ Etiket normalizasyonu ═══════════════════════ */

// routeLabel yol etiketini güvenli hale getirir ve sayısını sınırlar.
//
// Sınır aşıldığında yeni yollar "other" altında toplanır: sınırsız etiket,
// izleme sunucusunun belleğini tüketen klasik arızadır.
// test: obs_test.go#TestRouteLabelCardinalityIsCapped
func (m *Metrics) routeLabel(route string) string {
	r := normalizeRoute(route)

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.routes[r]; ok {
		return r
	}
	if len(m.routes) >= maxRouteLabels {
		return "other"
	}
	m.routes[r] = struct{}{}
	return r
}

// normalizeRoute değişken segmentleri sabitler.
//
// Rakam içeren veya çok uzun bir segment kimlik sayılır ve `:id`'ye indirgenir:
// UUID, sipariş numarası ve telefon numarası bu şekilde etiketten düşer.
// test: obs_test.go#TestObserveHTTPNormalizesRouteLabel
func normalizeRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return "unknown"
	}
	// Sorgu dizesi etikete hiç girmez; içinde token/e-posta olabilir.
	if i := strings.IndexAny(route, "?#"); i >= 0 {
		route = route[:i]
	}
	parts := strings.Split(route, "/")
	for i, p := range parts {
		switch {
		case p == "" || strings.HasPrefix(p, ":") || strings.HasPrefix(p, "*"):
			// gin şablonunun kendi yer tutucusu — zaten güvenli.
		case isVersionSegment(p):
			// `/api/v1/...` — rakam taşıyor ama kimlik değil, sabit sayıda.
		case strings.ContainsAny(p, "0123456789") || len(p) > 40:
			parts[i] = ":id"
		}
	}
	out := strings.Join(parts, "/")
	if out == "" {
		return "unknown"
	}
	return out
}

// isVersionSegment `v1`, `v2` gibi API sürüm segmentlerini tanır.
func isVersionSegment(p string) bool {
	if len(p) < 2 || (p[0] != 'v' && p[0] != 'V') {
		return false
	}
	for _, c := range p[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
