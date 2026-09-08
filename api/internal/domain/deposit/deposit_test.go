package deposit

import (
	"errors"
	"testing"
)

// TestDepositTransitionTable 4×4 matrisin TAMAMINI tek tek doğrular.
//
// Tek tek yazılıyor çünkü "izinli olanları döngüyle kontrol et" biçiminde
// yazılan bir test, tabloya fazladan bir geçiş EKLENDİĞİNDE sessizce geçer.
func TestDepositTransitionTable(t *testing.T) {
	allowed := map[Status]map[Status]bool{
		StatusPending:   {StatusPending: true, StatusCompleted: true, StatusRejected: true, StatusRefunded: false},
		StatusCompleted: {StatusPending: false, StatusCompleted: true, StatusRejected: false, StatusRefunded: true},
		StatusRejected:  {StatusPending: false, StatusCompleted: false, StatusRejected: true, StatusRefunded: false},
		StatusRefunded:  {StatusPending: false, StatusCompleted: false, StatusRejected: false, StatusRefunded: true},
	}

	for _, from := range AllStatuses() {
		for _, to := range AllStatuses() {
			want := allowed[from][to]
			if got := CanTransition(from, to); got != want {
				t.Errorf("🔴 CanTransition(%s, %s) = %v, %v bekleniyordu", from, to, got, want)
			}

			err := Transition(from, to)
			if want && err != nil {
				t.Errorf("🔴 Transition(%s, %s) hata verdi: %v", from, to, err)
			}
			if !want {
				var invalid ErrInvalidTransition
				if !errors.As(err, &invalid) {
					t.Errorf("🔴 Transition(%s, %s) ErrInvalidTransition dönmedi: %v", from, to, err)
				}
			}
		}
	}
}

// TestDepositTerminalStatuses COMPLETED'ın terminal SAYILMADIĞINI doğrular.
//
// COMPLETED terminal sayılsaydı ileride yazılacak iade (REFUNDED) akışı
// sessizce imkânsız olurdu ve sebebi bu fonksiyonda aranmazdı.
func TestDepositTerminalStatuses(t *testing.T) {
	want := map[Status]bool{
		StatusPending:   false,
		StatusCompleted: false,
		StatusRejected:  true,
		StatusRefunded:  true,
	}
	for s, w := range want {
		if got := s.IsTerminal(); got != w {
			t.Errorf("🔴 %s.IsTerminal() = %v, %v bekleniyordu", s, got, w)
		}
	}
}

// TestDepositUnknownStatusRejected bilinmeyen bir durum dizesi geçiş
// tablosunda "boş liste" bulup sessizce reddedilmemeli; açık hata dönmeli.
func TestDepositUnknownStatusRejected(t *testing.T) {
	if err := Transition("WAT", StatusCompleted); err == nil {
		t.Fatal("🔴 bilinmeyen kaynak durum kabul edildi")
	}
	if err := Transition(StatusPending, "WAT"); err == nil {
		t.Fatal("🔴 bilinmeyen hedef durum kabul edildi")
	}
	if Status("WAT").Valid() {
		t.Fatal("🔴 bilinmeyen durum Valid() dedi")
	}
}
