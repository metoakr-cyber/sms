//go:build integration

package worker

// `data-retention` işinin entegrasyon testleri — GERÇEK Postgres'e karşı.
//
// NEDEN GERÇEK VERİTABANI: sınanan şeyin tamamı SQL'dir. Sahte bir depoyla
// `LIMIT`, `!~ '^redacted:'` süzgeci, `ledger_no_delete` tetikleyicisi ve
// yabancı anahtar zincirleri hiç çalışmaz; test yeşil yanar, üretimde silme
// çalışmaz. Gizlilik metni "90 gün sonra sileriz" diyeceği için bu işin
// çalıştığının kanıtı METNİN ÖNKOŞULUDUR.
//
// 🔴 Bu dosya DELETE/UPDATE yapar. `testsupport.MustTestDatabaseURL` adı
// "_test" ile bitmeyen bir veritabanında süreci durdurur.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/testsupport"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	url, err := testsupport.MustTestDatabaseURL()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if url == "" {
		fmt.Println("⚠️  DATABASE_URL tanımsız — saklama testleri ATLANDI")
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

/* ═══════════════════════ Ortam ═══════════════════════ */

// fakeClock saati testin elinde tutar.
//
// Sınır davranışı ("tam 90 gün") ancak saat sabitken sınanabilir: `time.Now()`
// ile yazılmış bir test, kayıt ile karşılaştırma arasında geçen mikrosaniyeler
// yüzünden kararsız olurdu.
type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

type env struct {
	t      *testing.T
	ctx    context.Context
	clock  fakeClock
	q      *db.Queries
	deps   Deps
	userID int64
	provID int64
	prodID int64
	ordID  int64
}

// gun n gün öncesini verir (takvim aritmetiği — işin kendisi de AddDate kullanır).
func (e *env) gun(n int) time.Time { return e.clock.Now().AddDate(0, 0, -n) }

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	uniq := time.Now().UnixNano()

	clock := fakeClock{t: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)}
	tx := postgres.NewTxRunner(pool)
	e := &env{
		t: t, ctx: ctx, clock: clock, q: db.New(pool),
		deps: Deps{TxRunner: tx, Clock: clock},
	}

	must := func(sql string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("fikstür (%s): %v", sql, err)
		}
		return id
	}

	e.userID = must(`INSERT INTO users (email, username, password_hash, status, email_verified_at)
	                 VALUES ($1, $2, 'x', 'ACTIVE', now()) RETURNING id`,
		fmt.Sprintf("ret-%d@test.local", uniq), fmt.Sprintf("ret%d", uniq%1e9))
	e.provID = must(`INSERT INTO providers (name, protocol, is_active, capabilities)
	                 VALUES ($1, 'FAKE', true, '["SMS_ACTIVATION"]') RETURNING id`,
		fmt.Sprintf("ret-prov-%d", uniq))
	// Katalog satırları PAYLAŞILIR (varsa alınır, yoksa yazılır): `countries.iso2`
	// ve ürün SKU'su tekildir, her testte yenisini üretmek çakışırdı. Bu satırlar
	// temizlikte silinmez — testin ölçtüğü şey katalog değil.
	svcID := must(`INSERT INTO services (code, name) VALUES ('ret-fixture', 'Retention')
	               ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING id`)
	ctryID := must(`INSERT INTO countries (iso2, name, name_tr, phone_code)
	                VALUES ('ZZ', 'Retention', 'Retention', '90')
	                ON CONFLICT (iso2) DO UPDATE SET name = EXCLUDED.name RETURNING id`)
	e.prodID = must(`INSERT INTO products (kind, service_id, country_id, verification_type)
	                 VALUES ('SMS_ACTIVATION', $1, $2, 'sms')
	                 ON CONFLICT ON CONSTRAINT products_unique_sku
	                 DO UPDATE SET is_active = true RETURNING id`, svcID, ctryID)
	e.ordID = must(`
		INSERT INTO orders (user_id, provider_id, remote_order_id, phone_number,
		                    service_code, service_name, country_iso2, country_name,
		                    price_paid_minor, cost_micro, fx_rate, expires_at, cancellable_at,
		                    status, completed_at, created_at)
		VALUES ($1, $2, $3, '905550000000', 'wa', 'Whatsapp', 'TR', 'Türkiye',
		        7500, 1000000, 40.0, $4, $4, 'COMPLETED', $4, $4)
		RETURNING id`,
		e.userID, e.provID, fmt.Sprintf("ret-%d", uniq), e.gun(200))

	t.Cleanup(func() {
		bg := context.Background()
		for _, s := range []string{
			`DELETE FROM order_messages WHERE order_id IN (SELECT id FROM orders WHERE user_id=$1)`,
			`DELETE FROM orders WHERE user_id=$1`,
			`DELETE FROM price_quotes WHERE user_id=$1`,
			`DELETE FROM sessions WHERE user_id=$1`,
			`DELETE FROM auth_tokens WHERE user_id=$1`,
			`DELETE FROM audit_logs WHERE actor_user_id=$1`,
		} {
			if _, err := pool.Exec(bg, s, e.userID); err != nil {
				t.Errorf("temizlik (%s): %v", s, err)
			}
		}
		// Kullanıcı ve defter satırları BIRAKILIR: `ledger_no_delete`
		// tetikleyicisi (migration 00003) satır silmeyi reddeder ve kullanıcı
		// da defter satırlarının yabancı anahtarıdır. Her test kendi
		// kullanıcısını açtığı için artık satır başka testi etkilemez.
		if _, err := pool.Exec(bg, `DELETE FROM providers WHERE id=$1`, e.provID); err != nil {
			t.Errorf("temizlik (providers): %v", err)
		}
	})
	return e
}

