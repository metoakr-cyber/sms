//go:build integration

package deposit_test

// Bakiye yükleme akışının GERÇEK Postgres'e karşı garantileri.
//
// SQLite ile taklit edilemez: `SELECT ... FOR UPDATE`, kısmi UNIQUE indeks ve
// BEFORE UPDATE tetikleyicisi burada ölçülen şeylerin ta kendisidir.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/storage"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	auditsvc "github.com/ikmetrik/sms-platform/api/internal/service/audit"
	depositsvc "github.com/ikmetrik/sms-platform/api/internal/service/deposit"
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

/* ═══════════════════════ Kurulum ═══════════════════════ */

func newService(t *testing.T) *depositsvc.Service {
	t.Helper()
	tx := postgres.NewTxRunner(pool)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("depo: %v", err)
	}
	return depositsvc.New(depositsvc.Deps{
		TxRunner: tx,
		Wallet:   walletsvc.New(tx),
		Clock:    port.RealClock{},
		Receipts: store,
	})
}

// seedUser oturum sahibi bir kullanıcı ekler ve temizliğini kurar.
func seedUser(t *testing.T, suffix string) int64 {
	t.Helper()
	tag := uuid.NewString()[:8]
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, username, password_hash, status, balance_minor)
		VALUES ($1, $2, '$2a$10$x', 'ACTIVE', 0)
		RETURNING id`,
		fmt.Sprintf("dep-%s-%s@ornek.test", suffix, tag),
		fmt.Sprintf("d_%s_%s", suffix, tag)).Scan(&id)
	if err != nil {
		t.Fatalf("kullanıcı eklenemedi: %v", err)
	}
	// LIFO: bu temizlik, kullanıcı silinmeden ÖNCE koşar.
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM deposits WHERE user_id = $1 OR reviewed_by_user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_entries WHERE user_id = $1 OR created_by_user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

type methodOpts struct {
	kind        db.DepositMethodKind
	min, max    int64
	active      bool
	configExtra map[string]string
}

func seedMethod(t *testing.T, o methodOpts) db.DepositMethod {
	t.Helper()
	cfg := map[string]string{"iban": "TR000000000000000000000000", "hesapAdi": "Test A.Ş."}
	if o.kind == db.DepositMethodKindCRYPTO {
		cfg = map[string]string{"ag": "TRC-20", "cuzdanAdresi": "TTestCuzdanAdresi000000000"}
	}
	for k, v := range o.configExtra {
		cfg[k] = v
	}
	raw, _ := json.Marshal(cfg)

	code := "test-" + uuid.NewString()[:8]
	var m db.DepositMethod
	err := pool.QueryRow(context.Background(), `
		INSERT INTO deposit_methods (code, kind, name, instructions, config,
		                             min_amount_minor, max_amount_minor, is_active)
		VALUES ($1, $2, $3, 'talimat', $4, $5, $6, $7)
		RETURNING id, public_id, code, kind, name, instructions, config,
		          min_amount_minor, max_amount_minor, is_active, sort_order,
		          created_at, updated_at`,
		code, o.kind, "Test Yöntemi "+code, raw, o.min, o.max, o.active).
		Scan(&m.ID, &m.PublicID, &m.Code, &m.Kind, &m.Name, &m.Instructions, &m.Config,
			&m.MinAmountMinor, &m.MaxAmountMinor, &m.IsActive, &m.SortOrder,
			&m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		t.Fatalf("yöntem eklenemedi: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM deposit_methods WHERE id = $1`, m.ID)
	})
	return m
}

func activeBank(t *testing.T) db.DepositMethod {
	return seedMethod(t, methodOpts{kind: db.DepositMethodKindBANKTRANSFER, min: 1000, max: 5000000, active: true})
}

func balanceOf(t *testing.T, userID int64) int64 {
	t.Helper()
	var v int64
	if err := pool.QueryRow(context.Background(),
		`SELECT balance_minor FROM users WHERE id = $1`, userID).Scan(&v); err != nil {
		t.Fatalf("bakiye okunamadı: %v", err)
	}
	return v
}

func countRows(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("sayım başarısız (%s): %v", sql, err)
	}
	return n
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("hata bekleniyordu, nil geldi")
	}
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("🔴 uygulama hatası değil, HAM hata sızdı: %v", err)
	}
	return e.Code
}

func mustCreate(t *testing.T, s *depositsvc.Service, userID int64, m db.DepositMethod, amount int64, ref string) db.Deposit {
	t.Helper()
	d, err := s.Create(context.Background(), depositsvc.CreateInput{
		UserID: userID, MethodPublicID: m.PublicID, AmountMinor: amount, Reference: ref,
	})
	if err != nil {
		t.Fatalf("talep açılamadı: %v", err)
	}
	return d
}

/* ═══════════════════════ Talep oluşturma ═══════════════════════ */

