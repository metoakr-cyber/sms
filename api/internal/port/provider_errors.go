package port

import (
	"errors"
	"fmt"
	"time"
)

// Sağlayıcı hataları. Adaptörler ham yanıtları BU tiplere çevirir; servis
// katmanı sağlayıcıya özgü hiçbir dizgi tanımaz.
var (
	// ErrOutOfStock istenen kombinasyonda numara yok.
	ErrOutOfStock = errors.New("provider: stok yok")
	// ErrProviderAuth API anahtarı geçersiz. 🚨 Admin alarmı.
	ErrProviderAuth = errors.New("provider: kimlik doğrulama başarısız")
	// ErrProviderNoBalance SAĞLAYICIDAKİ bakiyemiz yetersiz. 🚨 Admin alarmı.
	ErrProviderNoBalance = errors.New("provider: sağlayıcı bakiyesi yetersiz")
	// ErrPriceChanged fiyat MaxCost'un üstüne çıktı; satın alma yapılmadı.
	ErrPriceChanged = errors.New("provider: fiyat değişti")
	// ErrMappingMissing servis/ülke eşleştirmesi bozuk. 🚨 Admin alarmı.
	ErrMappingMissing = errors.New("provider: eşleştirme eksik")
	// ErrOrderNotFound sağlayıcı siparişi tanımıyor.
	ErrOrderNotFound = errors.New("provider: sipariş bulunamadı")
	// ErrOrderClosed sipariş zaten kapanmış; mesajları artık okunamaz.
	ErrOrderClosed = errors.New("provider: sipariş kapalı")
	// ErrCancelDenied iptal kalıcı olarak reddedildi (OTP geldi veya süre doldu).
	ErrCancelDenied = errors.New("provider: iptal reddedildi")
	// ErrUnavailable sağlayıcıya ulaşılamıyor veya sunucu hatası.
	ErrUnavailable = errors.New("provider: erişilemiyor")
	// ErrUnsupported sağlayıcı bu işlemi desteklemiyor.
	ErrUnsupported = errors.New("provider: desteklenmeyen işlem")
)

// RetryAfterError sağlayıcının "şu kadar sonra tekrar dene" dediği hatalar.
//
// Sabit bir geri çekilme KULLANILMAZ: sağlayıcı süreyi kendisi söylüyorsa
// ona uyulur (FR-414). HeroSMS'te 429 RATE_LIMIT, 425 TOO_EARLY ve
// EARLY_CANCEL_DENIED bu sınıfa girer.
type RetryAfterError struct {
	Reason string
	After  time.Duration
	err    error
}

func NewRetryAfter(reason string, after time.Duration, cause error) *RetryAfterError {
	return &RetryAfterError{Reason: reason, After: after, err: cause}
}

func (e *RetryAfterError) Error() string {
	return fmt.Sprintf("provider: %s — %s sonra tekrar deneyin", e.Reason, e.After)
}
func (e *RetryAfterError) Unwrap() error { return e.err }

// AsRetryAfter hata zincirinde RetryAfterError arar.
func AsRetryAfter(err error) (*RetryAfterError, bool) {
	var e *RetryAfterError
	ok := errors.As(err, &e)
	return e, ok
}

// BanScope yasaklamanın kapsamı.
type BanScope string

const (
	BanGlobal   BanScope = "global"   // tüm sağlayıcı kapatılır
	BanSpecific BanScope = "specific" // yalnız o (servis, ülke) çifti
)

// BannedError sağlayıcı bizi (veya bir kombinasyonu) yasakladı.
//
// Kapsam ÖNEMLİDİR: 'specific' bir yasak tüm sağlayıcıyı kapatmamalıdır,
// yoksa tek bir servis yüzünden tüm satış durur (ADR-028).
type BannedError struct {
	Scope BanScope
	Until time.Time
	err   error
}

func NewBanned(scope BanScope, until time.Time, cause error) *BannedError {
	return &BannedError{Scope: scope, Until: until, err: cause}
}

func (e *BannedError) Error() string {
	return fmt.Sprintf("provider: yasaklandı (kapsam=%s, bitiş=%s)", e.Scope, e.Until.Format(time.RFC3339))
}
func (e *BannedError) Unwrap() error { return e.err }

func AsBanned(err error) (*BannedError, bool) {
	var e *BannedError
	ok := errors.As(err, &e)
	return e, ok
}

// ConcurrencyLimitError sağlayıcının EŞZAMANLI işlem limiti aşıldı.
// Hız limitinden (istek/saniye) FARKLI bir mekanizmadır (FR-413).
type ConcurrencyLimitError struct {
	Current    int
	MaxAllowed int
}

func (e *ConcurrencyLimitError) Error() string {
	return fmt.Sprintf("provider: eşzamanlılık limiti (%d/%d)", e.Current, e.MaxAllowed)
}

func AsConcurrencyLimit(err error) (*ConcurrencyLimitError, bool) {
	var e *ConcurrencyLimitError
	ok := errors.As(err, &e)
	return e, ok
}
