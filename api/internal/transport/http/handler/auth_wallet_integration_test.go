//go:build integration

package handler_test

// Oturum listeleme ve elle bakiye düzeltme uçlarının garantileri.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	authsvc "github.com/ikmetrik/sms-platform/api/internal/service/auth"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// seedUser oturum açmış bir kullanıcı oluşturur.
func seedUser(t *testing.T, suffix string) (id int64, publicID uuid.UUID) {
	t.Helper()
	// Benzersizlik testin adından DEĞİL, rastgele bir ekten gelir: test
	// adları kırpıldığında çakışıyordu (users_username_key).
	tag := uuid.NewString()[:8]
	email := fmt.Sprintf("test-%s-%s@ornek.test", suffix, tag)
	uname := fmt.Sprintf("k_%s_%s", suffix, tag)
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, username, password_hash, status, balance_minor)
		VALUES ($1, $2, '$2a$10$x', 'ACTIVE', 0)
		RETURNING id, public_id`, email, uname).Scan(&id, &publicID)
	if err != nil {
		t.Fatalf("kullanıcı eklenemedi: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_entries WHERE user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	})
	return id, publicID
}

// asUser isteği oturum açmış kullanıcı olarak çalıştıran ara katman.
func asUser(userID int64, sessionID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.CtxUserID, userID)
		if sessionID != "" {
			c.Set(middleware.CtxSessionID, sessionID)
		}
		c.Next()
	}
}

func testResponder() handler.Responder {
	return handler.Responder{
		OK:        func(c *gin.Context, b any) { c.JSON(http.StatusOK, b) },
		NoContent: func(c *gin.Context) { c.Status(http.StatusNoContent) },
		Fail: func(c *gin.Context, err error) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		},
		FailField: func(c *gin.Context, errs []dto.FieldError) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"fields": errs})
		},
	}
}

/* ═════════════════ Oturum listesi ═════════════════ */

// TestSessionListDoesNotLeakToken
//
// 🔴 Ham oturum kimliği bir TAŞIYICI TOKEN'dır: onu bilen kişi o oturumdur.
// Çerez httpOnly olduğu için JavaScript okuyamaz — ama listeleme ucu aynı
// değeri JSON'da dönerse bu koruma tümüyle etkisiz kalır: tek bir XSS,
// kullanıcının TÜM cihazlarındaki oturumları çalabilir.
//
// Yanıtta HANDLE döner: listelemeye ve iptale yeter, kimlik doğrulamaya
// yetmez.
func TestSessionListDoesNotLeakToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	uid, _ := seedUser(t, "oturum")

	// Tanınabilir, tahmin edilemez ham oturum kimlikleri.
	raw := []string{
		"HAM_OTURUM_KIMLIGI_BIRINCI_" + uuid.NewString(),
		"HAM_OTURUM_KIMLIGI_IKINCI_" + uuid.NewString(),
	}
	for _, sid := range raw {
		_, err := pool.Exec(context.Background(), `
			INSERT INTO sessions (id, user_id, user_agent, expires_at)
			VALUES ($1, $2, 'test-tarayici', now() + interval '1 hour')`, sid, uid)
		if err != nil {
			t.Fatalf("oturum eklenemedi: %v", err)
		}
	}

	svc := authsvc.New(authsvc.Deps{
		TxRunner:   postgres.NewTxRunner(pool),
		SessionTTL: time.Hour,
	})
	r := gin.New()
	r.Use(asUser(uid, raw[0]))
	h := handler.NewAuth(svc, db.New(pool), testResponder(), time.Hour, false)
	r.GET("/me/sessions", h.ListSessions)

	rig := &adminRig{router: r}
	w, body := rig.do(t, http.MethodGet, "/me/sessions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d — gövde: %s", w.Code, body)
	}

	// (1) HAM KİMLİK YANITTA YOK.
	for _, sid := range raw {
		if strings.Contains(body, sid) {
			t.Fatalf("🔴 ham oturum kimliği YANITTA döndü — httpOnly koruması etkisiz")
		}
	}

	var resp struct {
		Items []struct {
			ID      string `json:"id"`
			Current bool   `json:"current"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("yanıt çözümlenemedi: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("oturum sayısı = %d, 2 bekleniyordu — test bir şey doğrulamıyor", len(resp.Items))
	}

	// (2) Dönen değer HANDLE ve tek yönlü: handle'dan ham kimliğe gidilemez.
	for _, it := range resp.Items {
		if it.ID == "" {
			t.Error("boş oturum kimliği")
		}
		for _, sid := range raw {
			if it.ID == sid || strings.Contains(sid, it.ID) {
				t.Errorf("🔴 handle ham kimliğin bir parçası: %q", it.ID)
			}
		}
		if want := authsvc.SessionHandle(raw[0]); it.ID != want && it.Current {
			t.Errorf("geçerli oturum işareti yanlış handle'da: %q", it.ID)
		}
	}

	// (3) Handle'lar birbirinden FARKLI — aksi hâlde iptal yanlış oturumu düşürür.
	if resp.Items[0].ID == resp.Items[1].ID {
		t.Error("🔴 iki oturum aynı handle'a sahip")
	}

	// (4) Tam olarak BİR tanesi "geçerli oturum" işaretli ve karşılaştırma
	// sunucuda yapılmış olmalı.
	n := 0
	for _, it := range resp.Items {
		if it.Current {
			n++
		}
	}
	if n != 1 {
		t.Errorf("geçerli oturum sayısı = %d, 1 bekleniyordu", n)
	}
}

/* ═════════════════ Elle bakiye düzeltme ═════════════════ */

