//go:build integration

package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	"github.com/ikmetrik/sms-platform/api/internal/service/catalog"
)

// numericProvider ülkeleri SAYISAL kodla döndüren bir sağlayıcı taklidi.
//
// FakeProvider ülke kodu olarak zaten ISO2 kullanıyor ("TR", "RU"). Bu yüzden
// `iso = rc.RemoteCode` hatası testlerde GÖRÜNMÜYORDU: yanlış kod, doğru
// sonucu veriyordu. Gerçek sağlayıcı (HeroSMS) sayısal kimlik kullanıyor ve
// veritabanına `iso2 = '62'` gibi satırlar yazıldı.
//
// Bu taklit, tam olarak o farkı taşır.
type numericProvider struct{}

func (numericProvider) Protocol() string { return "FAKE" }
func (numericProvider) Capabilities() []port.ProductKind {
	return []port.ProductKind{port.KindSMSActivation}
}

func (numericProvider) ListCountries(context.Context, port.Creds) ([]port.RemoteDimension, error) {
	return []port.RemoteDimension{
		{RemoteCode: "62", Name: "Turkey"},         // referansta var → TR
		{RemoteCode: "16", Name: "United Kingdom"}, // referansta var → GB
		{RemoteCode: "187", Name: "USA"},           // referansta var → US
		{RemoteCode: "999", Name: "Atlantis"},      // referansta YOK → atlanmalı
	}, nil
}

func (numericProvider) ListServices(context.Context, port.Creds) ([]port.RemoteDimension, error) {
	return []port.RemoteDimension{{RemoteCode: "wa", Name: "Whatsapp"}}, nil
}

func (numericProvider) ListOffers(context.Context, port.Creds, port.VerificationType) ([]port.OfferSnapshot, error) {
	return nil, nil
}

func (numericProvider) GetPriceAndStock(context.Context, port.Creds, port.PriceQuery) (*port.PriceResult, error) {
	return &port.PriceResult{Stock: 0}, nil
}
func (numericProvider) Purchase(context.Context, port.Creds, port.PurchaseCmd) (*port.PurchaseResult, error) {
	return nil, port.ErrUnsupported
}
func (numericProvider) GetStatus(context.Context, port.Creds, string) (*port.RemoteStatus, error) {
	return nil, port.ErrUnsupported
}
func (numericProvider) Cancel(context.Context, port.Creds, string) error { return port.ErrUnsupported }
func (numericProvider) Finish(context.Context, port.Creds, string) error { return port.ErrUnsupported }
func (numericProvider) GetBalance(context.Context, port.Creds) (money.Money, error) {
	return money.New(0, money.USD), nil
}