// run işi bir tur çalıştırır.
func (e *env) run() {
	e.t.Helper()
	if err := dataRetention(e.deps).Run(e.ctx); err != nil {
		e.t.Fatalf("saklama işi: %v", err)
	}
}

// mesaj bir order_messages satırı yazar ve id'sini döner.
func (e *env) mesaj(otpID string, createdAt time.Time) int64 {
	e.t.Helper()
	var id int64
	if err := pool.QueryRow(e.ctx, `
		INSERT INTO order_messages (order_id, provider_otp_id, code, body, sender, received_at, created_at)
		VALUES ($1, $2, '4821', 'Kodunuz 4821', 'WHATSAPP', $3, $3)
		RETURNING id`, e.ordID, otpID, createdAt).Scan(&id); err != nil {
		e.t.Fatalf("mesaj yazılamadı: %v", err)
	}
	return id
}

type mesajSatiri struct {
	code, body, sender, otpID string
	orderID                   int64
	receivedAt                time.Time
}

func (e *env) mesajOku(id int64) mesajSatiri {
	e.t.Helper()
	var m mesajSatiri
	if err := pool.QueryRow(e.ctx, `
		SELECT code, body, sender, provider_otp_id, order_id, received_at
		FROM order_messages WHERE id=$1`, id).
		Scan(&m.code, &m.body, &m.sender, &m.otpID, &m.orderID, &m.receivedAt); err != nil {
		e.t.Fatalf("mesaj %d okunamadı: %v", id, err)
	}
	return m
}

func (e *env) sayi(sql string, args ...any) int64 {
	e.t.Helper()
	var n int64
	if err := pool.QueryRow(e.ctx, sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("sayım (%s): %v", sql, err)
	}
	return n
}

/* ═══════════════════════ SMS gövdesi — 90 gün ═══════════════════════ */

