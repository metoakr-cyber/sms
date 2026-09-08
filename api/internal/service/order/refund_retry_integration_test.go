//go:build integration

package order_test

// FR-406b / KK-406b — sağlayıcı iade mutabakatı, rol ayrımı ve kapatma
// sahiplenmesi.
//
// Ortak kurulum (setup, env, clk, stubProvider, seedRefundCandidate)
// order_integration_test.go içindedir.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// waitUntil koşul sağlanana kadar bekler. Arka planda kapatma yapan
// goroutine'ler (scheduleProviderClose) için sabit uyku yerine kullanılır.
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// TestRefundRetrySucceedsAfterProviderRetryAfter — KK-406b mutlu yol.
//
// SÖZLEŞME: sağlayıcı "henüz erken" derse iade kuyruğa alınır, VERDİĞİ süre
// dolmadan yeniden denenmez (FR-414: sabit geri çekilme kullanılmaz) ve süre
// dolunca denenip REFUNDED olur.
func TestRefundRetrySucceedsAfterProviderRetryAfter(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()
	ord := e.seedRefundCandidate(t, "PENDING", 1, nil, false)

	e.stub.mu.Lock()
	e.stub.cancelResults = []error{
		port.NewRetryAfter("asgari bekleme süresi dolmadı", 120*time.Second, port.ErrCancelDenied),
		nil,
	}
	e.stub.mu.Unlock()
	cancelsBefore := e.stub.cancels.Load()

	// (1) İlk tur: sağlayıcı erteledi.
	rep, err := e.svc.RetryProviderRefunds(ctx, 100)
	if err != nil {
		t.Fatalf("birinci tur: %v", err)
	}
	if rep.Attempted != 1 {
		t.Fatalf("rapor = %+v, 1 deneme bekleniyordu", rep)
	}
	st, _, next, closedAt := e.refundRow(t, ord.ID)
	if st != "RETRY_SCHEDULED" {
		t.Fatalf("iade durumu = %s, RETRY_SCHEDULED bekleniyordu", st)
	}
	if next == nil {
		t.Fatal("refund_next_attempt_at yazılmadı — sağlayıcının verdiği süre yok sayılmış")
	}
	if want := e.clock.Now().Add(120 * time.Second); next.Sub(want).Abs() > time.Second {
		t.Errorf("sonraki deneme %s, ≈%s bekleniyordu (sağlayıcının verdiği süre)", *next, want)
	}
	if closedAt != nil {
		t.Error("başarısız iptalde provider_closed_at yazıldı — aktivasyon sağlayıcıda açık kalır")
	}

	// (2) Süre dolmadan ikinci tur: aday YOK.
	rep, err = e.svc.RetryProviderRefunds(ctx, 100)
	if err != nil {
		t.Fatalf("ikinci tur: %v", err)
	}
	if rep.Scanned != 0 {
		t.Fatalf("süre dolmadan %d aday çekildi — sağlayıcının 120 saniyesi çiğnendi", rep.Scanned)
	}
	if got := e.stub.cancels.Load(); got != cancelsBefore+1 {
		t.Fatalf("sağlayıcıya %d iptal gitti, %d bekleniyordu", got-cancelsBefore, 1)
	}

	// (3) Süre dolunca: iade alınır.
	e.clock.Advance(121 * time.Second)
	rep, err = e.svc.RetryProviderRefunds(ctx, 100)
	if err != nil {
		t.Fatalf("üçüncü tur: %v", err)
	}
	if rep.Refunded != 1 {
		t.Fatalf("rapor = %+v, 1 iade bekleniyordu", rep)
	}
	st, _, next, closedAt = e.refundRow(t, ord.ID)
	if st != "REFUNDED" {
		t.Fatalf("iade durumu = %s, REFUNDED bekleniyordu", st)
	}
	if closedAt == nil {
		t.Error("başarılı kapatmada provider_closed_at yazılmadı")
	}
	var amount int64
	if err := pool.QueryRow(ctx,
		`SELECT provider_refund_amount_minor FROM orders WHERE id=$1`, ord.ID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != ord.CostMicro {
		t.Errorf("iade tutarı = %d, maliyet %d bekleniyordu", amount, ord.CostMicro)
	}
}

// TestRefundRetryStopsAtCeiling
//
// SÖZLEŞME: sonsuz yeniden deneme yoktur. Üst sınıra ulaşan satır DENIED
// yazılır ve sağlayıcıya BİR DAHA istek gönderilmez; kapatma ekseni
// (provider_closed_at) ise terk edilmez.
func TestRefundRetryStopsAtCeiling(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()
	// 8 = maxRefundAttempts.
	ord := e.seedRefundCandidate(t, "RETRY_SCHEDULED", 8, nil, false)
	cancelsBefore := e.stub.cancels.Load()

	rep, err := e.svc.RetryProviderRefunds(ctx, 100)
	if err != nil {
		t.Fatalf("tur: %v", err)
	}
	if rep.Denied != 1 || rep.Attempted != 0 {
		t.Fatalf("rapor = %+v, 1 gider / 0 deneme bekleniyordu", rep)
	}
	st, _, _, closedAt := e.refundRow(t, ord.ID)
	if st != "DENIED" {
		t.Fatalf("iade durumu = %s, DENIED bekleniyordu", st)
	}
	if closedAt != nil {
		t.Error("iade üst sınırı kapatma eksenini de kapattı — FR-412 terk edilemez")
	}
	if got := e.stub.cancels.Load(); got != cancelsBefore {
		t.Fatalf("üst sınır aşılmışken sağlayıcıya %d istek gitti", got-cancelsBefore)
	}

	// Sonraki turlar: aday yok, istek yok.
	for i := 0; i < 3; i++ {
		rep, err := e.svc.RetryProviderRefunds(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Scanned != 0 {
			t.Fatalf("tur %d: %d aday çekildi — DENIED satır kuyrukta kalmış", i, rep.Scanned)
		}
	}
	if got := e.stub.cancels.Load(); got != cancelsBefore {
		t.Fatalf("sonraki turlarda sağlayıcıya %d istek gitti", got-cancelsBefore)
	}
}

// TestRefundRetrySettlesOrdersAlreadyClosedAtProvider
//
// Sağlayıcıda kapatılmış ama iade ekseni açık kalmış satır kuyrukta sonsuza
// kadar dönmez; sağlayıcıya hiç istek gönderilmeden eksen kapatılır.
func TestRefundRetrySettlesOrdersAlreadyClosedAtProvider(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()
	ord := e.seedRefundCandidate(t, "PENDING", 1, nil, true)
	cancelsBefore := e.stub.cancels.Load()
	finishesBefore := e.stub.finishes.Load()

	rep, err := e.svc.RetryProviderRefunds(ctx, 100)
	if err != nil {
		t.Fatalf("tur: %v", err)
	}
	if rep.Settled != 1 || rep.Attempted != 0 {
		t.Fatalf("rapor = %+v, 1 kapatma / 0 deneme bekleniyordu", rep)
	}
	if st, _, _, _ := e.refundRow(t, ord.ID); st != "NOT_APPLICABLE" {
		t.Fatalf("iade durumu = %s, NOT_APPLICABLE bekleniyordu", st)
	}
	if e.stub.cancels.Load() != cancelsBefore || e.stub.finishes.Load() != finishesBefore {
		t.Error("zaten kapalı sipariş için sağlayıcıya istek gitti")
	}

	rep, err = e.svc.RetryProviderRefunds(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 0 {
		t.Fatalf("ikinci turda %d aday çekildi — satır kuyrukta kalmış", rep.Scanned)
	}
}

// TestFinishedOrderLeavesRefundQueue — KÖK DÜZELTME.
//
// SÖZLEŞME (değişmez #24): kod geldiyse Finish(), gelmediyse Cancel(). Finish
// ile kapatılan siparişte sağlayıcıdan iade TALEP EDİLMEZ; iade ekseni de
// kapanmalıdır. Kapanmazsa satır iade kuyruğunda sonsuza kadar döner —
// sipariş sağlayıcıda kapalı olduğu için hiçbir deneme durumu değiştiremez.
func TestFinishedOrderLeavesRefundQueue(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()

	// Kurulum DOĞRUDAN SQL ile: DeliverMessages kapatmayı bir goroutine ile
	// tetikler ve sağlayıcı sayaçları kararsızlaşır.
	ord := e.newPendingOrder(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO order_messages (order_id, provider_otp_id, code, body, sender, received_at)
		VALUES ($1, 'otp-1', '4821', 'Kodunuz 4821', 'SERVIS', $2)`,
		ord.ID, e.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE orders SET status='COMPLETED', completed_at=$2 WHERE id=$1`,
		ord.ID, e.clock.Now()); err != nil {
		t.Fatal(err)
	}
	// İade ekseni AÇIK bırakılmış bir satır (iptal edilmiş sanılan siparişin
	// kodu son anda gelmiş senaryosu).
	if _, err := pool.Exec(ctx,
		`UPDATE orders SET provider_refund_status='PENDING' WHERE id=$1`, ord.ID); err != nil {
		t.Fatal(err)
	}

	cancelsBefore := e.stub.cancels.Load()
	finishesBefore := e.stub.finishes.Load()

	if err := e.svc.CloseAtProvider(ctx, ord.ID); err != nil {
		t.Fatalf("kapatma: %v", err)
	}
	if e.stub.finishes.Load() != finishesBefore+1 {
		t.Fatalf("Finish çağrılmadı (kod gelmişti)")
	}
	if e.stub.cancels.Load() != cancelsBefore {
		t.Fatalf("kod gelmiş siparişe Cancel gönderildi — iade hakkı yanardı")
	}
	st, _, _, closedAt := e.refundRow(t, ord.ID)
	if st != "NOT_APPLICABLE" {
		t.Fatalf("iade durumu = %s, NOT_APPLICABLE bekleniyordu", st)
	}
	if closedAt == nil {
		t.Fatal("provider_closed_at yazılmadı")
	}

	rep, err := e.svc.RetryProviderRefunds(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 0 {
		t.Fatalf("Finish edilmiş sipariş iade kuyruğunda: %+v", rep)
	}
}

// TestConcurrentCloseSendsSingleProviderCall — EŞZAMANLILIK.
//
// SÖZLEŞME: iki işçi (activation-reaper, provider-refund-retry, kullanıcı
// iptalinin arka plan goroutine'i) aynı siparişe aynı anda düşse bile
// sağlayıcıya TEK kapatma çağrısı gider.
//
// Bu test, `CloseAtProvider` içindeki eski SAHTE KİLİDİN kanıtıdır:
// `GetOrderForUpdate` transaction dışında çalıştığı için hiçbir şeyi
// korumuyordu; koruma artık koşullu UPDATE ile yapılan sahiplenmededir.
func TestConcurrentCloseSendsSingleProviderCall(t *testing.T) {
	e := setup(t, 10000)
	ord := e.seedRefundCandidate(t, "PENDING", 1, nil, false)

	// Gecikme, iki çağrının gerçekten üst üste binmesini sağlar.
	e.stub.mu.Lock()
	e.stub.cancelDelay = 50 * time.Millisecond
	e.stub.mu.Unlock()
	cancelsBefore := e.stub.cancels.Load()

	const n = 8
	var errs atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := e.svc.CloseAtProvider(context.Background(), ord.ID); err != nil {
				errs.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := e.stub.cancels.Load() - cancelsBefore; got != 1 {
		t.Fatalf("sağlayıcıya %d iptal çağrısı gitti, TAM 1 bekleniyordu "+
			"(eşzamanlı kapatma sahiplenmesi çalışmıyor)", got)
	}
	if errs.Load() != 0 {
		t.Errorf("%d çağrı hata döndürdü — yarışı kaybeden sessizce çekilmeliydi", errs.Load())
	}
	st, _, _, closedAt := e.refundRow(t, ord.ID)
	if st != "REFUNDED" || closedAt == nil {
		t.Errorf("iade durumu = %s, closedAt = %v — kapatma tamamlanmalıydı", st, closedAt)
	}
	// Mutabakat bozulmadı.
	e.assertLedgerMatchesBalance(t)
}

// TestReaperIgnoresScheduledRefundOrders — ROL AYRIMI.
//
// SÖZLEŞME: iade ekseni AÇIK satır `activation-reaper`'ın kümesinde değildir
// (aksi hâlde sağlayıcının Retry-After süresi 60 saniyede bir çiğnenir), ama
// iade ekseni kapandığında kapatma için GERİ DÖNER (FR-412 terk edilmez).
func TestReaperIgnoresScheduledRefundOrders(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()
	next := e.clock.Now().Add(120 * time.Second)
	ord := e.seedRefundCandidate(t, "RETRY_SCHEDULED", 2, &next, false)

	contains := func(t *testing.T) bool {
		t.Helper()
		rows, err := e.q.ListUnclosedTerminalOrders(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.ID == ord.ID {
				return true
			}
		}
		return false
	}

	if contains(t) {
		t.Fatal("iade ekseni açık satır reaper kümesinde — iki iş aynı satıra Cancel gönderir")
	}
	for _, st := range []string{"DENIED", "REFUNDED", "NOT_APPLICABLE"} {
		if _, err := pool.Exec(ctx,
			`UPDATE orders SET provider_refund_status=$2::refund_status WHERE id=$1`,
			ord.ID, st); err != nil {
			t.Fatal(err)
		}
		if !contains(t) {
			t.Errorf("iade ekseni %s iken satır reaper kümesine dönmedi — kapatma terk edilir (FR-412)", st)
		}
	}
}

// TestUserRefundIsUnaffectedByProviderRefundOutcome — KK-406.
//
// SÖZLEŞME: sağlayıcı iadeyi reddetse bile KULLANICI tam iadesini almıştır ve
// iade ekseni deftere HİÇBİR kayıt yazmaz. Fark bizim giderimizdir.
func TestUserRefundIsUnaffectedByProviderRefundOutcome(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()

	// Sağlayıcı KALICI olarak reddedecek.
	e.stub.mu.Lock()
	e.stub.cancelResults = []error{fmt.Errorf("%w: OTP alınmış", port.ErrCancelDenied)}
	e.stub.mu.Unlock()

	ord := e.newPendingOrder(t)
	if got := e.balance(t); got != 7500 {
		t.Fatalf("satın alma sonrası bakiye = %d", got)
	}
	e.clock.Advance(3 * time.Minute)
	if _, err := e.svc.Cancel(ctx, e.userID, ord.PublicID); err != nil {
		t.Fatalf("iptal: %v", err)
	}
	// Kullanıcı iadesi KOŞULSUZ ve ANINDA.
	if got := e.balance(t); got != 10000 {
		t.Fatalf("iptal sonrası bakiye = %d, 10000 bekleniyordu", got)
	}

	// Arka plandaki kapatma denemesinin sonucunu bekle.
	if !waitUntil(t, 5*time.Second, func() bool {
		st, _, _, _ := e.refundRow(t, ord.ID)
		return st == "DENIED"
	}) {
		st, _, _, _ := e.refundRow(t, ord.ID)
		t.Fatalf("iade durumu = %s, DENIED bekleniyordu (kalıcı red gider yazılmalı)", st)
	}

	rep, err := e.svc.RetryProviderRefunds(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 0 {
		t.Errorf("kalıcı reddedilmiş satır kuyrukta: %+v", rep)
	}

	// Defter: TAM BİR iade kaydı; iade ekseni deftere dokunmaz.
	var refunds int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_entries WHERE user_id=$1 AND entry_type='REFUND'`,
		e.userID).Scan(&refunds); err != nil {
		t.Fatal(err)
	}
	if refunds != 1 {
		t.Errorf("%d REFUND kaydı var, TAM 1 bekleniyordu", refunds)
	}
	if got := e.balance(t); got != 10000 {
		t.Errorf("bakiye = %d, 10000 bekleniyordu", got)
	}
	e.assertLedgerMatchesBalance(t)
}

// assertLedgerMatchesBalance mutabakat: Σ ledger == users.balance_minor.
func (e *env) assertLedgerMatchesBalance(t *testing.T) {
	t.Helper()
	var sum, bal int64
	if err := pool.QueryRow(context.Background(), `
		SELECT coalesce(sum(amount_minor),0), (SELECT balance_minor FROM users WHERE id=$1)
		FROM ledger_entries WHERE user_id=$1`, e.userID).Scan(&sum, &bal); err != nil {
		t.Fatal(err)
	}
	if sum != bal {
		t.Errorf("MUTABAKAT BOZUK: Σ ledger = %d, bakiye = %d", sum, bal)
	}
}
