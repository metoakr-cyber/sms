//go:build integration

package pricing_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/fx"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	"github.com/ikmetrik/sms-platform/api/internal/service/catalog"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
)

// recordingProvider sorulan kodları KAYDEDEN, ülkeleri SAYISAL kodla veren
// sağlayıcı taklidi.
//
// FakeProvider ülke kodu olarak ISO2 kullanır ("TR"); bu yüzden "sağlayıcıya
// bizim kodumuzu gönderiyoruz" hatası onunla GÖRÜNMEZ — yanlış kod, doğru
// sonucu verir. Gerçek sağlayıcı (HeroSMS) "62" der ve teklif düşer.
type recordingProvider struct {
	mu     sync.Mutex
	asked  []port.PriceQuery
	remote map[string]string // sağlayıcının bildiği ülke kodu → ad
}

func newRecordingProvider() *recordingProvider {
	return &recordingProvider{remote: map[string]string{"62": "Turkey", "16": "United Kingdom"}}
}

func (p *recordingProvider) Protocol() string { return "FAKE" }
func (p *recordingProvider) Capabilities() []port.ProductKind {
	return []port.ProductKind{port.KindSMSActivation}
}

func (p *recordingProvider) ListCountries(context.Context, port.Creds) ([]port.RemoteDimension, error) {
	out := make([]port.RemoteDimension, 0, len(p.remote))
	for code, name := range p.remote {
		out = append(out, port.RemoteDimension{RemoteCode: code, Name: name})
	}
	return out, nil
}

func (p *recordingProvider) ListServices(context.Context, port.Creds) ([]port.RemoteDimension, error) {
	// Sağlayıcının servis kodu da BİZİMKİNDEN FARKLI olabilir.
	return []port.RemoteDimension{{RemoteCode: "wa", Name: "Whatsapp"}}, nil
}

func (p *recordingProvider) ListOffers(_ context.Context, _ port.Creds, vt port.VerificationType) ([]port.OfferSnapshot, error) {
	out := make([]port.OfferSnapshot, 0, len(p.remote))
	for code := range p.remote {
		out = append(out, port.OfferSnapshot{
			ServiceCode: "wa", CountryCode: code, VerificationType: vt,
			Cost: money.New(1_500_000, money.USD), Stock: 100,
		})
	}
	return out, nil
}

func (p *recordingProvider) GetPriceAndStock(_ context.Context, _ port.Creds, q port.PriceQuery) (*port.PriceResult, error) {
	p.mu.Lock()
	p.asked = append(p.asked, q)
	p.mu.Unlock()

	// KENDİ kodunu tanımayan sağlayıcı stok döndürmez — gerçek davranış budur.
	if _, ok := p.remote[q.CountryCode]; !ok {
		return &port.PriceResult{Stock: 0}, nil
	}
	return &port.PriceResult{Cost: money.New(1_500_000, money.USD), Stock: 100}, nil
}

func (p *recordingProvider) Purchase(context.Context, port.Creds, port.PurchaseCmd) (*port.PurchaseResult, error) {
	return nil, port.ErrUnsupported
}
func (p *recordingProvider) GetStatus(context.Context, port.Creds, string) (*port.RemoteStatus, error) {
	return nil, port.ErrUnsupported
}
func (p *recordingProvider) Cancel(context.Context, port.Creds, string) error {
	return port.ErrUnsupported
}
func (p *recordingProvider) Finish(context.Context, port.Creds, string) error {
	return port.ErrUnsupported
}
func (p *recordingProvider) GetBalance(context.Context, port.Creds) (money.Money, error) {
	return money.New(0, money.USD), nil
}

func (p *recordingProvider) askedCountries() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.asked))
	for _, q := range p.asked {
		out = append(out, q.CountryCode)
	}
	return out
}

