//go:build integration

package order_test

// KİRALIK SİPARİŞ YAŞAM DÖNGÜSÜ.
//
// Buradaki her test bir PARA KAYBINI ölçüyor. Düzeltmeden önceki davranış:
//
//	· İlk SMS geldiğinde kiralık sipariş PENDING → COMPLETED oluyor, hemen
//	  Finish() gönderiliyordu: kullanıcının 30 gün için ödediği numara ilk
//	  mesajda ölüyor, kalan sürenin tamamı yanıyordu.
//	· Hiç mesaj gelmeyen bir kiralık 30. günde `order-expirer`a düşüp TAM
//	  İADE alıyordu: kullanıcı 30 gün numara kullanıp sıfır lira ödüyor,
//	  sağlayıcı maliyeti bizde kalıyordu. Tekrarlanabilir bir sızıntı.
//
// İki yön de aynı eksik ayrımdan doğuyordu: ürün türü hiç sorulmuyordu.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	ordersvc "github.com/ikmetrik/sms-platform/api/internal/service/order"
)

// waitForBackground arka plandaki kapatma goroutine'lerinin
// (scheduleProviderClose) bitmesini bekler.
//
// SABİT UYKU BİLEREK SEÇİLDİ: buradaki iddiaların çoğu bir olayın
// GERÇEKLEŞMEMESİ üzerine ("sağlayıcıya kapatma çağrısı gitmedi"). Koşul
// bekleyen bir yardımcı olumsuz iddiayı ölçemez — hiç gerçekleşmeyecek bir
// koşulu zaman aşımına kadar bekler. Süre cömert: goroutine yalnız bir stub
// çağrısı yapıyor.
func waitForBackground() { time.Sleep(300 * time.Millisecond) }

// assertReconciled mutabakat değişmezini doğrular: Σ defter == bakiye.
//
// Her para testinin sonunda çağrılır. Bir kiralık yolu bakiyeyi deftere
// yazmadan değiştirseydi (değişmez #2 ihlali) yalnız bu kontrol yakalardı.
func assertReconciled(t *testing.T, userID int64) {
	t.Helper()
	ctx := context.Background()
	var defter, bakiye int64
	if err := pool.QueryRow(ctx,
		`SELECT coalesce(sum(amount_minor),0) FROM ledger_entries WHERE user_id=$1`,
		userID).Scan(&defter); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT balance_minor FROM users WHERE id=$1`, userID).Scan(&bakiye); err != nil {
		t.Fatal(err)
	}
	if defter != bakiye {
		t.Errorf("MUTABAKAT BOZUK: Σ defter = %d, bakiye = %d (fark %d)",
			defter, bakiye, bakiye-defter)
	}
}

// rentalHours test kiralamasının süresi (30 gün) — HeroSMS'in kabul ettiği
// listede olan bir değer.
const rentalHours = 720

/* ═══════════════════════ Yardımcılar ═══════════════════════ */

// makeRentalProduct kiralık bir ürün satırı yazar ve kimliğini döner.
//
// `duration_minutes` DAKİKA cinsindendir; servis katmanı saate çevirir.
// Testin bu çevrimi taklit etmemesi önemli: 60 kat hatanın yakalanacağı yer
// tam olarak burasıdır.
func makeRentalProduct(t *testing.T, hours int) int64 {
	t.Helper()
	var svcID, ctryID, prodID int64
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT id FROM services WHERE code='wa'`).Scan(&svcID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM countries WHERE iso2='TR'`).Scan(&ctryID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO products (kind, service_id, country_id, verification_type, duration_minutes)
		VALUES ('SMS_RENTAL', $1, $2, 'sms', $3) RETURNING id`,
		svcID, ctryID, hours*60).Scan(&prodID); err != nil {
		t.Fatal(err)
	}
	return prodID
}

// makeQuoteFor belirli bir ürün için teklif satırı yazar.
func (e *env) makeQuoteFor(t *testing.T, productID int64, sellMinor int64) uuid.UUID {
	t.Helper()
	var pubID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO price_quotes
			(user_id, product_id, provider_id, cost_micro, fx_rate, margin_percent,
			 sell_price_minor, stock_at_quote, expires_at)
		VALUES ($1,$2,$3,1000000,43.20,40,$4,500,$5)
		RETURNING public_id`,
		e.userID, productID, e.provID, sellMinor, e.clock.Now().Add(2*time.Minute)).Scan(&pubID)
	if err != nil {
		t.Fatal(err)
	}
	return pubID
}

// buyRental kiralık bir sipariş oluşturur.
func (e *env) buyRental(t *testing.T, sellMinor int64) db.Order {
	t.Helper()
	prodID := makeRentalProduct(t, rentalHours)
	q := e.makeQuoteFor(t, prodID, sellMinor)
	ord, err := e.svc.Create(context.Background(), ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatalf("kiralık satın alınamadı: %v", err)
	}
	if ord.ProductKind != db.ProductKindSMSRENTAL {
		t.Fatalf("sipariş product_kind = %q, SMS_RENTAL bekleniyordu — ürün türü "+
			"anlık görüntüsü yazılmadan hiçbir kiralık kuralı çalışmaz", ord.ProductKind)
	}
	return ord
}

// reload siparişi veritabanından tazeler.
func (e *env) reload(t *testing.T, id int64) db.Order {
	t.Helper()
	ord, err := e.q.GetOrderForUpdate(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ord
}

// refundCount kullanıcının REFUND defter satırı sayısı.
func (e *env) refundCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ledger_entries WHERE user_id=$1 AND entry_type='REFUND'`,
		e.userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// balanceMinor kullanıcının güncel bakiyesi.
func (e *env) balanceMinor(t *testing.T) int64 {
	t.Helper()
	var b int64
	if err := pool.QueryRow(context.Background(),
		`SELECT balance_minor FROM users WHERE id=$1`, e.userID).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

// msg test mesajı üretir. RemoteID BOŞ bırakılır — gerçek adaptör de sağlayıcı
// kimlik vermediğinde öyle yapar ve dedup içerik hash'ine düşer.
func msg(code, body string, at time.Time) port.RemoteMessage {
	return port.RemoteMessage{Code: code, Body: body, Sender: "SERVIS", ReceivedAt: at}
}

/* ═══════════════════ 1. İlk mesaj siparişi KAPATMAZ ═══════════════════ */

// TestRentalFirstMessageDoesNotCloseAtProvider
//
// 🔴 ASIL HATANIN REGRESYON TESTİ.
//
// Kiralıkta ürün SÜREdir. İlk SMS dönemin başıdır, sonu değil: sipariş
// terminal olmamalı ve sağlayıcıya HİÇBİR kapatma çağrısı gitmemeli.
func TestRentalFirstMessageDoesNotCloseAtProvider(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)

	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{msg("111111", "Kod 111111", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}

	got := e.reload(t, ord.ID)
	if got.Status != db.OrderStatusACTIVE {
		t.Fatalf("durum = %q, ACTIVE bekleniyordu — kiralık ilk mesajda "+
			"tamamlanırsa kalan kira süresi yanar", got.Status)
	}

	// Kapatma arka planda goroutine ile denenir; bir tur bekleyip gerçekten
	// hiç çağrı gitmediğini görürüz.
	waitForBackground()

	if n := e.stub.finishes.Load(); n != 0 {
		t.Errorf("Finish() %d kez çağrıldı — dönemi süren numara sağlayıcıda "+
			"kapatıldı, kullanıcının ödediği süre yandı", n)
	}
	if n := e.stub.cancels.Load(); n != 0 {
		t.Errorf("Cancel() %d kez çağrıldı — dönemi süren numara kapatıldı", n)
	}
	if got.ProviderClosedAt != nil {
		t.Error("provider_closed_at yazıldı — numara sağlayıcıda açık kalmalıydı")
	}
	if e.refundCount(t) != 0 {
		t.Error("mesaj gelen kiralığa iade yazıldı")
	}
}

/* ═══════════════════ 2. Dönem boyunca ÇOKLU mesaj ═══════════════════ */

// TestRentalReceivesEveryMessageInPeriod
//
// Kiralığın satış vaadi budur: numara dönem boyunca gelen HER mesajı gösterir.
// İkinci ve üçüncü mesaj da `order_messages` satırına yazılmalı ve SSE'de
// yayınlanmalıdır (FR-415).
func TestRentalReceivesEveryMessageInPeriod(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)

	kodlar := []string{"111111", "222222", "333333"}
	for i, k := range kodlar {
		e.clock.Advance(time.Duration(i) * time.Hour)
		if err := e.svc.DeliverMessages(context.Background(), ord.ID,
			[]port.RemoteMessage{msg(k, "Kod "+k, e.clock.Now())}); err != nil {
			t.Fatalf("%d. mesaj kaydedilemedi: %v", i+1, err)
		}
	}

	if n := e.messageCount(t, ord.ID); n != 3 {
		t.Fatalf("mesaj sayısı = %d, beklenen 3 — kiralık ilk mesajdan sonra da "+
			"mesaj almalı", n)
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusACTIVE {
		t.Errorf("durum = %q, ACTIVE bekleniyordu", got.Status)
	}

	// SSE ilk koddan sonra da yayın yapmalı: üç kodun üçü de abonenin
	// gördüğü olaylarda olmalı.
	for _, k := range kodlar {
		if e.pub.kodIcerenOlaylar(k) == 0 {
			t.Errorf("%s kodu hiç yayınlanmadı — kullanıcı ekranda göremezdi", k)
		}
	}

	waitForBackground()
	if e.stub.finishes.Load()+e.stub.cancels.Load() != 0 {
		t.Error("dönem sürerken sağlayıcıya kapatma çağrısı gitti")
	}
}

/* ═══════════════════ 3. Dedup ═══════════════════ */

// TestSameMessageFromBothPathsIsStoredOnce
//
// AYNI SMS İKİ SATIR OLMAZ. Sağlayıcı mesaj kimliği vermediğinde adaptör
// UYDURMA anahtar üretmez (`{id}:last`, `{id}:0`); anahtar tek yerde, içerikten
// türetilir. Eskiden yoklama `{id}:0`, webhook teyidi `{id}:last` üretiyordu ve
// tek SMS kullanıcıya iki kez görünüyordu.
//
// Kiralıkta hata daha da ağırdı: `{id}:last` SABİT bir anahtar olduğu için 2.
// ve sonraki mesajlar sessizce yutuluyordu.
func TestSameMessageFromBothPathsIsStoredOnce(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)

	at := e.clock.Now()
	first := msg("111111", "Kod 111111", at)

	// Aynı mesaj iki kez teslim edilir (yoklama + webhook teyidi).
	for i := 0; i < 2; i++ {
		if err := e.svc.DeliverMessages(context.Background(), ord.ID,
			[]port.RemoteMessage{first}); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.messageCount(t, ord.ID); n != 1 {
		t.Fatalf("mesaj sayısı = %d, beklenen 1 — aynı SMS iki satır oldu", n)
	}

	// Aynı an, FARKLI içerik → ayrı mesaj. Anahtar zamandan ibaret olsaydı
	// ikinci mesaj yutulurdu.
	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{msg("222222", "Kod 222222", at)}); err != nil {
		t.Fatal(err)
	}
	if n := e.messageCount(t, ord.ID); n != 2 {
		t.Fatalf("mesaj sayısı = %d, beklenen 2 — farklı içerikli mesaj yutuldu", n)
	}

	// Aynı an, farklı saat dilimi gösterimi → AYNI mesaj. `/otp/last` "+00:00",
	// `ListActive` "Z" döndürüyor; UTC'ye normalize edilmezse dedup iki yol
	// arasında çalışmaz.
	other := first
	other.ReceivedAt = at.In(time.FixedZone("UTC+3", 3*3600))
	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{other}); err != nil {
		t.Fatal(err)
	}
	if n := e.messageCount(t, ord.ID); n != 2 {
		t.Fatalf("mesaj sayısı = %d, beklenen 2 — aynı an farklı saat diliminde "+
			"gösterildiğinde yeni satır yazıldı", n)
	}
}

