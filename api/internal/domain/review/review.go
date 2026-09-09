// Package review müşteri yorumu durum makinesidir.
//
// Durum geçişleri KODDA DAĞILMAZ: `status` alanına yalnız bu paketin
// doğruladığı bir geçişten sonra yazılır (CLAUDE.md değişmez #13). Handler
// içinde doğrudan bir durum ataması, "reddedildi ama sitede duruyor" gibi
// sessiz hatalar üretir — ve bu hata SİTEDE görünür.
//
// Aynı kural veritabanında da zorlanır (migration 00013,
// reviews_guard_transition): iki savunma hattı.
//
// test: review_test.go#TestTransitionTable
package review

import (
	"errors"
	"fmt"
	"strings"
)

// Uzunluk ve puan sınırları — TEK KAYNAK burasıdır.
//
// DTO doğrulaması ve veritabanı CHECK'i bu değerleri yansıtır.
const (
	// MinBodyLen 10: "iyi" ya da "👍" bir yorum değildir; moderasyon kuyruğunu
	// doldurur ve sitede hiçbir şey anlatmaz.
	MinBodyLen = 10
	// MaxBodyLen 1000: kart düzeninde okunabilir kalan üst sınır.
	MaxBodyLen = 1000

	MinRating = 1
	MaxRating = 5

	// MaxRejectionReasonLen red gerekçesi kullanıcıya gösterilir.
	MaxRejectionReasonLen = 500
)

// Status yorum durumu. Üç değer — dördüncüsü YOKTUR.
type Status string

const (
	// StatusPending yazıldı, moderasyon bekliyor. YÖNETİCİNİN İŞİ.
	StatusPending Status = "PENDING"
	// StatusApproved yayında. Sitede YALNIZ bu durumdakiler görünür.
	StatusApproved Status = "APPROVED"
	// StatusRejected yayımlanmadı ya da yayından kaldırıldı. TERMİNALDİR.
	StatusRejected Status = "REJECTED"
)

// AllStatuses tanımlı tüm durumlar (doğrulama ve test için).
func AllStatuses() []Status {
	return []Status{StatusPending, StatusApproved, StatusRejected}
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

// IsPublic yorum sitede gösterilir mi.
//
// Tek doğru "APPROVED" karşılaştırması burasıdır: koda dağılmış
// `status == "APPROVED"` karşılaştırmaları, yeni bir durum eklendiğinde
// birinin unutulmasına ve reddedilmiş bir yorumun sitede kalmasına yol açar.
func (s Status) IsPublic() bool { return s == StatusApproved }

// ParseStatus dış dünyadan gelen metni durum değerine çevirir.
func ParseStatus(raw string) (Status, bool) {
	s := Status(strings.ToUpper(strings.TrimSpace(raw)))
	return s, s.Valid()
}

// transitions izin verilen geçişler.
//
// 🔴 APPROVED → REJECTED oku BİLEREK VARDIR: yanlışlıkla onaylanmış ya da
// sonradan hakaret içerdiği anlaşılan bir yorumu siteden indirmenin başka
// yolu olmazdı. Gerekçe o durumda da zorunludur.
//
// 🔴 REJECTED'DAN ÇIKIŞ YOKTUR: reddedilen bir metni geri getirmek, kullanıcının
// artık savunmadığı bir cümleyi yayımlamak demektir. Kullanıcı isterse yeni
// bir yorum yazar.
var transitions = map[Status][]Status{
	StatusPending:  {StatusApproved, StatusRejected},
	StatusApproved: {StatusRejected},
	StatusRejected: {},
}

// ErrInvalidTransition geçersiz durum geçişi.
type ErrInvalidTransition struct {
	From, To Status
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("review: geçersiz durum geçişi: %s → %s", e.From, e.To)
}

// ErrReasonRequired gerekçesiz red denemesi.
var ErrReasonRequired = errors.New("review: red gerekçesi zorunludur")

// CanTransition geçiş izinli mi. Aynı duruma yeniden yazım serbesttir.
func CanTransition(from, to Status) bool {
	if from == to {
		return true
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
// test: review_test.go#TestTransitionTable
func Transition(from, to Status) error {
	if !from.Valid() {
		return fmt.Errorf("review: bilinmeyen kaynak durum %q", from)
	}
	if !to.Valid() {
		return fmt.Errorf("review: bilinmeyen hedef durum %q", to)
	}
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}
	return nil
}

// Decision yöneticinin verdiği karar ve yazılacak alanlar.
type Decision struct {
	Status Status
	// Reason YALNIZ redde doludur. Onayda boş olmak ZORUNDADIR —
	// review_reason_only_when_rejected CHECK'i aksini reddeder.
	// test: review_test.go#TestApprovalClearsReason
	Reason string
}

// Decide bir karar isteğini doğrular ve yazılacak alanları üretir.
//
// Gerekçe zorunluluğu BURADA yaşar, handler'da değil: "reddedildi" deyip
// sebebini söylememek kullanıcıyı aynı yorumu tekrar yazmaya iter ve kuyruğu
// kendi kendine besler.
//
// test: review_test.go#TestRejectionRequiresReason
func Decide(current, target Status, reason string) (Decision, error) {
	if err := Transition(current, target); err != nil {
		return Decision{}, err
	}
	switch target {
	case StatusApproved:
		// Onayda gerekçe alanı TEMİZLENİR: "onaylandı ama gerekçesi 'küfür
		// içeriyor'" gibi kendi kendisiyle çelişen bir satır oluşamaz.
		// review_reason_only_when_rejected CHECK'i aksini reddeder.
		// test: review_test.go#TestApprovalClearsReason
		return Decision{Status: StatusApproved, Reason: ""}, nil
	case StatusRejected:
		r := strings.TrimSpace(reason)
		if r == "" {
			return Decision{}, ErrReasonRequired
		}
		if len([]rune(r)) > MaxRejectionReasonLen {
			return Decision{}, fmt.Errorf("review: red gerekçesi en fazla %d karakter olabilir",
				MaxRejectionReasonLen)
		}
		return Decision{Status: StatusRejected, Reason: r}, nil
	default:
		// PENDING'e geri dönüş transitions tablosunda yok; buraya düşmek
		// programlama hatasıdır.
		return Decision{}, ErrInvalidTransition{From: current, To: target}
	}
}

// ValidRating puan aralıkta mı.
func ValidRating(r int) bool { return r >= MinRating && r <= MaxRating }