// TestRetentionRedactsOldSmsBodyKeepsRow
//
// SÖZLEŞME: 90 günü geçen SMS'in İÇERİĞİ boşalır, SATIRI durur.
//
// Satırın durması "kod geldi mi gelmedi mi" tartışmasının kanıtıdır; içeriğin
// gitmesi gizlilik metninde verilen sözdür. İkisi aynı anda doğru olmalı.
func TestRetentionRedactsOldSmsBodyKeepsRow(t *testing.T) {
	e := setup(t)
	eski := e.mesaj("otp-eski", e.gun(91))
	yeni := e.mesaj("otp-yeni", e.gun(89))

	e.run()

	got := e.mesajOku(eski)
	if got.code != "" || got.body != "" || got.sender != "" {
		t.Errorf("eski mesaj boşaltılmadı: code=%q body=%q sender=%q",
			got.code, got.body, got.sender)
	}
	if got.otpID != fmt.Sprintf("redacted:%d", eski) {
		t.Errorf("provider_otp_id = %q — metinden türeyen özet duruyor", got.otpID)
	}
	// Satır ve kanıt değeri yerinde mi?
	if got.orderID != e.ordID {
		t.Errorf("order_id = %d, %d bekleniyordu", got.orderID, e.ordID)
	}
	if got.receivedAt.IsZero() {
		t.Error("received_at boşaltılmış — mesajın geldiği OLGU kaybolmuş")
	}
	if n := e.sayi(`SELECT count(*) FROM order_messages WHERE order_id=$1`, e.ordID); n != 2 {
		t.Errorf("%d satır kaldı, 2 bekleniyordu — satır silinmiş olmamalı", n)
	}

	taze := e.mesajOku(yeni)
	if taze.body != "Kodunuz 4821" || taze.code != "4821" || taze.otpID != "otp-yeni" {
		t.Errorf("89 günlük mesaja dokunulmuş: %+v", taze)
	}
}

// TestRetentionKeepsSmsAtExactlyNinetyDays
//
// SINIR: sorgu `created_at < now-90g` yazar, yani TAM 90 günlük kayıt HÂLÂ
// pencerenin içindedir. "En fazla 90 gün saklarız" sözünün okunuşu budur:
// 90'ıncı günde veri hâlâ bizdedir, 90'ı GEÇTİĞİNDE gider. Sınırı `<=` yapmak
// ilan edilen süreyi bir gün kaydırır.
func TestRetentionKeepsSmsAtExactlyNinetyDays(t *testing.T) {
	e := setup(t)
	tamSinir := e.mesaj("otp-tam-90", e.clock.Now().AddDate(0, 0, -90))
	sinirUstu := e.mesaj("otp-90-arti", e.clock.Now().AddDate(0, 0, -90).Add(-time.Second))

	e.run()

	if got := e.mesajOku(tamSinir); got.body == "" {
		t.Error("tam 90 günlük mesaj boşaltıldı — sınır bir gün erkene kaymış")
	}
	if got := e.mesajOku(sinirUstu); got.body != "" {
		t.Errorf("90 günü 1 saniye geçen mesaj duruyor: body=%q", got.body)
	}
}

// TestRetentionRedactionIsIdempotent
//
// İşler at-least-once koşar (worker.go). Boşaltma ikinci turda AYNI satırları
// yeniden yazmamalıdır: `!~ '^redacted:'` süzgeci onları dışarıda tutar.
// Yazsaydı her tur tüm eski satırları UPDATE eder, tabloyu şişirir ve
// autovacuum'u boşuna çalıştırırdı.
func TestRetentionRedactionIsIdempotent(t *testing.T) {
	e := setup(t)
	id := e.mesaj("otp-tekrar", e.gun(120))

	e.run()
	ilk := e.mesajOku(id)

	// İkinci koşuyu doğrudan sorguyla ölçüyoruz: iş yalnız hata döner,
	// "kaç satır etkilendi" bilgisi sorgudan gelir.
	n, err := e.q.RedactOldOrderMessages(e.ctx, db.RedactOldOrderMessagesParams{
		OlderThan: e.gun(90), Lim: retentionBatch,
	})
	if err != nil {
		t.Fatalf("ikinci koşu: %v", err)
	}
	if n != 0 {
		t.Errorf("ikinci koşu %d satır etkiledi, 0 bekleniyordu", n)
	}
	if son := e.mesajOku(id); son != ilk {
		t.Errorf("ikinci koşu satırı değiştirdi: %+v → %+v", ilk, son)
	}
}

