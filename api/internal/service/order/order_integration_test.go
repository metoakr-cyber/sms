//go:build integration

package order_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	ordersvc "github.com/ikmetrik/sms-platform/api/internal/service/order"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
	"github.com/ikmetrik/sms-platform/api/internal/testsupport"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	// 🔴 GÜVENLİK KAPISI: bu testler DELETE FROM yapar. Veritabanı adı
	// "_test" ile bitmiyorsa süreç durur — kapı Makefile'da değil burada,
	// çünkü `go test` komutunu elle yazan kişiyi Makefile korumaz.
	url, err := testsupport.MustTestDatabaseURL()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if url == "" {
		fmt.Println("⚠️  DATABASE_URL tanımsız — entegrasyon testleri ATLANDI")
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

type clk struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clk) Now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clk) Advance(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

/* ═══════════════════ Sağlayıcı taklidi ═══════════════════ */

// stubProvider satın alma davranışını test başına ayarlanabilen taklit.
type stubProvider struct {
	mu sync.Mutex

	failPurchase error
	purchases    atomic.Int32
	maxPrices    []int64 // her çağrıdaki maxPrice (mikro-USD)
	cancels      atomic.Int32
	finishes     atomic.Int32
	// statusMessages GetStatus'un döndüreceği mesajlar. Boşken sağlayıcı
	// "kod yok" der — sahte webhook testinin dayanağı budur.
	statusMessages []port.RemoteMessage
	// purchaseHook satın alma çağrısının ortasında çalıştırılır.
	purchaseHook func()
	expiresIn    time.Duration
	now          func() time.Time

	/* ── Toplu yoklama (port.BatchPoller) ── */

	// activeMessages: remoteID → toplu yoklamada dönecek mesajlar. Anahtarın
	// varlığı aktivasyonun ListActive sonucunda görüneceği anlamına gelir;
	// mesaj listesi boş olabilir (kod henüz gelmedi).
	activeMessages map[string][]port.RemoteMessage
	// activeStates: sağlayıcının bildirdiği durum. Yazılmazsa StateWaiting.
	activeStates    map[string]port.RemoteOrderState
	listActiveCalls atomic.Int32
	statusCalls     atomic.Int32

	// cancelResults SIRAYLA tüketilir; tükendiğinde Cancel nil döner (mevcut
	// davranış korunur, eski testler etkilenmez).
	cancelResults []error
	// cancelDelay Cancel çağrısına yapay gecikme ekler — eşzamanlılık
	// testinde iki işçinin çağrıyı gerçekten üst üste bindirmesi için.
	cancelDelay time.Duration
}

func newStub(now func() time.Time) *stubProvider {
	return &stubProvider{
		expiresIn:      20 * time.Minute,
		now:            now,
		activeMessages: map[string][]port.RemoteMessage{},
		activeStates:   map[string]port.RemoteOrderState{},
	}
}

func (p *stubProvider) Protocol() string { return "FAKE" }
func (p *stubProvider) Capabilities() []port.ProductKind {
	return []port.ProductKind{port.KindSMSActivation}
}
func (p *stubProvider) ListCountries(context.Context, port.Creds) ([]port.RemoteDimension, error) {
	return []port.RemoteDimension{{RemoteCode: "62", Name: "Turkey"}}, nil
}
func (p *stubProvider) ListServices(context.Context, port.Creds) ([]port.RemoteDimension, error) {
	return []port.RemoteDimension{{RemoteCode: "wa", Name: "Whatsapp"}}, nil
}
func (p *stubProvider) ListOffers(_ context.Context, _ port.Creds, vt port.VerificationType) ([]port.OfferSnapshot, error) {
	return []port.OfferSnapshot{{
		ServiceCode: "wa", CountryCode: "62", VerificationType: vt,
		Cost: money.New(1_000_000, money.USD), Stock: 500,
	}}, nil
}
func (p *stubProvider) GetPriceAndStock(context.Context, port.Creds, port.PriceQuery) (*port.PriceResult, error) {
	return &port.PriceResult{Cost: money.New(1_000_000, money.USD), Stock: 500}, nil
}

func (p *stubProvider) Purchase(_ context.Context, _ port.Creds, cmd port.PurchaseCmd) (*port.PurchaseResult, error) {
	p.mu.Lock()
	p.maxPrices = append(p.maxPrices, cmd.MaxCost.Minor())
	fail := p.failPurchase
	hook := p.purchaseHook
	p.mu.Unlock()

	// purchaseHook satın alma ÇAĞRISI SIRASINDA çalışır: istemci kopmasını
	// gerçek anında (T1 bitmiş, T2 başlamamış) simüle etmenin tek yolu.
	if hook != nil {
		hook()
	}

	n := p.purchases.Add(1)
	if fail != nil {
		return nil, fail
	}
	return &port.PurchaseResult{
		RemoteOrderID: fmt.Sprintf("%d", 900000+n),
		PhoneNumber:   "+905551234567",
		Cost:          money.New(1_000_000, money.USD),
		ExpiresAt:     p.now().Add(p.expiresIn),
		Subtype:       port.KindSMSActivation,
	}, nil
}

func (p *stubProvider) GetStatus(context.Context, port.Creds, string) (*port.RemoteStatus, error) {
	p.statusCalls.Add(1)
	p.mu.Lock()
	msgs := append([]port.RemoteMessage(nil), p.statusMessages...)
	p.mu.Unlock()
	if len(msgs) == 0 {
		return &port.RemoteStatus{State: port.StateWaiting}, nil
	}
	return &port.RemoteStatus{State: port.StateCompleted, Messages: msgs}, nil
}

var _ port.BatchPoller = (*stubProvider)(nil)

// ListActive gerçek sağlayıcının sayfalama sözleşmesini taklit eder: size
// 25'e kırpılır, cursor 1 tabanlı sayfa numarası, son sayfada NextCursor boş.
func (p *stubProvider) ListActive(_ context.Context, _ port.Creds, cursor string, size int) (port.ActivePage, error) {
	p.listActiveCalls.Add(1)
	const maxSize = 25
	if size <= 0 || size > maxSize {
		size = maxSize
	}
	page := 1
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 1 {
			return port.ActivePage{}, fmt.Errorf("stub: geçersiz imleç %q", cursor)
		}
		page = n
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	ids := make([]string, 0, len(p.activeMessages))
	for id := range p.activeMessages {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	start := (page - 1) * size
	if start > len(ids) {
		start = len(ids)
	}
	end := start + size
	if end > len(ids) {
		end = len(ids)
	}

	items := make([]port.ActiveOrder, 0, end-start)
	for _, id := range ids[start:end] {
		msgs := append([]port.RemoteMessage(nil), p.activeMessages[id]...)
		st := p.activeStates[id]
		if st == "" {
			st = port.StateWaiting
		}
		if len(msgs) > 0 {
			st = port.StateCompleted
		}
		items = append(items, port.ActiveOrder{RemoteOrderID: id, State: st, Messages: msgs})
	}
	next := ""
	if end < len(ids) {
		next = strconv.Itoa(page + 1)
	}
	return port.ActivePage{Items: items, NextCursor: next}, nil
}

func (p *stubProvider) Cancel(context.Context, port.Creds, string) error {
	p.cancels.Add(1)
	p.mu.Lock()
	delay := p.cancelDelay
	var out error
	if len(p.cancelResults) > 0 {
		out = p.cancelResults[0]
		p.cancelResults = p.cancelResults[1:]
	}
	p.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return out
}
func (p *stubProvider) Finish(context.Context, port.Creds, string) error {
	p.finishes.Add(1)
	return nil
}
func (p *stubProvider) GetBalance(context.Context, port.Creds) (money.Money, error) {
	return money.New(100_000_000, money.USD), nil
}

/* ═══════════════════ Kurulum ═══════════════════ */

type env struct {
	svc    *ordersvc.Service
	stub   *stubProvider
	clock  *clk
	q      *db.Queries
	userID int64
	provID int64
	prodID int64
	pub    *kayitYayinci
}

func setup(t *testing.T, balanceMinor int64) *env {
	t.Helper()
	ctx := context.Background()

	for _, s := range []string{
		"DELETE FROM order_messages", "DELETE FROM orders",
		"DELETE FROM price_quotes", "DELETE FROM provider_offers",
		"DELETE FROM provider_dimension_maps", "DELETE FROM products",
		"DELETE FROM providers", "DELETE FROM countries", "DELETE FROM services",
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("temizlik (%s): %v", s, err)
		}
	}

	c := &clk{t: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)}
	stub := newStub(c.Now)
	reg := provider.NewRegistry()
	reg.Register(stub)

	box, err := crypto.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	tx := postgres.NewTxRunner(pool)
	q := db.New(pool)

	prov, err := q.CreateProvider(ctx, db.CreateProviderParams{
		Name: "stub", Protocol: db.ProviderProtocolFAKE, IsActive: true, Priority: 100,
		CostMultiplier: numeric(t, "1.0"), Capabilities: []byte(`["SMS_ACTIVATION"]`),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Katalog: servis, ülke, ürün, eşleştirmeler.
	var svcID, ctryID, prodID int64
	if err := pool.QueryRow(ctx, `INSERT INTO services (code, name) VALUES ('wa','Whatsapp') RETURNING id`).Scan(&svcID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO countries (iso2, name, name_tr, phone_code) VALUES ('TR','Turkey','Türkiye','90') RETURNING id`).Scan(&ctryID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO products (kind, service_id, country_id, verification_type)
		VALUES ('SMS_ACTIVATION', $1, $2, 'sms') RETURNING id`, svcID, ctryID).Scan(&prodID); err != nil {
		t.Fatal(err)
	}
	for _, m := range []struct {
		dim, code string
		local     int64
	}{
		{"service", "wa", svcID}, {"country", "62", ctryID},
	} {
		if err := q.UpsertDimensionMap(ctx, db.UpsertDimensionMapParams{
			ProviderID: prov.ID, Dimension: m.dim, LocalID: m.local, RemoteCode: m.code,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Kullanıcı + bakiye (defter üzerinden — değişmez #2).
	var userID int64
	email := fmt.Sprintf("o-%d@test.local", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, username, password_hash, status, email_verified_at)
		VALUES ($1, $2, 'x', 'ACTIVE', now()) RETURNING id`,
		email, fmt.Sprintf("ou%d", time.Now().UnixNano()%1e9)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM order_messages WHERE order_id IN (SELECT id FROM orders WHERE user_id=$1)`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM orders WHERE user_id=$1`, userID)
		_, _ = pool.Exec(bg, `BEGIN; SET LOCAL app.allow_ledger_truncate='on'; DELETE FROM ledger_entries WHERE user_id=`+fmt.Sprint(userID)+`; COMMIT;`)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id=$1`, userID)
	})

	wallet := walletsvc.New(tx)
	if balanceMinor > 0 {
		if err := tx.InTx(ctx, func(qq *db.Queries) error {
			_, err := wallet.Apply(ctx, qq, walletsvc.Input{
				UserID: userID, Amount: money.New(balanceMinor, money.TRY),
				Type: db.LedgerTypeADJUSTMENT, IdempotencyKey: fmt.Sprintf("test:%d", userID),
				Note: "test bakiyesi",
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}

	pub := &kayitYayinci{}
	svc := ordersvc.New(ordersvc.Deps{
		TxRunner: tx, Registry: reg, Secrets: box, Wallet: wallet, Clock: c,
		Publisher: pub,
	})
	return &env{svc: svc, stub: stub, clock: c, q: q, userID: userID, provID: prov.ID, prodID: prodID, pub: pub}
}

// kayitYayinci yayınlanan olayları biriktirir.
//
// SSE ABONESİNİN GÖRDÜĞÜNÜ gözlemlemek için gerekli: kodun kullanıcıya
// ulaşmasının İKİ kanalı var (GET yanıtı ve akış); yalnız veritabanına bakan
// bir test akış kanalını hiç görmez.
type kayitYayinci struct {
	mu      sync.Mutex
	olaylar []ordersvc.Event
}

func (k *kayitYayinci) Publish(_ context.Context, _ string, ev ordersvc.Event) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.olaylar = append(k.olaylar, ev)
	return nil
}

// kodIcerenOlaylar yayınlanmış olaylardan içinde kod geçenleri sayar.
func (k *kayitYayinci) kodIcerenOlaylar(kod string) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	n := 0
	for _, ev := range k.olaylar {
		for _, m := range ev.Messages {
			if m.Code == kod {
				n++
			}
		}
	}
	return n
}

// makeQuote doğrudan bir teklif satırı yazar (fiyatlandırma servisini atlar).
func (e *env) makeQuote(t *testing.T, sellMinor int64) uuid.UUID {
	t.Helper()
	var pubID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO price_quotes
			(user_id, product_id, provider_id, cost_micro, fx_rate, margin_percent,
			 sell_price_minor, stock_at_quote, expires_at)
		VALUES ($1,$2,$3,1000000,43.20,40,$4,500,$5)
		RETURNING public_id`,
		e.userID, e.prodID, e.provID, sellMinor, e.clock.Now().Add(2*time.Minute)).Scan(&pubID)
	if err != nil {
		t.Fatal(err)
	}
	return pubID
}

func (e *env) balance(t *testing.T) int64 {
	t.Helper()
	var b int64
	if err := pool.QueryRow(context.Background(),
		`SELECT balance_minor FROM users WHERE id=$1`, e.userID).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

// newPendingOrder bekleyen bir sipariş oluşturur (teklif + satın alma).
func (e *env) newPendingOrder(t *testing.T) db.Order {
	t.Helper()
	q := e.makeQuote(t, 2500)
	ord, err := e.svc.Create(context.Background(), ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatalf("sipariş oluşturma: %v", err)
	}
	return ord
}

// seedRefundCandidate iade kuyruğunda bekleyen bir sipariş kurar.
//
// DURUM DOĞRUDAN SQL İLE KURULUR, servis üzerinden değil: `Cancel` ve `Expire`
// kapatmayı bir GOROUTINE içinde tetikler (scheduleProviderClose) ve sağlayıcı
// çağrı sayaçları testte kararsızlaşır.
//
// İKİ ADIMDA yazılır (PENDING→CANCELLED→REFUNDED): `orders_guard_transition`
// tetikleyicisi doğrudan REFUNDED'a geçişi REDDEDER.
func (e *env) seedRefundCandidate(
	t *testing.T, st string, attempts int32, next *time.Time, closed bool,
) db.Order {
	t.Helper()
	ctx := context.Background()
	ord := e.newPendingOrder(t)
	now := e.clock.Now()

	if _, err := pool.Exec(ctx,
		`UPDATE orders SET status='CANCELLED', cancelled_at=$2 WHERE id=$1`, ord.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE orders SET status='REFUNDED', refunded_at=$2 WHERE id=$1`, ord.ID, now); err != nil {
		t.Fatal(err)
	}
	var closedAt *time.Time
	if closed {
		closedAt = &now
	}
	if _, err := pool.Exec(ctx, `
		UPDATE orders SET provider_refund_status=$2::refund_status, refund_attempts=$3,
		                  refund_next_attempt_at=$4, provider_closed_at=$5
		WHERE id=$1`, ord.ID, st, attempts, next, closedAt); err != nil {
		t.Fatal(err)
	}

	out, err := e.q.GetOrderForUser(ctx, db.GetOrderForUserParams{PublicID: ord.PublicID, UserID: e.userID})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// refundRow iade eksenini tek sorguda okur.
func (e *env) refundRow(t *testing.T, id int64) (status string, attempts int32, next, closedAt *time.Time) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `
		SELECT provider_refund_status::text, refund_attempts, refund_next_attempt_at, provider_closed_at
		FROM orders WHERE id=$1`, id).Scan(&status, &attempts, &next, &closedAt); err != nil {
		t.Fatal(err)
	}
	return
}

// orderStatus siparişin durumunu okur.
func (e *env) orderStatus(t *testing.T, id int64) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(context.Background(),
		`SELECT status::text FROM orders WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// messageCount siparişin kayıtlı mesaj sayısı.
func (e *env) messageCount(t *testing.T, id int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM order_messages WHERE order_id=$1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func numeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatal(err)
	}
	return n
}

/* ═══════════════════ Testler ═══════════════════ */

// TestPurchaseDeductsExactlyOnce mutlu yol: para tam bir kez düşer.
func TestPurchaseDeductsExactlyOnce(t *testing.T) {
	e := setup(t, 10000)
	q := e.makeQuote(t, 2500)

	ord, err := e.svc.Create(context.Background(), ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatalf("satın alma: %v", err)
	}
	if ord.Status != db.OrderStatusPENDING {
		t.Errorf("durum = %s, beklenen PENDING", ord.Status)
	}
	if got := e.balance(t); got != 7500 {
		t.Errorf("bakiye = %d, beklenen 7500", got)
	}
	if n := e.stub.purchases.Load(); n != 1 {
		t.Errorf("sağlayıcıya %d satın alma çağrısı gitti, 1 bekleniyordu", n)
	}
}

// TestMaxPriceIsAlwaysSent
//
// SÖZLEŞME (ADR-018): her satın almada maxPrice gönderilir. Gönderilmezse
// sağlayıcı fiyatı yükseltmişse fark bize yazılır ve teklifteki fiyatla
// tahsil ettiğimiz tutar ayrışır — doğrudan zarar.
func TestMaxPriceIsAlwaysSent(t *testing.T) {
	e := setup(t, 10000)
	for i := 0; i < 2; i++ {
		q := e.makeQuote(t, 2500)
		if _, err := e.svc.Create(context.Background(), ordersvc.CreateInput{UserID: e.userID, QuoteID: q}); err != nil {
			t.Fatalf("satın alma %d: %v", i, err)
		}
	}
	e.stub.mu.Lock()
	defer e.stub.mu.Unlock()
	if len(e.stub.maxPrices) != 2 {
		t.Fatalf("%d çağrı kaydedildi, 2 bekleniyordu", len(e.stub.maxPrices))
	}
	for i, mp := range e.stub.maxPrices {
		if mp != 1_000_000 {
			t.Errorf("çağrı %d: maxPrice = %d mikro-USD, teklifteki maliyet (1000000) bekleniyordu", i, mp)
		}
	}
}

// TestProviderFailureRefundsFully — KK-400.
//
// SÖZLEŞME: sağlayıcı hata verirse SİPARİŞ OLUŞMAZ ve bakiye işlem öncesi
// değerine döner. Defterde bir PURCHASE ve bir REFUND bulunur.
func TestProviderFailureRefundsFully(t *testing.T) {
	e := setup(t, 10000)
	e.stub.mu.Lock()
	e.stub.failPurchase = port.ErrOutOfStock
	e.stub.mu.Unlock()

	q := e.makeQuote(t, 2500)
	_, err := e.svc.Create(context.Background(), ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err == nil {
		t.Fatal("sağlayıcı hata verdiği hâlde satın alma başarılı döndü")
	}

	if got := e.balance(t); got != 10000 {
		t.Errorf("bakiye = %d, beklenen 10000 (tam iade)", got)
	}
	var orders int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM orders WHERE user_id=$1`, e.userID).Scan(&orders); err != nil {
		t.Fatal(err)
	}
	if orders != 0 {
		t.Errorf("%d sipariş oluştu, 0 bekleniyordu", orders)
	}

	var types []string
	rows, err := pool.Query(context.Background(),
		`SELECT entry_type::text FROM ledger_entries WHERE user_id=$1 AND entry_type IN ('PURCHASE','REFUND') ORDER BY id`, e.userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		types = append(types, s)
	}
	if len(types) != 2 || types[0] != "PURCHASE" || types[1] != "REFUND" {
		t.Errorf("defter kayıtları = %v, [PURCHASE REFUND] bekleniyordu", types)
	}
}

// TestSameQuoteCannotBeBoughtTwice — KK-402.
func TestSameQuoteCannotBeBoughtTwice(t *testing.T) {
	e := setup(t, 10000)
	q := e.makeQuote(t, 2500)
	ctx := context.Background()

	if _, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q}); err != nil {
		t.Fatalf("ilk satın alma: %v", err)
	}
	_, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err == nil {
		t.Fatal("aynı teklifle ikinci satın alma GEÇTİ — tek kullanımlık olmalı")
	}
	if got := e.balance(t); got != 7500 {
		t.Errorf("bakiye = %d, beklenen 7500 — ikinci kez düşülmemeli", got)
	}
	if n := e.stub.purchases.Load(); n != 1 {
		t.Errorf("sağlayıcıya %d çağrı gitti, 1 bekleniyordu", n)
	}
}