// TestCreateDepositOnlyForActiveMethod
//
// PASİF yöntemle talep açılamaz: pasif bir yöntemin IBAN'ı boş olabilir ve
// kullanıcı parayı hiçbir yere göndermiş olur. Var OLMAYAN bir yöntem de
// AYNI hatayı alır — hangi kimliklerin var olduğu sızmasın.
func TestCreateDepositOnlyForActiveMethod(t *testing.T) {
	s := newService(t)
	uid := seedUser(t, "aktif")

	passive := seedMethod(t, methodOpts{kind: db.DepositMethodKindBANKTRANSFER, min: 1000, max: 5000000})
	_, err := s.Create(context.Background(), depositsvc.CreateInput{
		UserID: uid, MethodPublicID: passive.PublicID, AmountMinor: 5000, Reference: "ABC12345",
	})
	if got := codeOf(t, err); got != "DEPOSIT_METHOD_INACTIVE" {
		t.Fatalf("🔴 pasif yöntemle talep açıldı/yanlış hata: %s", got)
	}

	_, err = s.Create(context.Background(), depositsvc.CreateInput{
		UserID: uid, MethodPublicID: uuid.New(), AmountMinor: 5000, Reference: "ABC12345",
	})
	if got := codeOf(t, err); got != "DEPOSIT_METHOD_INACTIVE" {
		t.Fatalf("🔴 var olmayan yöntem farklı hata verdi (numaralandırma sızıntısı): %s", got)
	}

	// Aktif yöntemle GERÇEKTEN açılabilmeli.
	d := mustCreate(t, s, uid, activeBank(t), 5000, "ABC12345")
	if d.Status != db.DepositStatusPENDING {
		t.Fatalf("yeni talep durumu = %s", d.Status)
	}
}

// TestInactiveMethodIsNotVisibleToUser
//
// 🔴 PASİF YÖNTEMİN IBAN'I / CÜZDAN ADRESİ KULLANICI YANITINDA GÖRÜNMEZ.
// config alanı hesap bilgisi taşır; yanlış ya da kapatılmış bir hesabı
// göstermek, paranın ulaşmayacağı bir yere gönderilmesi demektir.
func TestInactiveMethodIsNotVisibleToUser(t *testing.T) {
	s := newService(t)

	const secretIBAN = "TR999GIZLI9999999999999999"
	passive := seedMethod(t, methodOpts{
		kind: db.DepositMethodKindBANKTRANSFER, min: 1000, max: 5000000,
		configExtra: map[string]string{"iban": secretIBAN},
	})
	active := activeBank(t)

	rows, err := s.Methods(context.Background())
	if err != nil {
		t.Fatalf("yöntemler okunamadı: %v", err)
	}

	sawActive := false
	for _, m := range rows {
		if m.PublicID == passive.PublicID {
			t.Fatal("🔴 PASİF yöntem kullanıcıya döndü")
		}
		if strings.Contains(string(m.Config), secretIBAN) {
			t.Fatalf("🔴 pasif yöntemin IBAN'ı yanıtta sızdı: %s", m.Config)
		}
		if m.PublicID == active.PublicID {
			sawActive = true
		}
	}
	if !sawActive {
		t.Fatal("aktif yöntem listede yok — test bir şey ispatlamıyor")
	}
}

// TestAmountOutOfMethodRangeRejected
//
// Yöntem sınırları ve `max_amount_minor = 0` semantiği.
// 🔴 Sıfır "ÜST SINIR YOK" demektir; 0,00 ₺ tavan DEĞİL.
func TestAmountOutOfMethodRangeRejected(t *testing.T) {
	s := newService(t)
	uid := seedUser(t, "aralik")
	ctx := context.Background()

	tight := seedMethod(t, methodOpts{kind: db.DepositMethodKindBANKTRANSFER, min: 5000, max: 20000, active: true})

	// (a) alt sınırın altı
	_, err := s.Create(ctx, depositsvc.CreateInput{
		UserID: uid, MethodPublicID: tight.PublicID, AmountMinor: 3000, Reference: "REF12345"})
	if got := codeOf(t, err); got != "DEPOSIT_AMOUNT_RANGE" {
		t.Fatalf("🔴 alt sınırın altındaki tutar için kod = %s", got)
	}

	// (b) üst sınırın üstü
	_, err = s.Create(ctx, depositsvc.CreateInput{
		UserID: uid, MethodPublicID: tight.PublicID, AmountMinor: 30000, Reference: "REF12345"})
	if got := codeOf(t, err); got != "DEPOSIT_AMOUNT_RANGE" {
		t.Fatalf("🔴 üst sınırın üstündeki tutar için kod = %s", got)
	}

	// (c) 🔴 max = 0 → ÜST SINIR YOK. Tablo tavanı (50.000 ₺) geçerli olur.
	unlimited := seedMethod(t, methodOpts{kind: db.DepositMethodKindBANKTRANSFER, min: 1000, max: 0, active: true})
	if _, err := s.Create(ctx, depositsvc.CreateInput{
		UserID: uid, MethodPublicID: unlimited.PublicID, AmountMinor: 5000000, Reference: "REF12345",
	}); err != nil {
		t.Fatalf("🔴 max=0 (üst sınır yok) yönteminde 50.000 ₺ reddedildi: %v", err)
	}

	// (d) tablo CHECK'inin ötesi — ham 23514 değil, Türkçe doğrulama hatası.
	_, err = s.Create(ctx, depositsvc.CreateInput{
		UserID: uid, MethodPublicID: unlimited.PublicID, AmountMinor: 5000001, Reference: "REF12345"})
	if got := codeOf(t, err); got != "DEPOSIT_AMOUNT_RANGE" {
		t.Fatalf("🔴 tablo tavanının üstü için kod = %s", got)
	}
	_, err = s.Create(ctx, depositsvc.CreateInput{
		UserID: uid, MethodPublicID: unlimited.PublicID, AmountMinor: 999, Reference: "REF12345"})
	if got := codeOf(t, err); got != "DEPOSIT_AMOUNT_RANGE" {
		t.Fatalf("🔴 tablo tabanının altı için kod = %s", got)
	}
}

