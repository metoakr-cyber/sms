//go:build integration

package order_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	ordersvc "github.com/ikmetrik/sms-platform/api/internal/service/order"
)

// TestFakeWebhookCannotCompleteOrder — KK-410.
//
// 🔴 ÜRETİME ALMADAN ÖNCE GEÇMESİ ZORUNLU TEST.
//
// SENARYO: saldırgan webhook URL'ini ele geçirdi ve gövdesinde GEÇERLİ
// görünen bir kod olan sahte bir bildirim gönderdi.
//
// BEKLENEN: sipariş TAMAMLANMAZ. Kod sağlayıcıdan teyit edilir; sağlayıcıda
// kod yoksa durum DEĞİŞMEZ.
//
// Bu test geçmezse webhook ucu bedava kod dağıtan bir kapıdır: saldırgan
// numara alır, sahte webhook gönderir, sipariş "tamamlandı" olur ve iade
// hakkı yanar.
func TestFakeWebhookCannotCompleteOrder(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()

	q := e.makeQuote(t, 2500)
	ord, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}

	// Sağlayıcı BEKLEMEDE diyor — yani gerçekte kod GELMEDİ.
	// (stubProvider.GetStatus zaten StateWaiting döndürüyor.)

	// SAHTE WEBHOOK: gövdede kusursuz görünen bir kod var.
	fake, _ := json.Marshal(map[string]any{
		"activationId": ord.RemoteOrderID,
		"id":           "sahte-1",
		"phoneFrom":    "89854",
		"service":      "wa",
		"text":         "Your code is 99999",
		"code":         "99999",
		"country":      62,
		"receivedAt":   e.clock.Now().Format(time.RFC3339),
	})

	if err := e.svc.HandleWebhook(ctx, e.provID, fake); err != nil {
		t.Fatalf("webhook işlenirken hata: %v", err)
	}

	// (1) DURUM DEĞİŞMEMELİ.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM orders WHERE id=$1`, ord.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "PENDING" {
		t.Fatalf("sahte webhook siparişi %s yaptı — teyit atlanmış, "+
			"URL'i bilen herkes bedava kod alabilir", status)
	}

	// (2) SAHTE KOD KAYDEDİLMEMELİ.
	var msgs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM order_messages WHERE order_id=$1`, ord.ID).Scan(&msgs); err != nil {
		t.Fatal(err)
	}
	if msgs != 0 {
		t.Fatalf("%d sahte mesaj kaydedildi — gövdedeki koda güvenilmiş", msgs)
	}

	// (3) Bakiye DEĞİŞMEMELİ (yanlış bir iade/tahsilat tetiklenmemiş).
	if got := e.balance(t); got != 7500 {
		t.Errorf("bakiye = %d, beklenen 7500", got)
	}
}

// TestWebhookForUnknownActivationIsIgnored — FR-410/3.
//
// Webhook URL slotları HESAP GENELİDİR: prod ve staging aynı hesabı
// kullanıyorsa her ortam tüm hesabın olaylarını alır. Tanınmayan aktivasyon
// NORMAL bir durumdur — hata değil, alarm değil.
func TestWebhookForUnknownActivationIsIgnored(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()

	raw, _ := json.Marshal(map[string]any{
		"activationId": "999999999",
		"code":         "1234",
		"text":         "Kod 1234",
		"receivedAt":   e.clock.Now().Format(time.RFC3339),
	})
	if err := e.svc.HandleWebhook(ctx, e.provID, raw); err != nil {
		t.Fatalf("bilinmeyen aktivasyon hata üretti: %v — "+
			"başka bir ortamın bildirimi bizim log'umuzu kirletmemeli", err)
	}
}

// TestWebhookWithVerifiedCodeCompletesOrder
//
// Teyit BAŞARILI olduğunda akış çalışmalı — güvenlik kontrolü, çalışan bir
// sistemi engellememeli.
func TestWebhookWithVerifiedCodeCompletesOrder(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()

	q := e.makeQuote(t, 2500)
	ord, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}

	// Sağlayıcı artık kodu DOĞRULUYOR.
	e.stub.mu.Lock()
	e.stub.statusMessages = []port.RemoteMessage{{
		RemoteID: "otp-77", Code: "4455", Body: "Kodunuz 4455",
		ReceivedAt: e.clock.Now(),
	}}
	e.stub.mu.Unlock()

	raw, _ := json.Marshal(map[string]any{
		"activationId": ord.RemoteOrderID,
		"id":           "otp-77",
		"text":         "Kodunuz 4455",
		"receivedAt":   e.clock.Now().Format(time.RFC3339),
	})
	if err := e.svc.HandleWebhook(ctx, e.provID, raw); err != nil {
		t.Fatalf("webhook: %v", err)
	}

	var status, code string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM orders WHERE id=$1`, ord.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "COMPLETED" {
		t.Fatalf("teyit edilmiş kodla sipariş %s kaldı", status)
	}
	if err := pool.QueryRow(ctx,
		`SELECT code FROM order_messages WHERE order_id=$1`, ord.ID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	// KOD SAĞLAYICIDAN gelir, webhook gövdesinden DEĞİL.
	if code != "4455" {
		t.Errorf("kaydedilen kod = %q, sağlayıcının verdiği %q olmalı", code, "4455")
	}
}