// TestConcurrentPurchaseOfSameQuote — KK-402 eşzamanlılık.
//
// Aynı teklifle N paralel satın alma denemesinden TAM BİRİ geçmelidir.
func TestConcurrentPurchaseOfSameQuote(t *testing.T) {
	e := setup(t, 100000)
	q := e.makeQuote(t, 2500)

	const n = 8
	var ok, failed atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := e.svc.Create(context.Background(),
				ordersvc.CreateInput{UserID: e.userID, QuoteID: q}); err == nil {
				ok.Add(1)
			} else {
				failed.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if ok.Load() != 1 {
		t.Fatalf("%d satın alma başarılı oldu, TAM 1 bekleniyordu (çift numara/çift tahsilat)", ok.Load())
	}
	if got := e.balance(t); got != 97500 {
		t.Errorf("bakiye = %d, beklenen 97500 — yalnız bir kez düşülmeli", got)
	}
}

// TestTerminalOrderCannotChangeStatus
//
// SÖZLEŞME: terminal durumdan çıkış yoktur ve bu VERİTABANINDA da zorlanır.
// Kod kontrolü tek başına yetmez: arka plan işleri yarış durumunda
// tamamlanmış bir siparişe iade yazabilir.
func TestTerminalOrderCannotChangeStatus(t *testing.T) {
	e := setup(t, 10000)
	q := e.makeQuote(t, 2500)
	ctx := context.Background()

	ord, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}
	// Doğrudan SQL ile COMPLETED yap (durum makinesini atlayarak kurulum).
	if _, err := pool.Exec(ctx,
		`UPDATE orders SET status='COMPLETED', completed_at=now() WHERE id=$1`, ord.ID); err != nil {
		t.Fatal(err)
	}

	// Şimdi terminal durumdan çıkmayı dene — VERİTABANI reddetmeli.
	_, err = pool.Exec(ctx, `UPDATE orders SET status='PENDING' WHERE id=$1`, ord.ID)
	if err == nil {
		t.Fatal("COMPLETED → PENDING geçişi veritabanında KABUL EDİLDİ")
	}
	_, err = pool.Exec(ctx, `UPDATE orders SET status='REFUNDED', refunded_at=now(), cancelled_at=now() WHERE id=$1`, ord.ID)
	if err == nil {
		t.Fatal("COMPLETED → REFUNDED geçişi KABUL EDİLDİ — tamamlanmış siparişe iade yapılabilir")
	}
}