// TestBankTransferDepositLeavesTxHashNull
//
// Havalede tx_hash NULL kalır. Boş string yazılsaydı, kısmi UNIQUE indeks
// (WHERE tx_hash IS NOT NULL) ikinci havale talebini 23505 ile reddederdi ve
// kullanıcı bir daha ASLA havale bildiremezdi.
func TestBankTransferDepositLeavesTxHashNull(t *testing.T) {
	s := newService(t)
	uid := seedUser(t, "havale")
	m := activeBank(t)

	first := mustCreate(t, s, uid, m, 5000, "DEKONT-0001")
	second := mustCreate(t, s, uid, m, 7000, "DEKONT-0002")

	for _, d := range []db.Deposit{first, second} {
		if d.TxHash != nil {
			t.Fatalf("🔴 havale talebinde tx_hash yazıldı: %q", *d.TxHash)
		}
		var isNull bool
		if err := pool.QueryRow(context.Background(),
			`SELECT tx_hash IS NULL FROM deposits WHERE id = $1`, d.ID).Scan(&isNull); err != nil {
			t.Fatalf("okunamadı: %v", err)
		}
		if !isNull {
			t.Fatal("🔴 veritabanında tx_hash NULL değil")
		}
	}
	// Referans kaybolmamalı: kullanıcı notuna taşınır.
	if !strings.Contains(first.UserNote, "DEKONT-0001") {
		t.Fatalf("🔴 havale açıklaması kayboldu: %q", first.UserNote)
	}
}

// TestDuplicateTxHashRejected — KK-501.
//
// Aynı tx_hash ile ikinci yükleme talebi 409 döner; hata TİPLİDİR, ham pgx
// hatası değildir. İndeks GLOBALDİR: farklı bir kullanıcı da aynı hash'i
// bildiremez.
func TestDuplicateTxHashRejected(t *testing.T) {
	s := newService(t)
	alice := seedUser(t, "alice")
	bob := seedUser(t, "bob")
	m := seedMethod(t, methodOpts{kind: db.DepositMethodKindCRYPTO, min: 1000, max: 5000000, active: true})
	hash := "0xTEST" + uuid.NewString()

	d := mustCreate(t, s, alice, m, 5000, hash)
	if d.TxHash == nil || *d.TxHash != hash {
		t.Fatalf("kripto talebinde tx_hash yazılmadı: %v", d.TxHash)
	}
	if d.Network != "TRC-20" {
		t.Fatalf("🔴 ağ yöntemin config'inden alınmadı: %q", d.Network)
	}

	_, err := s.Create(context.Background(), depositsvc.CreateInput{
		UserID: alice, MethodPublicID: m.PublicID, AmountMinor: 5000, Reference: hash})
	if got := codeOf(t, err); got != "TX_HASH_DUPLICATE" {
		t.Fatalf("🔴 aynı hash ikinci kez kabul edildi/yanlış hata: %s", got)
	}
	appErr, _ := apperr.As(err)
	if appErr.HTTPStatus() != 409 {
		t.Fatalf("🔴 durum kodu = %d, 409 bekleniyordu (KK-501)", appErr.HTTPStatus())
	}

	_, err = s.Create(context.Background(), depositsvc.CreateInput{
		UserID: bob, MethodPublicID: m.PublicID, AmountMinor: 5000, Reference: hash})
	if got := codeOf(t, err); got != "TX_HASH_DUPLICATE" {
		t.Fatalf("🔴 aynı hash BAŞKA kullanıcı tarafından kabul edildi: %s", got)
	}
}

