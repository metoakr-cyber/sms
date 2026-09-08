//go:build integration

package pricing_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/fx"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	"github.com/ikmetrik/sms-platform/api/internal/service/catalog"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	"github.com/ikmetrik/sms-platform/api/internal/testsupport"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	// 🔴 GÜVENLİK KAPISI: bu testler DELETE FROM yapar. Veritabanı adı
	// "_test" ile bitmiyorsa süreç durur — kapı Makefile'da değil burada,
	// çünkü `go test` komutunu elle yazan kişiyi Makefile korumaz.
	if _, gerr := testsupport.MustTestDatabaseURL(); gerr != nil {
		fmt.Fprintln(os.Stderr, gerr)
		os.Exit(1)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		// SESSİZ ATLAMA YOK.
		//
		// Buradaki eski davranış `os.Exit(0)` idi: DATABASE_URL tanımsızsa
		// tüm entegrasyon testleri atlanır ve `go test` "ok" derdi. Yani
		// veritabanı olmayan bir ortamda paket YEŞİL geçiyordu — sıfır
		// entegrasyon kapsamıyla. Bu, testin olmamasından kötüdür: kimse
		// eksik olduğunu fark etmez.
		//
		// Bilerek atlamak için ALLOW_SKIP_INTEGRATION=1 gerekir; o zaman da
		// atlama AÇIKÇA yazılır.
		if os.Getenv("ALLOW_SKIP_INTEGRATION") == "1" {
			fmt.Println("⚠️  DATABASE_URL tanımsız — entegrasyon testleri ATLANDI (ALLOW_SKIP_INTEGRATION=1)")
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "DATABASE_URL tanımsız — entegrasyon testleri çalıştırılamıyor.")
		fmt.Fprintln(os.Stderr, "  Çözüm: `set -a; source .env; set +a`  veya  `make check`")
		fmt.Fprintln(os.Stderr, "  Bilerek atlamak için: ALLOW_SKIP_INTEGRATION=1")
		os.Exit(1)
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

type clk struct {
	mu sync.Mutex
	t  time.Time
}

func newClk() *clk                     { return &clk{t: time.Now()} }
func (c *clk) Now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clk) Advance(d time.Duration) { c.mu.Lock(); defer c.mu.Unlock(); c.t = c.t.Add(d) }

type env struct {
	quotes *pricingsvc.QuoteService
	fxsvc  *pricingsvc.FXService
	tx     *postgres.TxRunner
	fp     *fake.Provider
	clock  *clk
	userID int64
	q      *db.Queries
}

