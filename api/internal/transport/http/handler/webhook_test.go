package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeQueue struct {
	calls   atomic.Int32
	lastRaw atomic.Value
	fail    bool
	delay   time.Duration
}

func (q *fakeQueue) Enqueue(ctx context.Context, raw []byte) error {
	q.calls.Add(1)
	q.lastRaw.Store(string(raw))
	if q.delay > 0 {
		select {
		case <-time.After(q.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if q.fail {
		return context.DeadlineExceeded
	}
	return nil
}

func newTestRouter(q *fakeQueue, secret string, ips []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewWebhook(q, secret, ips, Responder{
		OK:        func(c *gin.Context, b any) { c.JSON(200, b) },
		NoContent: func(c *gin.Context) { c.Status(204) },
		Fail:      func(c *gin.Context, err error) { c.Status(500) },
	})
	r.POST("/webhooks/herosms/:secret", h.HeroSMS)
	return r
}

func post(r *gin.Engine, path, body, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestHandlerDoesNoWorkBeyondEnqueue
//
// SÖZLEŞME: handler YALNIZ (a) IP kontrolü, (b) kuyruğa atma, (c) 200 yapar.
// 3 saniye bütçesi var; aşılırsa sağlayıcı "en az 7 kez", 3 dakika boyunca
// yeniden dener ve aynı SMS sekiz kez işlenir.
//
// Bu test, handler'ın kuyruğa atmanın ÖTESİNDE iş yapmadığını gösterir:
// kuyruk çağrısı bir kez olur ve yanıt anında döner.
func TestHandlerDoesNoWorkBeyondEnqueue(t *testing.T) {
	q := &fakeQueue{}
	r := newTestRouter(q, "sirr", []string{"1.2.3.4"})

	start := time.Now()
	w := post(r, "/webhooks/herosms/sirr", `{"activationId":123,"code":"1111"}`, "1.2.3.4:5000")
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d, 200 bekleniyordu", w.Code)
	}
	if n := q.calls.Load(); n != 1 {
		t.Fatalf("kuyruğa %d kez yazıldı, 1 bekleniyordu", n)
	}
	// Bütçenin çok altında olmalı: handler'da senkron iş yok.
	if elapsed > 500*time.Millisecond {
		t.Errorf("handler %v sürdü — 3 sn bütçesi var ama iş kuyruğa gitmeli", elapsed)
	}
}

// TestWebhookAlwaysReturns200
//
// Hata durumlarında bile 200. 200 dışı bir yanıt sağlayıcıyı yeniden deneme
// fırtınasına sokar; bizim hatamızı ona yük olarak geri veremeyiz.
func TestWebhookAlwaysReturns200(t *testing.T) {
	cases := []struct {
		name, body, addr string
		queueFails       bool
	}{
		{"geçerli", `{"activationId":1}`, "1.2.3.4:1", false},
		{"bozuk JSON", `{bozuk`, "1.2.3.4:1", false},
		{"boş gövde", ``, "1.2.3.4:1", false},
		{"kuyruk arızalı", `{"activationId":1}`, "1.2.3.4:1", true},
		{"izin listesi dışı IP", `{"activationId":1}`, "9.9.9.9:1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := &fakeQueue{fail: c.queueFails}
			r := newTestRouter(q, "sirr", []string{"1.2.3.4"})
			w := post(r, "/webhooks/herosms/sirr", c.body, c.addr)
			if w.Code != http.StatusOK {
				t.Errorf("durum = %d, 200 bekleniyordu — sağlayıcı fırtınası riski", w.Code)
			}
		})
	}
}

// TestWrongSecretIsNotFound
//
// Yanlış sır 404 alır: "yanlış sır" demek, doğru sırrın var olduğunu
// söylemektir.
func TestWrongSecretIsNotFound(t *testing.T) {
	q := &fakeQueue{}
	r := newTestRouter(q, "dogru-sir", []string{"1.2.3.4"})
	w := post(r, "/webhooks/herosms/yanlis-sir", `{"activationId":1}`, "1.2.3.4:1")
	if w.Code != http.StatusNotFound {
		t.Errorf("durum = %d, 404 bekleniyordu", w.Code)
	}
	if n := q.calls.Load(); n != 0 {
		t.Errorf("yanlış sırla kuyruğa yazıldı (%d)", n)
	}
}

// TestForwardedForHeaderIsIgnored
//
// 🔴 `X-Forwarded-For` istemci tarafından UYDURULABİLİR. Ona bakmak izin
// listesini tamamen anlamsız kılar: saldırgan başlığa izinli bir IP yazıp
// geçer.
func TestForwardedForHeaderIsIgnored(t *testing.T) {
	q := &fakeQueue{}
	r := newTestRouter(q, "sirr", []string{"1.2.3.4"})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/herosms/sirr",
		strings.NewReader(`{"activationId":1}`))
	req.RemoteAddr = "9.9.9.9:1234"              // GERÇEK kaynak: izinli değil
	req.Header.Set("X-Forwarded-For", "1.2.3.4") // uydurulmuş
	req.Header.Set("X-Real-IP", "1.2.3.4")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if n := q.calls.Load(); n != 0 {
		t.Fatal("X-Forwarded-For başlığına bakılmış — izin listesi atlanabilir")
	}
}

// TestEmptyAllowListRejectsEverything
//
// İzin listesi boşsa HİÇBİR istek kabul edilmez. "Boşsa hepsine izin ver"
// davranışı, yapılandırma unutulduğunda ucu herkese açardı.
func TestEmptyAllowListRejectsEverything(t *testing.T) {
	q := &fakeQueue{}
	r := newTestRouter(q, "sirr", nil)
	post(r, "/webhooks/herosms/sirr", `{"activationId":1}`, "1.2.3.4:1")
	if n := q.calls.Load(); n != 0 {
		t.Fatal("boş izin listesiyle istek kabul edildi")
	}
}

// TestIPv6MappedAddressIsAccepted
//
// ::ffff:1.2.3.4 ile 1.2.3.4 AYNI adrestir. Metin karşılaştırması yapan bir
// kontrol, sağlayıcı IPv6 üzerinden bağlandığında meşru bildirimleri reddeder.
func TestIPv6MappedAddressIsAccepted(t *testing.T) {
	q := &fakeQueue{}
	r := newTestRouter(q, "sirr", []string{"1.2.3.4"})
	post(r, "/webhooks/herosms/sirr", `{"activationId":1}`, "[::ffff:1.2.3.4]:5000")
	if n := q.calls.Load(); n != 1 {
		t.Fatalf("IPv6 eşlenmiş adres reddedildi (kuyruk çağrısı: %d)", n)
	}
}
