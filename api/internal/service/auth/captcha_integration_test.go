//go:build integration

package auth_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	authsvc "github.com/ikmetrik/sms-platform/api/internal/service/auth"
	"github.com/ikmetrik/sms-platform/api/internal/testsupport"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	// 🔴 GÜVENLİK KAPISI: bu testler DELETE FROM yapar. Veritabanı adı
	// "_test" ile bitmiyorsa süreç durur — kapı Makefile'da değil burada,
	// çünkü `go test` komutunu elle yazan kişiyi Makefile korumaz.
	if _, gerr := testsupport.MustTestDatabaseURL(); gerr != nil {
		fmt.Fprintln(os.Stderr, gerr)
		os.Exit(1)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		// SESSİZ ATLAMA YOK.
		//
		// Buradaki eski davranış `os.Exit(0)` idi: DATABASE_URL tanımsızsa
		// tüm entegrasyon testleri atlanır ve `go test` "ok" derdi. Yani
		// veritabanı olmayan bir ortamda paket YEŞİL geçiyordu — sıfır
		// entegrasyon kapsamıyla. Bu, testin olmamasından kötüdür: kimse
		// eksik olduğunu fark etmez.
		//
		// Bilerek atlamak için ALLOW_SKIP_INTEGRATION=1 gerekir; o zaman da
		// atlama AÇIKÇA yazılır.
		if os.Getenv("ALLOW_SKIP_INTEGRATION") == "1" {
			fmt.Println("⚠️  DATABASE_URL tanımsız — entegrasyon testleri ATLANDI (ALLOW_SKIP_INTEGRATION=1)")
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "DATABASE_URL tanımsız — entegrasyon testleri çalıştırılamıyor.")
		fmt.Fprintln(os.Stderr, "  Çözüm: `set -a; source .env; set +a`  veya  `make check`")
		fmt.Fprintln(os.Stderr, "  Bilerek atlamak için: ALLOW_SKIP_INTEGRATION=1")
		os.Exit(1)
	}
	p, err := postgres.NewPool(context.Background(), url)
	if err != nil {
		fmt.Printf("postgres: %v\n", err)
		os.Exit(1)
	}
	pool = p
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// ─────────────────────── sahte bağımlılıklar ───────────────────────

type stubCaptcha struct {
	enabled bool
	err     error
	calls   int
	mu      sync.Mutex
}

func (s *stubCaptcha) Enabled() bool { return s.enabled }
func (s *stubCaptcha) Verify(_ context.Context, token, _ string) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.err
}
func (s *stubCaptcha) Calls() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }

type stubSessions struct {
	mu sync.Mutex
	m  map[string]port.Session
}

func newStubSessions() *stubSessions { return &stubSessions{m: map[string]port.Session{}} }
func (s *stubSessions) Create(_ context.Context, sess port.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[sess.ID] = sess
	return nil
}
func (s *stubSessions) Get(_ context.Context, id string) (port.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[id]; ok {
		return v, nil
	}
	return port.Session{}, apperr.ErrUnauthenticated
}
func (s *stubSessions) Touch(context.Context, string, time.Duration) error { return nil }
func (s *stubSessions) Revoke(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	return nil
}
func (s *stubSessions) RevokeAllForUser(context.Context, int64) error { return nil }

type stubMailer struct {
	mu   sync.Mutex
	sent []port.Mail
}

func (m *stubMailer) Send(_ context.Context, mail port.Mail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, mail)
	return nil
}

type noLimiter struct{}

func (noLimiter) Allow(context.Context, string, int, time.Duration) (bool, time.Duration, error) {
	return true, 0, nil
}
func (noLimiter) Reset(context.Context, string) error { return nil }

func newService(t *testing.T, cap port.Captcha) *authsvc.Service {
	t.Helper()
	return authsvc.New(authsvc.Deps{
		TxRunner:   postgres.NewTxRunner(pool),
		Sessions:   newStubSessions(),
		Mailer:     &stubMailer{},
		Captcha:    cap,
		Limiter:    noLimiter{},
		Clock:      port.RealClock{},
		SessionTTL: time.Hour,
		BaseURL:    "http://test.local",
	})
}