// TestUserCannotSeeOthersDeposit — değişmez #7.
//
// "Başkasının" ile "yok" AYNI hataya düşer (404, 403 DEĞİL): 403 dönmek
// geçerli kimliklerin varlığını sızdırır.
func TestUserCannotSeeOthersDeposit(t *testing.T) {
	s := newService(t)
	alice := seedUser(t, "alice")
	bob := seedUser(t, "bob")
	d := mustCreate(t, s, alice, activeBank(t), 5000, "DEKONT-1")

	_, err := s.Get(context.Background(), bob, d.PublicID)
	if got := codeOf(t, err); got != "DEPOSIT_NOT_FOUND" {
		t.Fatalf("🔴 Bob, Alice'in talebini görebiliyor (kod: %s)", got)
	}
	appErr, _ := apperr.As(err)
	if appErr.HTTPStatus() != 404 {
		t.Fatalf("🔴 durum kodu = %d, 404 bekleniyordu", appErr.HTTPStatus())
	}

	rows, total, err := s.List(context.Background(), bob, nil, 20, 0)
	if err != nil {
		t.Fatalf("liste: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("🔴 Bob'un listesinde %d kayıt var", len(rows))
	}

	// Alice kendi talebini GÖREBİLMELİ.
	if _, err := s.Get(context.Background(), alice, d.PublicID); err != nil {
		t.Fatalf("🔴 sahibi kendi talebini göremedi: %v", err)
	}
}

/* ═══════════════════════ Onay ve red ═══════════════════════ */

// TestApproveWritesAuditLog — FR-502'nin üçüncü ayağı.
//
// Onay TEK TRANSACTION'da üç şey yazar: durum, defter kaydı, denetim kaydı.
// Ayrıca denetim kaydında KİŞİSEL VERİ VE DEKONT YOLU BULUNMAZ.
func TestApproveWritesAuditLog(t *testing.T) {
	s := newService(t)
	admin := seedUser(t, "yonetici")
	uid := seedUser(t, "musteri")
	d := mustCreate(t, s, uid, activeBank(t), 12345, "DEKONT-XYZ")

	// Dekont ekle: yolun denetim kaydına sızmadığını da ölçebilelim.
	if _, err := s.AttachReceipt(context.Background(), uid, d.PublicID,
		bytes.NewReader(jpeg())); err != nil {
		t.Fatalf("dekont eklenemedi: %v", err)
	}
	var receiptPath string
	if err := pool.QueryRow(context.Background(),
		`SELECT receipt_path FROM deposits WHERE id = $1`, d.ID).Scan(&receiptPath); err != nil {
		t.Fatalf("dekont yolu okunamadı: %v", err)
	}

	res, err := s.Approve(context.Background(), depositsvc.ApproveInput{
		AdminID: admin, PublicID: d.PublicID, AdminNote: "ekstre ile eşleşti",
		Audit: auditsvc.Meta{RequestID: "istek-123", UserAgent: "test"},
	})
	if err != nil {
		t.Fatalf("onay: %v", err)
	}
	if res.Deposit.Status != db.DepositStatusCOMPLETED {
		t.Fatalf("durum = %s", res.Deposit.Status)
	}
	if res.NewBalance.Minor() != 12345 || balanceOf(t, uid) != 12345 {
		t.Fatalf("🔴 bakiye = %d, 12345 bekleniyordu", balanceOf(t, uid))
	}

	// Denetim kaydı VAR.
	var action, entityType, entityID, requestID string
	var actor int64
	var before, after []byte
	err = pool.QueryRow(context.Background(), `
		SELECT action, entity_type, entity_id, actor_user_id, request_id, before, after
		FROM audit_logs WHERE entity_id = $1 AND action = 'deposit.approve'`,
		d.PublicID.String()).Scan(&action, &entityType, &entityID, &actor, &requestID, &before, &after)
	if err != nil {
		t.Fatalf("🔴 denetim kaydı YAZILMADI — 'kim onayladı?' cevapsız: %v", err)
	}
	if entityType != "deposit" || actor != admin || requestID != "istek-123" {
		t.Fatalf("🔴 denetim kaydı eksik: type=%s actor=%d req=%s", entityType, actor, requestID)
	}

	// 🔴 Kişisel veri ve dekont yolu denetim kaydında YOK.
	blob := string(before) + string(after)
	for _, forbidden := range []string{receiptPath, "DEKONT-XYZ", "@ornek.test"} {
		if forbidden != "" && strings.Contains(blob, forbidden) {
			t.Fatalf("🔴 denetim kaydına sızdı (%q): %s", forbidden, blob)
		}
	}
	if !strings.Contains(string(after), "COMPLETED") {
		t.Fatalf("denetim kaydı durum değişimini taşımıyor: %s", after)
	}

	// Defter kaydı TEK ve anahtarı DETERMİNİSTİK.
	key := "deposit:" + d.PublicID.String()
	if n := countRows(t, `SELECT count(*) FROM ledger_entries WHERE idempotency_key = $1`, key); n != 1 {
		t.Fatalf("🔴 %d defter kaydı (anahtar %s)", n, key)
	}
}

// TestApproveRepeatedReturnsAlreadyApplied
//
// Tekrarlanan onay bir HATA değil, bir TEKRAR'dır: 200 + alreadyApplied.
// Hiçbir şey yeniden yazılmaz — ne bakiye, ne defter, ne denetim kaydı.
func TestApproveRepeatedReturnsAlreadyApplied(t *testing.T) {
	s := newService(t)
	admin := seedUser(t, "yonetici")
	uid := seedUser(t, "musteri")
	d := mustCreate(t, s, uid, activeBank(t), 9000, "DEKONT-1")

	first, err := s.Approve(context.Background(), depositsvc.ApproveInput{
		AdminID: admin, PublicID: d.PublicID, CreditedMinor: 8500})
	if err != nil {
		t.Fatalf("ilk onay: %v", err)
	}
	if first.AlreadyApplied {
		t.Fatal("ilk onay 'tekrar' olarak işaretlendi")
	}
	firstReviewed := first.Deposit.ReviewedAt

	second, err := s.Approve(context.Background(), depositsvc.ApproveInput{
		AdminID: admin, PublicID: d.PublicID, CreditedMinor: 4000000})
	if err != nil {
		t.Fatalf("🔴 ikinci onay hata verdi: %v", err)
	}
	if !second.AlreadyApplied {
		t.Fatal("🔴 ikinci onay 'tekrar' olarak işaretlenmedi")
	}
	if got := balanceOf(t, uid); got != 8500 {
		t.Fatalf("🔴 ikinci onay bakiyeyi değiştirdi: %d (beklenen 8500)", got)
	}
	if second.Deposit.CreditedMinor != 8500 {
		t.Fatalf("🔴 credited_minor yeniden yazıldı: %d", second.Deposit.CreditedMinor)
	}
	if !second.Deposit.ReviewedAt.Equal(*firstReviewed) {
		t.Fatal("🔴 reviewed_at yeniden yazıldı")
	}
	if n := countRows(t, `SELECT count(*) FROM ledger_entries WHERE user_id = $1`, uid); n != 1 {
		t.Fatalf("🔴 %d defter kaydı yazıldı, 1 bekleniyordu", n)
	}
	if n := countRows(t,
		`SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'deposit.approve'`,
		d.PublicID.String()); n != 1 {
		t.Fatalf("🔴 %d denetim kaydı yazıldı, 1 bekleniyordu", n)
	}

	// REDDEDİLMİŞ bir talep onaylanamaz — 'alreadyApplied' yolu yalnız
	// COMPLETED içindir.
	rejected := mustCreate(t, s, uid, activeBank(t), 5000, "DEKONT-2")
	if _, err := s.Reject(context.Background(), depositsvc.RejectInput{
		AdminID: admin, PublicID: rejected.PublicID, Reason: "belge eksik"}); err != nil {
		t.Fatalf("red: %v", err)
	}
	_, err = s.Approve(context.Background(), depositsvc.ApproveInput{
		AdminID: admin, PublicID: rejected.PublicID})
	if got := codeOf(t, err); got != "DEPOSIT_NOT_PENDING" {
		t.Fatalf("🔴 reddedilmiş talep onaylandı/yanlış hata: %s", got)
	}
}

// TestRejectDoesNotChangeBalance — FR-503.
//
// Red bir para hareketi DEĞİLDİR: bakiye değişmez, defter kaydı yazılmaz.
func TestRejectDoesNotChangeBalance(t *testing.T) {
	s := newService(t)
	admin := seedUser(t, "yonetici")
	uid := seedUser(t, "musteri")
	d := mustCreate(t, s, uid, activeBank(t), 25000, "DEKONT-1")

	before := balanceOf(t, uid)
	res, err := s.Reject(context.Background(), depositsvc.RejectInput{
		AdminID: admin, PublicID: d.PublicID,
		Reason: "Dekont tutarı bildirilen tutarla eşleşmiyor.",
		Audit:  auditsvc.Meta{RequestID: "istek-red"},
	})
	if err != nil {
		t.Fatalf("red: %v", err)
	}

	if res.Deposit.Status != db.DepositStatusREJECTED {
		t.Fatalf("durum = %s", res.Deposit.Status)
	}
	if res.Deposit.RejectionReason != "Dekont tutarı bildirilen tutarla eşleşmiyor." {
		t.Fatalf("red nedeni kaydedilmedi: %q", res.Deposit.RejectionReason)
	}
	if res.Deposit.ReviewedAt == nil || res.Deposit.ReviewedByUserID == nil {
		t.Fatal("🔴 reviewed_at / reviewed_by yazılmadı")
	}
	if got := balanceOf(t, uid); got != before {
		t.Fatalf("🔴 RED BAKİYEYİ DEĞİŞTİRDİ: %d → %d", before, got)
	}
	if n := countRows(t, `SELECT count(*) FROM ledger_entries WHERE user_id = $1`, uid); n != 0 {
		t.Fatalf("🔴 red için %d defter kaydı yazıldı", n)
	}
	if n := countRows(t,
		`SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'deposit.reject'`,
		d.PublicID.String()); n != 1 {
		t.Fatalf("🔴 red denetim kaydı sayısı = %d", n)
	}

	// ONAYLANMIŞ bir talep reddedilemez.
	ok := mustCreate(t, s, uid, activeBank(t), 5000, "DEKONT-2")
	if _, err := s.Approve(context.Background(), depositsvc.ApproveInput{
		AdminID: admin, PublicID: ok.PublicID}); err != nil {
		t.Fatalf("onay: %v", err)
	}
	_, err = s.Reject(context.Background(), depositsvc.RejectInput{
		AdminID: admin, PublicID: ok.PublicID, Reason: "vazgeçtim"})
	if got := codeOf(t, err); got != "DEPOSIT_NOT_PENDING" {
		t.Fatalf("🔴 onaylanmış talep reddedildi/yanlış hata: %s", got)
	}
}

// TestTerminalDepositCannotChangeStatus
//
// Değişmez #13'ün İKİNCİ savunma hattı: uygulama katmanı atlansa bile
// (elle psql, bir veri düzeltme betiği) terminal durumdan çıkılamaz.
// Bu test HAM SQL kullanır ve hatanın VARLIĞINI arar.
func TestTerminalDepositCannotChangeStatus(t *testing.T) {
	s := newService(t)
	admin := seedUser(t, "yonetici")
	uid := seedUser(t, "musteri")
	ctx := context.Background()

	completed := mustCreate(t, s, uid, activeBank(t), 5000, "DEKONT-1")
	if _, err := s.Approve(ctx, depositsvc.ApproveInput{AdminID: admin, PublicID: completed.PublicID}); err != nil {
		t.Fatalf("onay: %v", err)
	}
	rejected := mustCreate(t, s, uid, activeBank(t), 5000, "DEKONT-2")
	if _, err := s.Reject(ctx, depositsvc.RejectInput{
		AdminID: admin, PublicID: rejected.PublicID, Reason: "belge eksik"}); err != nil {
		t.Fatalf("red: %v", err)
	}

	forbidden := []struct {
		name string
		id   uuid.UUID
		to   string
	}{
		{"COMPLETED → PENDING", completed.PublicID, "PENDING"},
		{"COMPLETED → REJECTED", completed.PublicID, "REJECTED"},
		{"REJECTED → COMPLETED", rejected.PublicID, "COMPLETED"},
		{"REJECTED → PENDING", rejected.PublicID, "PENDING"},
	}
	for _, tc := range forbidden {
		_, err := pool.Exec(ctx,
			`UPDATE deposits SET status = $1::deposit_status WHERE public_id = $2`, tc.to, tc.id)
		if err == nil {
			t.Fatalf("🔴 %s HAM SQL ile yapılabildi — tetikleyici korumuyor", tc.name)
		}
	}

	// İzinli geçiş (COMPLETED → REFUNDED) GEÇEBİLMELİ: tetikleyici fazla
	// kısıtlı olursa ileride yazılacak iade akışı sessizce engellenir.
	if _, err := pool.Exec(ctx, `
		UPDATE deposits SET status = 'REFUNDED' WHERE public_id = $1`,
		completed.PublicID); err != nil {
		t.Fatalf("🔴 izinli COMPLETED → REFUNDED geçişi reddedildi: %v", err)
	}
}

/* ═══════════════════════ Dekont ═══════════════════════ */

func jpeg() []byte {
	return append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x41}, 128)...)
}

