//go:build integration

package catalog_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	"github.com/ikmetrik/sms-platform/api/internal/service/catalog"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Println("DATABASE_URL tanımsız — atlanıyor")
		os.Exit(0)
	}
	p, err := postgres.NewPool(context.Background(), url)
	if err != nil {
		fmt.Printf("postgres: %v\n", err)
		os.Exit(1)
	}
	pool = p
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

type clk struct{ t time.Time }

func (c *clk) Now() time.Time { return c.t }
func (c *clk) Advance(d time.Duration) { c.t = c.t.Add(d) }

// setup temiz bir katalog ve FakeProvider kayıtlı bir servis üretir.
func setup(t *testing.T) (*catalog.Service, *fake.Provider, int64, *db.Queries) {
	t.Helper()
	ctx := context.Background()

	// Katalog tablolarını temizle (defter/kullanıcıya dokunmadan).
	for _, q := range []string{
		"DELETE FROM provider_offers", "DELETE FROM provider_dimension_maps",
		"DELETE FROM products", "DELETE FROM providers",
		"DELETE FROM operators", "DELETE FROM countries", "DELETE FROM services",
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("temizlik (%s): %v", q, err)
		}
	}

	key := make([]byte, 32)
	box, err := crypto.New(key)
	if err != nil {
		t.Fatal(err)
	}

	c := &clk{t: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)}
	fp := fake.New(c)
	reg := provider.NewRegistry()
	reg.Register(fp)

	q := db.New(pool)
	prov, err := q.CreateProvider(ctx, db.CreateProviderParams{
		Name: "fake-test", Protocol: db.ProviderProtocolFAKE,
		BaseUrl: "", ApiKeyEnc: nil, IsActive: true, Priority: 100,
		CostMultiplier: mustNumeric("1.0"), Capabilities: []byte(`["SMS_ACTIVATION"]`),
	})
	if err != nil {
		t.Fatalf("sağlayıcı oluşturulamadı: %v", err)
	}

	svc := catalog.New(catalog.Deps{
		TxRunner: postgres.NewTxRunner(pool), Registry: reg, Secrets: box, Clock: c,
	})
	return svc, fp, prov.ID, q
}

func TestSyncDimensionsCreatesCatalogAndMappings(t *testing.T) {
	ctx := context.Background()
	svc, _, provID, q := setup(t)

	rep, err := svc.SyncDimensions(ctx, provID)
	if err != nil {
		t.Fatalf("SyncDimensions: %v", err)
	}
	if len(rep.Errors) > 0 {
		t.Fatalf("senkron hataları: %v", rep.Errors)
	}
	if rep.Countries == 0 || rep.Services == 0 {
		t.Fatalf("ülke=%d servis=%d — ikisi de dolu olmalı", rep.Countries, rep.Services)
	}

	// Eşleştirmeler kurulmuş olmalı: bunlar olmadan sipariş verilemez.
	nc, err := q.CountDimensionMaps(ctx, db.CountDimensionMapsParams{
		ProviderID: provID, Dimension: string(port.DimCountry)})
	if err != nil {
		t.Fatal(err)
	}
	ns, err := q.CountDimensionMaps(ctx, db.CountDimensionMapsParams{
		ProviderID: provID, Dimension: string(port.DimService)})
	if err != nil {
		t.Fatal(err)
	}
	if nc != int64(rep.Countries) || ns != int64(rep.Services) {
		t.Fatalf("eşleştirme sayısı uyuşmuyor: ülke %d/%d servis %d/%d",
			nc, rep.Countries, ns, rep.Services)
	}

	// Türkçe ad ve telefon kodu taşınmış olmalı.
	tr, err := q.GetCountryByISO(ctx, "TR")
	if err != nil {
		t.Fatal(err)
	}
	if tr.NameTr != "Türkiye" || tr.PhoneCode != "90" || !tr.SupportsRent {
		t.Fatalf("TR kaydı eksik: %+v", tr)
	}
}

// Senkron İDEMPOTENT olmalı: ikinci tur çift kayıt üretmemeli.
func TestSyncIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, _, provID, _ := setup(t)

	first, err := svc.SyncDimensions(ctx, provID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.SyncDimensions(ctx, provID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Countries != second.Countries || first.Services != second.Services {
		t.Fatalf("tur1 (%d/%d) ile tur2 (%d/%d) farklı",
			first.Countries, first.Services, second.Countries, second.Services)
	}

	var nCountries, nServices int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM countries`).Scan(&nCountries)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM services`).Scan(&nServices)
	if nCountries != first.Countries || nServices != first.Services {
		t.Fatalf("çift kayıt oluştu: ülke=%d servis=%d", nCountries, nServices)
	}
}

