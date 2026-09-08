// Package contract her ProviderPort uygulamasının geçmesi gereken ORTAK
// davranış testlerini içerir.
//
// Amaç: "adaptör derlendi" ile "adaptör doğru davranıyor" arasındaki farkı
// kapatmak. FakeProvider ve HeroSMS adaptörü AYNI testleri geçer; böylece
// sahte sağlayıcıyla yazılan servis kodu gerçek sağlayıcıda da çalışır.
//
// Kullanım:
//
//	func TestFakeContract(t *testing.T) {
//	    contract.Run(t, contract.Subject{Provider: fake.New(clk), ...})
//	}
package contract

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// Subject test edilecek adaptör ve onu sürmek için gereken bilgiler.
type Subject struct {
	Provider port.ProviderPort
	Creds    port.Creds

	// StockedService/Country stoğu OLAN bir kombinasyon.
	StockedService string
	StockedCountry string
	// EmptyService/Country stoğu OLMAYAN bir kombinasyon.
	EmptyService string
	EmptyCountry string

	// DeliverSMS kodu elle düşürür. Gerçek sağlayıcıda uygulanamaz;
	// nil ise kod bekleme testleri atlanır.
	DeliverSMS func(remoteOrderID string) error
	// AdvanceTime sahte saati ilerletir. nil ise zamana bağlı testler atlanır.
	AdvanceTime func(d time.Duration)

	// ReadOnly true ise para harcayan testler (Purchase) ATLANIR.
	// Gerçek sağlayıcıya karşı koşarken bakiye tükenmesin diye.
	ReadOnly bool
}