// TestReceiptUploadRejectsFakeImage — KK-500, servis katmanından.
//
// ".php uzantılı, image/jpeg MIME başlığı gönderilen bir dosya reddedilir":
// servise dosya adı ve MIME hiç ulaşmaz, karar içerikten verilir.
func TestReceiptUploadRejectsFakeImage(t *testing.T) {
	s := newService(t)
	uid := seedUser(t, "dekont")
	d := mustCreate(t, s, uid, activeBank(t), 5000, "DEKONT-1")
	ctx := context.Background()

	php := []byte("<?php system($_GET['c']); ?>" + strings.Repeat("A", 64))
	_, err := s.AttachReceipt(ctx, uid, d.PublicID, bytes.NewReader(php))
	if got := codeOf(t, err); got != "RECEIPT_UNSUPPORTED" {
		t.Fatalf("🔴 sahte MIME'lı PHP dosyası kabul edildi/yanlış hata: %s", got)
	}
	var path string
	if err := pool.QueryRow(ctx, `SELECT receipt_path FROM deposits WHERE id = $1`, d.ID).Scan(&path); err != nil {
		t.Fatalf("okunamadı: %v", err)
	}
	if path != "" {
		t.Fatalf("🔴 reddedilen dosya talebe bağlandı: %q", path)
	}

	// 5 MB üstü de reddedilir.
	big := append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{0x41}, int(storage.MaxReceiptBytes))...)
	_, err = s.AttachReceipt(ctx, uid, d.PublicID, bytes.NewReader(big))
	if got := codeOf(t, err); got != "RECEIPT_TOO_LARGE" {
		t.Fatalf("🔴 5 MB üstü dosya için kod = %s", got)
	}

	// Geçerli JPEG kabul edilir ve yol yazılır.
	updated, err := s.AttachReceipt(ctx, uid, d.PublicID, bytes.NewReader(jpeg()))
	if err != nil {
		t.Fatalf("🔴 geçerli JPEG reddedildi: %v", err)
	}
	if updated.ReceiptPath == "" {
		t.Fatal("🔴 dekont yolu yazılmadı")
	}
}

