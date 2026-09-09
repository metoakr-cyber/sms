package ticket

import (
	"errors"
	"testing"
)

// TestTransitionTable durum makinesinin TAM tablosunu doğrular.
//
// Her (kaynak, hedef) çifti tek tek denenir — "izin verilenler çalışıyor mu"
// yetmez, "izin verilmeyenler GERÇEKTEN reddediliyor mu" da gerekir. Eksik bir
// ret, kapalı bir talebin kullanıcı tarafından ANSWERED'a çevrilmesine izin
// verirdi.
func TestTransitionTable(t *testing.T) {
	allowed := map[Status]map[Status]bool{
		StatusOpen:        {StatusAnswered: true, StatusClosed: true},
		StatusAnswered:    {StatusUserReplied: true, StatusClosed: true},
		StatusUserReplied: {StatusAnswered: true, StatusClosed: true},
		StatusClosed:      {StatusOpen: true},
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

func TestTransitionRejectsUnknownStatus(t *testing.T) {
	if err := Transition(Status("YOK"), StatusOpen); err == nil {
		t.Error("bilinmeyen kaynak durum kabul edildi")
	}
	if err := Transition(StatusOpen, Status("YOK")); err == nil {
		t.Error("bilinmeyen hedef durum kabul edildi")
	}
}

// TestAfterUserMessage kullanıcı mesajının durum etkisi.
//
// SÖZLEŞME: ANSWERED → USER_REPLIED. Bu ok olmadan, kullanıcı yanıt yazdığında
// talep "yanıtlandı" kalır ve yöneticinin bekleyen listesinde HİÇ görünmez —
// yani mesaj sessizce kaybolur.
func TestAfterUserMessage(t *testing.T) {
	cases := []struct {
		from Status
		want Status
	}{
		{StatusOpen, StatusOpen},
		{StatusAnswered, StatusUserReplied},
		{StatusUserReplied, StatusUserReplied},
	}
	for _, c := range cases {
		got, err := AfterUserMessage(c.from)
		if err != nil {
			t.Fatalf("%s: beklenmeyen hata: %v", c.from, err)
		}
		if got != c.want {
			t.Errorf("%s sonrası %s bekleniyordu, %s geldi", c.from, c.want, got)
		}
		// Hesaplanan hedef durum makinesinde de geçerli olmalı; aksi hâlde
		// veritabanı tetikleyicisi yazmayı reddeder ve kullanıcı 500 görür.
		if err := Transition(c.from, got); err != nil {
			t.Errorf("%s → %s durum makinesinde geçersiz: %v", c.from, got, err)
		}
	}
}

// TestClosedTicketRejectsUserMessage kapalı talep kullanıcı tarafından
// yeniden açılamaz (karar gerekçesi ticket.go#AfterUserMessage).
func TestClosedTicketRejectsUserMessage(t *testing.T) {
	if _, err := AfterUserMessage(StatusClosed); !errors.Is(err, ErrClosed) {
		t.Fatalf("kapalı talebe kullanıcı mesajı kabul edildi: %v", err)
	}
}

func TestAfterStaffMessage(t *testing.T) {
	for _, from := range []Status{StatusOpen, StatusUserReplied, StatusAnswered} {
		got, err := AfterStaffMessage(from)
		if err != nil {
			t.Fatalf("%s: beklenmeyen hata: %v", from, err)
		}
		if got != StatusAnswered {
			t.Errorf("%s sonrası ANSWERED bekleniyordu, %s geldi", from, got)
		}
		if err := Transition(from, got); err != nil {
			t.Errorf("%s → %s durum makinesinde geçersiz: %v", from, got, err)
		}
	}
}

// TestClosedTicketRejectsStaffMessage personel de kapalı talebe doğrudan
// yazamaz; önce yeniden açar.
func TestClosedTicketRejectsStaffMessage(t *testing.T) {
	if _, err := AfterStaffMessage(StatusClosed); !errors.Is(err, ErrClosed) {
		t.Fatalf("kapalı talebe personel mesajı kabul edildi: %v", err)
	}
}

// TestAdminSettableStatuses yönetici ANSWERED/USER_REPLIED yazamaz.
//
// Bu iki durum bir MESAJIN sonucudur. Elle yazılabilseydi, hiç yanıt
// gelmemiş bir talep "yanıtlandı" görünür ve kullanıcı boş bir yazışmaya
// bakarak beklerdi.
func TestAdminSettableStatuses(t *testing.T) {
	want := map[Status]bool{StatusOpen: true, StatusClosed: true}
	for _, s := range AllStatuses() {
		if AdminCanSet(s) != want[s] {
			t.Errorf("%s: AdminCanSet=%v, beklenen %v", s, AdminCanSet(s), want[s])
		}
	}
}

func TestNeedsStaffAttention(t *testing.T) {
	want := map[Status]bool{
		StatusOpen: true, StatusUserReplied: true,
		StatusAnswered: false, StatusClosed: false,
	}
	for _, s := range AllStatuses() {
		if s.NeedsStaffAttention() != want[s] {
			t.Errorf("%s: NeedsStaffAttention=%v, beklenen %v",
				s, s.NeedsStaffAttention(), want[s])
		}
	}
}

func TestParseStatus(t *testing.T) {
	if s, ok := ParseStatus(" open "); !ok || s != StatusOpen {
		t.Errorf("boşluklu/küçük harf durum ayrıştırılamadı: %q %v", s, ok)
	}
	if _, ok := ParseStatus("SILINDI"); ok {
		t.Error("bilinmeyen durum kabul edildi")
	}
	if _, ok := ParseStatus(""); ok {
		t.Error("boş durum kabul edildi")
	}
}

// TestParsePriorityDefaultsToNormal boş öncelik hata değil, varsayılandır.
func TestParsePriorityDefaultsToNormal(t *testing.T) {
	if p, ok := ParsePriority(""); !ok || p != PriorityNormal {
		t.Errorf("boş öncelik NORMAL olmalıydı: %q %v", p, ok)
	}
	if p, ok := ParsePriority("high"); !ok || p != PriorityHigh {
		t.Errorf("küçük harf öncelik ayrıştırılamadı: %q %v", p, ok)
	}
	if _, ok := ParsePriority("ACIL"); ok {
		t.Error("bilinmeyen öncelik kabul edildi")
	}
}