// Run sözleşme testlerini koşar.
func Run(t *testing.T, s Subject) {
	t.Helper()
	ctx := context.Background()

	t.Run("Protocol boş değil", func(t *testing.T) {
		if s.Provider.Protocol() == "" {
			t.Fatal("Protocol() boş — adaptör seçimi bu alandan yapılır")
		}
	})

	t.Run("Capabilities en az bir ürün tipi bildirir", func(t *testing.T) {
		if len(s.Provider.Capabilities()) == 0 {
			t.Fatal("Capabilities() boş")
		}
	})

	t.Run("ListCountries boyut döndürür", func(t *testing.T) {
		got, err := s.Provider.ListCountries(ctx, s.Creds)
		if err != nil {
			t.Fatalf("ListCountries: %v", err)
		}
		if len(got) == 0 {
			t.Fatal("hiç ülke dönmedi")
		}
		for _, d := range got {
			if d.RemoteCode == "" {
				t.Fatal("RemoteCode boş — eşleştirme yapılamaz")
			}
		}
	})

	t.Run("ListServices boyut döndürür", func(t *testing.T) {
		got, err := s.Provider.ListServices(ctx, s.Creds)
		if err != nil {
			t.Fatalf("ListServices: %v", err)
		}
		if len(got) == 0 {
			t.Fatal("hiç servis dönmedi")
		}
	})

	t.Run("GetPriceAndStock maliyeti USD ve mikro ölçekte döndürür", func(t *testing.T) {
		got, err := s.Provider.GetPriceAndStock(ctx, s.Creds, port.PriceQuery{
			ServiceCode: s.StockedService, CountryCode: s.StockedCountry,
			VerificationType: port.VerifySMS,
		})
		if err != nil {
			t.Fatalf("GetPriceAndStock: %v", err)
		}
		if got.Cost.Currency() != money.USD {
			t.Fatalf("maliyet para birimi = %v, USD bekleniyordu", got.Cost.Currency())
		}
		if got.Cost.Minor() <= 0 {
			t.Fatal("maliyet sıfır veya negatif")
		}
		if got.Stock <= 0 {
			t.Fatalf("stoklu kombinasyonda stok = %d", got.Stock)
		}
	})

	t.Run("bilinmeyen kombinasyon ErrMappingMissing döndürür", func(t *testing.T) {
		_, err := s.Provider.GetPriceAndStock(ctx, s.Creds, port.PriceQuery{
			ServiceCode: "olmayan_servis", CountryCode: "XX",
			VerificationType: port.VerifySMS,
		})
		if err == nil {
			t.Fatal("bilinmeyen kombinasyon hata vermedi")
		}
	})

	t.Run("ListOffers toplu anlık görüntü döndürür", func(t *testing.T) {
		got, err := s.Provider.ListOffers(ctx, s.Creds, port.VerifySMS)
		if err != nil {
			t.Fatalf("ListOffers: %v", err)
		}
		if len(got) == 0 {
			t.Fatal("hiç teklif dönmedi")
		}
		for _, o := range got {
			if o.ServiceCode == "" || o.CountryCode == "" {
				t.Fatal("teklif boyut kodu taşımıyor")
			}
			if o.Cost.Currency() != money.USD {
				t.Fatalf("teklif para birimi = %v", o.Cost.Currency())
			}
		}
	})

	t.Run("GetBalance USD döndürür", func(t *testing.T) {
		bal, err := s.Provider.GetBalance(ctx, s.Creds)
		if err != nil {
			t.Fatalf("GetBalance: %v", err)
		}
		if bal.Currency() != money.USD {
			t.Fatalf("bakiye para birimi = %v", bal.Currency())
		}
	})

	if s.ReadOnly {
		t.Log("ReadOnly — para harcayan testler atlandı")
		return
	}

	t.Run("stoksuz kombinasyon ErrOutOfStock döndürür", func(t *testing.T) {
		if s.EmptyService == "" {
			t.Skip("stoksuz kombinasyon tanımlanmamış")
		}
		_, err := s.Provider.Purchase(ctx, s.Creds, port.PurchaseCmd{
			ServiceCode: s.EmptyService, CountryCode: s.EmptyCountry,
			VerificationType: port.VerifySMS,
		})
		if !errors.Is(err, port.ErrOutOfStock) {
			t.Fatalf("hata = %v, ErrOutOfStock bekleniyordu", err)
		}
	})

	t.Run("MaxCost aşılırsa satın alma YAPILMAZ", func(t *testing.T) {
		// Fiyat garantisinin sağlayıcı sınırındaki sınavı (ADR-018).
		_, err := s.Provider.Purchase(ctx, s.Creds, port.PurchaseCmd{
			ServiceCode: s.StockedService, CountryCode: s.StockedCountry,
			VerificationType: port.VerifySMS,
			MaxCost:          money.New(1, money.USD), // 1 mikro-USD: kesinlikle yetersiz
		})
		if !errors.Is(err, port.ErrPriceChanged) {
			t.Fatalf("hata = %v, ErrPriceChanged bekleniyordu — MaxCost zorlanmıyor", err)
		}
	})

	t.Run("Purchase geçerli sipariş döndürür", func(t *testing.T) {
		res, err := s.Provider.Purchase(ctx, s.Creds, port.PurchaseCmd{
			ServiceCode: s.StockedService, CountryCode: s.StockedCountry,
			VerificationType: port.VerifySMS,
			MaxCost:          money.New(100_000_000, money.USD),
			ClientRef:        "sozlesme-testi",
		})
		if err != nil {
			t.Fatalf("Purchase: %v", err)
		}
		if res.RemoteOrderID == "" {
			t.Fatal("RemoteOrderID boş — sipariş takip edilemez")
		}
		if res.PhoneNumber == "" {
			t.Fatal("PhoneNumber boş")
		}
		if res.ExpiresAt.IsZero() {
			t.Fatal("ExpiresAt boş — süre koda gömülemez, sağlayıcıdan gelmeli")
		}
		if res.Cost.Currency() != money.USD || res.Cost.Minor() <= 0 {
			t.Fatalf("maliyet geçersiz: %s", res.Cost)
		}

		// Durum sorgusu çalışmalı.
		st, err := s.Provider.GetStatus(ctx, s.Creds, res.RemoteOrderID)
		if err != nil {
			t.Fatalf("GetStatus: %v", err)
		}
		if st.State == "" {
			t.Fatal("durum boş")
		}

		// Kod teslimi
		if s.DeliverSMS != nil {
			if err := s.DeliverSMS(res.RemoteOrderID); err != nil {
				t.Fatalf("DeliverSMS: %v", err)
			}
			st, err = s.Provider.GetStatus(ctx, s.Creds, res.RemoteOrderID)
			if err != nil {
				t.Fatalf("GetStatus (kod sonrası): %v", err)
			}
			if st.State != port.StateCompleted {
				t.Fatalf("durum = %v, COMPLETED bekleniyordu", st.State)
			}
			if len(st.Messages) == 0 {
				t.Fatal("mesaj listesi boş")
			}
			m := st.Messages[0]
			if m.Code == "" || m.Body == "" || m.ReceivedAt.IsZero() {
				t.Fatalf("mesaj alanları eksik: %+v", m)
			}
		}

		// Kod teslim edildikten sonra Finish çağrılmalı (Cancel DEĞİL).
		if err := s.Provider.Finish(ctx, s.Creds, res.RemoteOrderID); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		// Finish idempotent olmalı.
		if err := s.Provider.Finish(ctx, s.Creds, res.RemoteOrderID); err != nil {
			t.Fatalf("Finish ikinci kez: %v — idempotent olmalı", err)
		}
	})

	t.Run("bilinmeyen sipariş ErrOrderNotFound döndürür", func(t *testing.T) {
		_, err := s.Provider.GetStatus(ctx, s.Creds, "kesinlikle-olmayan-siparis-id")
		if !errors.Is(err, port.ErrOrderNotFound) {
			t.Fatalf("hata = %v, ErrOrderNotFound bekleniyordu", err)
		}
	})

	t.Run("erken iptal RetryAfter döndürür", func(t *testing.T) {
		res, err := s.Provider.Purchase(ctx, s.Creds, port.PurchaseCmd{
			ServiceCode: s.StockedService, CountryCode: s.StockedCountry,
			VerificationType: port.VerifySMS,
			MaxCost:          money.New(100_000_000, money.USD),
		})
		if err != nil {
			t.Fatalf("Purchase: %v", err)
		}
		err = s.Provider.Cancel(ctx, s.Creds, res.RemoteOrderID)
		ra, ok := port.AsRetryAfter(err)
		if !ok {
			t.Fatalf("hata = %v, RetryAfterError bekleniyordu — sağlayıcılar asgari bekleme uygular", err)
		}
		if ra.After <= 0 {
			t.Fatal("RetryAfter süresi sıfır — istemci ne kadar bekleyeceğini bilemez")
		}

		// Süre geçtikten sonra iptal başarılı olmalı.
		if s.AdvanceTime != nil {
			s.AdvanceTime(ra.After + time.Second)
			if err := s.Provider.Cancel(ctx, s.Creds, res.RemoteOrderID); err != nil {
				t.Fatalf("bekleme sonrası Cancel: %v", err)
			}
			st, err := s.Provider.GetStatus(ctx, s.Creds, res.RemoteOrderID)
			if err == nil && st.State != port.StateRefunded && st.State != port.StateCancelled {
				t.Fatalf("iptal sonrası durum = %v", st.State)
			}
		}
	})
}