func cleanup(t *testing.T, email string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	})
}

// ─────────────────────── testler ───────────────────────

// reCAPTCHA etkinken token OLMADAN kayıt YAPILAMAZ.
func TestCaptchaBlocksRegistrationWhenEnabled(t *testing.T) {
	ctx := context.Background()
	const email = "captcha-kayit@test.local"
	cleanup(t, email)

	cap := &stubCaptcha{enabled: true, err: apperr.ErrCaptchaFailed}
	s := newService(t, cap)

	err := s.Register(ctx, authsvc.RegisterInput{
		Email: email, Username: "captchakayit",
		Password: "yeterince-uzun-parola", CaptchaToken: "",
	})
	if !errIs(err, apperr.ErrCaptchaFailed) {
		t.Fatalf("hata = %v, ErrCaptchaFailed bekleniyordu", err)
	}
	if cap.Calls() != 1 {
		t.Fatalf("captcha doğrulaması %d kez çağrıldı, 1 bekleniyordu", cap.Calls())
	}

	// Kullanıcı OLUŞMAMALI: captcha kontrolü kayıt yazımından ÖNCE olmalı.
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email=$1`, email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("captcha başarısız olmasına rağmen kullanıcı oluştu")
	}
}

// reCAPTCHA etkinken giriş de engellenir.
func TestCaptchaBlocksLoginWhenEnabled(t *testing.T) {
	ctx := context.Background()
	const email = "captcha-giris@test.local"
	cleanup(t, email)

	// Önce captcha KAPALIYKEN bir hesap oluştur.
	if err := newService(t, &stubCaptcha{enabled: false}).Register(ctx, authsvc.RegisterInput{
		Email: email, Username: "captchagiris", Password: "yeterince-uzun-parola",
	}); err != nil {
		t.Fatal(err)
	}

	cap := &stubCaptcha{enabled: true, err: apperr.ErrCaptchaFailed}
	_, err := newService(t, cap).Login(ctx, authsvc.LoginInput{
		Email: email, Password: "yeterince-uzun-parola", CaptchaToken: "",
	})
	if !errIs(err, apperr.ErrCaptchaFailed) {
		t.Fatalf("hata = %v, ErrCaptchaFailed bekleniyordu", err)
	}
	if cap.Calls() != 1 {
		t.Fatalf("captcha %d kez çağrıldı", cap.Calls())
	}
}

// Captcha geçerliyse akış normal ilerler.
func TestCaptchaAllowsWhenValid(t *testing.T) {
	ctx := context.Background()
	const email = "captcha-gecerli@test.local"
	cleanup(t, email)

	cap := &stubCaptcha{enabled: true, err: nil}
	s := newService(t, cap)

	if err := s.Register(ctx, authsvc.RegisterInput{
		Email: email, Username: "captchagecerli",
		Password: "yeterince-uzun-parola", CaptchaToken: "gecerli-token",
	}); err != nil {
		t.Fatalf("geçerli captcha ile kayıt başarısız: %v", err)
	}
	if cap.Calls() != 1 {
		t.Fatalf("captcha %d kez çağrıldı", cap.Calls())
	}
}

// Captcha KAPALIYKEN hiç çağrılmaz (geliştirme kolaylığı).
func TestCaptchaSkippedWhenDisabled(t *testing.T) {
	ctx := context.Background()
	const email = "captcha-kapali@test.local"
	cleanup(t, email)

	cap := &stubCaptcha{enabled: false, err: apperr.ErrCaptchaFailed}
	if err := newService(t, cap).Register(ctx, authsvc.RegisterInput{
		Email: email, Username: "captchakapali", Password: "yeterince-uzun-parola",
	}); err != nil {
		t.Fatalf("captcha kapalıyken kayıt başarısız: %v", err)
	}
	if cap.Calls() != 0 {
		t.Fatalf("captcha kapalıyken %d kez çağrıldı", cap.Calls())
	}
}

func errIs(err error, target *apperr.Error) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == target.Code
}

var _ = errors.Is
