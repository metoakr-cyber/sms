package fake_test

// Sahte sağlayıcının KİRALIK davranışı.
//
// NEDEN AYRI BİR DOSYA: taklit bu turdan önce kiralık konusunda YALAN
// SÖYLÜYORDU. `Capabilities()` KindSMSRental bildiriyor ama `RentalProvider`
// uygulanmıyordu; `Purchase` `DurationHours`u tümüyle yok sayıp 20 dakikalık
// bir aktivasyon üretiyordu; `deliverDue` en fazla BİR mesaj düşürüp siparişi
// hemen tamamlanmış sayıyordu.
//
// Sonucu şuydu: "kiralık sipariş ilk SMS'te sağlayıcıda kapatılıyor" hatası
// hiçbir testle YAKALANAMIYORDU — ikinci mesaj hiç oluşmadığı için kaybı
// ölçecek bir gözlem yoktu. Buradaki testler o gözlemi geri getiriyor.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// rentalService/rentalCountry tohum kataloğunda kiralık desteği OLAN bir
// kombinasyon (fakeCountries["RU"].rent = true).
const (
	rentalService = "tg"
	rentalCountry = "RU"
	rentalHours   = 720 // 30 gün
)

func buyRental(t *testing.T, p *fake.Provider, hours int) *port.PurchaseResult {
	t.Helper()
	res, err := p.Purchase(context.Background(), port.Creds{}, port.PurchaseCmd{
		ServiceCode: rentalService, CountryCode: rentalCountry,
		VerificationType: port.VerifySMS,
		DurationHours:    hours,
		MaxCost:          money.New(1_000_000_000, money.USD),
	})
	if err != nil {
		t.Fatalf("kiralık satın alınamadı: %v", err)
	}
	return res
}

// TestFakePurchaseHonoursRentalDuration
//
// TTL SAĞLAYICIDAN gelir (değişmez #22). Taklit `DurationHours`u yok sayıp
// OrderTTL (20 dk) döndürdüğünde, "30 günlük kiralık" testte 20 dakikada
// doluyordu ve süre sonu davranışı hiç sınanmıyordu.
func TestFakePurchaseHonoursRentalDuration(t *testing.T) {
	clk := newClock()
	p := fake.New(clk)

	res := buyRental(t, p, rentalHours)

	if res.Subtype != port.KindSMSRental {
		t.Errorf("subtype = %q, beklenen %q", res.Subtype, port.KindSMSRental)
	}
	want := clk.Now().Add(rentalHours * time.Hour)
	if !res.ExpiresAt.Equal(want) {
		t.Errorf("expiresAt = %v, beklenen %v (süre sağlayıcıdan gelmeli)",
			res.ExpiresAt, want)
	}
}

// TestFakeRejectsUnsupportedRentalDuration
//
// Sağlayıcı sabit bir süre kümesi kabul ediyor (BAD_DURATION). Taklidin daha
// cömert olması, kabul edilmeyen bir süreyle satın almanın yalnız canlıda
// görülmesi demektir.
func TestFakeRejectsUnsupportedRentalDuration(t *testing.T) {
	p := fake.New(newClock())
	_, err := p.Purchase(context.Background(), port.Creds{}, port.PurchaseCmd{
		ServiceCode: rentalService, CountryCode: rentalCountry,
		DurationHours: 13, // enum'da yok
		MaxCost:       money.New(1_000_000_000, money.USD),
	})
	if !errors.Is(err, port.ErrUnsupported) {
		t.Fatalf("hata = %v, ErrUnsupported bekleniyordu", err)
	}
}

