//go:build integration

package order_test

// FR-404 / KK-404 / KK-411 — sunucu tarafı yoklama.
//
// Ortak kurulum (setup, env, clk, stubProvider) order_integration_test.go
// içindedir; TestMain burada yeniden tanımlanmaz.

import (
	"context"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// TestPollerDeliversCodeWithoutWebhook — KK-411.
//
// SÖZLEŞME: webhook HİÇ gelmese bile kod yoklamayla bulunur ve sipariş
// tamamlanır. Webhook'un teslim garantisi yoktur; bu iş olmadan kaybolan bir
// bildirim "kullanıcı kodunu hiç göremedi" demektir.
func TestPollerDeliversCodeWithoutWebhook(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()
	ord := e.newPendingOrder(t)

	// Sağlayıcıda kod var; webhook GÖNDERİLMİYOR.
	e.stub.mu.Lock()
	e.stub.activeMessages[ord.RemoteOrderID] = []port.RemoteMessage{{
		RemoteID: "otp-1", Code: "4821", Body: "Kodunuz 4821", ReceivedAt: e.clock.Now(),
	}}
	e.stub.mu.Unlock()

	rep, err := e.svc.PollPending(ctx, 100)
	if err != nil {
		t.Fatalf("yoklama: %v", err)
	}
	if rep.Scanned != 1 || rep.Matched != 1 || rep.Delivered != 1 {
		t.Fatalf("rapor = %+v, 1/1/1 bekleniyordu", rep)
	}
	if got := e.orderStatus(t, ord.ID); got != "COMPLETED" {
		t.Errorf("durum = %s, COMPLETED bekleniyordu — kod yoklamayla gelmeliydi", got)
	}
	if n := e.messageCount(t, ord.ID); n != 1 {
		t.Errorf("%d mesaj kaydedildi, 1 bekleniyordu", n)
	}
	var code string
	if err := pool.QueryRow(ctx,
		`SELECT code FROM order_messages WHERE order_id=$1`, ord.ID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	if code != "4821" {
		t.Errorf("kod = %q, 4821 bekleniyordu", code)
	}
	// Kod geldi → iade YOK.
	if got := e.balance(t); got != 7500 {
		t.Errorf("bakiye = %d, 7500 bekleniyordu (iade yazılmamalı)", got)
	}
}

// TestPollerUsesAtMostFourRequestsFor100Orders — KK-404.
//
// SÖZLEŞME: 100 bekleyen sipariş varken sağlayıcıya giden istek sayısı
// sipariş sayısıyla ORANTILI DEĞİLDİR ve tur başına ≤ 4'tür (100 ÷ 25).
// Eski prototip sipariş başına tekil istek atıyordu.
func TestPollerUsesAtMostFourRequestsFor100Orders(t *testing.T) {
	e := setup(t, 1_000_000)
	ctx := context.Background()

	const n = 100
	for i := 0; i < n; i++ {
		ord := e.newPendingOrder(t)
		// Kod YOK: burada ölçülen tek şey sayfalama.
		e.stub.mu.Lock()
		e.stub.activeMessages[ord.RemoteOrderID] = nil
		e.stub.mu.Unlock()
	}

	e.stub.listActiveCalls.Store(0)
	e.stub.statusCalls.Store(0)

	rep, err := e.svc.PollPending(ctx, 100)
	if err != nil {
		t.Fatalf("yoklama: %v", err)
	}
	if rep.Scanned != n {
		t.Fatalf("taranan = %d, %d bekleniyordu", rep.Scanned, n)
	}
	if calls := e.stub.listActiveCalls.Load(); calls < 1 || calls > 4 {
		t.Fatalf("sağlayıcıya %d toplu istek gitti, ≤ 4 olmalıydı (KK-404)", calls)
	}
	if calls := e.stub.statusCalls.Load(); calls != 0 {
		t.Fatalf("%d tekil GetStatus çağrısı yapıldı — toplu yol kullanılmalıydı", calls)
	}
	if rep.ProviderCalls > 4 {
		t.Errorf("rapor %d sağlayıcı isteği söylüyor, ≤ 4 olmalıydı", rep.ProviderCalls)
	}
}

// TestPollerIsIdempotentAcrossRounds
//
// İşler at-least-once çalışır: aynı tur iki kez koşarsa sonuç DEĞİŞMEZ.
func TestPollerIsIdempotentAcrossRounds(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()
	ord := e.newPendingOrder(t)

	e.stub.mu.Lock()
	e.stub.activeMessages[ord.RemoteOrderID] = []port.RemoteMessage{{
		RemoteID: "otp-1", Code: "1234", Body: "Kodunuz 1234", ReceivedAt: e.clock.Now(),
	}}
	e.stub.mu.Unlock()

	if _, err := e.svc.PollPending(ctx, 100); err != nil {
		t.Fatalf("birinci tur: %v", err)
	}
	var firstCompleted time.Time
	if err := pool.QueryRow(ctx, `SELECT completed_at FROM orders WHERE id=$1`, ord.ID).
		Scan(&firstCompleted); err != nil {
		t.Fatal(err)
	}

	// İkinci tur: sipariş artık PENDING değil, aday kümesinde de yok.
	if _, err := e.svc.PollPending(ctx, 100); err != nil {
		t.Fatalf("ikinci tur: %v", err)
	}
	// Üçüncü kez AYNI mesajı doğrudan teslim et — dedup son savunma hattı.
	if err := e.svc.DeliverMessages(ctx, ord.ID, []port.RemoteMessage{{
		RemoteID: "otp-1", Code: "1234", Body: "Kodunuz 1234", ReceivedAt: e.clock.Now(),
	}}); err != nil {
		t.Fatalf("tekrar teslim: %v", err)
	}

	if n := e.messageCount(t, ord.ID); n != 1 {
		t.Errorf("%d mesaj kaydedildi, 1 bekleniyordu (dedup)", n)
	}
	var secondCompleted time.Time
	if err := pool.QueryRow(ctx, `SELECT completed_at FROM orders WHERE id=$1`, ord.ID).
		Scan(&secondCompleted); err != nil {
		t.Fatal(err)
	}
	if !secondCompleted.Equal(firstCompleted) {
		t.Errorf("completed_at değişti: %s → %s", firstCompleted, secondCompleted)
	}
	var refunds int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_entries WHERE user_id=$1 AND entry_type='REFUND'`,
		e.userID).Scan(&refunds); err != nil {
		t.Fatal(err)
	}
	if refunds != 0 {
		t.Errorf("%d iade kaydı oluştu, 0 bekleniyordu", refunds)
	}
	if got := e.balance(t); got != 7500 {
		t.Errorf("bakiye = %d, 7500 bekleniyordu", got)
	}
}

// TestPollerNeverWritesProviderStateToOrderStatus — değişmez #13.
//
// SÖZLEŞME: sağlayıcının durum kodu `orders.status` alanına YANSITILMAZ.
// Sağlayıcının [1,2,3,4,7] kodlarının anlamı spec'te yoktur; tahmin etmek
// yanlış para hareketi tetikler.
func TestPollerNeverWritesProviderStateToOrderStatus(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()
	ord := e.newPendingOrder(t)

	e.stub.mu.Lock()
	e.stub.activeMessages[ord.RemoteOrderID] = nil // mesaj YOK
	e.stub.activeStates[ord.RemoteOrderID] = port.StateCancelled
	e.stub.mu.Unlock()

	if _, err := e.svc.PollPending(ctx, 100); err != nil {
		t.Fatalf("yoklama: %v", err)
	}

	if got := e.orderStatus(t, ord.ID); got != "PENDING" {
		t.Fatalf("durum = %s — sağlayıcının durumu siparişe yazılmış", got)
	}
	var cancelledAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT cancelled_at FROM orders WHERE id=$1`, ord.ID).
		Scan(&cancelledAt); err != nil {
		t.Fatal(err)
	}
	if cancelledAt != nil {
		t.Errorf("cancelled_at yazılmış: %v", *cancelledAt)
	}
	if got := e.balance(t); got != 7500 {
		t.Errorf("bakiye = %d, 7500 bekleniyordu — iade yazılmamalı", got)
	}
}

// TestPollerSkipsForeignActivations
//
// SÖZLEŞME: `GET /activations` HESAP GENELİNDEDİR (prod ve staging aynı
// anahtarı kullanırsa birbirinin kayıtlarını görür). Bize ait olmayan
// aktivasyon sessizce atlanır; başka bir siparişe YAZILMAZ.
func TestPollerSkipsForeignActivations(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()
	ord := e.newPendingOrder(t)

	e.stub.mu.Lock()
	e.stub.activeMessages["yabanci-999"] = []port.RemoteMessage{{
		RemoteID: "otp-x", Code: "0000", Body: "Baskasinin kodu", ReceivedAt: e.clock.Now(),
	}}
	e.stub.mu.Unlock()

	rep, err := e.svc.PollPending(ctx, 100)
	if err != nil {
		t.Fatalf("yoklama: %v", err)
	}
	if rep.Delivered != 0 {
		t.Fatalf("%d teslim yapıldı, 0 bekleniyordu", rep.Delivered)
	}
	if n := e.messageCount(t, ord.ID); n != 0 {
		t.Fatalf("%d mesaj yazıldı — yabancı aktivasyonun kodu bizim siparişimize teslim edilmiş", n)
	}
	if got := e.orderStatus(t, ord.ID); got != "PENDING" {
		t.Errorf("durum = %s, PENDING bekleniyordu", got)
	}
}

// TestPollerLeavesExpiredOrdersToExpirer
//
// SÖZLEŞME: yoklama yalnız `expires_at > now` olan siparişleri tarar. Süresi
// dolmuş sipariş `order-expirer`'ın işidir; yoklama onu "tamamlandı" yapıp
// kullanıcının iadesini yakamaz.
func TestPollerLeavesExpiredOrdersToExpirer(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()
	ord := e.newPendingOrder(t)

	e.clock.Advance(21 * time.Minute)
	e.stub.mu.Lock()
	e.stub.activeMessages[ord.RemoteOrderID] = []port.RemoteMessage{{
		RemoteID: "otp-gec", Code: "7777", Body: "Gec gelen kod", ReceivedAt: e.clock.Now(),
	}}
	e.stub.mu.Unlock()

	rep, err := e.svc.PollPending(ctx, 100)
	if err != nil {
		t.Fatalf("yoklama: %v", err)
	}
	if rep.Scanned != 0 {
		t.Fatalf("süresi dolmuş sipariş yoklama kümesine girdi (%+v)", rep)
	}
	if got := e.orderStatus(t, ord.ID); got != "PENDING" {
		t.Fatalf("durum = %s, PENDING bekleniyordu", got)
	}

	if err := e.svc.Expire(ctx, ord.ID); err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if got := e.orderStatus(t, ord.ID); got != "REFUNDED" {
		t.Errorf("durum = %s, REFUNDED bekleniyordu", got)
	}
	if got := e.balance(t); got != 10000 {
		t.Errorf("bakiye = %d, 10000 bekleniyordu (tam iade)", got)
	}
}
