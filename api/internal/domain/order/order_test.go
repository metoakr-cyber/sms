package order

import (
	"errors"
	"testing"
	"time"
)

// TestTransitionTable durum makinesinin TAM tablosunu doğrular.
//
// Her (kaynak, hedef) çifti tek tek denenir — "izin verilenler çalışıyor mu"
// yetmez, "izin verilmeyenler GERÇEKTEN reddediliyor mu" da gerekir. Eksik bir
// ret, tamamlanmış bir siparişe iade yazılmasına izin verir.
func TestTransitionTable(t *testing.T) {
	allowed := map[Status]map[Status]bool{
		StatusPending:   {StatusCompleted: true, StatusCancelled: true, StatusFailed: true},
		StatusCancelled: {StatusRefunded: true},
		StatusFailed:    {StatusRefunded: true},
		StatusCompleted: {}, // terminal
		StatusRefunded:  {}, // terminal
	}

	for _, from := range AllStatuses() {
		for _, to := range AllStatuses() {
			err := Transition(from, to)
			want := from == to || allowed[from][to]
			if want && err != nil {
				t.Errorf("%s → %s reddedildi ama izinli olmalı: %v", from, to, err)
			}
			if !want && err == nil {
				t.Errorf("%s → %s KABUL EDİLDİ ama yasak olmalı", from, to)
			}
		}
	}
}

// TestTerminalStatesHaveNoExit
//
// SÖZLEŞME: COMPLETED ve REFUNDED terminaldir; CANCELLED ve FAILED DEĞİLDİR.
// CANCELLED'ı terminal saymak, iade kaydının hiç yazılmaması demektir —
// kullanıcı parasını geri alamaz.
func TestTerminalStatesHaveNoExit(t *testing.T) {
	terminal := map[Status]bool{StatusCompleted: true, StatusRefunded: true}
	for _, s := range AllStatuses() {
		if s.IsTerminal() != terminal[s] {
			t.Errorf("%s.IsTerminal() = %v, beklenen %v", s, s.IsTerminal(), terminal[s])
		}
	}

	for s := range terminal {
		for _, to := range AllStatuses() {
			if to == s {
				continue
			}
			if err := Transition(s, to); err == nil {
				t.Errorf("terminal %s durumundan %s'e geçişe izin verildi", s, to)
			} else {
				var ite ErrInvalidTransition
				if !errors.As(err, &ite) {
					t.Errorf("%s → %s: ErrInvalidTransition bekleniyordu, alınan %T", s, to, err)
				}
			}
		}
	}
}

// TestDecideClose
//
// SÖZLEŞME: kod geldiyse Finish (iade YOK), gelmediyse Cancel (iade TALEP).
// Yanlış tarafa düşmek doğrudan zarardır — ikisi de.
func TestDecideClose(t *testing.T) {
	if got := DecideClose(true); got != CloseFinish {
		t.Errorf("kod teslim edilmiş sipariş için %q, beklenen %q (iade talebi yapılmamalı)",
			got, CloseFinish)
	}
	if got := DecideClose(false); got != CloseCancel {
		t.Errorf("kod gelmemiş sipariş için %q, beklenen %q (iade talep edilmeli)",
			got, CloseCancel)
	}
}

// TestCanUserCancel
func TestCanUserCancel(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	t.Run("beklemede ve süre geçmiş → izinli", func(t *testing.T) {
		c := CanUserCancel(StatusPending, now.Add(-time.Second), now)
		if !c.Allowed {
			t.Fatalf("iptal reddedildi: %s", c.Reason)
		}
	})

	t.Run("beklemede ama erken → kalan süre bildirilir", func(t *testing.T) {
		c := CanUserCancel(StatusPending, now.Add(90*time.Second), now)
		if c.Allowed {
			t.Fatal("erken iptal kabul edildi — sağlayıcı reddeder ve kullanıcı 'çalışmıyor' der")
		}
		if c.RetryAfter != 90*time.Second {
			t.Errorf("kalan süre = %v, beklenen 90s", c.RetryAfter)
		}
	})

	t.Run("tam sınırda → izinli", func(t *testing.T) {
		if c := CanUserCancel(StatusPending, now, now); !c.Allowed {
			t.Error("cancellableAt anında iptal reddedildi")
		}
	})

	t.Run("terminal veya iptal edilmiş → reddedilir", func(t *testing.T) {
		for _, s := range []Status{StatusCompleted, StatusRefunded, StatusCancelled, StatusFailed} {
			if c := CanUserCancel(s, now.Add(-time.Hour), now); c.Allowed {
				t.Errorf("%s durumundaki sipariş iptal edilebilir sayıldı", s)
			}
		}
	})
}

// TestIsExpired sunucu saatiyle karar verildiğini gösterir.
func TestIsExpired(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	if IsExpired(now.Add(time.Second), now) {
		t.Error("henüz dolmamış sipariş dolmuş sayıldı")
	}
	if !IsExpired(now, now) {
		t.Error("tam sınırda dolmuş sayılmalı")
	}
	if !IsExpired(now.Add(-time.Second), now) {
		t.Error("dolmuş sipariş dolmamış sayıldı")
	}
}

// TestUnknownStatusIsRejected
//
// Veritabanından beklenmedik bir değer okunursa (migration hatası, elle
// müdahale) durum makinesi onu SESSİZCE kabul etmemeli.
func TestUnknownStatusIsRejected(t *testing.T) {
	if Status("SOMETHING_ELSE").Valid() {
		t.Error("bilinmeyen durum geçerli sayıldı")
	}
	if err := Transition(Status("REFUND_PENDING"), StatusRefunded); err == nil {
		t.Error("bilinmeyen kaynak durumdan geçişe izin verildi")
	}
	if err := Transition(StatusPending, Status("ARCHIVED")); err == nil {
		t.Error("bilinmeyen hedef duruma geçişe izin verildi")
	}
}
