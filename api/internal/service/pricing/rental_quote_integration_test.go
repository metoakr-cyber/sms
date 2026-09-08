//go:build integration

package pricing_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/fx"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
)

// activationOnlyProvider CANLI fiyat ucunda YALNIZ aktivasyon fiyatını bilen
// sağlayıcı — gerçek HeroSMS'in davranışı budur.
//
// Kiralık fiyat sorulduğunda da aynı aktivasyon fiyatını döndürür; süre
// boyutu canlı şemada YOKTUR. Testin amacı: fiyatlandırmanın bu tuzağa
// düşmediğini göstermek.
type activationOnlyProvider struct{ recordingProvider }

func (p *activationOnlyProvider) GetPriceAndStock(context.Context, port.Creds, port.PriceQuery) (*port.PriceResult, error) {
	// Süreden BAĞIMSIZ, hep aynı aktivasyon fiyatı.
	return &port.PriceResult{Cost: money.New(1_000_000, money.USD), Stock: 500}, nil
}

// TestRentalDurationsHaveDifferentPrices
//
// SÖZLEŞME: kiralık ürünün fiyatı SÜREYE göre değişir ve canlı aktivasyon
// fiyatıyla EZİLMEZ.
//
// Bu hata canlıda gerçekleşti: sekiz kiralama süresinin sekizi de 31,15 ₺
// döndü — 1 günlük ile 180 günlük numara aynı fiyata satılıyordu. Sebep,
// canlı fiyat kontrolünün sağlayıcının AKTİVASYON ucunu çağırması ve
// önbellekteki kiralık maliyeti ezmesiydi.
func TestRentalDurationsHaveDifferentPrices(t *testing.T) {
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
	reg := provider.NewRegistry()
	reg.Register(&activationOnlyProvider{*newRecordingProvider()})

	tx := postgres.NewTxRunner(pool)
	q := db.New(pool)

	prov, err := q.CreateProvider(ctx, db.CreateProviderParams{
		Name: "rental-test", Protocol: db.ProviderProtocolFAKE, IsActive: true, Priority: 100,
		CostMultiplier: numeric(t, "1.0"), Capabilities: []byte(`["SMS_ACTIVATION","SMS_RENTAL"]`),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Katalog: bir servis, bir ülke.
	var svcID, ctryID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO services (code, name) VALUES ('wa','Whatsapp') RETURNING id`).Scan(&svcID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO countries (iso2, name, name_tr, phone_code)
		 VALUES ('TR','Turkey','Türkiye','90') RETURNING id`).Scan(&ctryID); err != nil {
		t.Fatal(err)
	}
	for _, m := range []struct {
		dim, code string
		local     int64
	}{{"service", "wa", svcID}, {"country", "62", ctryID}} {
		if err := q.UpsertDimensionMap(ctx, db.UpsertDimensionMapParams{
			ProviderID: prov.ID, Dimension: m.dim, LocalID: m.local, RemoteCode: m.code,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Üç kiralık ürün, ÜÇ FARKLI maliyet — gerçek sağlayıcıdaki gibi
	// süre arttıkça maliyet artıyor.
	type tier struct {
		minutes   int32
		costMicro int64
	}
	tiers := []tier{
		{1440, 2_000_000},   // 1 gün  → 2 USD
		{10080, 6_500_000},  // 7 gün  → 6,5 USD
		{43200, 25_000_000}, // 30 gün → 25 USD
	}
	for _, ti := range tiers {
		m := ti.minutes
		prod, err := q.UpsertRentalProduct(ctx, db.UpsertRentalProductParams{
			ServiceID: &svcID, CountryID: &ctryID, DurationMinutes: &m,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := q.UpsertOffer(ctx, db.UpsertOfferParams{
			ProviderID: prov.ID, ProductID: prod.ID,
			CostMicro: ti.costMicro, CostCurrency: db.CurrencyCodeUSD,
			Stock: 50, IsAvailable: true, SyncedAt: c.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := q.InsertFXRate(ctx, db.InsertFXRateParams{
		Base: db.CurrencyCodeUSD, Quote: db.CurrencyCodeTRY,
		Rate: numeric(t, "40.00"), Source: "test", FetchedAt: c.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	var userID int64
	email := fmt.Sprintf("rq-%d@test.local", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, username, password_hash, status)
		VALUES ($1, $2, 'x', 'ACTIVE') RETURNING id`,
		email, fmt.Sprintf("rq%d", time.Now().UnixNano()%1e9)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID) })

	fxsvc := pricingsvc.NewFXService(tx, &fx.Static{Rate: money.RateOne()}, c, time.Hour)
	safety, err := money.RateFromString("1.0")
	if err != nil {
		t.Fatal(err)
	}
	quotes := pricingsvc.NewQuoteService(pricingsvc.QuoteDeps{
		TxRunner: tx, Registry: reg, Secrets: box, FX: fxsvc, Clock: c, FXSafetyMargin: safety,
	})

	// Her süre için teklif al.
	prices := make(map[int32]int64, len(tiers))
	for _, ti := range tiers {
		res, err := quotes.Create(ctx, pricingsvc.QuoteRequest{
			UserID: userID, ServiceCode: "wa", CountryISO: "TR",
			DurationMinutes: ti.minutes,
		})
		if err != nil {
			t.Fatalf("%d dakikalık kiralık teklifi: %v", ti.minutes, err)
		}
		prices[ti.minutes] = res.SellPrice.Minor()
	}

	// (1) Fiyatlar BİRBİRİNDEN FARKLI olmalı.
	if prices[1440] == prices[43200] {
		t.Fatalf("1 günlük ve 30 günlük kiralama AYNI fiyat (%d kuruş) — "+
			"canlı aktivasyon fiyatı önbellekteki kiralık maliyeti eziyor",
			prices[1440])
	}

	// (2) Süre arttıkça fiyat ARTMALI.
	if !(prices[1440] < prices[10080] && prices[10080] < prices[43200]) {
		t.Errorf("fiyatlar süreyle artmıyor: 1g=%d 7g=%d 30g=%d",
			prices[1440], prices[10080], prices[43200])
	}

	// (3) Fiyat, ÖNBELLEKTEKİ maliyetten türemeli (canlı 1 USD'den değil).
	// 30 gün: 25 USD × 40 TRY × 1.40 marj = 1400,00 ₺ = 140000 kuruş.
	const want30 = 140000
	if prices[43200] != want30 {
		t.Errorf("30 günlük fiyat = %d kuruş, beklenen %d "+
			"(25 USD × 40 kur × 1.40 marj)", prices[43200], want30)
	}
}
