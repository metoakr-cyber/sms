// Package ticket destek talebi durum makinesidir (FR-600).
//
// Durum geçişleri KODDA DAĞILMAZ: `status` alanına yalnız bu paketin
// doğruladığı bir geçişten sonra yazılır (CLAUDE.md değişmez #13). Handler
// içinde doğrudan bir durum ataması, "kullanıcı yazdı ama talep hâlâ
// yanıtlandı görünüyor" gibi sessiz hatalar üretir ve talep yöneticinin
// listesinden düşer.
//
// Aynı kural veritabanında da zorlanır (migration 00012,
// tickets_guard_transition): iki savunma hattı.
//
// test: ticket_test.go#TestTransitionTable
package ticket

import (
	"errors"
	"fmt"
	"strings"
)

// Uzunluk sınırları — TEK KAYNAK burasıdır.
//
// DTO doğrulaması ve veritabanı CHECK'i bu değerleri yansıtır. Sınırsız metin
// hem depolama (tek istekle megabaytlar) hem gösterim (liste ekranını bozan
// tek satır) sorunudur.
const (
	// MinSubjectLen / MaxSubjectLen docs/trd.md §10 doğrulama tablosundan.
	MinSubjectLen = 5
	MaxSubjectLen = 120
	// MinBodyLen / MaxBodyLen — trd.md 5000 diyor, biz 4000'de duruyoruz.
	MinBodyLen = 1
	MaxBodyLen = 4000
)

// Status talep durumu. Dört değer — beşincisi YOKTUR (docs/trd.md FR-600).
type Status string

const (
	// StatusOpen kullanıcı açtı, personel yanıtı bekleniyor. YÖNETİCİNİN İŞİ.
	StatusOpen Status = "OPEN"
	// StatusAnswered personel yanıtladı. KULLANICININ İŞİ.
	StatusAnswered Status = "ANSWERED"
	// StatusUserReplied kullanıcı yeniden yazdı. YÖNETİCİNİN İŞİ.
	StatusUserReplied Status = "USER_REPLIED"
	// StatusClosed kapatıldı.
	//
	// Kullanıcı için TERMİNALDİR: kapalı bir talebe mesaj yazılamaz
	// (bkz. AfterUserMessage). Yönetici yeniden açabilir.
	StatusClosed Status = "CLOSED"
)

// AllStatuses tanımlı tüm durumlar (doğrulama ve test için).
func AllStatuses() []Status {
	return []Status{StatusOpen, StatusAnswered, StatusUserReplied, StatusClosed}
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

// NeedsStaffAttention talep yöneticinin kuyruğunda mı.
//
// Yönetim ekranının varsayılan süzgeci bu ikilidir; "açık" tek bir durum
// değildir ve öyle sanmak USER_REPLIED taleplerini görünmez kılar.
func (s Status) NeedsStaffAttention() bool {
	return s == StatusOpen || s == StatusUserReplied
}

// ParseStatus dış dünyadan gelen metni durum değerine çevirir.
func ParseStatus(raw string) (Status, bool) {
	s := Status(strings.ToUpper(strings.TrimSpace(raw)))
	return s, s.Valid()
}

// Priority talep önceliği.
type Priority string

const (
	PriorityLow    Priority = "LOW"
	PriorityNormal Priority = "NORMAL"
	PriorityHigh   Priority = "HIGH"
)

// AllPriorities tanımlı tüm öncelikler.
func AllPriorities() []Priority {
	return []Priority{PriorityLow, PriorityNormal, PriorityHigh}
}

func (p Priority) Valid() bool {
	for _, x := range AllPriorities() {
		if p == x {
			return true
		}
	}
	return false
}

// ParsePriority boş girdide NORMAL döner: öncelik seçmeyen kullanıcı hata
// almamalı, makul bir varsayılan almalı.
func ParsePriority(raw string) (Priority, bool) {
	t := strings.TrimSpace(raw)
	if t == "" {
		return PriorityNormal, true
	}
	p := Priority(strings.ToUpper(t))
	return p, p.Valid()
}

// transitions izin verilen geçişler (docs/trd.md FR-600).
//
// OPEN → ANSWERED → USER_REPLIED → CLOSED zincirine EK olarak yalnız iki ok
// vardır: her durumdan CLOSED'a ve CLOSED'dan OPEN'a (yeniden açma).
var transitions = map[Status][]Status{
	StatusOpen:        {StatusAnswered, StatusClosed},
	StatusAnswered:    {StatusUserReplied, StatusClosed},
	StatusUserReplied: {StatusAnswered, StatusClosed},
	// 🔴 CLOSED'dan çıkan TEK ok OPEN'dır ve onu yalnız yönetici kullanır.
	StatusClosed: {StatusOpen},
}

// ErrInvalidTransition geçersiz durum geçişi.
type ErrInvalidTransition struct {
	From, To Status
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("ticket: geçersiz durum geçişi: %s → %s", e.From, e.To)
}

// ErrClosed kapalı talebe yazma denemesi.
var ErrClosed = errors.New("ticket: talep kapalı")

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
// test: ticket_test.go#TestTransitionTable
func Transition(from, to Status) error {
	if !from.Valid() {
		return fmt.Errorf("ticket: bilinmeyen kaynak durum %q", from)
	}
	if !to.Valid() {
		return fmt.Errorf("ticket: bilinmeyen hedef durum %q", to)
	}
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}
	return nil
}