// Admin'in düzenlediği Türkçe ad senkronda EZİLMEMELİ.
func TestSyncPreservesAdminEdits(t *testing.T) {
	ctx := context.Background()
	svc, _, provID, q := setup(t)

	if _, err := svc.SyncDimensions(ctx, provID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE services SET name_tr = 'WhatsApp Doğrulama' WHERE code = 'wa'`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncDimensions(ctx, provID); err != nil {
		t.Fatal(err)
	}
	wa, err := q.GetServiceByCode(ctx, "wa")
	if err != nil {
		t.Fatal(err)
	}
	if wa.NameTr != "WhatsApp Doğrulama" {
		t.Fatalf("admin düzenlemesi ezildi: %q", wa.NameTr)
	}
}

func TestSyncOffersPopulatesCatalog(t *testing.T) {
	ctx := context.Background()
	svc, _, provID, q := setup(t)

	if _, err := svc.SyncDimensions(ctx, provID); err != nil {
		t.Fatal(err)
	}
	rep, err := svc.SyncOffers(ctx, provID)
	if err != nil {
		t.Fatalf("SyncOffers: %v", err)
	}
	if len(rep.Errors) > 0 {
		t.Fatalf("hatalar: %v", rep.Errors)
	}
	if rep.Offers == 0 {
		t.Fatal("hiç teklif yazılmadı")
	}

	rows, err := q.ListAvailableProductsForCatalog(ctx, db.ListAvailableProductsForCatalogParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("katalog boş")
	}

	// WhatsApp/TR STOKSUZ olmalı: FakeProvider gerçek gözlemi taklit ediyor
	// (canlıda count=56964 ama physicalCount=0). Stoksuz ürün katalogda
	// GÖRÜNMEMELİ, yoksa kullanıcıdan para çekip boş döneriz.
	for _, r := range rows {
		if r.ServiceCode == "wa" && r.CountryIso2 == "TR" {
			t.Fatalf("stoksuz kombinasyon katalogda göründü: stok=%d", r.TotalStock)
		}
		if r.TotalStock <= 0 {
			t.Fatalf("stoksuz ürün katalogda: %s/%s", r.ServiceCode, r.CountryIso2)
		}
		if r.MinCostMicro <= 0 {
			t.Fatalf("maliyetsiz ürün: %s/%s", r.ServiceCode, r.CountryIso2)
		}
	}
	t.Logf("katalogda %d ürün, wa/TR yok (stoksuz)", len(rows))
}

// Sağlayıcı bir kombinasyonu listeden çıkarırsa o teklif "yok" olmalı.
// Aksi halde stokta olmayan bir ürün sonsuza kadar satılabilir görünürdü.
func TestStaleOffersAreMarkedUnavailable(t *testing.T) {
	ctx := context.Background()
	svc, fp, provID, _ := setup(t)

	if _, err := svc.SyncDimensions(ctx, provID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SyncOffers(ctx, provID); err != nil {
		t.Fatal(err)
	}

	var beforeCount int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM provider_offers WHERE is_available`).Scan(&beforeCount)
	if beforeCount == 0 {
		t.Fatal("ilk turda hiç teklif yok")
	}

	// Sağlayıcıda bir kombinasyonun stoku sıfırlansın.
	fp.SetStock("tg", "RU", 0)
	if _, err := svc.SyncOffers(ctx, provID); err != nil {
		t.Fatal(err)
	}

	var stock int32
	var avail bool
	err := pool.QueryRow(ctx, `
		SELECT o.stock, o.is_available FROM provider_offers o
		JOIN products p ON p.id = o.product_id
		JOIN services s ON s.id = p.service_id
		JOIN countries c ON c.id = p.country_id
		WHERE s.code='tg' AND c.iso2='RU'`).Scan(&stock, &avail)
	if err != nil {
		t.Fatal(err)
	}
	if avail || stock != 0 {
		t.Fatalf("stoku biten teklif hâlâ mevcut: stok=%d available=%v", stock, avail)
	}
}

func TestSyncProviderBalance(t *testing.T) {
	ctx := context.Background()
	svc, fp, provID, q := setup(t)

	fp.BalanceMicro = 42_500_000 // 42,50 USD
	if err := svc.SyncProviderBalance(ctx, provID); err != nil {
		t.Fatal(err)
	}
	p, err := q.GetProvider(ctx, provID)
	if err != nil {
		t.Fatal(err)
	}
	if p.AccountBalanceMicro != 42_500_000 {
		t.Fatalf("bakiye = %d, beklenen 42500000", p.AccountBalanceMicro)
	}
	if p.AccountSyncedAt == nil {
		t.Fatal("senkron zamanı yazılmadı")
	}
}

func mustNumeric(s string) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		panic(err)
	}
	return n
}
