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
		StatusPending: {
			StatusActive: true, StatusCompleted: true,
			StatusCancelled: true, StatusFailed: true,
		},
		// ACTIVE'in TEK çıkışı COMPLETED'dır: kiralığa ilk SMS geldikten sonra
		// iade yolu kapalıdır ve ACTIVE → CANCELLED oku bilerek yoktur.
		StatusActive:    {StatusCompleted: true},
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
		c := CanUserCancel(CancelInput{
			Status: StatusPending, CancellableAt: now.Add(-time.Second), Now: now,
		})
		if !c.Allowed {
			t.Fatalf("iptal reddedildi: %s", c.Reason)
		}
	})

	t.Run("beklemede ama erken → kalan süre bildirilir", func(t *testing.T) {
		c := CanUserCancel(CancelInput{
			Status: StatusPending, CancellableAt: now.Add(90 * time.Second), Now: now,
		})
		if c.Allowed {
			t.Fatal("erken iptal kabul edildi — sağlayıcı reddeder ve kullanıcı 'çalışmıyor' der")
		}
		if c.RetryAfter != 90*time.Second {
			t.Errorf("kalan süre = %v, beklenen 90s", c.RetryAfter)
		}
	})

	t.Run("tam sınırda → izinli", func(t *testing.T) {
		c := CanUserCancel(CancelInput{Status: StatusPending, CancellableAt: now, Now: now})
		if !c.Allowed {
			t.Error("cancellableAt anında iptal reddedildi")
		}
	})

	t.Run("terminal veya iptal edilmiş → reddedilir", func(t *testing.T) {
		for _, s := range []Status{StatusActive, StatusCompleted, StatusRefunded, StatusCancelled, StatusFailed} {
			c := CanUserCancel(CancelInput{
				Status: s, CancellableAt: now.Add(-time.Hour), Now: now,
			})
			if c.Allowed {
				t.Errorf("%s durumundaki sipariş iptal edilebilir sayıldı", s)
			}
		}
	})

	// İADE PENCERESİNİN ÜST SINIRI — kiralığın tek para koruması.
	//
	// Sağlayıcı ~20 dakika sonra iptali kalıcı reddediyor; o andan sonra
	// yazacağımız iade sağlayıcıdan geri gelmez ve tamamı bizim giderimizdir.
	t.Run("iade penceresi kapandıysa → reddedilir", func(t *testing.T) {
		until := now.Add(-time.Second)
		c := CanUserCancel(CancelInput{
			Status: StatusPending, CancellableAt: now.Add(-time.Hour),
			RefundableUntil: &until, Now: now,
		})
		if c.Allowed {
			t.Fatal("pencere kapalıyken iptal kabul edildi — iadenin tamamı bizim giderimiz olurdu")
		}
		if c.RetryAfter != 0 {
			t.Errorf("kapanmış pencere için yeniden deneme süresi verildi: %v", c.RetryAfter)
		}
	})

	t.Run("iade penceresi açıksa → izinli", func(t *testing.T) {
		until := now.Add(time.Second)
		c := CanUserCancel(CancelInput{
			Status: StatusPending, CancellableAt: now.Add(-time.Hour),
			RefundableUntil: &until, Now: now,
		})
		if !c.Allowed {
			t.Fatalf("pencere açıkken iptal reddedildi: %s", c.Reason)
		}
	})

	t.Run("aktivasyonda üst sınır yok (nil) → davranış değişmez", func(t *testing.T) {
		c := CanUserCancel(CancelInput{
			Status: StatusPending, CancellableAt: now.Add(-time.Hour),
			RefundableUntil: nil, Now: now.Add(365 * 24 * time.Hour),
		})
		if !c.Allowed {
			t.Fatalf("nil pencere bir üst sınır gibi davrandı: %s", c.Reason)
		}
	})

	// Mesajı olan sipariş iade edilemez: kullanıcıya hem kodu hem parayı
	// vermek olurdu ve sağlayıcı o iadeyi OTP_RECEIVED ile geri çevirir.
	t.Run("teslim edilmiş mesaj varsa → reddedilir", func(t *testing.T) {
		c := CanUserCancel(CancelInput{
			Status: StatusPending, CancellableAt: now.Add(-time.Hour),
			HasMessage: true, Now: now,
		})
		if c.Allowed {
			t.Fatal("mesajı olan sipariş iade için iptal edilebilir sayıldı")
		}
	})

	// 🔴 F1 REGRESYONU — EKSİK VERİ GUARD'I AÇMAZ.
	//
	// `RefundableUntil` nil iken üst sınır kontrolü tümüyle atlanıyordu
	// (fail-open). Aktivasyonda doğru; kiralıkta 30 gün kullanılmış bir
	// numaranın TAM İADESİ demek. Ölçülmüş sonucu: bakiye 55000 → 100000.
	t.Run("kiralıkta pencere BİLİNMİYORSA → reddedilir", func(t *testing.T) {
		c := CanUserCancel(CancelInput{
			Status: StatusPending, IsRental: true,
			CancellableAt:   now.Add(-29 * 24 * time.Hour),
			RefundableUntil: nil,
			Now:             now,
		})
		if c.Allowed {
			t.Fatal("🔴 kiralıkta iade penceresi NULL iken iptal KABUL EDİLDİ — " +
				"29 gün kullanılmış numara tam iade alırdı")
		}
		if c.RetryAfter != 0 {
			t.Errorf("eksik veri için yeniden deneme süresi verildi: %v — bu "+
				"kalıcı bir rettir, geçici değil", c.RetryAfter)
		}
	})

	t.Run("kiralıkta pencere doluysa → normal kurallar işler", func(t *testing.T) {
		until := now.Add(time.Minute)
		c := CanUserCancel(CancelInput{
			Status: StatusPending, IsRental: true,
			CancellableAt: now.Add(-time.Hour), RefundableUntil: &until, Now: now,
		})
		if !c.Allowed {
			t.Fatalf("pencere açık bir kiralık iptal edilemedi: %s", c.Reason)
		}
	})

	// Aynı nil, aktivasyonda hâlâ "üst sınır yok" demektir: F1'in düzeltmesi
	// aktivasyon davranışını DEĞİŞTİRMEZ.
	t.Run("aktivasyonda nil pencere → hâlâ izinli", func(t *testing.T) {
		c := CanUserCancel(CancelInput{
			Status: StatusPending, IsRental: false,
			CancellableAt: now.Add(-time.Hour), RefundableUntil: nil, Now: now,
		})
		if !c.Allowed {
			t.Fatalf("aktivasyon davranışı değişti: %s", c.Reason)
		}
	})
}