// AfterUserMessage kullanıcı mesaj yazdığında talebin yeni durumu.
//
// ── KAPALI TALEP KULLANICI TARAFINDAN YENİDEN AÇILMAZ ──
//
// Karar ve gerekçesi:
//   - "Kapalı" bir yöneticinin verdiği karardır. Kullanıcının tek bir
//     mesajla o kararı geri alabilmesi, kapatma işlemini anlamsız kılar:
//     kuyruk sayısı hiçbir zaman düşmez ve altı ay önce kapanmış bir talep
//     bugün en üste çıkar.
//   - Alternatif (sessizce yeniden aç) daha da kötüdür: yönetici kapattığını
//     sanır, talep kuyruğa geri döner ve kimse fark etmez.
//   - Kullanıcı kaybolmaz: uç 409 ve Türkçe bir mesajla "yeni talep açın"
//     der; eski yazışma okunur hâlde kalır. Yöneticinin yeniden açma yolu
//     ise vardır (AdminReopen) — erken kapatılmış bir talep için kaçış
//     kapısı personelin elindedir.
//
// test: ticket_test.go#TestClosedTicketRejectsUserMessage
func AfterUserMessage(current Status) (Status, error) {
	switch current {
	case StatusClosed:
		return "", ErrClosed
	case StatusAnswered:
		// Yanıtlandı → kullanıcı yazdı: top yeniden yöneticide.
		return StatusUserReplied, nil
	case StatusOpen, StatusUserReplied:
		// Zaten yöneticinin kuyruğunda; durum değişmez ama last_reply_at güncellenir.
		return current, nil
	default:
		return "", fmt.Errorf("ticket: bilinmeyen durum %q", current)
	}
}

// AfterStaffMessage personel yanıt yazdığında talebin yeni durumu.
//
// Kapalı talebe personel de doğrudan yazamaz: önce yeniden açmalıdır. Böylece
// "kapalı ama içinde yeni mesaj var" gibi okunamaz bir durum oluşmaz.
//
// test: ticket_test.go#TestClosedTicketRejectsStaffMessage
func AfterStaffMessage(current Status) (Status, error) {
	switch current {
	case StatusClosed:
		return "", ErrClosed
	case StatusOpen, StatusUserReplied, StatusAnswered:
		return StatusAnswered, nil
	default:
		return "", fmt.Errorf("ticket: bilinmeyen durum %q", current)
	}
}

// AdminSettableStatuses yöneticinin PATCH ile YAZABİLECEĞİ durumlar.
//
// ANSWERED ve USER_REPLIED bu listede YOKTUR ve olmayacaktır: ikisi de bir
// MESAJIN sonucudur. Elle "yanıtlandı" yazabilmek, hiç yanıt gelmemiş bir
// talebi yanıtlanmış göstermek demektir.
//
// test: ticket_test.go#TestAdminSettableStatuses
func AdminSettableStatuses() []Status {
	return []Status{StatusOpen, StatusClosed}
}

// AdminCanSet yönetici bu durumu elle yazabilir mi.
func AdminCanSet(target Status) bool {
	for _, s := range AdminSettableStatuses() {
		if s == target {
			return true
		}
	}
	return false
}