// TestCountryIsoComesFromReferenceNotProviderCode
//
// SÖZLEŞME: `countries.iso2` ISO 3166-1 alpha-2'dir ve SAĞLAYICININ kodundan
// türetilmez. Sağlayıcının kodu yalnız `provider_dimension_maps`te yaşar.
func TestCountryIsoComesFromReferenceNotProviderCode(t *testing.T) {
	ctx := context.Background()

	for _, q := range []string{
		// price_quotes ÖNCE silinir: products'a RESTRICT bir FK ile bağlıdır.
		// Başka bir paketten kalan tek bir teklif satırı, buradaki
		// `DELETE FROM products` çağrısını FK ihlaliyle düşürür ve paketteki
		// TÜM testler ortak setup'ta patlar — "bazen düşen testler" böyle olur.
		"DELETE FROM price_quotes",
		"DELETE FROM provider_offers", "DELETE FROM provider_dimension_maps",
		"DELETE FROM products", "DELETE FROM providers",
		"DELETE FROM operators", "DELETE FROM countries", "DELETE FROM services",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("temizlik (%s): %v", q, err)
		}
	}

	box, err := crypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	c := &clk{t: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)}
	reg := provider.NewRegistry()
	reg.Register(numericProvider{})

	q := db.New(pool)
	prov, err := q.CreateProvider(ctx, db.CreateProviderParams{
		Name: "numeric-test", Protocol: db.ProviderProtocolFAKE,
		BaseUrl: "", ApiKeyEnc: nil, IsActive: true, Priority: 100,
		CostMultiplier: mustNumeric("1.0"), Capabilities: []byte(`["SMS_ACTIVATION"]`),
	})
	if err != nil {
		t.Fatalf("sağlayıcı oluşturulamadı: %v", err)
	}

	svc := catalog.New(catalog.Deps{
		TxRunner: postgres.NewTxRunner(pool), Registry: reg, Secrets: box, Clock: c,
	})

	rep, err := svc.SyncDimensions(ctx, prov.ID)
	if err != nil {
		t.Fatalf("SyncDimensions: %v", err)
	}

	// Üç ülke çözülmeli, "Atlantis" atlanmalı.
	if rep.Countries != 3 {
		t.Fatalf("çözülen ülke sayısı = %d, beklenen 3", rep.Countries)
	}
	// Çözülemeyen ülke SESSİZCE yutulmamalı — raporda görünmeli.
	if len(rep.Errors) == 0 {
		t.Fatal("çözülemeyen ülke raporlanmadı; sessiz veri kaybı")
	}

	rows, err := pool.Query(ctx, `SELECT iso2, name_tr, phone_code FROM countries ORDER BY iso2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	got := map[string][2]string{}
	for rows.Next() {
		var iso, tr, phone string
		if err := rows.Scan(&iso, &tr, &phone); err != nil {
			t.Fatal(err)
		}
		got[iso] = [2]string{tr, phone}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	want := map[string][2]string{
		"GB": {"Birleşik Krallık", "44"},
		"TR": {"Türkiye", "90"},
		"US": {"Amerika Birleşik Devletleri", "1"},
	}
	if len(got) != len(want) {
		t.Fatalf("ülke satırları = %v, beklenen %v", got, want)
	}
	for iso, w := range want {
		g, ok := got[iso]
		if !ok {
			t.Fatalf("%s satırı yok — sağlayıcının sayısal kodu ISO2 sanılmış olabilir: %v", iso, got)
		}
		if g != w {
			t.Errorf("%s: Türkçe ad/telefon kodu = %v, beklenen %v", iso, g, w)
		}
	}

	// Sağlayıcının kodu eşleştirme tablosunda YAŞAMALI — kaybolmamalı.
	var remote string
	err = pool.QueryRow(ctx, `
		SELECT m.remote_code FROM provider_dimension_maps m
		JOIN countries c ON c.id = m.local_id
		WHERE m.provider_id = $1 AND m.dimension = 'country' AND c.iso2 = 'TR'`,
		prov.ID).Scan(&remote)
	if err != nil {
		t.Fatalf("TR eşleştirmesi bulunamadı: %v", err)
	}
	if remote != "62" {
		t.Errorf("TR için sağlayıcı kodu = %q, beklenen %q", remote, "62")
	}
}

// unmappedProvider teklif döndüren ama HİÇBİRİ eşleştirilemeyen sağlayıcı.
type unmappedProvider struct{ numericProvider }

func (unmappedProvider) ListOffers(_ context.Context, _ port.Creds, vt port.VerificationType) ([]port.OfferSnapshot, error) {
	return []port.OfferSnapshot{
		{ServiceCode: "bilinmeyen", CountryCode: "9999", VerificationType: vt,
			Cost: money.New(1_000_000, money.USD), Stock: 50},
	}, nil
}

// TestUnmatchedOffersDoNotWipeCatalog
//
// SÖZLEŞME: sağlayıcı teklif döndürdüğü hâlde hiçbiri yerel ürüne
// eşleştirilemezse, MEVCUT KATALOG KORUNUR — bayat işaretleme yapılmaz.
//
// Bu koruma olmadan tek bir eşleştirme hatası tüm katalogu "stok yok" hâline
// getirir: kullanıcı boş bir site görür, log'da tek bir hata satırı yoktur.
// Gerçekte yaşandı: ülke araması bozulunca senkron "0 teklif, 0 hata" dedi ve
// 9754 teklifi bayat işaretledi.
func TestUnmatchedOffersDoNotWipeCatalog(t *testing.T) {
	ctx := context.Background()

	for _, s := range []string{
		// price_quotes ÖNCE silinir: products'a RESTRICT bir FK ile bağlıdır.
		// Başka bir paketten kalan tek bir teklif satırı, buradaki
		// `DELETE FROM products` çağrısını FK ihlaliyle düşürür ve paketteki
		// TÜM testler ortak setup'ta patlar — "bazen düşen testler" böyle olur.
		"DELETE FROM price_quotes",
		"DELETE FROM provider_offers", "DELETE FROM provider_dimension_maps",
		"DELETE FROM products", "DELETE FROM providers",
		"DELETE FROM operators", "DELETE FROM countries", "DELETE FROM services",
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("temizlik (%s): %v", s, err)
		}
	}

	box, err := crypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	c := &clk{t: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)}
	q := db.New(pool)
	tx := postgres.NewTxRunner(pool)

	// 1) Sağlam bir katalog kur.
	regOK := provider.NewRegistry()
	regOK.Register(numericProvider{})
	provOK, err := q.CreateProvider(ctx, db.CreateProviderParams{
		Name: "ok-provider", Protocol: db.ProviderProtocolFAKE,
		IsActive: true, Priority: 100,
		CostMultiplier: mustNumeric("1.0"), Capabilities: []byte(`["SMS_ACTIVATION"]`),
	})
	if err != nil {
		t.Fatal(err)
	}
	svcOK := catalog.New(catalog.Deps{TxRunner: tx, Registry: regOK, Secrets: box, Clock: c})
	if _, err := svcOK.SyncDimensions(ctx, provOK.ID); err != nil {
		t.Fatal(err)
	}

	// numericProvider teklif döndürmüyor; elle bir teklif koyalım ki
	// "korunacak katalog" gerçekten var olsun.
	var productID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO products (kind, service_id, country_id, verification_type)
		SELECT 'SMS_ACTIVATION', s.id, c.id, 'sms'
		FROM services s, countries c WHERE s.code='wa' AND c.iso2='TR'
		RETURNING id`).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO provider_offers
			(provider_id, product_id, cost_micro, cost_currency, stock, is_available, synced_at)
		VALUES ($1, $2, 1000000, 'USD', 42, true, $3)`,
		provOK.ID, productID, c.Now()); err != nil {
		t.Fatal(err)
	}

	// 2) Eşleşmeyen teklifler döndüren bir tur çalıştır.
	c.Advance(time.Hour)
	regBad := provider.NewRegistry()
	regBad.Register(unmappedProvider{})
	svcBad := catalog.New(catalog.Deps{TxRunner: tx, Registry: regBad, Secrets: box, Clock: c})

	rep, err := svcBad.SyncOffers(ctx, provOK.ID)
	if err != nil {
		t.Fatalf("SyncOffers: %v", err)
	}
	if rep.Offers != 0 {
		t.Fatalf("eşleşmeyen tur %d teklif yazdı", rep.Offers)
	}
	if len(rep.Errors) == 0 {
		t.Fatal("eşleşmeme raporlanmadı — sessiz başarısızlık")
	}
	if rep.StaleMarked != 0 {
		t.Fatalf("bayat işaretleme yapıldı (%d) — katalog korunmalıydı", rep.StaleMarked)
	}

	// 3) Katalog HÂLÂ ayakta olmalı.
	var stok int32
	var mevcut bool
	if err := pool.QueryRow(ctx,
		`SELECT stock, is_available FROM provider_offers WHERE product_id=$1`,
		productID).Scan(&stok, &mevcut); err != nil {
		t.Fatal(err)
	}
	if !mevcut || stok != 42 {
		t.Fatalf("katalog silindi: stok=%d mevcut=%v — beklenen 42/true", stok, mevcut)
	}
}