// TestFakeRentalStaysOpenAfterFirstMessage
//
// KİRALIKTA İLK MESAJ SİPARİŞİ BİTİRMEZ. Aktivasyonda ürün o tek koddur;
// kiralıkta ürün SÜREdir ve numara dönem boyunca yeni mesaj almalıdır.
func TestFakeRentalStaysOpenAfterFirstMessage(t *testing.T) {
	clk := newClock()
	p := fake.New(clk)
	res := buyRental(t, p, rentalHours)

	if err := p.DeliverSMS(res.RemoteOrderID, "111111"); err != nil {
		t.Fatal(err)
	}
	st, err := p.GetStatus(context.Background(), port.Creds{}, res.RemoteOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if st.State == port.StateCompleted {
		t.Error("kiralık ilk mesajda TAMAMLANDI — numara dönem boyunca açık kalmalı")
	}

	// İkinci mesaj hâlâ kabul edilmeli.
	if err := p.DeliverSMS(res.RemoteOrderID, "222222"); err != nil {
		t.Fatalf("ikinci mesaj reddedildi: %v", err)
	}
	st, err = p.GetStatus(context.Background(), port.Creds{}, res.RemoteOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Messages) != 2 {
		t.Errorf("mesaj sayısı = %d, beklenen 2", len(st.Messages))
	}
}

// TestFakeRentalDeliversMultipleMessages
//
// Otomatik teslim de çoklu olmalı: `len(o.messages) == 0` koruması kiralıkta
// kalksın diye. Aksi hâlde yoklama yolunun çoklu mesaj davranışı test
// edilemez.
func TestFakeRentalDeliversMultipleMessages(t *testing.T) {
	clk := newClock()
	p := fake.New(clk)
	p.RentMessageInterval = time.Hour
	p.RentMessageCount = 4

	res := buyRental(t, p, rentalHours)

	// 3 saat sonra: ilk mesaj (t0) + 1 sa + 2 sa + 3 sa = 4 mesaj.
	clk.Advance(3 * time.Hour)
	st, err := p.GetStatus(context.Background(), port.Creds{}, res.RemoteOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Messages) != 4 {
		t.Fatalf("mesaj sayısı = %d, beklenen 4 — kiralık numara dönem boyunca "+
			"birden çok mesaj alır", len(st.Messages))
	}
	if st.State == port.StateCompleted {
		t.Error("dönem sürerken kiralık TAMAMLANDI sayıldı")
	}

	// Üst sınır aşılmamalı: sonsuz mesaj üreten bir taklit, 30 günlük bir
	// dönemde testleri kilitler.
	clk.Advance(100 * time.Hour)
	st, err = p.GetStatus(context.Background(), port.Creds{}, res.RemoteOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Messages) != 4 {
		t.Errorf("mesaj sayısı = %d, RentMessageCount üst sınırı 4", len(st.Messages))
	}
}

// TestFakeRentalListActiveMatchesGetStatus
//
// TEKİL VE TOPLU YOL AYNI SİMÜLASYONDAN BESLENİR (docs/memory.md §3.15).
// İki uygulama ayrışırsa toplu yolun hatası testte görünmez — kiralıkta toplu
// yol (ListActive → otpList[]) çoklu mesajın ASIL taşıyıcısıdır.
func TestFakeRentalListActiveMatchesGetStatus(t *testing.T) {
	clk := newClock()
	p := fake.New(clk)
	p.RentMessageInterval = time.Hour
	p.RentMessageCount = 3

	res := buyRental(t, p, rentalHours)
	clk.Advance(2 * time.Hour)

	st, err := p.GetStatus(context.Background(), port.Creds{}, res.RemoteOrderID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := p.ListActive(context.Background(), port.Creds{}, "", 25)
	if err != nil {
		t.Fatal(err)
	}
	var found *port.ActiveOrder
	for i := range page.Items {
		if page.Items[i].RemoteOrderID == res.RemoteOrderID {
			found = &page.Items[i]
		}
	}
	if found == nil {
		t.Fatal("kiralık aktivasyon toplu listede YOK — yoklama onu hiç göremezdi")
	}
	if len(found.Messages) != len(st.Messages) {
		t.Errorf("toplu yol %d mesaj, tekil yol %d mesaj döndü",
			len(found.Messages), len(st.Messages))
	}
}

// TestFakeRentalCancelWindowExpires
//
// docs/provider-herosms.md §6.1: 24 saat ve üzeri aktivasyonda iptal yalnız
// ilk 20 dakikada mümkün. Tavan taklit edilmezse "pencere kapandıktan sonra
// iade" yolu ilk kez canlıda görülür.
func TestFakeRentalCancelWindowExpires(t *testing.T) {
	clk := newClock()
	p := fake.New(clk)
	res := buyRental(t, p, rentalHours)

	// 5. dakika: pencere açık (asgari 120 sn de geçti).
	clk.Advance(5 * time.Minute)
	if err := p.Cancel(context.Background(), port.Creds{}, res.RemoteOrderID); err != nil {
		t.Fatalf("pencere içinde iptal reddedildi: %v", err)
	}

	// Yeni bir kiralık, 25. dakikada: pencere kapandı.
	res2 := buyRental(t, p, rentalHours)
	clk.Advance(25 * time.Minute)
	err := p.Cancel(context.Background(), port.Creds{}, res2.RemoteOrderID)
	if !errors.Is(err, port.ErrCancelDenied) {
		t.Fatalf("hata = %v, ErrCancelDenied bekleniyordu (FREE_CANCELLATION_EXPIRED)", err)
	}
}

// TestFakeCanIgnoreRentalDuration
//
// 🔴 SÜREYİ ONURLANDIRMAYAN SAĞLAYICI KİPİNİN KENDİ TESTİ.
//
// Taklit sağlayıcıların ikisi de `DurationHours`u HER ZAMAN onurlandırıyordu.
// Bu iyi niyetli görünüyor ama bir kör nokta üretiyordu: "sağlayıcı ödenen
// süreyi vermezse satın alma katmanı ne yapar" sorusunun testte hiç karşılığı
// olmuyordu — ve cevabı "kullanıcı parasını kaybeder"di (720 saatlik kiralık
// 20 dakika sürdü, iade yazılmadı).
//
// Bir kip, onu KULLANAN testten önce kendi kendini kanıtlamalıdır: kip sessizce
// çalışmazsa, onunla yazılan servis testi de hiçbir şey ölçmez.
func TestFakeCanIgnoreRentalDuration(t *testing.T) {
	clk := newClock()
	p := fake.New(clk)
	p.SetFaults(fake.Faults{RentalDurationIgnored: true})

	res := buyRental(t, p, rentalHours)

	if res.Subtype != port.KindSMSActivation {
		t.Errorf("subtype = %q, beklenen %q — kip sağlayıcının kiralığı "+
			"aktivasyona düşürmesini taklit etmeli", res.Subtype, port.KindSMSActivation)
	}
	if got := res.ExpiresAt.Sub(clk.Now()); got >= rentalHours*time.Hour {
		t.Errorf("TTL = %v — kip süreyi yok saymadı, hata görünmez kalırdı", got)
	}

	// Kip KAPALIYKEN davranış değişmemeli: aksi hâlde tüm kiralık testleri
	// sessizce bozulurdu.
	q := fake.New(clk)
	if res2 := buyRental(t, q, rentalHours); res2.Subtype != port.KindSMSRental {
		t.Errorf("kip kapalıyken subtype = %q — varsayılan davranış bozuldu", res2.Subtype)
	}
}
