// Package deposit bakiye yükleme talebinin durum makinesidir.
//
// Durum geçişleri KODDA DAĞILMAZ: `deposits.status` alanına yalnız bu paketin
// doğruladığı bir geçişten sonra yazılır (CLAUDE.md değişmez #13,
// docs/design.md §7.2).
//
// Aynı kural veritabanında da zorlanır (migration 00009,
// deposits_guard_transition): iki savunma hattı, çünkü onaylanmış bir talebi
// tekrar PENDING yapıp ikinci kez onaylamak gerçek para kaybettirir.
//
// test: ../../service/deposit/deposit_integration_test.go#TestTerminalDepositCannotChangeStatus
package deposit

import "fmt"

// Status yükleme talebinin durumu. Dört değer — beşincisi YOKTUR.
type Status string

const (
	// StatusPending yönetici incelemesi bekleniyor.
	StatusPending Status = "PENDING"
	// StatusCompleted onaylandı, bakiye yazıldı. Terminal DEĞİL —
	// itiraz/hata durumunda REFUNDED'a çıkan oku vardır.
	StatusCompleted Status = "COMPLETED"
	// StatusRejected yönetici reddetti. TERMİNAL. Bakiye hiç değişmemiştir.
	StatusRejected Status = "REJECTED"
	// StatusRefunded onaylanmış bir yükleme geri alındı. TERMİNAL.
	//
	// v1 KAPSAMINDA DEĞİLDİR: bu duruma geçiren bir uç nokta yoktur ve ters
	// yönlü defter kaydı (CHARGEBACK) ayrı bir tasarımdır. Geçiş burada ve
	// tetikleyicide TANIMLI bırakıldı ki akış eklendiğinde şema değişmesin.
	StatusRefunded Status = "REFUNDED"
)

// AllStatuses tanımlı tüm durumlar (doğrulama ve test için).
func AllStatuses() []Status {
	return []Status{StatusPending, StatusCompleted, StatusRejected, StatusRefunded}
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
// YALNIZ REJECTED ve REFUNDED terminaldir. COMPLETED bir GEÇİŞTİR: REFUNDED'a
// çıkan oku vardır. COMPLETED'ı terminal saymak, ileride iade akışı
// yazıldığında onu sessizce imkânsız kılardı.
//
// test: deposit_test.go#TestDepositTerminalStatuses
func (s Status) IsTerminal() bool {
	return s == StatusRejected || s == StatusRefunded
}

// transitions izin verilen geçişler (docs/design.md §7.2).
//
// Bu tablo migration 00009'daki deposits_guard_transition() ile BİREBİR aynı
// olmalıdır: ikisi ayrışırsa veritabanı, kodun izin verdiği bir geçişi
// reddeder ve arıza "beklenmeyen hata" olarak görünür.
//
// test: deposit_test.go#TestDepositTransitionTable
var transitions = map[Status][]Status{
	StatusPending:   {StatusCompleted, StatusRejected},
	StatusCompleted: {StatusRefunded},
	// Terminal durumlardan çıkış yok — bilerek boş.
	StatusRejected: {},
	StatusRefunded: {},
}

// ErrInvalidTransition geçersiz durum geçişi.
type ErrInvalidTransition struct {
	From, To Status
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("deposit: geçersiz durum geçişi: %s → %s", e.From, e.To)
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
// test: deposit_test.go#TestDepositTransitionTable
func Transition(from, to Status) error {
	if !from.Valid() {
		return fmt.Errorf("deposit: bilinmeyen kaynak durum %q", from)
	}
	if !to.Valid() {
		return fmt.Errorf("deposit: bilinmeyen hedef durum %q", to)
	}
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}
	return nil
}