/* ═══════════════════ 4-5. Dönem sonu ═══════════════════ */

// TestRentalEndOfPeriodFinishesAtProvider
//
// Dönem sonunda sipariş COMPLETED olur ve sağlayıcıda `Finish()` ile kapanır.
// `Cancel()` DEĞİL: iade talep edilecek bir şey yok ve sağlayıcının ücretsiz
// iptal penceresi 1. günde kapandığı için Cancel garantili reddedilirdi.
func TestRentalEndOfPeriodFinishesAtProvider(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)

	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{msg("111111", "Kod 111111", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}

	// Kira dönemi biter.
	e.clock.Advance(rentalHours*time.Hour + time.Minute)

	// `rental-closer` işinin sorgusu bu siparişi görmeli.
	rows, err := e.q.ListEndedRentals(context.Background(), db.ListEndedRentalsParams{
		Now: e.clock.Now(), Lim: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != ord.ID {
		t.Fatalf("ListEndedRentals %d satır döndü, dönemi biten kiralık bulunamadı", len(rows))
	}

	if err := e.svc.EndRental(context.Background(), ord.ID); err != nil {
		t.Fatal(err)
	}
	waitForBackground()

	got := e.reload(t, ord.ID)
	if got.Status != db.OrderStatusCOMPLETED {
		t.Errorf("durum = %q, COMPLETED bekleniyordu", got.Status)
	}
	if n := e.stub.finishes.Load(); n != 1 {
		t.Errorf("Finish() %d kez çağrıldı, beklenen 1", n)
	}
	if n := e.stub.cancels.Load(); n != 0 {
		t.Errorf("Cancel() %d kez çağrıldı — dönem sonunda iade TALEP EDİLMEZ", n)
	}
	if got.ProviderClosedAt == nil {
		t.Error("terminal sipariş sağlayıcıda kapatılmadı (FR-412)")
	}
}

// TestRentalNeverAutoRefundsAtEndOfPeriod
//
// 🔴 EN PAHALI SENARYO. Hiç mesaj gelmemiş bir kiralık, dönem sonunda TAM
// İADE almamalıdır.
//
// Eski davranış: sipariş 30 gün PENDING kalır, `order-expirer` onu görür ve
// `price_paid_minor` kadar iade yazardı. Sağlayıcıdan o parayı geri almanın
// yolu yoktur (ücretsiz iptal penceresi 1. günde kapandı), yani iadenin %100'ü
// bizim cebimizden çıkardı — üstelik "kiralık al, 30. günde paranı al" diye
// tekrarlanabilir bir sızıntı olarak.
func TestRentalNeverAutoRefundsAtEndOfPeriod(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)
	afterPurchase := e.balanceMinor(t)

	// Dönem, tek bir mesaj gelmeden biter.
	e.clock.Advance(rentalHours*time.Hour + time.Minute)

	// (a) `order-expirer`ın sorgusu kiralığı GÖRMEMELİ.
	expired, err := e.q.ListExpiredPendingOrders(context.Background(),
		db.ListExpiredPendingOrdersParams{Now: e.clock.Now(), Lim: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range expired {
		if o.ID == ord.ID {
			t.Fatal("kiralık sipariş order-expirer kuyruğunda — o kuyruğun her " +
				"satırı TAM İADE demektir")
		}
	}

	// (b) İkinci savunma hattı: `Expire` doğrudan çağrılsa bile dokunmamalı.
	if err := e.svc.Expire(context.Background(), ord.ID); err != nil {
		t.Fatal(err)
	}
	if e.refundCount(t) != 0 {
		t.Fatal("Expire kiralığa iade yazdı — ikinci savunma hattı çalışmıyor")
	}

	// (c) Doğru yol: EndRental. Deftere HİÇBİR kayıt yazmaz.
	if err := e.svc.EndRental(context.Background(), ord.ID); err != nil {
		t.Fatal(err)
	}
	waitForBackground()

	if n := e.refundCount(t); n != 0 {
		t.Errorf("REFUND defter satırı = %d, beklenen 0", n)
	}
	if b := e.balanceMinor(t); b != afterPurchase {
		t.Errorf("bakiye %d → %d değişti; kira dönemi sonu bir para hareketi değildir",
			afterPurchase, b)
	}

	got := e.reload(t, ord.ID)
	if got.Status != db.OrderStatusCOMPLETED {
		t.Errorf("durum = %q, COMPLETED bekleniyordu", got.Status)
	}
	// Mesaj hiç gelmedi ama yine de Finish: pencere kapalı, Cancel garantili
	// reddedilir ve `provider_refund_denied_total` metriğini kirletirdi.
	if n := e.stub.finishes.Load(); n != 1 {
		t.Errorf("Finish() %d kez çağrıldı, beklenen 1", n)
	}
	if n := e.stub.cancels.Load(); n != 0 {
		t.Errorf("Cancel() %d kez çağrıldı — 30. günde iptal penceresi çoktan kapalı", n)
	}
}

/* ═══════════════════ 6-8. Kullanıcı iptali ═══════════════════ */

// TestRentalCancelInsideWindowRefundsExactlyOnce
//
// EŞZAMANLILIK TESTİ (para yolu zorunluluğu): pencere içinde N paralel iptal
// isteği TEK iade yazar ve sağlayıcıya TEK Cancel gider.
func TestRentalCancelInsideWindowRefundsExactlyOnce(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)
	beforeCancel := e.balanceMinor(t)

	// 5. dakika: asgari bekleme (120 sn) geçti, iade penceresi (15 dk) açık.
	e.clock.Advance(5 * time.Minute)

	const N = 16
	var g errgroup.Group
	var basarili int32
	var mu sync.Mutex
	for i := 0; i < N; i++ {
		g.Go(func() error {
			_, err := e.svc.Cancel(context.Background(), e.userID, ord.PublicID)
			if err == nil {
				mu.Lock()
				basarili++
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	waitForBackground()

	if basarili != 1 {
		t.Errorf("%d iptal başarılı oldu, beklenen 1", basarili)
	}
	if n := e.refundCount(t); n != 1 {
		t.Fatalf("REFUND defter satırı = %d, beklenen TAM 1 — çift iade", n)
	}
	if got, want := e.balanceMinor(t), beforeCancel+45_000; got != want {
		t.Errorf("bakiye = %d, beklenen %d (tam iade)", got, want)
	}
	if n := e.stub.cancels.Load(); n != 1 {
		t.Errorf("sağlayıcıya %d Cancel gitti, beklenen 1", n)
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusREFUNDED {
		t.Errorf("durum = %q, REFUNDED bekleniyordu", got.Status)
	}
}

// TestRentalCancelOutsideWindowWritesNoLedgerEntry
//
// Pencere kapandıktan sonra iptal REDDEDİLİR ve deftere hiçbir şey yazılmaz.
// Sağlayıcı o iadeyi vermeyeceği için yazacağımız her kuruş net giderdir.
func TestRentalCancelOutsideWindowWritesNoLedgerEntry(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)
	before := e.balanceMinor(t)

	// Pencere 15 dakika; 30. dakikada kapalı.
	e.clock.Advance(30 * time.Minute)

	_, err := e.svc.Cancel(context.Background(), e.userID, ord.PublicID)
	if err == nil {
		t.Fatal("pencere kapalıyken iptal KABUL EDİLDİ — iadenin tamamı bizim giderimiz olurdu")
	}
	if !errors.Is(err, apperr.ErrOrderNotCancellable) {
		t.Errorf("hata = %v, ErrOrderNotCancellable bekleniyordu", err)
	}
	if n := e.refundCount(t); n != 0 {
		t.Errorf("REFUND defter satırı = %d, beklenen 0", n)
	}
	if b := e.balanceMinor(t); b != before {
		t.Errorf("bakiye %d → %d değişti", before, b)
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusPENDING {
		t.Errorf("durum = %q, PENDING kalmalıydı", got.Status)
	}
}

// TestRentalWithMessageCannotBeCancelledForRefund
//
// Mesaj gelmiş bir kiralık iade için iptal edilemez — pencere hâlâ açık olsa
// bile. Aksi hâlde kullanıcı hem kodu hem parayı alırdı; sağlayıcı da o iadeyi
// `OTP_RECEIVED` ile geri çevirir, yani zarar tümüyle bizde kalırdı.
func TestRentalWithMessageCannotBeCancelledForRefund(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)

	e.clock.Advance(5 * time.Minute) // pencere AÇIK
	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{msg("111111", "Kod 111111", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}

	_, err := e.svc.Cancel(context.Background(), e.userID, ord.PublicID)
	if err == nil {
		t.Fatal("mesaj gelmiş kiralık iade için iptal edildi — kullanıcı hem kodu " +
			"hem parayı alırdı")
	}
	if n := e.refundCount(t); n != 0 {
		t.Errorf("REFUND defter satırı = %d, beklenen 0", n)
	}
}

/* ═══════════════════ 9. Eşzamanlılık: dönem sonu × kapatma ═══════════════════ */

// TestRentalEndRaceWithProviderCloseHasSingleEffect
//
// Dönem sonu işi (`rental-closer`) ile kapatma işi (`activation-reaper`) aynı
// siparişi aynı anda görürse: sağlayıcıya TEK çağrı gider, deftere hiçbir şey
// yazılmaz ve durum bir kez değişir.
//
// `ClaimOrderClose` koşullu UPDATE'i yarışı seri hâle getirir; kilit değil
// SAHİPLENMEdir, çünkü kapatma bir HTTP çağrısı içerir ve dış çağrı
// transaction içinde yapılamaz (değişmez #5).
func TestRentalEndRaceWithProviderCloseHasSingleEffect(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)
	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{msg("111111", "Kod 111111", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}
	before := e.balanceMinor(t)
	e.clock.Advance(rentalHours*time.Hour + time.Minute)

	var g errgroup.Group
	for i := 0; i < 8; i++ {
		g.Go(func() error {
			_ = e.svc.EndRental(context.Background(), ord.ID)
			return nil
		})
		g.Go(func() error {
			_ = e.svc.CloseAtProvider(context.Background(), ord.ID)
			return nil
		})
	}
	_ = g.Wait()
	waitForBackground()

	if n := e.stub.finishes.Load() + e.stub.cancels.Load(); n != 1 {
		t.Errorf("sağlayıcıya %d kapatma çağrısı gitti, beklenen TAM 1", n)
	}
	if n := e.refundCount(t); n != 0 {
		t.Errorf("REFUND defter satırı = %d, beklenen 0", n)
	}
	if b := e.balanceMinor(t); b != before {
		t.Errorf("bakiye %d → %d değişti", before, b)
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusCOMPLETED {
		t.Errorf("durum = %q, COMPLETED bekleniyordu", got.Status)
	}
	assertReconciled(t, e.userID)
}

/* ═══════════════════ 10. İade kuyruğu ═══════════════════ */

// TestRentalNeverEntersProviderRefundRetryQueue
//
// Dönemi süren ya da tamamlanmış bir kiralık, sağlayıcı iade kuyruğuna
// GİRMEMELİDİR. Girseydi `provider-refund-retry` iki dakikada bir
// `CloseAtProvider` çağırır, mesajı olmayan bir kiralığa `Cancel()` gönderir
// ve kullanıcının parası iade edilmemişken numarayı öldürürdü.
func TestRentalNeverEntersProviderRefundRetryQueue(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)

	kuyruktaMi := func(asama string) {
		t.Helper()
		simdi := e.clock.Now()
		rows, err := e.q.ListRefundRetryOrders(context.Background(),
			db.ListRefundRetryOrdersParams{Now: &simdi, Lim: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range rows {
			if o.ID == ord.ID {
				t.Fatalf("%s: kiralık sipariş iade kuyruğunda — refund-retry ona "+
					"Cancel() gönderip canlı numarayı öldürürdü", asama)
			}
		}
	}

	kuyruktaMi("satın alma sonrası")

	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{msg("111111", "Kod 111111", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}
	kuyruktaMi("ilk mesaj sonrası")

	e.clock.Advance(rentalHours*time.Hour + time.Minute)
	if err := e.svc.EndRental(context.Background(), ord.ID); err != nil {
		t.Fatal(err)
	}
	waitForBackground()
	kuyruktaMi("dönem sonu sonrası")
}

/* ═══════════════════ 11. Aktivasyon regresyonu ═══════════════════ */

// TestActivationLifecycleIsUnchangedByRentalWork
//
// Kiralık için yapılan hiçbir değişiklik aktivasyon davranışını
// değiştirmemelidir: ilk kodda COMPLETED, ardından `Finish()`, iade ekseni
// NOT_APPLICABLE.
func TestActivationLifecycleIsUnchangedByRentalWork(t *testing.T) {
	e := setup(t, 100_000)
	q := e.makeQuote(t, 25_000)
	ord, err := e.svc.Create(context.Background(), ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err != nil {
		t.Fatal(err)
	}
	if ord.ProductKind != db.ProductKindSMSACTIVATION {
		t.Fatalf("product_kind = %q, SMS_ACTIVATION bekleniyordu", ord.ProductKind)
	}
	if ord.RefundableUntil != nil {
		t.Error("aktivasyona iade penceresi üst sınırı yazıldı — bugünkü davranış " +
			"değişmemeliydi")
	}

	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{msg("999999", "Kod 999999", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}
	waitForBackground()

	got := e.reload(t, ord.ID)
	if got.Status != db.OrderStatusCOMPLETED {
		t.Fatalf("durum = %q, COMPLETED bekleniyordu — aktivasyonda ilk kod ürünün "+
			"tamamıdır", got.Status)
	}
	if n := e.stub.finishes.Load(); n != 1 {
		t.Errorf("Finish() %d kez çağrıldı, beklenen 1", n)
	}
	if n := e.stub.cancels.Load(); n != 0 {
		t.Errorf("Cancel() %d kez çağrıldı, beklenen 0", n)
	}
	if got.ProviderRefundStatus != db.RefundStatusNOTAPPLICABLE {
		t.Errorf("iade ekseni = %q, NOT_APPLICABLE bekleniyordu", got.ProviderRefundStatus)
	}
	if e.refundCount(t) != 0 {
		t.Error("kodu gelen aktivasyona iade yazıldı")
	}
}

/* ═══════════════════ 12-13. Yoklama ═══════════════════ */

// TestRentalsDoNotStarveActivationPoll
//
// Kiralıklar aktivasyon yoklama kuyruğunu İŞGAL ETMEZ.
//
// `ListPendingOrdersForPoll` `created_at`e göre sıralı ve LIMIT'li. Kiralık 30
// gün PENDING kalabildiği için kuyruğun başını günlerce tutar; aktivasyonlar
// hiç yoklanmaz, webhook kaybolduğunda kod hiç gelmez ve order-expirer tam
// iade yazar — hem sağlayıcı maliyeti hem satış kaybı.
func TestRentalsDoNotStarveActivationPoll(t *testing.T) {
	e := setup(t, 200_000)

	// Kiralık ÖNCE oluşturulur: sıralamada başta olurdu.
	rental := e.buyRental(t, 45_000)
	e.clock.Advance(time.Minute)
	act, err := e.svc.Create(context.Background(),
		ordersvc.CreateInput{UserID: e.userID, QuoteID: e.makeQuote(t, 25_000)})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := e.q.ListPendingOrdersForPoll(context.Background(),
		db.ListPendingOrdersForPollParams{Now: e.clock.Now(), Lim: 100})
	if err != nil {
		t.Fatal(err)
	}
	var aktivasyonVar, kiralikVar bool
	for _, o := range rows {
		if o.ID == act.ID {
			aktivasyonVar = true
		}
		if o.ID == rental.ID {
			kiralikVar = true
		}
	}
	if !aktivasyonVar {
		t.Error("aktivasyon yoklama turunda YOK")
	}
	if kiralikVar {
		t.Error("kiralık aktivasyon yoklama turunda — kendi turu (rental-poller) var")
	}

	// Kiralık kendi sorgusunda görünmeli.
	rentalRows, err := e.q.ListActiveRentalsForPoll(context.Background(),
		db.ListActiveRentalsForPollParams{Now: e.clock.Now(), Lim: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(rentalRows) != 1 || rentalRows[0].ID != rental.ID {
		t.Fatalf("ListActiveRentalsForPoll %d satır döndü, kiralık bulunamadı — "+
			"kiralık hiç yoklanmazdı", len(rentalRows))
	}
}

// TestRentalPollerDeliversLaterMessages
//
// Kiralık yoklaması İLK MESAJLA BİTMEZ: sipariş ACTIVE olduktan sonra da
// taranmalı ve toplu uçtan (otpList[]) gelen yeni mesajlar teslim edilmelidir.
func TestRentalPollerDeliversLaterMessages(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)

	// 1. tur: tek mesaj → sipariş ACTIVE olur.
	e.stub.activeMessages[ord.RemoteOrderID] = []port.RemoteMessage{
		msg("111111", "Kod 111111", e.clock.Now()),
	}
	if _, err := e.svc.PollRentals(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusACTIVE {
		t.Fatalf("durum = %q, ACTIVE bekleniyordu", got.Status)
	}

	// 2. tur: sağlayıcı iki mesaj döndürür (otpList birikimlidir).
	e.clock.Advance(2 * time.Hour)
	e.stub.activeMessages[ord.RemoteOrderID] = append(
		e.stub.activeMessages[ord.RemoteOrderID],
		msg("222222", "Kod 222222", e.clock.Now()),
	)
	rep, err := e.svc.PollRentals(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scanned != 1 {
		t.Fatalf("ACTIVE kiralık ikinci turda taranmadı (Scanned=%d) — ilk mesajdan "+
			"sonra gelen kodlar hiç görünmezdi", rep.Scanned)
	}
	if n := e.messageCount(t, ord.ID); n != 2 {
		t.Errorf("mesaj sayısı = %d, beklenen 2", n)
	}
	waitForBackground()
	if e.stub.finishes.Load()+e.stub.cancels.Load() != 0 {
		t.Error("yoklama dönemi süren kiralığı sağlayıcıda kapattı")
	}
}

/* ═══════════════════ 14. Süre sessizce kaybolmaz ═══════════════════ */

// TestRentalDurationCannotBeSilentlyLost
//
// Kiralık ürünün süresi okunamıyorsa satın alma YAPILMAZ.
//
// Eskiden bu blok `if …; err == nil` ile yazılıydı: süre okunamazsa
// `DurationHours` sessizce 0 kalıyor, sağlayıcıya `duration` gönderilmiyor ve
// kullanıcı 30 günlük kiralık fiyatını ödeyip 20 dakikalık bir aktivasyon
// alıyordu.
func TestRentalDurationCannotBeSilentlyLost(t *testing.T) {
	e := setup(t, 100_000)

	// Süresi olmayan bir "kiralık" ürün — bozuk katalog satırı.
	var svcID, ctryID, prodID int64
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT id FROM services WHERE code='wa'`).Scan(&svcID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM countries WHERE iso2='TR'`).Scan(&ctryID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO products (kind, service_id, country_id, verification_type, duration_minutes)
		VALUES ('SMS_RENTAL', $1, $2, 'sms', NULL) RETURNING id`,
		svcID, ctryID).Scan(&prodID); err != nil {
		t.Fatal(err)
	}

	q := e.makeQuoteFor(t, prodID, 45_000)
	if _, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q}); err == nil {
		t.Fatal("süresi olmayan kiralık ürün satın alındı — kullanıcı kiralık " +
			"fiyatını ödeyip aktivasyon alırdı")
	}
	if n := e.stub.purchases.Load(); n != 0 {
		t.Errorf("sağlayıcıya %d Purchase gitti; süre bilinmiyorken çağrı yapılmamalı", n)
	}
	// Para T1'de düşüldü, T2'de geri verilmeli: mutabakat bozulmamalı.
	assertReconciled(t, e.userID)
}

/* ═══════════════════ 15-16. Veritabanı savunma hattı ═══════════════════ */

// TestActivationCannotEnterActiveStatus
//
// 'ACTIVE' yalnız kiralıkta anlamlıdır. Bir aktivasyon o duruma düşerse iade
// işleri onu hiç görmez ve kullanıcının parası sessizce askıda kalır; CHECK
// kısıtı bunu veritabanı düzeyinde durdurur.
func TestActivationCannotEnterActiveStatus(t *testing.T) {
	e := setup(t, 100_000)
	ord, err := e.svc.Create(context.Background(),
		ordersvc.CreateInput{UserID: e.userID, QuoteID: e.makeQuote(t, 25_000)})
	if err != nil {
		t.Fatal(err)
	}

	_, err = pool.Exec(context.Background(),
		`UPDATE orders SET status='ACTIVE' WHERE id=$1`, ord.ID)
	if err == nil {
		t.Fatal("aktivasyon siparişi ACTIVE yapıldı — veritabanı kısıtı çalışmıyor")
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusPENDING {
		t.Errorf("durum = %q, PENDING kalmalıydı", got.Status)
	}
}

// TestActiveRentalCannotBeCancelledByDB
//
// ACTIVE → CANCELLED oku BİLEREK YOKTUR: ilk SMS geldikten sonra iade yolu
// kapalıdır. Kural hem domain'de hem tetikleyicide durur; buradaki test
// tetikleyici hattını sınar.
func TestActiveRentalCannotBeCancelledByDB(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)
	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{msg("111111", "Kod 111111", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusACTIVE {
		t.Fatalf("durum = %q, ACTIVE bekleniyordu", got.Status)
	}

	for _, hedef := range []string{"CANCELLED", "REFUNDED", "FAILED", "PENDING"} {
		_, err := pool.Exec(context.Background(),
			fmt.Sprintf(`UPDATE orders SET status='%s', cancelled_at=now(), refunded_at=now() WHERE id=$1`, hedef),
			ord.ID)
		if err == nil {
			t.Errorf("ACTIVE → %s geçişine izin verildi", hedef)
		}
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusACTIVE {
		t.Errorf("durum = %q, ACTIVE kalmalıydı", got.Status)
	}
}

// TestRentalDetailIsWrittenWithOrder
//
// Kira dönemi kaydı sipariş satırıyla AYNI transaction'da yazılır.
//
// `rental_details` tablosu 00008'den beri duruyordu ama hiçbir kod ona
// yazmıyordu: `duration_hours` sağlayıcı çağrısından sonra çöpe gidiyor,
// `rental_ends_at` diye bir gerçek hiç oluşmuyordu. Kira dönemi üzerine
// kurulacak her rapor "hiç kiralık yok" derdi — para akarken.
func TestRentalDetailIsWrittenWithOrder(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)

	det, err := e.q.GetRentalDetail(context.Background(), ord.ID)
	if err != nil {
		t.Fatalf("kira dönemi kaydı YOK: %v", err)
	}
	if det.DurationHours != rentalHours {
		t.Errorf("duration_hours = %d, beklenen %d (dakika→saat çevrimi tek yerde)",
			det.DurationHours, rentalHours)
	}
	if !det.RentalEndsAt.Equal(ord.ExpiresAt) {
		t.Errorf("rental_ends_at = %v, orders.expires_at = %v — dönem kaydı "+
			"yetkili kaynağın aynası olmalı", det.RentalEndsAt, ord.ExpiresAt)
	}

	// TTL sağlayıcıdan gelmeli (değişmez #22): 720 saat, 20 dakika değil.
	// Ölçüm SAHTE SAATE göre yapılır: `created_at` veritabanının gerçek
	// now()'ıdır ve testin sabit saatiyle karşılaştırılamaz.
	süre := ord.ExpiresAt.Sub(e.clock.Now())
	if süre < (rentalHours-1)*time.Hour {
		t.Errorf("sipariş ömrü %v — kiralık süresi sağlayıcıdan alınmamış "+
			"(kullanıcı 30 günlük fiyata 20 dakikalık numara aldı)", süre)
	}
	if len(e.stub.durations) != 1 || e.stub.durations[0] != rentalHours {
		t.Errorf("sağlayıcıya gönderilen duration = %v, beklenen [%d]",
			e.stub.durations, rentalHours)
	}
}

// TestActiveRentalIsNotClosedAtProvider
//
// ASIL KORUMA BURADA: dönemi süren (ACTIVE) bir siparişe kapatma çağrısı
// gelse bile sağlayıcıya HİÇBİR istek gitmez ve sahiplenme bırakılır.
//
// `activation-reaper` ya da elle bir çağrı bu yola girebilir; girdiğinde
// numara ölmemelidir. Sahiplenmenin bırakılması da önemli: kira boşuna
// tutulursa sipariş gerçekten terminal olduğunda kapatma iki dakika gecikir.
func TestActiveRentalIsNotClosedAtProvider(t *testing.T) {
	e := setup(t, 100_000)
	ord := e.buyRental(t, 45_000)
	if err := e.svc.DeliverMessages(context.Background(), ord.ID,
		[]port.RemoteMessage{msg("111111", "Kod 111111", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}
	waitForBackground()

	if err := e.svc.CloseAtProvider(context.Background(), ord.ID); err != nil {
		t.Fatal(err)
	}

	if n := e.stub.finishes.Load() + e.stub.cancels.Load(); n != 0 {
		t.Errorf("sağlayıcıya %d kapatma çağrısı gitti — dönemi süren numara öldü", n)
	}
	got := e.reload(t, ord.ID)
	if got.ProviderClosedAt != nil {
		t.Error("dönemi süren kiralık 'sağlayıcıda kapatıldı' işaretlendi")
	}
	if got.CloseClaimedAt != nil {
		t.Error("kapatma sahiplenmesi bırakılmadı — sipariş terminal olduğunda " +
			"kapatma kira süresi kadar gecikirdi")
	}
}

/* ═══════════════════════════════════════════════════════════════════════
   DENETİM DÜZELTMELERİ — F1…F9

   Buradaki her test bir ADVERSARYAL DENETİMDE ölçülmüş davranışı geri
   üretiyor. Hepsi düzeltme olmadan DÜŞER; her biri sabotajla doğrulandı.
   ═══════════════════════════════════════════════════════════════════════ */

// TestRentalRowCannotLoseRefundWindow — F1 (veri hattı)
//
// 🔴 PARA GUARD'ININ GİRDİSİ VERİTABANINDA SAVUNULUR.
//
// `CanUserCancel` üst sınırı yalnız `refundable_until` doluyken uyguluyordu;
// NULL sessizce "sınır yok" demekti. Bir kiralık satırında NULL kalması 29 gün
// kullanılmış numaranın TAM İADESİ demek (ölçüldü: 55000 → 100000). Go
// tarafındaki kontrol KARARI savunur — bu kısıt VERİYİ savunur, çünkü kod
// yolu atlanabilir (veri taşıma, admin kaydı, ileride `prolong`).
func TestRentalRowCannotLoseRefundWindow(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()
	ord := e.buyRental(t, 45_000)

	if ord.RefundableUntil == nil {
		t.Fatal("kiralık satırına iade penceresi hiç yazılmamış")
	}

	// (a) Doldurulmuş bir pencere BOŞALTILAMAZ.
	if _, err := pool.Exec(ctx,
		`UPDATE orders SET refundable_until=NULL WHERE id=$1`, ord.ID); err == nil {
		t.Error("🔴 kiralık siparişin iade penceresi NULL yapıldı — iptal " +
			"guard'ı sessizce açılır ve 29 gün kullanılmış numara tam iade alır")
	}
	if got := e.reload(t, ord.ID); got.RefundableUntil == nil {
		t.Error("pencere gerçekten silindi")
	}

	// (b) Bir satır penceresi OLMADAN kiralığa DÖNÜŞTÜRÜLEMEZ. Aktivasyonda
	// kolon NULL'dır; ürün türünü elle çevirmek tam olarak fail-open satırı
	// üretirdi.
	act, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: e.makeQuote(t, 25_000)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE orders SET product_kind='SMS_RENTAL' WHERE id=$1`, act.ID); err == nil {
		t.Error("🔴 penceresi olmayan bir satır kiralık ilan edildi")
	}
}

// TestRentalCancelIsClosedWhenWindowUnknown — F1 (karar hattı)
//
// Kısıt VERİYİ savunuyor; bu test KARARIN da savunulduğunu gösterir. İkisi
// ayrı hata sınıflarını yakalar ve biri diğerinin yerine geçmez: kısıt
// düşürülse (ya da bir sonraki migration'da unutulsa) bile iptal açılmamalı.
//
// KISIT BU TESTTE BİLEREK DÜŞÜRÜLÜYOR. Fail-open satırı üretmenin başka yolu
// yok — ve asıl soru zaten bu: "veri hattı olmasaydı karar hattı tutar mıydı?"
// Denetimin ürettiği durumun birebir kendisi (29 gün kullanılmış kiralık,
// `refundable_until IS NULL`, bakiye 55000 → 100000).
func TestRentalCancelIsClosedWhenWindowUnknown(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()
	ord := e.buyRental(t, 45_000)
	sonra := e.balanceMinor(t)

	// Veri hattını geçici olarak devre dışı bırak.
	if _, err := pool.Exec(ctx,
		`ALTER TABLE orders DROP CONSTRAINT order_rental_has_refund_window`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		// Kısıt geri konmadan önce satır tekrar geçerli hâle getirilmeli;
		// aksi hâlde ALTER TABLE düşer ve sonraki testler kısıtsız koşar.
		_, _ = pool.Exec(bg, `UPDATE orders SET refundable_until = expires_at
			WHERE product_kind='SMS_RENTAL' AND refundable_until IS NULL`)
		if _, err := pool.Exec(bg, `ALTER TABLE orders ADD CONSTRAINT
			order_rental_has_refund_window CHECK (product_kind <> 'SMS_RENTAL'
			OR refundable_until IS NOT NULL)`); err != nil {
			t.Errorf("kısıt geri konamadı — sonraki testler savunmasız koşar: %v", err)
		}
	})

	if _, err := pool.Exec(ctx,
		`UPDATE orders SET refundable_until=NULL WHERE id=$1`, ord.ID); err != nil {
		t.Fatal(err)
	}

	// 29. gün: hiç mesaj gelmedi (kiralıkta `Expire` kapalı olduğu için sipariş
	// hâlâ PENDING), `cancellable_at` çoktan geçti.
	e.clock.Advance(29 * 24 * time.Hour)

	if _, err := e.svc.Cancel(ctx, e.userID, ord.PublicID); err == nil {
		t.Fatal("🔴 29 GÜN kullanılmış kiralık iptal edildi — TAM İADE yazılırdı " +
			"(ölçülmüş: bakiye 55000 → 100000)")
	}
	if n := e.refundCount(t); n != 0 {
		t.Errorf("REFUND defter satırı = %d, beklenen 0", n)
	}
	if b := e.balanceMinor(t); b != sonra {
		t.Errorf("bakiye %d → %d değişti", sonra, b)
	}
	assertReconciled(t, e.userID)
}

// TestRentalPurchaseFailsWhenProviderIgnoresDuration — F2
//
// 🔴 SAĞLAYICI ÖDENEN SÜREYİ VERMEZSE SATIN ALMA YAZILMAZ.
//
// Eski davranış: `expiredAt` hiç sorgulanmıyor, `PurchaseResult.Subtype`
// hiçbir yerde okunmuyordu. 720 saatlik kiralık satılıp 20 dakikalık
// aktivasyon teslim edildiğinde satır kendi içinde çelişkili yazılıyor
// (`duration_hours=720`, `expires_at=+20dk`) ve kimse bakmıyordu:
// `ListExpiredPendingOrders` kiralığı dışlar, `Expire` kiralıkta kapalıdır,
// `rental-closer` satırı sessizce COMPLETED yapar. Ölçüldü: bakiye
// 55000 → 55000, iade satırı 0 — kullanıcı 450 TL'ye 20 dakika aldı.
//
// Aynı satır bir AKTİVASYON olsaydı order-expirer tam iade yazardı.
func TestRentalPurchaseFailsWhenProviderIgnoresDuration(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()

	// SÜREYİ ONURLANDIRMAYAN SAĞLAYICI: kiralık istendi, aktivasyon verildi.
	e.stub.mu.Lock()
	e.stub.ignoreRentalDuration = true
	e.stub.mu.Unlock()

	before := e.balanceMinor(t)
	prodID := makeRentalProduct(t, rentalHours)
	q := e.makeQuoteFor(t, prodID, 45_000)

	_, err := e.svc.Create(ctx, ordersvc.CreateInput{UserID: e.userID, QuoteID: q})
	if err == nil {
		t.Fatal("🔴 sağlayıcı 720 saat yerine 20 dakika verdi ve satın alma " +
			"BAŞARILI sayıldı — kullanıcı 30 günlük fiyata 20 dakika alırdı")
	}
	if !errors.Is(err, apperr.ErrProviderUnavailable) {
		t.Errorf("hata = %v, ErrProviderUnavailable bekleniyordu (kullanıcıya "+
			"ham sağlayıcı hatası gösterilmez)", err)
	}

	// (a) Sipariş satırı YAZILMAMALI — yazılırsa hiçbir iş onu iade etmez.
	var siparis int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM orders WHERE user_id=$1`, e.userID).Scan(&siparis); err != nil {
		t.Fatal(err)
	}
	if siparis != 0 {
		t.Errorf("%d sipariş satırı yazıldı — çelişkili satır kalıcı olurdu", siparis)
	}

	// (b) PARA GERİ VERİLMELİ.
	if got := e.balanceMinor(t); got != before {
		t.Errorf("bakiye %d → %d — kullanıcının parası askıda kaldı", before, got)
	}
	assertReconciled(t, e.userID)

	// (c) Numara sağlayıcıda AÇIK BIRAKILMAMALI.
	if n := e.stub.cancels.Load(); n != 1 {
		t.Errorf("sağlayıcıya %d Cancel gitti, beklenen 1 — alınan numara "+
			"kapatılmadı, maliyeti bizde kalırdı", n)
	}
}

// TestRentalPollNeverStarvesNewestRental — F3
//
// 🔴 SABİT SIRALAMA + DEĞİŞMEYEN KÜME = AÇLIK.
//
// `rental-poller` bir turda LIMIT kadar satır alıyor ve sıralama `created_at`
// idi. Aktivasyonda güvenli (satırlar ~20 dakikada kümeden çıkar); kiralıkta
// küme dönem boyunca sabit: LIMIT'i aşan kiralık HİÇBİR turda görünmüyordu.
// Ölçüldü: 101 canlı kiralık, en yenisi (id=8117) hiçbir turda yok — 6 saat
// sonra bile aynı 5 satır taranıyordu.
//
// Test LIMIT'i 2'ye indirerek aynı koşulu üç kiralıkla üretiyor.
func TestRentalPollNeverStarvesNewestRental(t *testing.T) {
	e := setup(t, 300_000)
	ctx := context.Background()

	// SÜRELER FARKLI: `products_unique_sku` aynı servis × ülke × süre için tek
	// satıra izin veriyor. Üç ayrı kiralık ürün gerekiyor.
	sureler := []int{24, 72, 168}

	var kiralik []db.Order
	for i := 0; i < 3; i++ {
		prodID := makeRentalProduct(t, sureler[i])
		o, err := e.svc.Create(ctx, ordersvc.CreateInput{
			UserID: e.userID, QuoteID: e.makeQuoteFor(t, prodID, 45_000),
		})
		if err != nil {
			t.Fatalf("%d. kiralık alınamadı: %v", i+1, err)
		}
		kiralik = append(kiralik, o)
		// Sağlayıcı her numaraya bir mesaj hazırlamış olsun: yoklanan satır
		// mesajını alır, yoklanmayan alamaz — gözlemimiz budur.
		e.stub.activeMessages[o.RemoteOrderID] = []port.RemoteMessage{
			msg(fmt.Sprintf("%06d", 111111*(i+1)), fmt.Sprintf("Kod %d", i+1), e.clock.Now()),
		}
		e.clock.Advance(time.Minute) // created_at sırası belirgin olsun
	}
	enYeni := kiralik[2]

	// İki tur, her turda 2 satır. Küme değişmiyor: eski davranışta ikinci tur
	// da aynı iki satırı görürdü.
	for tur := 1; tur <= 2; tur++ {
		if _, err := e.svc.PollRentals(ctx, 2); err != nil {
			t.Fatalf("%d. tur: %v", tur, err)
		}
	}

	if n := e.messageCount(t, enYeni.ID); n == 0 {
		t.Fatalf("🔴 EN YENİ kiralık iki turda da yoklanmadı — küme dönem "+
			"boyunca (30 gün) değişmediği için o kullanıcı webhook kaybolursa "+
			"tek mesaj görmez (sipariş %s)", enYeni.PublicID)
	}

	// Dönüşüm damgası gerçekten yazılmalı: yazılmazsa sıralama ilk tura geri
	// döner ve açlık sessizce geri gelir.
	got := e.reload(t, enYeni.ID)
	if got.LastPolledAt == nil {
		t.Error("yoklama damgası yazılmadı — bir sonraki tur aynı satırlarla başlar")
	}
}

// TestRentalRefundRetryKeepsCancelDecision — F4
//
// 🔴 KARARIN ÇIPASI "ŞU AN" DEĞİL, İPTALİN İSTENDİĞİ ANDIR.
//
// `within` her denemede yeniden hesaplanıyordu ve 15 dakikalık pencere
// `provider-refund-retry` aralığının (2 dk × 8 deneme) tam ortasında bitiyor.
// Ölçülen sonuç:
//
//	ilk deneme:     cancel=1 finish=0  RETRY_SCHEDULED
//	yeniden deneme: cancel=1 finish=1  NOT_APPLICABLE
//
// Yani kullanıcıya para verildikten sonra sağlayıcıdan iade istemekten
// VAZGEÇİLİYOR (sağlayıcının kendi penceresi 20 dakikayken 15. dakikada) ve
// eksene "hiç iade talep etmedik" yazılıyordu: gider metriği bu kaybı hiç
// görmezdi. Sessiz gider, sıfır alarm.
func TestRentalRefundRetryKeepsCancelDecision(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()
	ord := e.buyRental(t, 45_000)

	// 5. dakika: iptal penceresi (15 dk) AÇIK.
	e.clock.Advance(5 * time.Minute)

	// İlk iptal denemesi GEÇİCİ hatayla düşsün — sağlayıcı "2 dakika sonra".
	e.stub.mu.Lock()
	e.stub.cancelResults = []error{
		port.NewRetryAfter("EARLY_CANCEL_DENIED", 2*time.Minute, errors.New("erken")),
	}
	e.stub.mu.Unlock()

	if _, err := e.svc.Cancel(ctx, e.userID, ord.PublicID); err != nil {
		t.Fatalf("pencere içinde iptal reddedildi: %v", err)
	}
	waitForBackground()

	ilk := e.reload(t, ord.ID)
	if ilk.ProviderRefundStatus != db.RefundStatusRETRYSCHEDULED {
		t.Fatalf("iade ekseni = %q, RETRY_SCHEDULED bekleniyordu", ilk.ProviderRefundStatus)
	}
	if n := e.stub.cancels.Load(); n != 1 {
		t.Fatalf("ilk denemede %d Cancel gitti, beklenen 1", n)
	}

	// 🔴 PENCERE KAPANIR — ama karar iptalin İSTENDİĞİ ana aittir.
	e.clock.Advance(20 * time.Minute)

	if _, err := e.svc.RetryProviderRefunds(ctx, 100); err != nil {
		t.Fatal(err)
	}
	waitForBackground()

	if n := e.stub.finishes.Load(); n != 0 {
		t.Errorf("🔴 yeniden denemede Finish() %d kez gitti — kullanıcıya para "+
			"verildikten sonra sağlayıcıdan iade istemekten VAZGEÇİLDİ", n)
	}
	if n := e.stub.cancels.Load(); n != 2 {
		t.Errorf("sağlayıcıya toplam %d Cancel gitti, beklenen 2 — yeniden "+
			"deneme iptal kararını korumalı", n)
	}
	son := e.reload(t, ord.ID)
	if son.ProviderRefundStatus == db.RefundStatusNOTAPPLICABLE {
		t.Error("🔴 iade ekseni NOT_APPLICABLE yazıldı — 'hiç iade talep etmedik' " +
			"demektir ve gider metriği bu kaybı hiç görmez")
	}
	if son.ProviderRefundStatus != db.RefundStatusREFUNDED {
		t.Errorf("iade ekseni = %q, REFUNDED bekleniyordu", son.ProviderRefundStatus)
	}
	assertReconciled(t, e.userID)
}

// TestFailedFinishDoesNotTouchRefundAxis — F6
//
// 🔴 `Cancel` İLE `Finish`İN BAŞARISIZLIĞI AYNI ŞEY DEĞİLDİR (değişmez #24).
//
// `recordCloseFailure` hangi çağrının denendiğini bilmiyordu: `ErrCancelDenied`
// gördüğünde iade eksenine DENIED, satıra da `provider_closed_at` yazıyordu.
// Kiralıkta bu İSTİSNA DEĞİL KURAL: `DecideRentalClose` dönem sonunda HER
// kiralığa `Finish` gönderir ve sağlayıcı aktivasyonu `expiredAt`te kendisi
// kapattıysa yanıt `ACTIVATION_NOT_ACTIVE` olur. Yani normal işleyen her
// kiralık gider metriğine düşer (KK-406b ölçülemez hâle gelir) ve sipariş
// KAPANMADIĞI hâlde kapalı işaretlenip yeniden deneme yolundan kalıcı olarak
// düşerdi — "her terminal sipariş sağlayıcıda kapatılır" güvencesinin tersi.
func TestFailedFinishDoesNotTouchRefundAxis(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()
	ord := e.buyRental(t, 45_000)

	e.clock.Advance(rentalHours*time.Hour + time.Minute)

	e.stub.mu.Lock()
	e.stub.finishResults = []error{
		fmt.Errorf("%w: ACTIVATION_NOT_ACTIVE", port.ErrCancelDenied),
	}
	e.stub.mu.Unlock()

	if err := e.svc.EndRental(ctx, ord.ID); err != nil {
		t.Fatal(err)
	}
	waitForBackground()

	got := e.reload(t, ord.ID)
	if n := e.stub.finishes.Load(); n != 1 {
		t.Fatalf("Finish() %d kez çağrıldı, beklenen 1", n)
	}
	if got.ProviderRefundStatus == db.RefundStatusDENIED {
		t.Error("🔴 başarısız Finish 'iade reddedildi' gideri yazdı — dönem sonu " +
			"kapanan HER kiralık gider metriğini kirletirdi")
	}
	if got.ProviderClosedAt != nil {
		t.Error("🔴 kapanmamış sipariş 'sağlayıcıda kapatıldı' işaretlendi — " +
			"reaper bir daha hiç denemez, numara sağlayıcıda açık kalır")
	}

	// Yeniden deneme yolunda KALMALI: reaper onu görmeye devam etmeli.
	rows, err := e.q.ListUnclosedTerminalOrders(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	var kuyrukta bool
	for _, o := range rows {
		if o.ID == ord.ID {
			kuyrukta = true
		}
	}
	if !kuyrukta {
		t.Error("🔴 sipariş kapatma kuyruğundan düştü — FR-412'de kapatmadan " +
			"asla vazgeçilmez")
	}
	if e.refundCount(t) != 0 {
		t.Error("başarısız kapatma kullanıcı defterine yazdı")
	}
}

// TestRefundedOrderCodeIsNotPublishedOverSSE — F7
//
// 🔴 KODUN KULLANICIYA ULAŞMASININ İKİ KANALI VAR; İKİSİ DE SÜZÜLMELİ.
//
// `handler.visibleMessages` iade edilmiş siparişin mesajlarını gizliyordu ama
// `DeliverMessages` durumdan BAĞIMSIZ yayın yapıyordu: ekranı açık olan
// kullanıcı iadesini almışken kodu canlı akıştan görüyordu — hem parayı hem
// hizmeti almış olurdu. Kiralıkta pencere 20 dakika değil 30 GÜN.
//
// (Bu hata kiralığa özgü DEĞİLDİR — aktivasyonda da vardı; test bilerek
// aktivasyon üzerinden yazıldı ki düzeltmenin kapsamı görünsün.)
func TestRefundedOrderCodeIsNotPublishedOverSSE(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()
	ord := e.newPendingOrder(t)

	e.clock.Advance(25 * time.Minute)
	if err := e.svc.Expire(ctx, ord.ID); err != nil {
		t.Fatal(err)
	}
	waitForBackground()

	durum := e.reload(t, ord.ID)
	if durum.Status != db.OrderStatusREFUNDED {
		t.Fatalf("durum = %q, REFUNDED bekleniyordu", durum.Status)
	}

	// Gecikmeli kod, yoklama ya da webhook yoluyla gelir — ikisi de
	// DeliverMessages'a düşer.
	if err := e.svc.DeliverMessages(ctx, ord.ID,
		[]port.RemoteMessage{msg("424242", "Kodunuz 424242", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}

	if n := e.pub.kodIcerenOlaylar("424242"); n != 0 {
		t.Errorf("🔴 iade edilmiş siparişin kodu SSE'den YAYINLANDI (%d olay) — "+
			"kullanıcı hem parayı hem kodu aldı", n)
	}
	// Mesaj yine de SAKLANIR: "kod gelmedi diye iade ettik ama aslında
	// gelmişti" tartışmasının tek kanıtı odur.
	if n := e.messageCount(t, ord.ID); n != 1 {
		t.Errorf("mesaj sayısı = %d, beklenen 1 — kanıt saklanmalı", n)
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusREFUNDED {
		t.Errorf("durum = %q, REFUNDED kalmalıydı", got.Status)
	}
}

// TestOrderStatsIncludesActiveRentalRevenue — F8
//
// Dönemi süren kiralık ÖDENMİŞTİR ve geri alınamaz: durum makinesinde
// ACTIVE → CANCELLED oku YOKTUR. Geliri yalnız 'COMPLETED' üzerinden saymak,
// panelin 30 gün boyunca sıfır göstermesi demekti — para akarken. 'PENDING'
// bilerek dışarıda: o tutar hâlâ iade edilebilir.
func TestOrderStatsIncludesActiveRentalRevenue(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()
	ord := e.buyRental(t, 45_000)

	if err := e.svc.DeliverMessages(ctx, ord.ID,
		[]port.RemoteMessage{msg("111111", "Kod 111111", e.clock.Now())}); err != nil {
		t.Fatal(err)
	}
	if got := e.reload(t, ord.ID); got.Status != db.OrderStatusACTIVE {
		t.Fatalf("durum = %q, ACTIVE bekleniyordu", got.Status)
	}
	waitForBackground()

	// `created_at` veritabanının GERÇEK saatidir; testin sahte saatiyle
	// karşılaştırılamaz.
	row, err := e.q.OrderStatsSummary(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if row.RevenueMinor != 45_000 {
		t.Errorf("🔴 gelir = %d, beklenen 45000 — dönemi süren kiralık ödenmiştir "+
			"ve iade edilemez; panel 30 gün boyunca sıfır gösterirdi", row.RevenueMinor)
	}
	if row.Active != 1 {
		t.Errorf("active kovası = %d, beklenen 1", row.Active)
	}
	if toplam := row.Pending + row.Active + row.Completed + row.Cancelled + row.Failed; toplam != row.Total {
		t.Errorf("kovalar toplamı %d ≠ total %d — bir sipariş hiçbir kovada "+
			"görünmüyor, gösterge okunamaz", toplam, row.Total)
	}
}

// TestDeniedCancelIsNotRecordedAsProviderRefund — F9 (denetim raporunda YOKTU)
//
// 🔴 REDDEDİLEN İPTAL "SAĞLAYICI PARAMIZI GERİ VERDİ" DİYE KAYDEDİLİYORDU.
//
// `Cancel` çağrısı `NEW_OTP_RECEIVED` ile reddedilip yerine `Finish`
// gönderildiğinde, karar değişkeni (`action`) hâlâ CloseCancel olduğu için
// satıra `provider_refund_status = REFUNDED` ve iade tutarı olarak tam maliyet
// yazılıyordu. Sağlayıcı iadeyi AÇIKÇA reddetmişken defterimizde "iade alındı"
// duruyordu: KK-406b gideri olduğundan az gösterir.
//
// F6 ile aynı sınıf: iade ekseni GERÇEKLEŞENİ yazmalı, NİYETİ değil.
func TestDeniedCancelIsNotRecordedAsProviderRefund(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()
	ord := e.newPendingOrder(t)

	e.stub.mu.Lock()
	e.stub.cancelResults = []error{port.NewOTPArrived(
		[]port.RemoteMessage{{RemoteID: "otp-gec", Code: "424242", Body: "Kodunuz 424242"}},
		port.ErrCancelDenied,
	)}
	e.stub.mu.Unlock()

	e.clock.Advance(25 * time.Minute)
	if err := e.svc.Expire(ctx, ord.ID); err != nil {
		t.Fatal(err)
	}
	waitForBackground()

	got := e.reload(t, ord.ID)
	if got.ProviderRefundStatus == db.RefundStatusREFUNDED {
		t.Errorf("🔴 reddedilen iptal 'sağlayıcı iade etti' (%q, tutar %d) diye "+
			"kaydedildi — gider olduğundan az görünür",
			got.ProviderRefundStatus, got.ProviderRefundAmountMinor)
	}
	if got.ProviderRefundAmountMinor != 0 {
		t.Errorf("iade tutarı = %d, beklenen 0 — sağlayıcı hiçbir şey iade etmedi",
			got.ProviderRefundAmountMinor)
	}
	if n := e.stub.finishes.Load(); n != 1 {
		t.Errorf("Finish() %d kez çağrıldı, beklenen 1", n)
	}
	assertReconciled(t, e.userID)
}

// TestRentalDurationToleranceIsBounded — F2 (tolerans)
//
// Süre kontrolü TAM EŞİTLİK aramaz: `expiredAt` sağlayıcının saatinde ve numara
// tahsis edildiği anda hesaplanır, biz onu HTTP yanıtından sonra kendi
// saatimizle karşılaştırırız; üstelik `expirySafetyMargin` (15 sn) zaten
// düşülür. Tam eşitlik beklemek her satın almada yanlış alarm üretirdi.
//
// Ama tolerans SINIRLI olmalı: sınırsız bir pay, kontrolü hiç yapmamakla aynı
// kapıya çıkar. Bu test iki yönü birden çiviliyor — payın içi KABUL,
// dışı RED.
func TestRentalDurationToleranceIsBounded(t *testing.T) {
	ctx := context.Background()
	paid := time.Duration(rentalHours) * time.Hour

	t.Run("pay içinde kalan sapma kabul edilir", func(t *testing.T) {
		e := setup(t, 100_000)
		e.stub.mu.Lock()
		e.stub.shortRentalTTL = paid - time.Minute // 1 dk eksik
		e.stub.mu.Unlock()

		prodID := makeRentalProduct(t, rentalHours)
		ord, err := e.svc.Create(ctx, ordersvc.CreateInput{
			UserID: e.userID, QuoteID: e.makeQuoteFor(t, prodID, 45_000),
		})
		if err != nil {
			t.Fatalf("saniyelik sapma satın almayı düşürdü: %v — her siparişte "+
				"yanlış alarm üretirdi", err)
		}
		if ord.ProductKind != db.ProductKindSMSRENTAL {
			t.Errorf("product_kind = %q", ord.ProductKind)
		}
	})

	t.Run("payın dışındaki sapma reddedilir", func(t *testing.T) {
		e := setup(t, 100_000)
		e.stub.mu.Lock()
		e.stub.shortRentalTTL = paid - 30*time.Minute // 30 dk eksik
		e.stub.mu.Unlock()

		before := e.balanceMinor(t)
		prodID := makeRentalProduct(t, rentalHours)
		if _, err := e.svc.Create(ctx, ordersvc.CreateInput{
			UserID: e.userID, QuoteID: e.makeQuoteFor(t, prodID, 45_000),
		}); err == nil {
			t.Fatal("🔴 sağlayıcı ödenen süreyi eksik verdi ve satın alma kabul " +
				"edildi — aradaki süre kullanıcının zararıdır ve hiçbir iş onu " +
				"iade etmez")
		}
		if got := e.balanceMinor(t); got != before {
			t.Errorf("bakiye %d → %d — para iade edilmedi", before, got)
		}
		assertReconciled(t, e.userID)
	})
}

// TestRentalPurchaseFailsWhenProviderDoesNotConfirmRental — F2 (subtype ölçütü)
//
// İkinci ölçüt: sağlayıcı satışın KİRALIK olduğunu teyit etmeli.
// `PurchaseResult.Subtype` bu turdan önce hiçbir yerde okunmuyordu — yazılıyor
// ama hiç sorulmuyordu.
//
// Süre ölçütünden AYRI sınanıyor: süre doğru gelip subtype gelmediğinde de
// elimizde "sağlayıcının kiralık olarak tanımadığı" bir numara kalır. O numara
// sağlayıcı tarafında bir aktivasyondur; kendi TTL'inde kapanabilir, `Finish`
// yerine farklı davranabilir ve kiralık uçlarında (uzatma, kiralık listesi) hiç
// görünmez. Eksik teyit "izin yok" tarafına düşer.
func TestRentalPurchaseFailsWhenProviderDoesNotConfirmRental(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()

	e.stub.mu.Lock()
	e.stub.rentalSubtypeMismatch = true
	e.stub.mu.Unlock()

	before := e.balanceMinor(t)
	prodID := makeRentalProduct(t, rentalHours)
	if _, err := e.svc.Create(ctx, ordersvc.CreateInput{
		UserID: e.userID, QuoteID: e.makeQuoteFor(t, prodID, 45_000),
	}); err == nil {
		t.Fatal("🔴 sağlayıcı satışı kiralık olarak TEYİT ETMEDİ ve sipariş " +
			"kiralık olarak yazıldı")
	}
	if got := e.balanceMinor(t); got != before {
		t.Errorf("bakiye %d → %d — para iade edilmedi", before, got)
	}
	assertReconciled(t, e.userID)
}

// TestNonTerminalOrderCannotBeClaimedForClose — R1 (belirlenimli hâli)
//
// 🔴 SAHİPLENME İLE BIRAKMA ARASINDAKİ ARALIK BİR KAPATMA ÇAĞRISI YUTUYORDU.
//
// `ClaimOrderClose` durumu süzmüyordu: bir işçi terminal OLMAYAN satırı
// sahipleniyor, durumu görüp `ReleaseOrderClose` ile bırakıyordu. Tam o
// aralıkta `EndRental` COMPLETED yazıp kendi kapatma goroutine'ini doğurursa,
// o goroutine sahiplenmeyi KAYBEDİP sessizce nil dönüyor, ardından gelen
// `release` de kirayı siliyordu. Sonuç: terminal sipariş, `provider_closed_at
// IS NULL` ve sağlayıcıya SIFIR çağrı — numara reaper'ın bir sonraki turuna
// kadar (≤60 sn) açıkta.
//
// `TestRentalEndRaceWithProviderCloseHasSingleEffect` bunu ZAMANLAMAYLA
// yakalıyor (sabotajda -race -count=10 → 10/10 düştü) ama tek koşumda
// kaçırabilir. Bu test aynı kuralı BELİRLENİMLİ sınar: kural "terminal olmayan
// satır hiç sahiplenilmez"dir ve yarışa gerek kalmadan doğrulanabilir.
func TestNonTerminalOrderCannotBeClaimedForClose(t *testing.T) {
	e := setup(t, 100_000)
	ctx := context.Background()
	ord := e.buyRental(t, 45_000)

	now := e.clock.Now()
	stale := now.Add(-2 * time.Minute)
	for _, durum := range []struct {
		ad      string
		hazirla func()
	}{
		{"PENDING", func() {}},
		{"ACTIVE", func() {
			if err := e.svc.DeliverMessages(ctx, ord.ID,
				[]port.RemoteMessage{msg("111111", "Kod 111111", now)}); err != nil {
				t.Fatal(err)
			}
			waitForBackground()
		}},
	} {
		durum.hazirla()
		if _, err := e.q.ClaimOrderClose(ctx, db.ClaimOrderCloseParams{
			ID: ord.ID, Now: &now, ClaimStaleBefore: &stale,
		}); err == nil {
			t.Errorf("🔴 %s siparişi kapatma için SAHİPLENİLDİ — sahiplenme ile "+
				"bırakma arasındaki aralıkta terminal olan siparişin kapatma "+
				"çağrısı yutulur", durum.ad)
		}
		if got := e.reload(t, ord.ID); got.CloseClaimedAt != nil {
			t.Errorf("%s siparişine kapatma kirası yazıldı", durum.ad)
		}
	}
}
