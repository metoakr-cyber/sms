// Package errors uygulamanın hata taksonomisini tanımlar.
//
// Kural: kullanıcıya ASLA ham hata mesajı gösterilmez (test: scripts/smoke-auth.sh
// — tek biçim hata yanıtı). Her hatanın
//   - makine okunur bir Code'u (istemci dallanması için),
//   - Türkçe, kullanıcıya gösterilebilir bir Message'ı,
//   - ve bir HTTP durum kodu eşlemesi vardır.
//
// Ayrıntı zincirlenen iç hatada (Unwrap) kalır ve yalnız log'a gider.
// Bkz. docs/design.md §11.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind hatanın üst kategorisidir; HTTP durum kodu buradan türer.
type Kind int

const (
	KindDomain    Kind = iota + 1 // iş kuralı ihlali        → 400/409
	KindAuth                      // kimlik/yetki            → 401/403
	KindNotFound                  // kaynak yok              → 404
	KindRateLimit                 // hız limiti              → 429
	KindInfra                     // altyapı / dış servis    → 502/503
	KindInternal                  // beklenmeyen             → 500
)

// Error uygulamanın taşıdığı tek hata tipidir.
type Error struct {
	Kind    Kind
	Code    string // makine okunur, örn. "INSUFFICIENT_BALANCE"
	Message string // kullanıcıya gösterilebilir, Türkçe
	Status  int    // HTTP durum kodu; 0 ise Kind'dan türetilir
	err     error  // sarmalanan iç hata — YALNIZ log'a gider
}

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.err)
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.err }

// HTTPStatus hatanın HTTP durum kodunu döner.
func (e *Error) HTTPStatus() int {
	if e.Status != 0 {
		return e.Status
	}
	switch e.Kind {
	case KindDomain:
		return http.StatusBadRequest
	case KindAuth:
		return http.StatusUnauthorized
	case KindNotFound:
		return http.StatusNotFound
	case KindRateLimit:
		return http.StatusTooManyRequests
	case KindInfra:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// Wrap bir iç hatayı bu Error'a ekler; Code ve Message korunur.
func (e *Error) Wrap(err error) *Error {
	c := *e
	c.err = err
	return &c
}

// WithMessage kullanıcıya gösterilecek mesajı değiştirir.
func (e *Error) WithMessage(msg string) *Error {
	c := *e
	c.Message = msg
	return &c
}

// New yeni bir hata tanımlar. Paket düzeyinde sabit hatalar için kullanılır.
func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// NewStatus belirli bir HTTP durum kodu zorlayan hata tanımlar.
func NewStatus(kind Kind, code, message string, status int) *Error {
	return &Error{Kind: kind, Code: code, Message: message, Status: status}
}

// As bir hata zincirinde *Error arar.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Is standart errors.Is'e köprü. İki *Error aynı Code'a sahipse eşit sayılır.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// Internal beklenmeyen bir hatayı sarmalar. Kullanıcı genel mesaj görür,
// gerçek sebep yalnız log'a ve Sentry'ye gider.
func Internal(err error) *Error {
	return ErrInternal.Wrap(err)
}