// TestOtherUserCannotReadReceipt
//
// Dekont yalnız SAHİBİ (ya da deposits:read izinli yönetim yolu) tarafından
// okunur. Başkası için 404 döner — "var ama senin değil" bile denmez.
func TestOtherUserCannotReadReceipt(t *testing.T) {
	s := newService(t)
	alice := seedUser(t, "alice")
	bob := seedUser(t, "bob")
	d := mustCreate(t, s, alice, activeBank(t), 5000, "DEKONT-1")
	ctx := context.Background()

	if _, err := s.AttachReceipt(ctx, alice, d.PublicID, bytes.NewReader(jpeg())); err != nil {
		t.Fatalf("dekont: %v", err)
	}

	// Bob okuyamaz.
	if _, _, err := s.OpenReceipt(ctx, &bob, d.PublicID); codeOf(t, err) != "DEPOSIT_NOT_FOUND" {
		t.Fatal("🔴 başka kullanıcı dekontu okuyabildi")
	}
	// Bob dekont YÜKLEYEMEZ.
	if _, err := s.AttachReceipt(ctx, bob, d.PublicID, bytes.NewReader(jpeg())); codeOf(t, err) != "DEPOSIT_NOT_FOUND" {
		t.Fatal("🔴 başka kullanıcı dekont yükleyebildi")
	}

	// Alice okur.
	rc, meta, err := s.OpenReceipt(ctx, &alice, d.PublicID)
	if err != nil {
		t.Fatalf("🔴 sahibi kendi dekontunu okuyamadı: %v", err)
	}
	_ = rc.Close()
	if meta.MIME != "image/jpeg" {
		t.Fatalf("MIME = %q", meta.MIME)
	}

	// Yönetim yolu (userID = nil) okur.
	rc2, _, err := s.OpenReceipt(ctx, nil, d.PublicID)
	if err != nil {
		t.Fatalf("🔴 yönetim yolu dekontu okuyamadı: %v", err)
	}
	_ = rc2.Close()
}

// TestApproveIsIdempotentUnderConcurrency — KK-502, SERVİS katmanında.
//
// HTTP katmanındaki eşi (handler/deposit_integration_test.go) tüm zinciri
// ölçer; bu test kilidin kendisini ölçer.
func TestApproveIsIdempotentUnderConcurrency(t *testing.T) {
	s := newService(t)
	admin := seedUser(t, "yonetici")
	uid := seedUser(t, "musteri")
	d := mustCreate(t, s, uid, activeBank(t), 4321, "DEKONT-1")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const n = 100
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := s.Approve(ctx, depositsvc.ApproveInput{AdminID: admin, PublicID: d.PublicID})
			done <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-done; err != nil {
			t.Fatalf("🔴 %d. eşzamanlı onay hata verdi: %v", i, err)
		}
	}

	if got := balanceOf(t, uid); got != 4321 {
		t.Fatalf("🔴 %d eşzamanlı onay bakiyeyi %d yaptı, 4321 bekleniyordu", n, got)
	}
	if c := countRows(t, `SELECT count(*) FROM ledger_entries WHERE user_id = $1`, uid); c != 1 {
		t.Fatalf("🔴 %d defter kaydı yazıldı, 1 bekleniyordu", c)
	}
	if c := countRows(t,
		`SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'deposit.approve'`,
		d.PublicID.String()); c != 1 {
		t.Fatalf("🔴 %d denetim kaydı yazıldı, 1 bekleniyordu", c)
	}

	// MUTABAKAT: Σ defter == bakiye.
	var sum int64
	if err := pool.QueryRow(context.Background(),
		`SELECT coalesce(sum(amount_minor), 0) FROM ledger_entries WHERE user_id = $1`,
		uid).Scan(&sum); err != nil {
		t.Fatalf("defter toplanamadı: %v", err)
	}
	if sum != balanceOf(t, uid) {
		t.Fatalf("🔴 mutabakat bozuk: Σ defter = %d, bakiye = %d", sum, balanceOf(t, uid))
	}
}