// TestRetentionDrainsBacklogBeyondOneBatch
//
// İLK KOŞU BİR BİRİKİMLE KARŞILAŞIR: iş bugüne kadar hiç çalışmadı. Tek bir
// SQL ifadesi `LIMIT retentionBatch` ile sınırlıdır (tabloyu kilitlememek
// için); birikimi eritmek `drainBatches` döngüsünün işidir. Parti sınırı
// olmasaydı tek ifade milyonlarca satıra dokunurdu, döngü olmasaydı birikim
// hiç bitmezdi — ikisi birlikte sınanır.
func TestRetentionDrainsBacklogBeyondOneBatch(t *testing.T) {
	e := setup(t)
	const fazla = 25
	if _, err := pool.Exec(e.ctx, `
		INSERT INTO order_messages (order_id, provider_otp_id, code, body, sender, received_at, created_at)
		SELECT $1, 'bulk-' || g, '1111', 'Kodunuz 1111', 'X', $2, $2
		FROM generate_series(1, $3) g`,
		e.ordID, e.gun(100), retentionBatch+fazla); err != nil {
		t.Fatalf("toplu fikstür: %v", err)
	}

	// Tek ifade parti sınırını AŞMAZ.
	n, err := e.q.RedactOldOrderMessages(e.ctx, db.RedactOldOrderMessagesParams{
		OlderThan: e.gun(90), Lim: retentionBatch,
	})
	if err != nil {
		t.Fatalf("tek parti: %v", err)
	}
	if n != retentionBatch {
		t.Fatalf("tek ifade %d satır işledi, %d bekleniyordu", n, retentionBatch)
	}

	// İş kalanı da eritir.
	e.run()
	kalan := e.sayi(`
		SELECT count(*) FROM order_messages
		WHERE order_id=$1 AND provider_otp_id !~ '^redacted:'`, e.ordID)
	if kalan != 0 {
		t.Errorf("%d satır boşaltılmadan kaldı — döngü birikimi eritmiyor", kalan)
	}
}

/* ═══════════════════════ Süresi dolanlar ═══════════════════════ */

// TestRetentionDeletesExpiredSessionsAndTokens
//
// SÖZLEŞME: süresi dolmuş oturum ve token satırları gider, süresi dolmamışlar
// yerinde kalır. Süresi dolmuş bir oturum satırı hiçbir işe yaramaz
// (GetSession `expires_at > now()` arar) ama IP ve tarayıcı imzası taşır.
func TestRetentionDeletesExpiredSessionsAndTokens(t *testing.T) {
	e := setup(t)
	now := e.clock.Now()

	for _, s := range []struct {
		id  string
		exp time.Time
	}{
		{"ret-dolmus", now.Add(-time.Hour)},
		{"ret-gecerli", now.Add(24 * time.Hour)},
	} {
		if _, err := pool.Exec(e.ctx, `
			INSERT INTO sessions (id, user_id, ip, user_agent, expires_at)
			VALUES ($1, $2, '10.0.0.1', 'Mozilla/5.0', $3)`,
			s.id, e.userID, s.exp); err != nil {
			t.Fatalf("oturum fikstürü: %v", err)
		}
	}
	for i, exp := range []time.Time{now.Add(-time.Minute), now.Add(time.Hour)} {
		if _, err := pool.Exec(e.ctx, `
			INSERT INTO auth_tokens (user_id, token_hash, purpose, expires_at)
			VALUES ($1, $2, 'PASSWORD_RESET', $3)`,
			e.userID, []byte(fmt.Sprintf("ret-hash-%d-%d", e.userID, i)), exp); err != nil {
			t.Fatalf("token fikstürü: %v", err)
		}
	}

	e.run()

	if n := e.sayi(`SELECT count(*) FROM sessions WHERE id='ret-dolmus'`); n != 0 {
		t.Error("süresi dolmuş oturum duruyor")
	}
	if n := e.sayi(`SELECT count(*) FROM sessions WHERE id='ret-gecerli'`); n != 1 {
		t.Error("geçerli oturum silinmiş — kullanıcılar oturumundan atılır")
	}
	if n := e.sayi(`SELECT count(*) FROM auth_tokens WHERE user_id=$1`, e.userID); n != 1 {
		t.Errorf("%d token kaldı, 1 bekleniyordu (yalnız süresi dolan gitmeli)", n)
	}
}

