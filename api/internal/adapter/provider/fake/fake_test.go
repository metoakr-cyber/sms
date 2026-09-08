package fake_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/contract"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// testClock ilerletilebilir sahte saat. Zamana bağlı kuralları (iptal
// penceresi, süre dolumu) gerçek zaman beklemeden sınamak için.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *testClock {
	return &testClock{t: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)}
}
func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// FakeProvider ortak sağlayıcı sözleşmesini geçmelidir.
// HeroSMS adaptörü de AYNI testleri geçecek (M3 devamı).
func TestContract(t *testing.T) {
	clk := newClock()
	p := fake.New(clk)
	p.SMSDelay = time.Hour // kod kendiliğinden gelmesin; testler elle tetikler

	contract.Run(t, contract.Subject{
		Provider:       p,
		StockedService: "tg", StockedCountry: "RU",
		EmptyService: "wa", EmptyCountry: "TR", // gerçek gözlem: WhatsApp/TR stoksuz
		DeliverSMS:  func(id string) error { return p.DeliverSMS(id, "123456") },
		AdvanceTime: clk.Advance,
	})
}

func TestPurchaseDecrementsStock(t *testing.T) {
	ctx := context.Background()
	p := fake.New(newClock())
	p.SMSDelay = time.Hour

	before, err := p.GetPriceAndStock(ctx, port.Creds{}, port.PriceQuery{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Purchase(ctx, port.Creds{}, port.PurchaseCmd{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS,
		MaxCost: money.New(100_000_000, money.USD)}); err != nil {
		t.Fatal(err)
	}
	after, err := p.GetPriceAndStock(ctx, port.Creds{}, port.PriceQuery{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS})
	if err != nil {
		t.Fatal(err)
	}
	if after.Stock != before.Stock-1 {
		t.Fatalf("stok %d → %d, bir azalmalıydı", before.Stock, after.Stock)
	}
}

// 'call' doğrulaması AYRI bir fiyat/stok eksenidir (docs/design.md §4).
func TestVerificationTypeIsSeparateAxis(t *testing.T) {
	ctx := context.Background()
	p := fake.New(newClock())

	sms, err := p.GetPriceAndStock(ctx, port.Creds{}, port.PriceQuery{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS})
	if err != nil {
		t.Fatal(err)
	}
	call, err := p.GetPriceAndStock(ctx, port.Creds{}, port.PriceQuery{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifyCall})
	if err != nil {
		t.Fatal(err)
	}
	if sms.Cost.Minor() == call.Cost.Minor() && sms.Stock == call.Stock {
		t.Fatal("sms ve call aynı fiyat/stoku döndürdü — ayrı eksen olmalı")
	}
}

// Kod teslim edildikten SONRA iptal edilemez: sağlayıcı iade etmez.
func TestCannotCancelAfterCodeDelivered(t *testing.T) {
	ctx := context.Background()
	clk := newClock()
	p := fake.New(clk)
	p.SMSDelay = time.Hour

	res, err := p.Purchase(ctx, port.Creds{}, port.PurchaseCmd{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS,
		MaxCost: money.New(100_000_000, money.USD)})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.DeliverSMS(res.RemoteOrderID, "999888"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(5 * time.Minute) // asgari bekleme geçti

	err = p.Cancel(ctx, port.Creds{}, res.RemoteOrderID)
	if !errors.Is(err, port.ErrCancelDenied) {
		t.Fatalf("hata = %v, ErrCancelDenied bekleniyordu", err)
	}
}

// Süre dolduğunda sipariş kendiliğinden iptale düşer.
func TestOrderExpires(t *testing.T) {
	ctx := context.Background()
	clk := newClock()
	p := fake.New(clk)
	p.SMSDelay = time.Hour
	p.OrderTTL = 10 * time.Minute

	res, err := p.Purchase(ctx, port.Creds{}, port.PurchaseCmd{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS,
		MaxCost: money.New(100_000_000, money.USD)})
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(11 * time.Minute)

	st, err := p.GetStatus(ctx, port.Creds{}, res.RemoteOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != port.StateCancelled {
		t.Fatalf("durum = %v, süre dolunca CANCELLED bekleniyordu", st.State)
	}
}

// Bir siparişe BİRDEN FAZLA mesaj gelebilir (FR-415).
func TestMultipleMessages(t *testing.T) {
	ctx := context.Background()
	p := fake.New(newClock())
	p.SMSDelay = time.Hour

	res, err := p.Purchase(ctx, port.Creds{}, port.PurchaseCmd{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS,
		MaxCost: money.New(100_000_000, money.USD)})
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"111111", "222222", "333333"} {
		if err := p.DeliverSMS(res.RemoteOrderID, code); err != nil {
			t.Fatal(err)
		}
	}
	st, err := p.GetStatus(ctx, port.Creds{}, res.RemoteOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Messages) != 3 {
		t.Fatalf("mesaj sayısı = %d, beklenen 3 — ilk koddan sonra akış kapanmamalı", len(st.Messages))
	}
}

// Hata enjeksiyonu: servis katmanının hata dallarını sınamak için.
func TestFaultInjection(t *testing.T) {
	ctx := context.Background()
	p := fake.New(newClock())

	p.SetFaults(fake.Faults{PurchaseFails: port.ErrProviderAuth})
	if _, err := p.Purchase(ctx, port.Creds{}, port.PurchaseCmd{
		ServiceCode: "tg", CountryCode: "RU"}); !errors.Is(err, port.ErrProviderAuth) {
		t.Fatalf("enjekte edilen hata dönmedi: %v", err)
	}

	// Fiyat kayması: teklif ile satın alma arasında fiyat arttı.
	p.SetFaults(fake.Faults{PriceDrift: 2.0})
	price, _ := p.GetPriceAndStock(ctx, port.Creds{}, port.PriceQuery{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS})
	_, err := p.Purchase(ctx, port.Creds{}, port.PurchaseCmd{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS,
		MaxCost: price.Cost}) // teklifteki fiyatı azami olarak gönder
	if !errors.Is(err, port.ErrPriceChanged) {
		t.Fatalf("fiyat iki katına çıktığı halde satın alma geçti: %v", err)
	}
}

// Zaman aşımı: yavaş sağlayıcı context iptaliyle elenmeli.
func TestLatencyRespectsContext(t *testing.T) {
	p := fake.New(newClock())
	p.SetFaults(fake.Faults{Latency: 2 * time.Second})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := p.GetPriceAndStock(ctx, port.Creds{}, port.PriceQuery{
		ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS})
	if err == nil {
		t.Fatal("yavaş sağlayıcı zaman aşımına uğramadı")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("context iptali beklenmedi, %v sürdü", elapsed)
	}
}

// Eşzamanlı satın alma stoku bozmamalı.
func TestConcurrentPurchasesRespectStock(t *testing.T) {
	ctx := context.Background()
	p := fake.New(newClock())
	p.SMSDelay = time.Hour
	p.SetStock("tg", "RU", 5)

	var ok, out int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.Purchase(ctx, port.Creds{}, port.PurchaseCmd{
				ServiceCode: "tg", CountryCode: "RU", VerificationType: port.VerifySMS,
				MaxCost: money.New(100_000_000, money.USD)})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, port.ErrOutOfStock):
				out++
			default:
				t.Errorf("beklenmeyen hata: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 5 || out != 15 {
		t.Fatalf("başarılı=%d stoksuz=%d, beklenen 5/15", ok, out)
	}
}