// TestDepositCreateIsIdempotent
//
// 🔴 BİR HAVALE, BİR TALEP.
//
// Kullanıcı 500 ₺ havale eder, formu gönderir, mobil ağda yanıt kaybolur.
// Kullanıcı hata görür ve tekrar basar. Koruma yoksa iki bekleyen talep
// oluşur; yönetici ikisini de onaylarsa 500 ₺'lik havaleye 1000 ₺ yazılır.
//
// Defterin idempotency anahtarı bunu YAKALAMAZ: o anahtar `deposit:{public_id}`
// üzerinden türüyor ve iki talebin iki ayrı public_id'si var. Koruma TALEP
// oluşturma anında olmalı.
func TestDepositCreateIsIdempotent(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	uid := seedUser(t, "idempotens")
	m := activeBank(t)

	const anahtar = "istemci-anahtari-9f3c"
	in := depositsvc.CreateInput{
		UserID: uid, MethodPublicID: m.PublicID, AmountMinor: 50_000,
		Reference: "Dekont 12345", IdempotencyKey: anahtar,
	}

	ilk, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("ilk talep: %v", err)
	}
	ikinci, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("ikinci talep hata verdi (geri alınmalıydı): %v", err)
	}

	if ilk.PublicID != ikinci.PublicID {
		t.Errorf("🔴 aynı anahtarla İKİ AYRI talep oluştu (%s, %s) — "+
			"yönetici ikisini de onaylarsa kullanıcıya iki kat yazılır",
			ilk.PublicID, ikinci.PublicID)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM deposits WHERE user_id=$1`, uid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("🔴 %d talep satırı var, 1 bekleniyordu", n)
	}

	// Test bir şey doğrulasın: FARKLI anahtar AYRI talep açmalı — yoksa
	// "her zaman ilkini döndür" diyen bozuk bir uygulama da geçerdi.
	in2 := in
	in2.IdempotencyKey = "baska-anahtar-0001"
	ucuncu, err := svc.Create(ctx, in2)
	if err != nil {
		t.Fatalf("farklı anahtarlı talep: %v", err)
	}
	if ucuncu.PublicID == ilk.PublicID {
		t.Error("🔴 farklı anahtar aynı talebi döndürdü — kullanıcı ikinci havalesini bildiremez")
	}

	// Anahtarsız istek eski davranışı korumalı (istemciler kırılmasın).
	in3 := in
	in3.IdempotencyKey = ""
	if _, err := svc.Create(ctx, in3); err != nil {
		t.Errorf("anahtarsız talep reddedildi: %v", err)
	}
}

// TestDepositCreateIsIdempotentUnderConcurrency
//
// EŞZAMANLILIK TESTİ ZORUNLUDUR (CLAUDE.md, "Para hareketi ekleme" §4):
// sıralı bir tekrar, tekil indeks olmadan da doğru sonuç verebilir. Yarış
// ancak paralel çağrıda ortaya çıkar — iki istek "önce kontrol et" adımını
// aynı anda geçer ve ikisi de yazmaya çalışır.
//
// Mobilde kullanıcı düğmeye iki kez basarsa tam olarak bu olur.
func TestDepositCreateIsIdempotentUnderConcurrency(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	uid := seedUser(t, "eszamanli")
	m := activeBank(t)

	in := depositsvc.CreateInput{
		UserID: uid, MethodPublicID: m.PublicID, AmountMinor: 50_000,
		Reference: "Dekont 777", IdempotencyKey: "paralel-anahtar-0001",
	}

	const n = 12
	var mu sync.Mutex
	kimlikler := map[string]int{}
	hatalar := 0

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dep, err := svc.Create(ctx, in)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				hatalar++
				return
			}
			kimlikler[dep.PublicID.String()]++
		}()
	}
	wg.Wait()

	var satir int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM deposits WHERE user_id=$1`, uid).Scan(&satir); err != nil {
		t.Fatal(err)
	}
	if satir != 1 {
		t.Fatalf("🔴 %d paralel istek %d talep satırı üretti, 1 bekleniyordu — "+
			"yönetici aynı havale için birden çok satır görür ve hepsini onaylayabilir", n, satir)
	}
	if len(kimlikler) != 1 {
		t.Fatalf("🔴 %d farklı talep kimliği döndü: %v", len(kimlikler), kimlikler)
	}
	if hatalar > 0 {
		t.Errorf("%d istek hata aldı — yarışı kaybeden istek de MEVCUT talebi almalıydı", hatalar)
	}
}

