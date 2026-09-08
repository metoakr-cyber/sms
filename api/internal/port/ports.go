// Package port uygulama katmanının dış dünyaya bakan ARAYÜZLERİNİ tanımlar.
//
// Bağımlılık yönü: service → port ← adapter.
// Servis katmanı somut adaptörü (Postgres, Redis, HeroSMS) tanımaz; yalnız buradaki
// sözleşmeyi bilir. Bu sayede tüm akışlar sahte (fake) adaptörlerle, ağ ve
// veritabanı olmadan test edilebilir.
package port

import (
	"context"
	"time"
)

// ─────────────────────────── Zaman ───────────────────────────

// Clock zamanı soyutlar. time.Now() doğrudan çağrılmaz: süre dolumu, token
// geçerliliği ve iptal penceresi gibi kuralları test edebilmek için zaman
// enjekte edilebilir olmalıdır.
type Clock interface {
	Now() time.Time
}

// RealClock sistem saatini kullanır.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// ─────────────────────────── Oturum ───────────────────────────

// Session bir oturumun uygulama katmanındaki görünümü.
type Session struct {
	ID        string
	UserID    int64
	IP        string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionStore oturumların hızlı arandığı depodur (Redis).
//
// Sunucu tarafı oturum tercih edildi çünkü JWT iptal edilemiyor: kullanıcı
// yasaklandığında veya şifresini değiştirdiğinde token ömrü boyunca geçerli
// kalırdı (docs/design.md ADR-003).
type SessionStore interface {
	Create(ctx context.Context, s Session) error
	Get(ctx context.Context, id string) (Session, error)
	Touch(ctx context.Context, id string, ttl time.Duration) error
	Revoke(ctx context.Context, id string) error
	// RevokeAllForUser bir kullanıcının TÜM oturumlarını düşürür.
	// Yasaklama ve şifre değişiminde çağrılır.
	RevokeAllForUser(ctx context.Context, userID int64) error
}

// ─────────────────────────── E-posta ───────────────────────────

// Mail gönderilecek bir e-posta.
type Mail struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Mailer e-posta gönderimini soyutlar.
type Mailer interface {
	Send(ctx context.Context, m Mail) error
}

// ─────────────────────────── Robot doğrulama ───────────────────────────

// Captcha robot doğrulamasını soyutlar.
type Captcha interface {
	// Verify token'ı doğrular. Yapılandırma eksikse hata döner — sessizce
	// başarılı saymaz (eski prototipin hatası: anahtar yokken doğrulama hep
	// başarısız oluyordu ve kimse fark etmiyordu).
	Verify(ctx context.Context, token, remoteIP string) error
	// Enabled doğrulamanın etkin olup olmadığını söyler.
	Enabled() bool
}

// ─────────────────────────── Hız limiti ───────────────────────────

// RateLimiter kaydırmalı pencere sayacı.
type RateLimiter interface {
	// Allow anahtarı için istek sayısını artırır ve limite uyulup uyulmadığını döner.
	// retryAfter, limit aşıldıysa ne kadar beklenmesi gerektiğidir.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error)
	// Reset sayacı sıfırlar (başarılı girişten sonra).
	Reset(ctx context.Context, key string) error
}