// TestRetentionKeepsConsumedQuotes
//
// SÖZLEŞME: süresi geçmiş ama TÜKETİLMİŞ teklif yerinde kalır.
//
// İki gerekçe: (a) tüketilmiş teklif `orders.quote_id` ile bir siparişin fiyat
// kanıtıdır ve sipariş kaydıyla aynı süre durur; (b) yetim provizyon işi
// (FR-408) tam olarak o satırları tarar — silinseydi T1/T2 arasında kesilen
// bir satın almanın parası kurtarılamazdı.
func TestRetentionKeepsConsumedQuotes(t *testing.T) {
	e := setup(t)
	gecmis := e.clock.Now().Add(-time.Hour)

	teklif := func(consumed *time.Time) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(e.ctx, `
			INSERT INTO price_quotes (user_id, product_id, provider_id, cost_micro,
			                          fx_rate, margin_percent, sell_price_minor,
			                          expires_at, consumed_at)
			VALUES ($1, $2, $3, 1000000, 40.0, 40.0, 7500, $4, $5) RETURNING id`,
			e.userID, e.prodID, e.provID, gecmis, consumed).Scan(&id); err != nil {
			t.Fatalf("teklif fikstürü: %v", err)
		}
		return id
	}
	bosta := teklif(nil)
	tuketilmis := teklif(&gecmis)

	// Süresi dolmamış teklif de yazılır: satış anında kilitlenen bir teklifi
	// temizlik işinin altından çekmek satın almayı düşürürdü.
	gelecek := e.clock.Now().Add(2 * time.Minute)
	acik := func() int64 {
		var id int64
		if err := pool.QueryRow(e.ctx, `
			INSERT INTO price_quotes (user_id, product_id, provider_id, cost_micro,
			                          fx_rate, margin_percent, sell_price_minor, expires_at)
			VALUES ($1, $2, $3, 1000000, 40.0, 40.0, 7500, $4) RETURNING id`,
			e.userID, e.prodID, e.provID, gelecek).Scan(&id); err != nil {
			t.Fatalf("açık teklif fikstürü: %v", err)
		}
		return id
	}()

	e.run()

	if n := e.sayi(`SELECT count(*) FROM price_quotes WHERE id=$1`, bosta); n != 0 {
		t.Error("tüketilmemiş ve süresi geçmiş teklif duruyor")
	}
	if n := e.sayi(`SELECT count(*) FROM price_quotes WHERE id=$1`, tuketilmis); n != 1 {
		t.Error("TÜKETİLMİŞ teklif silinmiş — siparişin fiyat kanıtı ve FR-408 dayanağı gitti")
	}
	if n := e.sayi(`SELECT count(*) FROM price_quotes WHERE id=$1`, acik); n != 1 {
		t.Error("süresi dolmamış teklif silinmiş — açık satın alma düşerdi")
	}
}

/* ═══════════════════════ Denetim kaydı — 2 yıl ═══════════════════════ */