// TestExpiredOrderRefundsWithoutUserAction — KK-405.
func TestExpiredOrderRefundsWithoutUserAction(t *testing.T) {
	e := setup(t, 10000)
	q := e.makeQuote(t, 2500)
	ctx := context.Background()

	ord, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.balance(t); got != 7500 {
		t.Fatalf("satın alma sonrası bakiye = %d", got)
	}

	// Süre dolsun.
	e.clock.Advance(21 * time.Minute)
	if err := e.svc.Expire(ctx, ord.ID); err != nil {
		t.Fatalf("Expire: %v", err)
	}

	if got := e.balance(t); got != 10000 {
		t.Errorf("bakiye = %d, beklenen 10000 — kullanıcı hiçbir şey yapmadan iadesini almalı", got)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM orders WHERE id=$1`, ord.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "REFUNDED" {
		t.Errorf("durum = %s, beklenen REFUNDED", status)
	}
}

// TestCancelTooEarlyIsRejected — FR-416.
func TestCancelTooEarlyIsRejected(t *testing.T) {
	e := setup(t, 10000)
	q := e.makeQuote(t, 2500)
	ctx := context.Background()

	ord, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Cancel(ctx, e.userID, ord.PublicID)
	if !errors.Is(err, apperr.ErrCancelTooEarly) {
		t.Fatalf("erken iptal hatası = %v, ErrCancelTooEarly bekleniyordu", err)
	}
	if got := e.balance(t); got != 7500 {
		t.Errorf("erken iptal denemesi bakiyeyi değiştirdi: %d", got)
	}

	// Süre geçince iptal edilebilmeli.
	e.clock.Advance(3 * time.Minute)
	if _, err := e.svc.Cancel(ctx, e.userID, ord.PublicID); err != nil {
		t.Fatalf("süre geçtikten sonra iptal: %v", err)
	}
	if got := e.balance(t); got != 10000 {
		t.Errorf("iptal sonrası bakiye = %d, beklenen 10000 (tam iade)", got)
	}
}

// TestOtherUsersOrderIsNotVisible — KK-403.
func TestOtherUsersOrderIsNotVisible(t *testing.T) {
	e := setup(t, 10000)
	q := e.makeQuote(t, 2500)
	ctx := context.Background()

	ord, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}
	const otherUser = int64(-1)
	if _, err := e.svc.Get(ctx, otherUser, ord.PublicID); !errors.Is(err, apperr.ErrOrderNotFound) {
		t.Fatalf("başkasının siparişi için hata = %v, ErrOrderNotFound bekleniyordu", err)
	}
	if _, err := e.svc.Cancel(ctx, otherUser, ord.PublicID); !errors.Is(err, apperr.ErrOrderNotFound) {
		t.Fatalf("başkasının siparişini iptal = %v, ErrOrderNotFound bekleniyordu", err)
	}
}

// TestDuplicateMessagesAreDeduplicated — FR-415 dedup.
func TestDuplicateMessagesAreDeduplicated(t *testing.T) {
	e := setup(t, 10000)
	q := e.makeQuote(t, 2500)
	ctx := context.Background()

	ord, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}

	msg := port.RemoteMessage{
		RemoteID: "otp-1", Code: "4821", Body: "Kodunuz 4821",
		ReceivedAt: e.clock.Now(),
	}
	// Aynı mesaj SEKİZ kez gelsin — webhook + yoklama + yeniden gönderimler.
	for i := 0; i < 8; i++ {
		if err := e.svc.DeliverMessages(ctx, ord.ID, []port.RemoteMessage{msg}); err != nil {
			t.Fatalf("teslim %d: %v", i, err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_messages WHERE order_id=$1`, ord.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d mesaj kaydedildi, 1 bekleniyordu (dedup)", n)
	}

	// İkinci FARKLI mesaj kaydedilmeli (FR-415: sipariş çok mesajlı olabilir).
	if err := e.svc.DeliverMessages(ctx, ord.ID, []port.RemoteMessage{{
		RemoteID: "otp-2", Code: "9911", Body: "Kodunuz 9911", ReceivedAt: e.clock.Now(),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_messages WHERE order_id=$1`, ord.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("ikinci mesaj sonrası %d kayıt, 2 bekleniyordu", n)
	}
}

// TestCompletedOrderIsFinishedNotCancelled — ADR-023.
//
// SÖZLEŞME: kod geldiyse Finish(), gelmediyse Cancel(). Yanlış tarafa düşmek
// ya iade hakkını yakar ya da sağlayıcıya yanlış sinyal verir.
func TestCompletedOrderIsFinishedNotCancelled(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()

	// (a) Kod GELEN sipariş → Finish
	q1 := e.makeQuote(t, 2500)
	o1, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q1})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeliverMessages(ctx, o1.ID, []port.RemoteMessage{{
		RemoteID: "m1", Code: "1234", Body: "Kod 1234", ReceivedAt: e.clock.Now(),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.CloseAtProvider(ctx, o1.ID); err != nil {
		t.Fatalf("kapatma: %v", err)
	}
	if e.stub.finishes.Load() != 1 {
		t.Errorf("kod gelen sipariş için Finish çağrısı = %d, 1 bekleniyordu", e.stub.finishes.Load())
	}

	// (b) Kod GELMEYEN sipariş → Cancel
	cancelsBefore := e.stub.cancels.Load()
	q2 := e.makeQuote(t, 2500)
	o2, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q2})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(21 * time.Minute)
	if err := e.svc.Expire(ctx, o2.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.CloseAtProvider(ctx, o2.ID); err != nil && !errors.Is(err, apperr.ErrInternal) {
		t.Logf("kapatma (beklenen olabilir): %v", err)
	}
	if e.stub.cancels.Load() <= cancelsBefore {
		t.Errorf("kod gelmeyen sipariş için Cancel çağrılmadı")
	}
	if e.stub.finishes.Load() != 1 {
		t.Errorf("kod gelmeyen siparişte Finish çağrıldı — iade hakkı yanardı")
	}
}

// TestOrphanHoldQuerySkipsAlreadyRefunded
//
// 🔴 GÜVENLİK AĞININ SESSİZCE YIRTILMASI.
//
// `orphan-hold-reaper`, parası çekilmiş ama siparişi olmayan teklifleri bulup
// iade eder. Sağlayıcı hatasıyla düşen HER satın alma tam olarak bu profilde
// bir satır bırakır — ama iadesi `refundHold` tarafından ZATEN yazılmıştır.
//
// Bu satırlar dışlanmazsa sorgu kalıcı olarak tıkanır: `LIMIT` ölü satırlarla
// dolar ve GERÇEKTEN iade edilmemiş yeni yetimler hiç görülmez. Tek bir
// sağlayıcı kesintisi dakikalar içinde yüz böyle satır üretir.
func TestOrphanHoldQuerySkipsAlreadyRefunded(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()

	// (1) İadesi YAPILMIŞ artık: teklif tüketilmiş, sipariş yok, defterde iade var.
	iadeliQuote := e.makeQuote(t, 2_500)
	// (2) GERÇEK yetim: teklif tüketilmiş, sipariş yok, iade YOK.
	yetimQuote := e.makeQuote(t, 3_000)

	eski := e.clock.Now().Add(-10 * time.Minute)
	for _, q := range []uuid.UUID{iadeliQuote, yetimQuote} {
		if _, err := pool.Exec(ctx,
			`UPDATE price_quotes SET consumed_at = $1 WHERE public_id = $2`, eski, q); err != nil {
			t.Fatal(err)
		}
	}
	// Yalnız birincisine iade yaz — reaper'ın kullandığı ANAHTARIN AYNISIYLA.
	tx := postgres.NewTxRunner(pool)
	if err := tx.InTx(ctx, func(qq *db.Queries) error {
		_, err := walletsvc.New(tx).Apply(ctx, qq, walletsvc.Input{
			UserID: e.userID, Amount: money.New(2_500, money.TRY),
			Type: db.LedgerTypeREFUND, IdempotencyKey: "order:" + iadeliQuote.String() + ":refund",
			Note: "sağlayıcı hatası — iade",
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	cutoff := e.clock.Now().Add(-2 * time.Minute)
	rows, err := e.q.ListOrphanHolds(ctx, db.ListOrphanHoldsParams{OlderThan: &cutoff, Lim: 100})
	if err != nil {
		t.Fatal(err)
	}

	var bulunan []string
	for _, r := range rows {
		bulunan = append(bulunan, r.PublicID.String())
	}
	for _, id := range bulunan {
		if id == iadeliQuote.String() {
			t.Errorf("🔴 iadesi YAPILMIŞ teklif hâlâ kuyrukta — sorgu kalıcı olarak tıkanır "+
				"ve gerçek yetimler LIMIT'in dışında kalır (bulunan: %v)", bulunan)
		}
	}
	var gorunduMu bool
	for _, id := range bulunan {
		if id == yetimQuote.String() {
			gorunduMu = true
		}
	}
	if !gorunduMu {
		t.Fatalf("🔴 GERÇEK yetim bulunamadı — güvenlik ağı hiç çalışmıyor (bulunan: %v)", bulunan)
	}
}

// TestRefundedOrderDoesNotLeakCode
//
// 🔴 KULLANICI HEM PARAYI HEM NUMARAYI ALAMAZ.
//
// Süre dolar → `Expire` iadeyi yazar, durum terminal olur. Hemen ardından
// sağlayıcıya iptal gider ve sağlayıcı "tam bu anda SMS geldi" diye reddeder
// (NEW_OTP_RECEIVED). Eski davranış: kod siparişe yazılıyor ve `GET /orders`
// onu duruma bakmadan döndürüyordu — kullanıcı iadesini almışken doğrulama
// kodunu da görüyordu, yani hizmeti bedavaya alıyordu.
//
// Yeni davranış: mesaj KAYDEDİLİR (destek ve mutabakat için) ama kullanıcıya
// AÇILMAZ ve durum değişmez.
func TestRefundedOrderDoesNotLeakCode(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()
	ord := e.newPendingOrder(t)

	bakiyeSatinAlmaSonrasi := e.balance(t)

	// Süre dolsun ve iade yazılsın.
	e.clock.Advance(25 * time.Minute)
	if err := e.svc.Expire(ctx, ord.ID); err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if got := e.balance(t); got <= bakiyeSatinAlmaSonrasi {
		t.Fatalf("iade yazılmamış: %d → %d", bakiyeSatinAlmaSonrasi, got)
	}
	var durum string
	if err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1`, ord.ID).Scan(&durum); err != nil {
		t.Fatal(err)
	}
	t.Logf("iade sonrası durum = %s", durum)

	// Sağlayıcı iptali "kod geldi" diye reddetsin.
	e.stub.mu.Lock()
	e.stub.cancelResults = []error{port.NewOTPArrived(
		[]port.RemoteMessage{{RemoteID: "otp-gec", Code: "424242", Body: "Kodunuz 424242"}},
		fmt.Errorf("NEW_OTP_RECEIVED"),
	)}
	e.stub.mu.Unlock()

	if err := e.svc.CloseAtProvider(ctx, ord.ID); err != nil {
		t.Fatalf("CloseAtProvider: %v", err)
	}

	// (1) Mesaj KAYDEDİLMİŞ olmalı — kanıt saklanır.
	var mesajSayisi int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM order_messages WHERE order_id=$1`, ord.ID).Scan(&mesajSayisi); err != nil {
		t.Fatal(err)
	}
	if mesajSayisi != 1 {
		t.Errorf("mesaj kaydedilmemiş (%d) — iade tartışmasının kanıtı kaybolur", mesajSayisi)
	}

	// (2) KOD SSE'YE YAYINLANMAMIŞ olmalı — akış ikinci sızıntı kanalıdır.
	if n := e.pub.kodIcerenOlaylar("424242"); n != 0 {
		t.Errorf("🔴 iade edilmiş siparişin kodu SSE ile YAYINLANDI (%d olay) — "+
			"ekranı açık olan kullanıcı kodu görür", n)
	}

	// (3) DURUM DEĞİŞMEMİŞ olmalı: iade edilmiş sipariş COMPLETED'a dönmez.
	var durumSonra string
	if err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1`, ord.ID).Scan(&durumSonra); err != nil {
		t.Fatal(err)
	}
	if durumSonra != durum {
		t.Fatalf("🔴 iade edilmiş siparişin durumu DEĞİŞTİ: %s → %s — "+
			"kullanıcı hem parayı hem numarayı aldı", durum, durumSonra)
	}
	if durumSonra == "COMPLETED" {
		t.Fatal("🔴 iade edilmiş sipariş COMPLETED oldu")
	}
}

// TestClientDisconnectStillRefunds
//
// 🔴 KULLANICININ SEKMEYİ KAPATMASI PARA KAYBI OLAMAZ.
//
// T1'de para ZATEN düşüldü. Sağlayıcı çağrısı sürerken kullanıcı sekmeyi
// kapatır / mobilde uygulamayı arka plana alır / ağ düşerse net/http istek
// bağlamını iptal eder. Telafi adımları o bağlamı kullanırsa `pool.Begin(ctx)`
// "context canceled" ile düşer ve defterde HİÇ iade kaydı oluşmaz:
// bakiye düşülmüş, sipariş yok, iade yok.
//
// Ölçülen eski davranış: bakiye 10000 → 7500, REFUND kaydı 0.
func TestClientDisconnectStillRefunds(t *testing.T) {
	e := setup(t, 100_000)

	// Sağlayıcı hata versin ki iade yolu çalışsın.
	e.stub.mu.Lock()
	e.stub.failPurchase = fmt.Errorf("%w: stok yok", port.ErrOutOfStock)
	e.stub.mu.Unlock()

	quote := e.makeQuote(t, 2_500)
	oncesi := e.balance(t)

	// İSTEMCİ KOPMASI: bağlam TAM satın alma çağrısı sırasında iptal edilir.
	// T1 bitmiş (para düşülmüş), T2 henüz başlamamıştır — net/http'nin
	// `c.Request.Context()` üzerinde yaptığı da tam olarak budur.
	ctx, cancel := context.WithCancel(context.Background())
	e.stub.mu.Lock()
	e.stub.purchaseHook = cancel
	e.stub.mu.Unlock()

	_, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: quote})
	if err == nil {
		t.Fatal("satın alma başarılı görünüyor — stub hata döndürmeliydi")
	}
	cancel()

	sonrasi := e.balance(t)
	if sonrasi != oncesi {
		t.Fatalf("🔴 İADE YAZILMADI: bakiye %d → %d (fark %d). İstemcinin bağlantıyı "+
			"koparması kullanıcının parasını askıda bıraktı.", oncesi, sonrasi, oncesi-sonrasi)
	}

	// İade defterde GERÇEKTEN olmalı — bakiyenin değişmemesi tek başına
	// "hiç düşülmedi" anlamına da gelebilirdi.
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ledger_entries WHERE idempotency_key = $1`,
		"order:"+quote.String()+":refund").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("🔴 iade defter kaydı yok (%d) — bakiye tesadüfen mi tuttu?", n)
	}
}