func newWalletRig(t *testing.T, adminID int64) *adminRig {
	t.Helper()
	gin.SetMode(gin.TestMode)
	q := db.New(pool)
	h := handler.NewWallet(walletsvc.New(postgres.NewTxRunner(pool)), q, testResponder())
	r := gin.New()
	r.Use(asUser(adminID, ""))
	r.POST("/admin/users/:id/balance", h.AdjustBalance)
	return &adminRig{router: r, queries: q}
}

func balanceOf(t *testing.T, uid int64) int64 {
	t.Helper()
	var b int64
	if err := pool.QueryRow(context.Background(),
		`SELECT balance_minor FROM users WHERE id = $1`, uid).Scan(&b); err != nil {
		t.Fatalf("bakiye okunamadı: %v", err)
	}
	return b
}

func ledgerCount(t *testing.T, uid int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ledger_entries WHERE user_id = $1`, uid).Scan(&n); err != nil {
		t.Fatalf("defter sayılamadı: %v", err)
	}
	return n
}

// TestAdjustIsIdempotent
//
// Anahtar İSTEĞİN İÇERİĞİNDEN türer, taşıyıcısından değil:
// `manual:{hedefPublicID}:{istemciAnahtarı}`.
//
// İki ayrı garanti var ve ikisi de para kaybettirir:
//   - AYNI anahtar, AYNI hedef → TEK etki. Ağ hatası sonrası tekrarlanan
//     istek bakiyeyi ikinci kez artırmaz.
//   - AYNI anahtar, FARKLI hedef → AYRI işlem. Hedef anahtara dahil
//     olmasaydı, ikinci kullanıcının düzeltmesi "zaten yapıldı" sayılıp
//     sessizce YUTULURDU.
func TestAdjustIsIdempotent(t *testing.T) {
	adminID, _ := seedUser(t, "yonetici")
	aliceID, alicePub := seedUser(t, "alice")
	bobID, bobPub := seedUser(t, "bob")
	rig := newWalletRig(t, adminID)

	const key = "panel-duzeltme-2026-09-08-0001"
	const amount int64 = 12_345 // 123,45 ₺

	adjust := func(pub uuid.UUID) (int, string) {
		w, body := rig.do(t, http.MethodPost, "/admin/users/"+pub.String()+"/balance",
			dto.AdjustBalanceRequest{AmountMinor: amount, Note: "elle düzeltme", IdempotencyKey: key})
		return w.Code, body
	}

	// (1) İlk düzeltme uygulanır.
	if code, body := adjust(alicePub); code != http.StatusOK {
		t.Fatalf("ilk düzeltme durum = %d — gövde: %s", code, body)
	}
	if got := balanceOf(t, aliceID); got != amount {
		t.Fatalf("bakiye = %d, %d bekleniyordu", got, amount)
	}

	// (2) AYNI anahtarla tekrar → bakiye DEĞİŞMEZ, yeni defter kaydı yok.
	before := ledgerCount(t, aliceID)
	if code, body := adjust(alicePub); code != http.StatusOK {
		t.Fatalf("tekrar durum = %d — gövde: %s", code, body)
	}
	if got := balanceOf(t, aliceID); got != amount {
		t.Fatalf("🔴 aynı anahtarla ikinci istek bakiyeyi değiştirdi: %d (beklenen %d)", got, amount)
	}
	if after := ledgerCount(t, aliceID); after != before {
		t.Fatalf("🔴 aynı anahtarla ikinci defter kaydı yazıldı: %d → %d", before, after)
	}

	// (3) AYNI anahtar, FARKLI hedef → AYRI işlem, yutulmaz.
	if code, body := adjust(bobPub); code != http.StatusOK {
		t.Fatalf("ikinci kullanıcı durum = %d — gövde: %s", code, body)
	}
	if got := balanceOf(t, bobID); got != amount {
		t.Fatalf("🔴 aynı anahtar farklı hedefte YUTULDU: bakiye = %d, %d bekleniyordu", got, amount)
	}
}

// TestAdjustIsIdempotentUnderConcurrency
//
// EŞZAMANLILIK TESTİ ZORUNLUDUR (CLAUDE.md, "Para hareketi ekleme" §4):
// sıralı bir tekrar, `SELECT ... FOR UPDATE` kilidi olmadan da doğru sonuç
// verebilir. Yarış ancak paralel çağrıda ortaya çıkar.
func TestAdjustIsIdempotentUnderConcurrency(t *testing.T) {
	adminID, _ := seedUser(t, "yonetici")
	uid, pub := seedUser(t, "hedef")
	rig := newWalletRig(t, adminID)

	const key = "eszamanli-duzeltme-0001"
	const amount int64 = 5_000

	var mu sync.Mutex
	codes := map[int]int{}
	var g errgroup.Group
	for i := 0; i < 8; i++ {
		g.Go(func() error {
			w, _ := rig.do(t, http.MethodPost, "/admin/users/"+pub.String()+"/balance",
				dto.AdjustBalanceRequest{AmountMinor: amount, Note: "eşzamanlı", IdempotencyKey: key})
			mu.Lock()
			codes[w.Code]++
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	if got := balanceOf(t, uid); got != amount {
		t.Fatalf("🔴 8 paralel çağrı %d kuruş yazdı, %d bekleniyordu — kilit tutmuyor (durumlar: %v)",
			got, amount, codes)
	}
	if n := ledgerCount(t, uid); n != 1 {
		t.Fatalf("🔴 %d defter kaydı yazıldı, 1 bekleniyordu", n)
	}

	// Mutabakat: Σ defter == bakiye.
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