// TestProviderReceivesItsOwnCodes
//
// SÖZLEŞME: sağlayıcıya SORARKEN onun kendi kodları gönderilir; bizim
// `countries.iso2` / `services.code` değerlerimiz değil.
//
// Bu test olmadan hata sessizdir: sağlayıcı "böyle bir ülke yok" der, teklif
// NO_PROVIDER_AVAILABLE ile düşer ve log'da yalnız "sağlayıcı fiyat vermedi"
// yazar — stokta 1469 numara varken.
func TestProviderReceivesItsOwnCodes(t *testing.T) {
	ctx := context.Background()

	for _, s := range []string{
		"DELETE FROM price_quotes", "DELETE FROM fx_rates",
		"DELETE FROM provider_offers", "DELETE FROM provider_dimension_maps",
		"DELETE FROM products", "DELETE FROM providers",
		"DELETE FROM countries", "DELETE FROM services",
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("temizlik (%s): %v", s, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO pricing_rules (scope, margin_percent, note)
		SELECT 'GLOBAL', 40.00, 'test tohumu'
		WHERE NOT EXISTS (SELECT 1 FROM pricing_rules WHERE scope='GLOBAL' AND is_active)`); err != nil {
		t.Fatal(err)
	}

	c := &clk{t: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)}
	box, err := crypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	rp := newRecordingProvider()
	reg := provider.NewRegistry()
	reg.Register(rp)

	tx := postgres.NewTxRunner(pool)
	q := db.New(pool)

	prov, err := q.CreateProvider(ctx, db.CreateProviderParams{
		Name: "recording", Protocol: db.ProviderProtocolFAKE,
		IsActive: true, Priority: 100,
		CostMultiplier: numeric(t, "1.0"), Capabilities: []byte(`["SMS_ACTIVATION"]`),
	})
	if err != nil {
		t.Fatal(err)
	}

	cat := catalog.New(catalog.Deps{TxRunner: tx, Registry: reg, Secrets: box, Clock: c})
	if _, err := cat.SyncDimensions(ctx, prov.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := cat.SyncOffers(ctx, prov.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := q.InsertFXRate(ctx, db.InsertFXRateParams{
		Base: db.CurrencyCodeUSD, Quote: db.CurrencyCodeTRY,
		Rate: numeric(t, "43.20"), Source: "test", FetchedAt: c.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	var userID int64
	email := fmt.Sprintf("rc-%d@test.local", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, username, password_hash, status)
		VALUES ($1, $2, 'x', 'ACTIVE') RETURNING id`,
		email, fmt.Sprintf("rc%d", time.Now().UnixNano()%1e9)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID) })

	fxsvc := pricingsvc.NewFXService(tx, &fx.Static{Rate: money.RateOne()}, c, time.Hour)
	safety, err := money.RateFromString("1.0") // tampon yok — hesap net görünsün
	if err != nil {
		t.Fatal(err)
	}
	quotes := pricingsvc.NewQuoteService(pricingsvc.QuoteDeps{
		TxRunner: tx, Registry: reg, Secrets: box, FX: fxsvc, Clock: c, FXSafetyMargin: safety,
	})

	// Ülke bizde ISO2 ("TR"); sağlayıcı "62" bilir.
	res, err := quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: userID, ServiceCode: "wa", CountryISO: "TR",
	})
	if err != nil {
		t.Fatalf("teklif alınamadı: %v  (sağlayıcıya sorulan ülkeler: %v)", err, rp.askedCountries())
	}
	if res.SellPrice.Minor() <= 0 {
		t.Fatalf("satış fiyatı = %d", res.SellPrice.Minor())
	}

	asked := rp.askedCountries()
	if len(asked) == 0 {
		t.Fatal("sağlayıcıya hiç sorulmadı")
	}
	for _, code := range asked {
		if code == "TR" {
			t.Fatalf("sağlayıcıya BİZİM ISO2 kodumuz gönderildi (%q); "+
				"eşleştirme tablosundaki %q gönderilmeliydi", code, "62")
		}
		if code != "62" {
			t.Errorf("sağlayıcıya gönderilen ülke kodu = %q, beklenen %q", code, "62")
		}
	}
}