// TestListUserDepositsDurumSuzgeci `GET /wallet/deposits?status=` süzgecini
// doğrular.
//
// ÜÇ İDDİA TEK TESTTE:
//  1. Süzgeç süzer.
//  2. SAYIM LİSTEYLE AYNI SÜZGECİ ALIR — `CountUserDeposits` süzgeçsiz
//     kalsaydı liste doğru görünür ama sayfalama "1–25 / 9" derken 3 satır
//     gösterirdi. Ayrı testte olsaydı bu ayrışma görünmezdi.
//  3. SAHİPLİK SÜZGEÇTEN ÜSTÜNDÜR (değişmez #7): başka kullanıcının aynı
//     durumdaki talebi hiçbir süzgeçle görünmez.
func TestListUserDepositsDurumSuzgeci(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	m := activeBank(t)
	alice := seedUser(t, "suzgec-alice")
	bob := seedUser(t, "suzgec-bob")

	mustCreate(t, s, alice, m, 5000, "SUZ-BEKLEYEN")
	reddedilen := mustCreate(t, s, alice, m, 6000, "SUZ-RED")
	// Bob'un talebi de REJECTED olur: Alice REJECTED süzdüğünde ONU GÖRMEMELİ.
	bobunki := mustCreate(t, s, bob, m, 7000, "SUZ-BOB")

	// Durum doğrudan SQL ile kurulur: `Reject` denetim kaydı, yönetici kimliği
	// ve saat ister; bu testin iddiası ise yalnız SORGUdur.
	//
	// `reviewed_at` ZORUNLU: `deposit_reviewed_has_time` kısıtı PENDING dışındaki
	// her durumda inceleme saatini şart koşar — şemanın kendisi terminal bir
	// talebin ne zaman karara bağlandığını bilmeyi garanti ediyor.
	for _, id := range []int64{reddedilen.ID, bobunki.ID} {
		if _, err := pool.Exec(ctx,
			`UPDATE deposits SET status='REJECTED', reviewed_at=now() WHERE id=$1`, id); err != nil {
			t.Fatalf("durum kurulumu: %v", err)
		}
	}

	durum := func(v db.DepositStatus) *db.DepositStatus { return &v }

	kontrol := func(ad string, st *db.DepositStatus, bekTutar ...int64) {
		t.Helper()
		rows, total, err := s.List(ctx, alice, st, 20, 0)
		if err != nil {
			t.Fatalf("%s: liste: %v", ad, err)
		}
		if int(total) != len(rows) {
			t.Errorf("%s: SAYIM LİSTEDEN AYRIŞTI — total=%d satır=%d "+
				"(CountUserDeposits süzgeci ListUserDeposits ile aynı değil)", ad, total, len(rows))
		}
		if len(rows) != len(bekTutar) {
			t.Fatalf("%s: %d kayıt bekleniyordu, %d geldi", ad, len(bekTutar), len(rows))
		}
		// Kayıtlar TUTARLA ayırt edilir: `db.Deposit` bir referans alanı
		// taşımaz ve üç talebin tutarı bilerek farklı seçildi.
		got := make(map[int64]bool, len(rows))
		for _, r := range rows {
			got[r.AmountMinor] = true
		}
		for _, tutar := range bekTutar {
			if !got[tutar] {
				t.Errorf("%s: %d kuruşluk talep listede yok", ad, tutar)
			}
		}
	}

	kontrol("süzgeçsiz", nil, 5000, 6000)
	kontrol("bekleyen", durum(db.DepositStatusPENDING), 5000)
	// 🔴 Bob'un talebi de REJECTED (7000) — bu satır sahiplik iddiasıdır.
	kontrol("reddedilen", durum(db.DepositStatusREJECTED), 6000)
	kontrol("hiç olmayan durum", durum(db.DepositStatusREFUNDED))
}

// TestListDepositsForAdminAramasi `GET /admin/deposits?q=` aramasını doğrular.
//
// 🔴 ASIL İDDİA SAYIMIN LİSTEYLE AYNI SORGUYU KURMASI. `q` kullanıcı
// sütunlarında (`users.email`, `users.username`) arandığı için `count`
// sorgusuna da bir `JOIN users` EKLENDİ. JOIN unutulsaydı liste doğru süzerdi
// ama sayım TÜM kayıtları sayardı: sayfalama "1–25 / 120" derken ekranda 2
// satır olurdu. Bu test her çağrıda ikisini birlikte ölçer.
func TestListDepositsForAdminAramasi(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	m := activeBank(t)
	// `seedUser` son eki hem e-postaya hem kullanıcı adına gömer, yani "ada"
	// araması bu kullanıcının HER İKİ sütununda da eşleşir.
	ada := seedUser(t, "ada")
	kemal := seedUser(t, "kemal")

	mustCreate(t, s, ada, m, 5000, "ARAMA-ADA")
	mustCreate(t, s, kemal, m, 6000, "ARAMA-KEMAL")

	metin := func(v string) *string { return &v }

	kontrol := func(ad string, ara *string, bekTutar ...int64) {
		t.Helper()
		rows, total, err := s.ListForAdmin(ctx, nil, ara, 50, 0)
		if err != nil {
			t.Fatalf("%s: liste: %v", ad, err)
		}
		if int(total) != len(rows) {
			t.Errorf("%s: SAYIM LİSTEDEN AYRIŞTI — total=%d satır=%d "+
				"(CountDepositsForAdmin sorgusu ListDepositsForAdmin ile aynı değil)",
				ad, total, len(rows))
		}
		if len(rows) != len(bekTutar) {
			t.Fatalf("%s: %d kayıt bekleniyordu, %d geldi", ad, len(bekTutar), len(rows))
		}
		got := make(map[int64]bool, len(rows))
		for _, r := range rows {
			got[r.AmountMinor] = true
		}
		for _, tutar := range bekTutar {
			if !got[tutar] {
				t.Errorf("%s: %d kuruşluk talep listede yok", ad, tutar)
			}
		}
	}

	kontrol("aramasız", nil, 5000, 6000)
	kontrol("kullanıcıya göre", metin("ada"), 5000)
	// ILIKE: büyük/küçük harf ayrımı yok.
	kontrol("büyük harf", metin("KEMAL"), 6000)
	kontrol("eşleşmeyen", metin("bulunmayan-kullanici"))
}
