package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

func init() { gin.SetMode(gin.TestMode) }

// captureLogs slog çıktısını yakalar.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

// Yorumun iddiası: "Yığın izi asla istemciye gitmez."
func TestPanicDoesNotLeakStack(t *testing.T) {
	logs := captureLogs(t)

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Recovery())
	r.GET("/patla", func(c *gin.Context) {
		panic("gizli detay: veritabanı şifresi hatalı")
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/patla", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("durum = %d, 500 bekleniyordu", w.Code)
	}
	body := w.Body.String()
	for _, leak := range []string{"gizli detay", "veritabanı şifresi", "goroutine", "panic", ".go:"} {
		if strings.Contains(body, leak) {
			t.Fatalf("yanıt gövdesi %q içeriyor — yığın izi/panik detayı sızdı:\n%s", leak, body)
		}
	}

	var parsed map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("yanıt JSON değil: %s", body)
	}
	e, _ := parsed["error"].(map[string]any)
	if e == nil || e["code"] != "INTERNAL" || e["requestId"] == "" {
		t.Fatalf("tek biçim hata yanıtı bekleniyordu: %v", parsed)
	}

	// Yığın izi LOG'a gitmeli — kaybolmamalı.
	if !strings.Contains(logs.String(), "gizli detay") {
		t.Fatal("panik detayı log'a da yazılmamış — teşhis imkânsız olur")
	}
}

// Yorumun iddiası: "Ama SESSİZ KALMAYIZ — alarm üretmelidir."
type brokenLimiter struct{}

func (brokenLimiter) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return false, 0, errors.New("redis erişilemiyor")
}
func (brokenLimiter) Reset(context.Context, string) error { return nil }

func TestFailOpenIsLogged(t *testing.T) {
	logs := captureLogs(t)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/x",
		middleware.RateLimit(brokenLimiter{}, "test", middleware.RateLimitConfig{
			Limit: 1, Window: time.Minute,
		}, func(c *gin.Context, err error) { c.AbortWithStatus(http.StatusTooManyRequests) }),
		func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	// Fail-OPEN: limitleyici çökse de istek geçmeli.
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d, 200 bekleniyordu (fail-open)", w.Code)
	}
	// Ama SESSİZ olmamalı.
	out := logs.String()
	if !strings.Contains(out, "level=ERROR") {
		t.Fatalf("fail-open ERROR seviyesinde log'lanmadı:\n%s", out)
	}
	if !strings.Contains(out, "koruma devre dışı") {
		t.Fatalf("log mesajı korumanın kapandığını söylemiyor:\n%s", out)
	}
}

// Bozuk X-Request-Id reddedilmeli (log enjeksiyonu koruması).
func TestRequestIDRejectsMalformed(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, bad := range []string{"kotu niyet", "a\nb", strings.Repeat("x", 100), "<script>"} {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set(middleware.HeaderRequestID, bad)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if got := w.Header().Get(middleware.HeaderRequestID); got == bad {
			t.Errorf("bozuk kimlik kabul edildi: %q", bad)
		}
	}
	// Geçerli olan korunmalı
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(middleware.HeaderRequestID, "gecerli-iz-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get(middleware.HeaderRequestID); got != "gecerli-iz-123" {
		t.Errorf("geçerli kimlik korunmadı: %q", got)
	}
}
