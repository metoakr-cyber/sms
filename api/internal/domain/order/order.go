// Package order sipariş durum makinesidir.
//
// Durum geçişleri KODDA DAĞILMAZ: `status` alanına yalnız bu paketin
// doğruladığı bir geçişten sonra yazılır (docs/design.md §7). Handler veya
// arka plan işi içinde doğrudan bir durum ataması, geçersiz geçişleri
// sessizce mümkün kılar.
//
// Aynı kural veritabanında da zorlanır (migration 00008, orders_guard_transition):
// iki savunma hattı, çünkü bir yarış durumunda tamamlanmış bir siparişe iade
// yazmak gerçek para kaybettirir.
//
// test: ../../service/order/order_integration_test.go#TestTerminalOrderCannotChangeStatus
package order

import (
	"fmt"
	"time"
)

// Status sipariş durumu. Beş değer — altıncısı YOKTUR.
type Status string

const (
	// StatusPending numara alındı, kod bekleniyor.
	StatusPending Status = "PENDING"
	// StatusCompleted kod geldi. TERMİNAL.
	StatusCompleted Status = "COMPLETED"
	// StatusCancelled kullanıcı iptali veya süre doldu. Terminal DEĞİL —
	// iade kaydı yazıldığında REFUNDED'a geçer.
	StatusCancelled Status = "CANCELLED"
	// StatusFailed sipariş oluştuktan SONRA kalıcı sağlayıcı hatası.
	// Terminal DEĞİL — otomatik iade ile REFUNDED'a geçer.
	//
	// DİKKAT: satın alma ANINDA sağlayıcı hata verirse sipariş HİÇ OLUŞMAZ
	// (FR-400/KK-400). Bu durum, sipariş var olduktan sonra sağlayıcının onu
	// kalıcı olarak kaybetmesi/reddetmesi içindir.
	StatusFailed Status = "FAILED"
	// StatusRefunded iade işlendi. TERMİNAL.
	StatusRefunded Status = "REFUNDED"
)

// AllStatuses tanımlı tüm durumlar (doğrulama ve test için).
func AllStatuses() []Status {
	return []Status{StatusPending, StatusCompleted, StatusCancelled, StatusFailed, StatusRefunded}
}

// Valid bilinen bir durum mu.
func (s Status) Valid() bool {
	for _, x := range AllStatuses() {
		if s == x {
			return true
		}
	}
	return false
}

// IsTerminal durumdan çıkış var mı.
//
// YALNIZ COMPLETED ve REFUNDED terminaldir. CANCELLED ve FAILED birer
// GEÇİŞTİR: ikisinin de REFUNDED'a çıkan oku vardır. CANCELLED'ı terminal
// saymak, iade kaydının hiç yazılmaması ve kullanıcının parasını geri
// alamaması demektir.
func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusRefunded
}

// transitions izin verilen geçişler (docs/design.md §7.1).
var transitions = map[Status][]Status{
	StatusPending:   {StatusCompleted, StatusCancelled, StatusFailed},
	StatusCancelled: {StatusRefunded},
	StatusFailed:    {StatusRefunded},
	// Terminal durumlardan çıkış yok — bilerek boş.
	StatusCompleted: {},
	StatusRefunded:  {},
}

// ErrInvalidTransition geçersiz durum geçişi.
type ErrInvalidTransition struct {
	From, To Status
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("order: geçersiz durum geçişi: %s → %s", e.From, e.To)
}

// CanTransition geçiş izinli mi.
func CanTransition(from, to Status) bool {
	if from == to {
		return true // yeniden yazım (idempotent güncelleme) serbesttir
	}
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// Transition geçişi doğrular.
//
// test: order_test.go#TestTransitionTable
func Transition(from, to Status) error {
	if !from.Valid() {
		return fmt.Errorf("order: bilinmeyen kaynak durum %q", from)
	}
	if !to.Valid() {
		return fmt.Errorf("order: bilinmeyen hedef durum %q", to)
	}
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}
	return nil
}

// ─────────────────────────── Kapatma kararı ───────────────────────────

// CloseAction sipariş sağlayıcıda nasıl kapatılacak.
type CloseAction string

const (
	// CloseFinish kod TESLİM EDİLDİ — iade talep edilmez.
	CloseFinish CloseAction = "FINISH"
	// CloseCancel kod gelmedi — iade TALEP EDİLİR.
	CloseCancel CloseAction = "CANCEL"
)

// DecideClose siparişin sağlayıcıda nasıl kapatılacağını söyler (ADR-023).
//
// Cancel() ve Finish() AYNI ŞEY DEĞİLDİR:
//   - Kod geldiği hâlde Cancel() → sağlayıcı OTP_RECEIVED ile kalıcı reddeder,
//     üstelik yanlış sinyal vermiş oluruz.
//   - Kod gelmediği hâlde Finish() → hak ettiğimiz iadeden vazgeçmiş oluruz.
//
// Karar TEK ÖLÇÜTE dayanır: elimizde teslim edilmiş bir mesaj var mı.
// Sipariş durumuna değil, çünkü durum kodu bir yorumdur; mesajın varlığı
// gözlemlenebilir bir gerçektir.
//
// test: order_test.go#TestDecideClose
func DecideClose(hasDeliveredMessage bool) CloseAction {
	if hasDeliveredMessage {
		return CloseFinish
	}
	return CloseCancel
}

// ─────────────────────────── İptal edilebilirlik ───────────────────────────

// CancelCheck iptal isteğinin sonucu.
type CancelCheck struct {
	Allowed bool
	// RetryAfter iptal henüz mümkün değilse ne kadar beklenmeli.
	RetryAfter time.Duration
	Reason     string
}

// CanUserCancel kullanıcı bu siparişi şu an iptal edebilir mi (FR-416).
//
// İKİ KOŞUL:
//  1. Sipariş PENDING olmalı — terminal ya da iptal edilmiş sipariş iptal edilemez.
//  2. `cancellableAt` geçmiş olmalı — sağlayıcı ilk ~120 saniye iptali
//     reddediyor. Bu süre SAĞLAYICIDAN gelir ve sunucuda tutulur; istemcinin
//     saatine güvenilmez.
//
// İkinci koşulu atlarsak kullanıcı butona basar, sağlayıcı reddeder ve
// kullanıcı "iptal çalışmıyor" der — oysa yalnız erkendir.
//
// test: order_test.go#TestCanUserCancel
func CanUserCancel(status Status, cancellableAt, now time.Time) CancelCheck {
	if status != StatusPending {
		return CancelCheck{Reason: "sipariş beklemede değil"}
	}
	if now.Before(cancellableAt) {
		return CancelCheck{
			RetryAfter: cancellableAt.Sub(now),
			Reason:     "iptal için henüz erken",
		}
	}
	return CancelCheck{Allowed: true}
}

// IsExpired sipariş süresi doldu mu.
//
// Karar SUNUCUDAKİ expires_at ile verilir; istemcinin gönderdiği hiçbir süre
// bilgisi kullanılmaz (KK-405).
//
// test: order_test.go#TestIsExpired
func IsExpired(expiresAt, now time.Time) bool {
	return !now.Before(expiresAt)
}