// setup temiz bir katalog + kur + kullanıcı hazırlar.
func setup(t *testing.T, fxRate string, maxAge time.Duration) *env {
	t.Helper()
	ctx := context.Background()

	for _, s := range []string{
		"DELETE FROM price_quotes", "DELETE FROM fx_rates",
		"DELETE FROM pricing_rules WHERE scope <> 'GLOBAL'",
		"DELETE FROM provider_offers", "DELETE FROM provider_dimension_maps",
		"DELETE FROM products", "DELETE FROM providers",
		"DELETE FROM countries", "DELETE FROM services",
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("temizlik (%s): %v", s, err)
		}
	}
	// GLOBAL kuralı VAR OLDUĞUNDAN EMİN OL, yalnız korumakla yetinme.
	//
	// Test yalnız "koru" deseydi, kuralı silen herhangi bir olay (bozuk bir
	// temizlik betiği, elle müdahale) veritabanını kalıcı olarak bozuk
	// bırakır ve sonraki tüm koşular gizemli bir NO_PRICING_RULE ile düşerdi.
	// Testler kendi ön koşullarını KURMALIDIR.
	if _, err := pool.Exec(ctx, `
		INSERT INTO pricing_rules (scope, margin_percent, note)
		SELECT 'GLOBAL', 40.00, 'test tohumu'
		WHERE NOT EXISTS (SELECT 1 FROM pricing_rules WHERE scope='GLOBAL' AND is_active)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE pricing_rules SET margin_percent = 40, fixed_fee_minor = 0, min_price_minor = 0
		 WHERE scope = 'GLOBAL'`); err != nil {
		t.Fatal(err)
	}

	c := newClk()
	box, err := crypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	fp := fake.New(c)
	fp.SMSDelay = time.Hour
	reg := provider.NewRegistry()
	reg.Register(fp)

	tx := postgres.NewTxRunner(pool)
	q := db.New(pool)

	prov, err := q.CreateProvider(ctx, db.CreateProviderParams{
		Name: "fake-pricing", Protocol: db.ProviderProtocolFAKE,
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

	// Kur
	if fxRate != "" {
		if _, err := q.InsertFXRate(ctx, db.InsertFXRateParams{
			Base: db.CurrencyCodeUSD, Quote: db.CurrencyCodeTRY,
			Rate: numeric(t, fxRate), Source: "test", FetchedAt: c.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	var userID int64
	email := fmt.Sprintf("q-%d@test.local", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, username, password_hash, status)
		VALUES ($1, $2, 'x', 'ACTIVE') RETURNING id`,
		email, fmt.Sprintf("qu%d", time.Now().UnixNano()%1e9)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM price_quotes WHERE user_id=$1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	fxsvc := pricingsvc.NewFXService(tx, &fx.Static{Rate: money.RateOne()}, c, maxAge)
	safety, _ := money.RateFromString("1.0") // tampon yok — hesap net görünsün
	quotes := pricingsvc.NewQuoteService(pricingsvc.QuoteDeps{
		TxRunner: tx, Registry: reg, Secrets: box, FX: fxsvc, Clock: c, FXSafetyMargin: safety,
	})
	return &env{quotes: quotes, fxsvc: fxsvc, tx: tx, fp: fp, clock: c, userID: userID, q: q}
}

func numeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatal(err)
	}
	return n
}

func errIs(err error, target *apperr.Error) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == target.Code
}

// ─────────────────────── KK-302 ───────────────────────

// Kur 30 dakikadan eskiyse SATIŞ DURUR. Bayat kurla satmaktansa durmak doğrudur.
func TestKK302_StaleFXStopsSelling(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "43.20", 30*time.Minute)

	// Taze kur: teklif verilir.
	if _, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"}); err != nil {
		t.Fatalf("taze kurla teklif başarısız: %v", err)
	}

	// 31 dakika sonra: kur bayat, satış durmalı.
	e.clock.Advance(31 * time.Minute)
	_, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"})
	if !errIs(err, apperr.ErrFxUnavailable) {
		t.Fatalf("bayat kurla teklif verildi: %v", err)
	}
}

func TestNoFXRateStopsSelling(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "", time.Hour) // hiç kur yok

	_, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"})
	if !errIs(err, apperr.ErrFxUnavailable) {
		t.Fatalf("kursuz teklif verildi: %v", err)
	}
}

// ─────────────────────── KK-305 ───────────────────────

// Teklif TEK KULLANIMLIK ve SÜRELİDİR.
func TestKK305_QuoteIsSingleUseAndExpires(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "43.20", time.Hour)

	q, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"})
	if err != nil {
		t.Fatal(err)
	}

	// İlk tüketim geçer.
	err = e.tx.InTx(ctx, func(qq *db.Queries) error {
		_, err := e.quotes.Consume(ctx, qq, e.userID, q.QuoteID)
		return err
	})
	if err != nil {
		t.Fatalf("ilk tüketim: %v", err)
	}

	// İkinci tüketim REDDEDİLİR.
	err = e.tx.InTx(ctx, func(qq *db.Queries) error {
		_, err := e.quotes.Consume(ctx, qq, e.userID, q.QuoteID)
		return err
	})
	if !errIs(err, apperr.ErrQuoteConsumed) {
		t.Fatalf("ikinci tüketim = %v, ErrQuoteConsumed bekleniyordu", err)
	}

	// Süresi geçmiş teklif de reddedilir.
	q2, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(pricingsvc.QuoteTTL + time.Second)
	err = e.tx.InTx(ctx, func(qq *db.Queries) error {
		_, err := e.quotes.Consume(ctx, qq, e.userID, q2.QuoteID)
		return err
	})
	if !errIs(err, apperr.ErrQuoteExpired) {
		t.Fatalf("süresi geçmiş teklif = %v, ErrQuoteExpired bekleniyordu", err)
	}
}

// Başkasının teklifi kullanılamaz; var olmayan teklifle AYNI hatayı verir.
func TestQuoteOwnershipIsEnforced(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "43.20", time.Hour)

	q, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"})
	if err != nil {
		t.Fatal(err)
	}

	const otherUser = int64(999999)
	err = e.tx.InTx(ctx, func(qq *db.Queries) error {
		_, err := e.quotes.Consume(ctx, qq, otherUser, q.QuoteID)
		return err
	})
	if !errIs(err, apperr.ErrNotFound) {
		t.Fatalf("başkasının teklifi = %v, ErrNotFound bekleniyordu", err)
	}

	err = e.tx.InTx(ctx, func(qq *db.Queries) error {
		_, err := e.quotes.Consume(ctx, qq, e.userID, uuid.New())
		return err
	})
	if !errIs(err, apperr.ErrNotFound) {
		t.Fatalf("olmayan teklif = %v, ErrNotFound bekleniyordu", err)
	}
}

// ─────────────────────── KK-402 ───────────────────────

// Aynı teklifle N eşzamanlı tüketimden TAM BİRİ geçer.
func TestKK402_ConcurrentConsumptionExactlyOnce(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "43.20", time.Hour)

	q, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"})
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	var ok, rejected atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := e.tx.InTx(ctx, func(qq *db.Queries) error {
				_, err := e.quotes.Consume(ctx, qq, e.userID, q.QuoteID)
				return err
			})
			switch {
			case err == nil:
				ok.Add(1)
			case errIs(err, apperr.ErrQuoteConsumed):
				rejected.Add(1)
			default:
				t.Errorf("beklenmeyen hata: %v", err)
			}
		}()
	}
	wg.Wait()

	if ok.Load() != 1 {
		t.Fatalf("başarılı tüketim = %d, beklenen TAM 1", ok.Load())
	}
	if rejected.Load() != n-1 {
		t.Fatalf("reddedilen = %d, beklenen %d", rejected.Load(), n-1)
	}
}

// ─────────────────────── Fiyat bütünlüğü ───────────────────────

// Teklif verildikten SONRA kural veya maliyet değişse bile teklifteki fiyat SABİT kalır.
func TestQuotePriceIsFrozen(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "43.20", time.Hour)

	q, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"})
	if err != nil {
		t.Fatal(err)
	}
	original := q.SellPrice.Minor()

	// Marjı ve sağlayıcı maliyetini değiştir.
	if _, err := pool.Exec(ctx,
		`UPDATE pricing_rules SET margin_percent = 200 WHERE scope = 'GLOBAL'`); err != nil {
		t.Fatal(err)
	}
	e.fp.SetCost("tg", "RU", 900_000)

	var consumedPrice int64
	err = e.tx.InTx(ctx, func(qq *db.Queries) error {
		row, err := e.quotes.Consume(ctx, qq, e.userID, q.QuoteID)
		if err != nil {
			return err
		}
		consumedPrice = row.SellPriceMinor
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if consumedPrice != original {
		t.Fatalf("tahsil edilecek fiyat DEĞİŞTİ: teklif %d, tüketim %d — sözleşme bozuldu",
			original, consumedPrice)
	}
}

// Stoksuz kombinasyon teklif ÜRETMEZ.
func TestOutOfStockYieldsNoQuote(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "43.20", time.Hour)

	// wa/TR FakeProvider'da stoksuz (gerçek gözlem taklidi).
	_, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "wa", CountryISO: "TR"})
	if !errIs(err, apperr.ErrOutOfStock) {
		t.Fatalf("stoksuz kombinasyon = %v, ErrOutOfStock bekleniyordu", err)
	}
}

// Yavaş sağlayıcı teklifi geciktirmez — elenir.
func TestSlowProviderIsDropped(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "43.20", time.Hour)

	e.fp.SetFaults(fake.Faults{Latency: 10 * time.Second})
	start := time.Now()
	_, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"})
	elapsed := time.Since(start)

	if !errIs(err, apperr.ErrNoProviderAvailable) {
		t.Fatalf("hata = %v, ErrNoProviderAvailable bekleniyordu", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("yavaş sağlayıcı %v bekletti — 3 sn'de elenmeliydi", elapsed)
	}
}

// En spesifik kural uygulanır ve teklifte kaydedilir.
func TestMostSpecificRuleWins(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "43.20", time.Hour)

	var svcID, ctryID int64
	_ = pool.QueryRow(ctx, `SELECT id FROM services WHERE code='tg'`).Scan(&svcID)
	_ = pool.QueryRow(ctx, `SELECT id FROM countries WHERE iso2='RU'`).Scan(&ctryID)

	// SERVICE_COUNTRY kuralı: %100 — GLOBAL %40'ı ezmeli.
	if _, err := pool.Exec(ctx, `
		INSERT INTO pricing_rules (scope, service_id, country_id, margin_percent)
		VALUES ('SERVICE_COUNTRY', $1, $2, 100)`, svcID, ctryID); err != nil {
		t.Fatal(err)
	}

	q, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU"})
	if err != nil {
		t.Fatal(err)
	}

	var margin string
	if err := pool.QueryRow(ctx,
		`SELECT margin_percent::text FROM price_quotes WHERE public_id=$1`, q.QuoteID).Scan(&margin); err != nil {
		t.Fatal(err)
	}
	if margin != "100.00" {
		t.Fatalf("uygulanan marj = %s, beklenen 100.00 — en spesifik kural kazanmadı", margin)
	}

	// Başka bir kombinasyon hâlâ GLOBAL kullanmalı.
	q2, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "ig", CountryISO: "RU"})
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT margin_percent::text FROM price_quotes WHERE public_id=$1`, q2.QuoteID).Scan(&margin); err != nil {
		t.Fatal(err)
	}
	if margin != "40.00" {
		t.Fatalf("ig/RU marjı = %s, beklenen 40.00 (GLOBAL)", margin)
	}
	_ = port.RealClock{}
}