// TestRetentionDeletesAuditLogsOlderThanTwoYears
//
// İki takvim yılı: `AddDate(-2, 0, 0)`. Sınır günü içeride kalır.
func TestRetentionDeletesAuditLogsOlderThanTwoYears(t *testing.T) {
	e := setup(t)
	kayit := func(at time.Time, entity string) {
		t.Helper()
		if _, err := pool.Exec(e.ctx, `
			INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, ip, created_at)
			VALUES ($1, 'deposit.approve', $2, '1', '10.0.0.1', $3)`,
			e.userID, entity, at); err != nil {
			t.Fatalf("denetim fikstürü: %v", err)
		}
	}
	now := e.clock.Now()
	kayit(now.AddDate(-2, 0, -1), "eski")     // 2 yılı 1 gün geçmiş
	kayit(now.AddDate(-2, 0, 0), "tam-sinir") // tam 2 yıl
	kayit(now.AddDate(-1, 0, 0), "yeni")      // 1 yıllık

	e.run()

	for _, c := range []struct {
		entity string
		want   int64
	}{{"eski", 0}, {"tam-sinir", 1}, {"yeni", 1}} {
		got := e.sayi(`SELECT count(*) FROM audit_logs WHERE actor_user_id=$1 AND entity_type=$2`,
			e.userID, c.entity)
		if got != c.want {
			t.Errorf("denetim kaydı %q: %d satır, %d bekleniyordu", c.entity, got, c.want)
		}
	}
}

/* ═══════════════════════ Dokunulmayanlar ═══════════════════════ */

// TestRetentionLeavesLedgerAndOrdersUntouched
//
// 🔴 EN KRİTİK TEST. Temizlik işi mali kayda UZANMAZ: `ledger_entries`,
// `orders` ve `deposits` 10 yıl saklanır ve defter ayrıca değişmezdir
// (Değişmez #4, migration 00003/00004 tetikleyicileri).
//
// Fikstür KASITLI olarak çok eskidir (11 yıllık sipariş ve defter satırı):
// yanlış yazılmış bir tarih süzgeci bu satırları yakalardı. Tablo TOPLAMLARI
// karşılaştırılır, yalnız bu testin satırları değil — iş kullanıcı kapsamı
// tanımaz, hatası da tanımazdı.
func TestRetentionLeavesLedgerAndOrdersUntouched(t *testing.T) {
	e := setup(t)

	if _, err := pool.Exec(e.ctx, `
		INSERT INTO ledger_entries (user_id, amount_minor, entry_type, balance_after_minor,
		                            idempotency_key, reference_type, reference_id, created_at)
		VALUES ($1, 10000, 'DEPOSIT', 10000, $2, 'deposit', '1', $3),
		       ($1, -7500, 'PURCHASE', 2500, $4, 'order', '1', $3)`,
		e.userID,
		fmt.Sprintf("ret-dep-%d", e.userID), e.clock.Now().AddDate(-11, 0, 0),
		fmt.Sprintf("ret-buy-%d", e.userID)); err != nil {
		t.Fatalf("defter fikstürü: %v", err)
	}
	// Siparişi de 11 yıl geriye çekiyoruz.
	if _, err := pool.Exec(e.ctx,
		`UPDATE orders SET created_at=$2 WHERE id=$1`,
		e.ordID, e.clock.Now().AddDate(-11, 0, 0)); err != nil {
		t.Fatalf("sipariş tarihi: %v", err)
	}
	e.mesaj("otp-cok-eski", e.gun(4000)) // boşaltılacak olan

	type anlik struct{ defter, tutar, siparis, mesaj int64 }
	oku := func() anlik {
		var a anlik
		if err := pool.QueryRow(e.ctx, `
			SELECT (SELECT count(*) FROM ledger_entries),
			       (SELECT COALESCE(sum(amount_minor),0) FROM ledger_entries),
			       (SELECT count(*) FROM orders),
			       (SELECT count(*) FROM order_messages)`).
			Scan(&a.defter, &a.tutar, &a.siparis, &a.mesaj); err != nil {
			t.Fatalf("anlık görüntü: %v", err)
		}
		return a
	}

	once := oku()
	e.run()
	sonra := oku()

	if once != sonra {
		t.Fatalf("temizlik işi mali kayda dokundu: %+v → %+v", once, sonra)
	}
	if s := e.sayi(`SELECT count(*) FROM orders WHERE id=$1 AND status='COMPLETED'`, e.ordID); s != 1 {
		t.Error("sipariş satırı ya silinmiş ya durumu değişmiş")
	}
}
