package review_test

// Yorum durum makinesinin saf birim testleri. Bağımlılık yok, G/Ç yok.

import (
	"errors"
	"strings"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/domain/review"
)

// TestTransitionTable izin verilen ve verilmeyen HER geçişi tek tek sınar.
//
// Tablo eksiksizdir: 3 durum × 3 hedef = 9 hücre. "İzinliler doğru mu"
// testleri, izinsizlerin de gerçekten reddedildiğini göstermez.
func TestTransitionTable(t *testing.T) {
	const (
		P = review.StatusPending
		A = review.StatusApproved
		R = review.StatusRejected
	)
	cases := []struct {
		from, to review.Status
		ok       bool
	}{
		{P, P, true}, // aynı duruma yazım serbest
		{P, A, true},
		{P, R, true},

		{A, A, true},
		{A, R, true}, // YAYINDAN KALDIRMA — bilerek var
		// 🔴 Onaylı yorum tekrar kuyruğa DÖNMEZ: yönetici kararını
		// geri almak isterse yayından kaldırır.
		{A, P, false},

		{R, R, true},
		// 🔴 Reddedilenden çıkış yok.
		{R, P, false},
		{R, A, false},
	}
	for _, c := range cases {
		err := review.Transition(c.from, c.to)
		if c.ok && err != nil {
			t.Errorf("%s → %s izinli olmalıydı: %v", c.from, c.to, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s → %s REDDEDİLMELİYDİ ama geçti", c.from, c.to)
		}
	}
}

func TestUnknownStatusIsRejected(t *testing.T) {
	if err := review.Transition("SILINDI", review.StatusApproved); err == nil {
		t.Fatal("bilinmeyen kaynak durum kabul edildi")
	}
	if err := review.Transition(review.StatusPending, "YAYINDA"); err == nil {
		t.Fatal("bilinmeyen hedef durum kabul edildi")
	}
	if _, ok := review.ParseStatus("yayinda"); ok {
		t.Fatal("ParseStatus bilinmeyen metni kabul etti")
	}
	if s, ok := review.ParseStatus(" approved "); !ok || s != review.StatusApproved {
		t.Fatalf("ParseStatus normalize etmedi: %q %v", s, ok)
	}
}

// TestOnlyApprovedIsPublic sitede gösterilecek tek durumun APPROVED olduğunu
// sabitler.
//
// Bu test ucuz görünür ama tam olarak kaçırılması en pahalı hatayı korur:
// reddedilmiş bir yorumun sitede görünmesi.
func TestOnlyApprovedIsPublic(t *testing.T) {
	for _, s := range review.AllStatuses() {
		want := s == review.StatusApproved
		if s.IsPublic() != want {
			t.Errorf("%s.IsPublic() = %v, beklenen %v", s, s.IsPublic(), want)
		}
	}
}

// TestRejectionRequiresReason gerekçesiz reddin domain'de durdurulduğunu
// doğrular.
func TestRejectionRequiresReason(t *testing.T) {
	if _, err := review.Decide(review.StatusPending, review.StatusRejected, "   "); err == nil {
		t.Fatal("boş gerekçeyle red kabul edildi")
	} else if !errors.Is(err, review.ErrReasonRequired) {
		t.Fatalf("beklenen ErrReasonRequired, gelen: %v", err)
	}

	d, err := review.Decide(review.StatusPending, review.StatusRejected, "  Reklam içeriyor.  ")
	if err != nil {
		t.Fatalf("geçerli red düştü: %v", err)
	}
	if d.Status != review.StatusRejected || d.Reason != "Reklam içeriyor." {
		t.Fatalf("karar yanlış: %+v", d)
	}

	// Aşırı uzun gerekçe reddedilir.
	long := strings.Repeat("ş", review.MaxRejectionReasonLen+1)
	if _, err := review.Decide(review.StatusPending, review.StatusRejected, long); err == nil {
		t.Fatal("sınırı aşan gerekçe kabul edildi")
	}
}

// TestApprovalClearsReason onayda gerekçe alanının TEMİZLENDİĞİNİ doğrular.
//
// Veritabanı CHECK'i (review_reason_only_when_rejected) aksini zaten
// reddederdi; bu test hatanın 23514 olarak değil, anlaşılır biçimde
// önlendiğini gösterir.
func TestApprovalClearsReason(t *testing.T) {
	d, err := review.Decide(review.StatusPending, review.StatusApproved, "boşuna yazılmış gerekçe")
	if err != nil {
		t.Fatalf("onay düştü: %v", err)
	}
	if d.Reason != "" {
		t.Fatalf("onayda gerekçe temizlenmedi: %q", d.Reason)
	}
}

// TestDecideRefusesInvalidTransition reddedilmiş bir yorumun onaylanamadığını
// doğrular.
func TestDecideRefusesInvalidTransition(t *testing.T) {
	if _, err := review.Decide(review.StatusRejected, review.StatusApproved, ""); err == nil {
		t.Fatal("REJECTED → APPROVED geçişi kabul edildi")
	}
	var inv review.ErrInvalidTransition
	_, err := review.Decide(review.StatusApproved, "PENDING", "")
	if !errors.As(err, &inv) && err == nil {
		t.Fatal("APPROVED → PENDING geçişi kabul edildi")
	}
}

// TestInvalidTransitionErrorNamesBothStates hata metninin hangi geçişin
// reddedildiğini SÖYLEDİĞİNİ doğrular.
//
// "geçersiz durum geçişi" tek başına yazılsaydı, log'da hatayı görüp neyin
// neye dönüşmeye çalıştığını bilmek imkânsız olurdu.
func TestInvalidTransitionErrorNamesBothStates(t *testing.T) {
	err := review.ErrInvalidTransition{From: review.StatusRejected, To: review.StatusApproved}
	msg := err.Error()
	if !strings.Contains(msg, "REJECTED") || !strings.Contains(msg, "APPROVED") {
		t.Fatalf("hata metni durumları içermiyor: %q", msg)
	}
}

// TestDecideRefusesPendingTarget "yeniden kuyruğa al" gibi bir hedefin
// domain tarafından reddedildiğini doğrular (Decide'ın son dalı).
func TestDecideRefusesPendingTarget(t *testing.T) {
	if _, err := review.Decide(review.StatusPending, review.StatusPending, ""); err == nil {
		t.Fatal("PENDING hedefi kabul edildi — karar bir sonuç yazmalıdır")
	}
}

func TestValidRating(t *testing.T) {
	for _, r := range []int{-1, 0, 6, 100} {
		if review.ValidRating(r) {
			t.Errorf("puan %d geçerli sayıldı", r)
		}
	}
	for r := review.MinRating; r <= review.MaxRating; r++ {
		if !review.ValidRating(r) {
			t.Errorf("puan %d geçersiz sayıldı", r)
		}
	}
}