// TestWebhookBodyCodeIsIgnored
//
// SÖZLEŞME: gövdedeki `code` alanı KULLANILMAZ. Sağlayıcı farklı bir kod
// döndürüyorsa SAĞLAYICININKİ yazılır.
func TestWebhookBodyCodeIsIgnored(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()

	q := e.makeQuote(t, 2500)
	ord, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}

	e.stub.mu.Lock()
	e.stub.statusMessages = []port.RemoteMessage{{
		RemoteID: "otp-gercek", Code: "1111", Body: "Gerçek kod 1111",
		ReceivedAt: e.clock.Now(),
	}}
	e.stub.mu.Unlock()

	// Gövde BAŞKA bir kod iddia ediyor.
	raw, _ := json.Marshal(map[string]any{
		"activationId": ord.RemoteOrderID,
		"id":           "otp-gercek",
		"code":         "9999",
		"text":         "Sahte kod 9999",
		"receivedAt":   e.clock.Now().Format(time.RFC3339),
	})
	if err := e.svc.HandleWebhook(ctx, e.provID, raw); err != nil {
		t.Fatal(err)
	}

	var code, body string
	if err := pool.QueryRow(ctx,
		`SELECT code, body FROM order_messages WHERE order_id=$1`, ord.ID).Scan(&code, &body); err != nil {
		t.Fatal(err)
	}
	if code == "9999" {
		t.Fatal("gövdedeki kod kaydedildi — sağlayıcı teyidi yok sayılmış")
	}
	if code != "1111" {
		t.Errorf("kod = %q, sağlayıcının verdiği 1111 olmalı", code)
	}
	if body != "Gerçek kod 1111" {
		t.Errorf("gövde metni = %q, sağlayıcınınki olmalı", body)
	}
}

var _ = fmt.Sprintf
var _ = db.OrderStatusPENDING

// TestWebhookRetriesDoNotDuplicateMessage — F5
//
// 🔴 WEBHOOK AT-LEAST-ONCE ÇALIŞIR; DEDUP ANAHTARI HAREKETLİ OLAMAZ.
//
// `hashMessage` sıfır `ReceivedAt`i bilerek sabit boş dizeye düşürüyor —
// anahtar kararlı olsun diye. `HandleWebhook` ise `DeliverMessages`ten ÖNCE o
// sıfırı `now()` ile dolduruyordu ve her yeniden denemede farklı bir `now()`
// farklı bir hash üretiyordu. Ölçüldü: TEK SMS 4 satır, dördü de "yeni"
// sayılıp SSE'den ayrı ayrı yayınlandı — kullanıcı aynı kodu dört kez gördü.
//
// Gövdedeki `receivedAt` de çözüm değil: webhook imzasızdır (değişmez #23) ve
// URL'i bilen biri her istekte farklı bir zaman göndererek aynı SMS'i
// defalarca yeni satıra çevirebilirdi.
func TestWebhookRetriesDoNotDuplicateMessage(t *testing.T) {
	e := setup(t, 10000)
	ctx := context.Background()

	q := e.makeQuote(t, 2500)
	ord, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}

	// SAĞLAYICI TEYİDİ ZAMANSIZ mesaj döner: `parseTime` RFC 3339 dışını sıfır
	// döndürür (docs/provider-herosms.md §7.3 — üç kaynak, üç tarih biçimi).
	e.stub.mu.Lock()
	e.stub.statusMessages = []port.RemoteMessage{
		{Code: "555555", Body: "Kodunuz 555555", Sender: "SERVIS"}, // ReceivedAt SIFIR
	}
	e.stub.mu.Unlock()

	// Aynı bildirim sekiz kez gelebilir; dördü yeter. Her denemede saat ilerler
	// — hareketli çıpanın hatayı ürettiği koşul tam olarak budur.
	body, _ := json.Marshal(map[string]any{
		"activationId": ord.RemoteOrderID,
		"phoneFrom":    "89854",
		"service":      "wa",
		"code":         "555555",
		"text":         "Kodunuz 555555",
		"receivedAt":   "8 Eylül 2026 12:00", // RFC 3339 DEĞİL: ayrıştırılamaz
	})
	for i := 0; i < 4; i++ {
		e.clock.Advance(30 * time.Second)
		if err := e.svc.HandleWebhook(ctx, e.provID, body); err != nil {
			t.Fatalf("%d. bildirim: %v", i+1, err)
		}
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM order_messages WHERE order_id=$1`, ord.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("🔴 TEK SMS %d satır oldu — webhook yeniden denemeleri dedup "+
			"anahtarını her seferinde değiştiriyor", n)
	}
	if k := e.pub.kodIcerenOlaylar("555555"); k != 1 {
		t.Errorf("aynı kod SSE'den %d kez yayınlandı, beklenen 1 — kullanıcı "+
			"aynı mesajı defalarca görürdü", k)
	}
}