// TestDecideRentalClose
//
// SÖZLEŞME: kiralıkta dönem sonu Finish'tir — mesaj gelmiş olsun ya da olmasın.
// Cancel yalnız ücretsiz iptal penceresi HÂLÂ AÇIKKEN ve hiç mesaj yokken
// doğrudur; sonrasında sağlayıcı FREE_CANCELLATION_EXPIRED ile reddeder ve
// gerçekte iade edilemez bir kayıt "gider" metriğini kirletir.
func TestDecideRentalClose(t *testing.T) {
	cases := []struct {
		name         string
		hasMessage   bool
		withinWindow bool
		want         CloseAction
	}{
		{"dönem sonu, mesaj geldi", true, false, CloseFinish},
		{"dönem sonu, hiç mesaj yok", false, false, CloseFinish},
		{"pencere açık, hiç mesaj yok → erken vazgeçme", false, true, CloseCancel},
		{"pencere açık ama mesaj gelmiş", true, true, CloseFinish},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DecideRentalClose(c.hasMessage, c.withinWindow); got != c.want {
				t.Errorf("DecideRentalClose(%v, %v) = %q, beklenen %q",
					c.hasMessage, c.withinWindow, got, c.want)
			}
		})
	}

	// Aktivasyon sözleşmesi DEĞİŞMEDİ: aynı girdilerde DecideClose hâlâ
	// yalnız "mesaj var mı"ya bakıyor.
	if DecideClose(false) != CloseCancel {
		t.Error("DecideClose aktivasyon davranışı değişmiş")
	}
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
