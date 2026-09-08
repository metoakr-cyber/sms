//go:build integration

package handler_test

// Bakiye yükleme uçlarının HTTP katmanı garantileri: yetki matrisi,
// eşzamanlı onay (KK-502) ve dekont erişimi (KK-500).
//
// TestMain, pool ve seedUser admin_integration_test.go içinde tanımlıdır.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/storage"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	depositsvc "github.com/ikmetrik/sms-platform/api/internal/service/deposit"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

/* ═══════════════════════ Kurulum ═══════════════════════ */

// depositRig gerçek yönlendiriciyi taklit eder.
//
// 🔴 HIZ LİMİTİ ARA KATMANI BAĞLANMAZ. Yönetim uçları üretimde 60 istek/dk
// ile sınırlıdır; test rig'ine bağlansaydı 100 paralel çağrının 40'ı 429 alır
// ve eşzamanlılık testi hiçbir şey ispatlamazdı.
type depositRig struct {
	router *gin.Engine
	store  *storage.Local
}

// kimlik başlıktan gelir: rig alanlarını mutasyona uğratmak, 100 paralel
// istekte veri yarışı olurdu.
const (
	hdrUser  = "X-Test-User"
	hdrPerms = "X-Test-Perms"
)

func depositResponder() handler.Responder {
	// Gerçek response.go ile AYNI eşleme: 401/403/404/409/422 ayrımı bu
	// testlerin ölçtüğü şeydir; hepsini 400'e indiren bir sahte yanıtlayıcı
	// yetki matrisini görünmez kılardı.
	return handler.Responder{
		OK:        func(c *gin.Context, b any) { c.JSON(http.StatusOK, b) },
		NoContent: func(c *gin.Context) { c.Status(http.StatusNoContent) },
		Fail: func(c *gin.Context, err error) {
			appErr, ok := apperr.As(err)
			if !ok {
				appErr = apperr.Internal(err)
			}
			c.AbortWithStatusJSON(appErr.HTTPStatus(), gin.H{
				"error": gin.H{"code": appErr.Code, "message": appErr.Message},
			})
		},
		FailField: func(c *gin.Context, errs []dto.FieldError) {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"fields": errs})
		},
	}
}

func newDepositRig(t *testing.T) *depositRig {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tx := postgres.NewTxRunner(pool)
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("depo: %v", err)
	}
	svc := depositsvc.New(depositsvc.Deps{
		TxRunner: tx, Wallet: walletsvc.New(tx),
		Clock: port.RealClock{}, Receipts: store,
	})
	r := depositResponder()
	h := handler.NewDeposit(svc, r)

	// requireAuth eşdeğeri: oturum yoksa 401 (403 DEĞİL). Yetki kontrolünden
	// ÖNCE çalışır — gerçek router'daki sıra budur.
	auth := func(c *gin.Context) {
		raw := c.GetHeader(hdrUser)
		if raw == "" {
			r.Fail(c, apperr.ErrUnauthenticated)
			return
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			r.Fail(c, apperr.ErrUnauthenticated)
			return
		}
		c.Set(middleware.CtxUserID, id)
		var perms []string
		if p := c.GetHeader(hdrPerms); p != "" {
			perms = strings.Split(p, ",")
		}
		c.Set(middleware.CtxPermissions, perms)
		c.Next()
	}

	e := gin.New()
	e.NoRoute(func(c *gin.Context) { c.JSON(http.StatusNotFound, gin.H{"error": "yok"}) })

	g := e.Group("/api/v1", auth)
	g.GET("/wallet/deposit-methods", h.Methods)
	g.POST("/wallet/deposits", h.Create)
	g.GET("/wallet/deposits", h.List)
	g.GET("/wallet/deposits/:id", h.Get)
	g.POST("/wallet/deposits/:id/receipt", h.UploadReceipt)
	g.GET("/wallet/deposits/:id/receipt", h.Receipt)

	g.GET("/admin/deposits",
		middleware.RequirePermission("deposits:read", r.Fail), h.AdminList)
	g.GET("/admin/deposits/:id/receipt",
		middleware.RequirePermission("deposits:read", r.Fail), h.AdminReceipt)
	g.POST("/admin/deposits/:id/approve",
		middleware.RequirePermission("deposits:approve", r.Fail), h.Approve)
	g.POST("/admin/deposits/:id/reject",
		middleware.RequirePermission("deposits:approve", r.Fail), h.Reject)

	return &depositRig{router: e, store: store}
}

type caller struct {
	userID int64
	perms  string
}

func (rig *depositRig) do(t *testing.T, as caller, method, path string, body any) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var rdr io.Reader = strings.NewReader("")
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("gövde: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if as.userID != 0 {
		req.Header.Set(hdrUser, strconv.FormatInt(as.userID, 10))
	}
	if as.perms != "" {
		req.Header.Set(hdrPerms, as.perms)
	}
	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, req)
	return w, w.Body.String()
}

// upload multipart bir dekont yükler.
//
// İstemcinin İDDİA ETTİĞİ dosya adı ve Content-Type buraya konur — sunucunun
// bunlara bakmadığını ölçebilmek için.
func (rig *depositRig) upload(t *testing.T, as caller, depositID, filename, mime string, content []byte) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := make(map[string][]string)
	hdr["Content-Disposition"] = []string{
		fmt.Sprintf(`form-data; name="dekont"; filename=%q`, filename)}
	hdr["Content-Type"] = []string{mime}
	part, err := mw.CreatePart(hdr)
	if err != nil {
		t.Fatalf("multipart: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("multipart yazımı: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart kapanışı: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/wallet/deposits/"+depositID+"/receipt", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(hdrUser, strconv.FormatInt(as.userID, 10))
	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, req)
	return w, w.Body.String()
}

/* ═══════════════════════ Veri hazırlığı ═══════════════════════ */

func seedActiveMethod(t *testing.T) uuid.UUID {
	t.Helper()
	code := "http-" + uuid.NewString()[:8]
	var pubID uuid.UUID
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO deposit_methods (code, kind, name, instructions, config,
		                             min_amount_minor, max_amount_minor, is_active)
		VALUES ($1, 'BANK_TRANSFER', $2, 'talimat',
		        '{"iban":"TR000000000000000000000000","hesapAdi":"Test"}'::jsonb,
		        1000, 5000000, true)
		RETURNING id, public_id`, code, "HTTP Yöntemi "+code).Scan(&id, &pubID)
	if err != nil {
		t.Fatalf("yöntem eklenemedi: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM deposit_methods WHERE id = $1`, id)
	})
	return pubID
}

// seedDepositUser kullanıcı ekler ve yükleme kayıtlarının temizliğini kurar.
//
// seedUser'ın kendi temizliği kullanıcıyı siler; deposits.user_id yabancı
// anahtarı yüzünden yükleme kayıtları ÖNCE silinmelidir. t.Cleanup LIFO
// çalıştığı için burada kaydedilen temizlik önce koşar.
func seedDepositUser(t *testing.T, suffix string) int64 {
	t.Helper()
	id, _ := seedUser(t, suffix)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM deposits WHERE user_id = $1 OR reviewed_by_user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_entries WHERE user_id = $1 OR created_by_user_id = $1`, id)
	})
	return id
}

func (rig *depositRig) newDeposit(t *testing.T, userID int64, method uuid.UUID, amount int64, ref string) string {
	t.Helper()
	w, body := rig.do(t, caller{userID: userID}, http.MethodPost, "/api/v1/wallet/deposits",
		dto.CreateDepositRequest{MethodID: method.String(), AmountMinor: amount, Reference: ref})
	if w.Code != http.StatusOK {
		t.Fatalf("talep açılamadı (%d): %s", w.Code, body)
	}
	var resp dto.DepositResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	return resp.ID
}

func depositBalance(t *testing.T, userID int64) int64 {
	t.Helper()
	var v int64
	if err := pool.QueryRow(context.Background(),
		`SELECT balance_minor FROM users WHERE id = $1`, userID).Scan(&v); err != nil {
		t.Fatalf("bakiye: %v", err)
	}
	return v
}

func depositCount(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("sayım: %v", err)
	}
	return n
}

func jpegBody() []byte {
	return append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x41}, 128)...)
}

/* ═══════════════════════ Yetki matrisi ═══════════════════════ */

// TestApproveRequiresPermission
//
// CLAUDE.md "Yeni uç nokta ekleme" §7 zorunlu üçlüsü + KK-502'nin yetki yarısı:
//
//	oturumsuz              → 401
//	yetkisiz (yanlış izin) → 403
//	var olmayan / başkası  → 404
//
// Sıra ÖNEMLİDİR: oturumsuz istek 403 değil 401 almalıdır, çünkü kullanıcıya
// "giriş yap" mı "yetkin yok" mu denileceği farklı bir eylem gerektirir.
func TestApproveRequiresPermission(t *testing.T) {
	rig := newDepositRig(t)
	admin := seedDepositUser(t, "yonetici")
	uid := seedDepositUser(t, "musteri")
	method := seedActiveMethod(t)
	depID := rig.newDeposit(t, uid, method, 5000, "DEKONT-1")

	approve := "/api/v1/admin/deposits/" + depID + "/approve"
	reject := "/api/v1/admin/deposits/" + depID + "/reject"
	list := "/api/v1/admin/deposits"

	type tc struct {
		name         string
		as           caller
		method, path string
		body         any
		want         int
	}
	cases := []tc{
		// ── oturumsuz → 401 ──
		{"onay: oturumsuz", caller{}, http.MethodPost, approve, dto.ApproveDepositRequest{}, 401},
		{"red: oturumsuz", caller{}, http.MethodPost, reject, dto.RejectDepositRequest{Reason: "gerekçe"}, 401},
		{"liste: oturumsuz", caller{}, http.MethodGet, list, nil, 401},
		{"dekont: oturumsuz", caller{}, http.MethodGet,
			"/api/v1/admin/deposits/" + depID + "/receipt", nil, 401},

		// ── yetkisiz → 403 ──
		{"onay: yalnız okuma izni", caller{admin, "deposits:read"}, http.MethodPost, approve, dto.ApproveDepositRequest{}, 403},
		{"red: yalnız okuma izni", caller{admin, "deposits:read"}, http.MethodPost, reject, dto.RejectDepositRequest{Reason: "gerekçe"}, 403},
		{"onay: izinsiz", caller{uid, ""}, http.MethodPost, approve, dto.ApproveDepositRequest{}, 403},
		{"liste: izinsiz", caller{uid, ""}, http.MethodGet, list, nil, 403},
		{"liste: yanlış izin", caller{uid, "users:read"}, http.MethodGet, list, nil, 403},

		// ── var olmayan kayıt → 404 ──
		{"onay: var olmayan talep", caller{admin, "deposits:approve"}, http.MethodPost,
			"/api/v1/admin/deposits/" + uuid.NewString() + "/approve", dto.ApproveDepositRequest{}, 404},
		{"onay: bozuk kimlik", caller{admin, "deposits:approve"}, http.MethodPost,
			"/api/v1/admin/deposits/bozuk-kimlik/approve", dto.ApproveDepositRequest{}, 404},
		{"red: var olmayan talep", caller{admin, "deposits:approve"}, http.MethodPost,
			"/api/v1/admin/deposits/" + uuid.NewString() + "/reject",
			dto.RejectDepositRequest{Reason: "gerekçe yeterli"}, 404},

		// ── kullanıcı uçları: başkasının talebi → 404 ──
		{"kullanıcı: başkasının talebi", caller{userID: admin}, http.MethodGet,
			"/api/v1/wallet/deposits/" + depID, nil, 404},
		{"kullanıcı: başkasının dekontu", caller{userID: admin}, http.MethodGet,
			"/api/v1/wallet/deposits/" + depID + "/receipt", nil, 404},
		{"kullanıcı: oturumsuz liste", caller{}, http.MethodGet, "/api/v1/wallet/deposits", nil, 401},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, body := rig.do(t, c.as, c.method, c.path, c.body)
			if w.Code != c.want {
				t.Fatalf("🔴 durum = %d, %d bekleniyordu — gövde: %s", w.Code, c.want, body)
			}
		})
	}

	// Talep hâlâ PENDING: yukarıdaki reddedilen isteklerin HİÇBİRİ yazmadı.
	if n := depositCount(t,
		`SELECT count(*) FROM deposits WHERE public_id = $1 AND status = 'PENDING'`, depID); n != 1 {
		t.Fatal("🔴 yetkisiz istekler talebin durumunu değiştirdi")
	}
	if got := depositBalance(t, uid); got != 0 {
		t.Fatalf("🔴 yetkisiz istekler bakiyeyi değiştirdi: %d", got)
	}

	// Doğru izinle GERÇEKTEN çalışmalı — aksi hâlde test "her şeyi reddet"
	// ile de geçerdi.
	w, body := rig.do(t, caller{admin, "deposits:approve"}, http.MethodPost, approve,
		dto.ApproveDepositRequest{})
	if w.Code != http.StatusOK {
		t.Fatalf("🔴 yetkili onay reddedildi (%d): %s", w.Code, body)
	}
	if got := depositBalance(t, uid); got != 5000 {
		t.Fatalf("bakiye = %d", got)
	}
}

// TestRejectReasonRequired — FR-503.
//
// rejection_reason veritabanında NOT NULL DEFAULT ” olduğu için bu
// zorunluluğun TEK uygulayıcısı DTO doğrulamasıdır.
func TestRejectReasonRequired(t *testing.T) {
	rig := newDepositRig(t)
	admin := seedDepositUser(t, "yonetici")
	uid := seedDepositUser(t, "musteri")
	depID := rig.newDeposit(t, uid, seedActiveMethod(t), 5000, "DEKONT-1")
	path := "/api/v1/admin/deposits/" + depID + "/reject"
	as := caller{admin, "deposits:approve"}

	for _, reason := range []string{"", "   ", "kisa"} {
		w, body := rig.do(t, as, http.MethodPost, path, dto.RejectDepositRequest{Reason: reason})
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("🔴 %q gerekçesiyle red kabul edildi (%d): %s", reason, w.Code, body)
		}
		var resp struct {
			Fields []dto.FieldError `json:"fields"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil || len(resp.Fields) == 0 {
			t.Fatalf("alan hatası dönmedi: %s", body)
		}
		if resp.Fields[0].Field != "reason" {
			t.Fatalf("🔴 hatalı alan = %q, 'reason' bekleniyordu", resp.Fields[0].Field)
		}
	}

	// Talep hâlâ PENDING ve bakiye değişmemiş.
	if n := depositCount(t,
		`SELECT count(*) FROM deposits WHERE public_id = $1 AND status = 'PENDING'`, depID); n != 1 {
		t.Fatal("🔴 gerekçesiz red talebi sonuçlandırdı")
	}
	if got := depositBalance(t, uid); got != 0 {
		t.Fatalf("🔴 red bakiyeyi değiştirdi: %d", got)
	}

	// Geçerli gerekçe kabul edilir ve kullanıcıya döner.
	const reason = "Dekont tutarı bildirilen tutarla eşleşmiyor."
	w, body := rig.do(t, as, http.MethodPost, path, dto.RejectDepositRequest{Reason: reason})
	if w.Code != http.StatusOK {
		t.Fatalf("geçerli red reddedildi (%d): %s", w.Code, body)
	}
	uw, ubody := rig.do(t, caller{userID: uid}, http.MethodGet, "/api/v1/wallet/deposits/"+depID, nil)
	if uw.Code != http.StatusOK {
		t.Fatalf("kullanıcı talebini okuyamadı: %s", ubody)
	}
	var user dto.DepositResponse
	if err := json.Unmarshal([]byte(ubody), &user); err != nil {
		t.Fatalf("çözülemedi: %v", err)
	}
	if user.RejectionReason != reason {
		t.Fatalf("🔴 red nedeni kullanıcıya gösterilmiyor: %q", user.RejectionReason)
	}

	// İkinci bir red 200 + alreadyApplied döner; onay ise 409.
	w2, _ := rig.do(t, as, http.MethodPost, path, dto.RejectDepositRequest{Reason: reason})
	if w2.Code != http.StatusOK {
		t.Fatalf("tekrarlanan red durumu = %d", w2.Code)
	}
	w3, b3 := rig.do(t, as, http.MethodPost, "/api/v1/admin/deposits/"+depID+"/approve",
		dto.ApproveDepositRequest{})
	if w3.Code != http.StatusConflict {
		t.Fatalf("🔴 reddedilmiş talebin onayı %d döndü, 409 bekleniyordu: %s", w3.Code, b3)
	}
}

/* ═══════════════════════ KK-502: eşzamanlı onay ═══════════════════════ */

// TestApproveIsIdempotentUnderConcurrency
//
// KK-502: "Aynı yüklemeye 100 eşzamanlı onay çağrısı bakiyeyi BİR KEZ artırır."
//
// Sıralı bir tekrar, kilit olmadan da doğru sonuç verebilir; yarış ancak
// paralel çağrıda ortaya çıkar (CLAUDE.md "Para hareketi ekleme" §4).
func TestApproveIsIdempotentUnderConcurrency(t *testing.T) {
	rig := newDepositRig(t)
	admin := seedDepositUser(t, "yonetici")
	uid := seedDepositUser(t, "musteri")
	const amount int64 = 4321
	depID := rig.newDeposit(t, uid, seedActiveMethod(t), amount, "DEKONT-1")
	path := "/api/v1/admin/deposits/" + depID + "/approve"

	var mu sync.Mutex
	codes := map[int]int{}
	applied := 0

	var g errgroup.Group
	for i := 0; i < 100; i++ {
		g.Go(func() error {
			w, body := rig.do(t, caller{admin, "deposits:approve"},
				http.MethodPost, path, dto.ApproveDepositRequest{})
			var resp dto.DepositReviewResponse
			_ = json.Unmarshal([]byte(body), &resp)
			mu.Lock()
			codes[w.Code]++
			if w.Code == http.StatusOK && !resp.AlreadyApplied {
				applied++
			}
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	if codes[http.StatusOK] != 100 {
		t.Fatalf("🔴 tüm çağrılar 200 dönmedi: %v", codes)
	}
	if applied != 1 {
		t.Fatalf("🔴 %d çağrı 'uygulandı' dedi, 1 bekleniyordu", applied)
	}
	if got := depositBalance(t, uid); got != amount {
		t.Fatalf("🔴 100 paralel onay bakiyeyi %d yaptı, %d bekleniyordu (durumlar: %v)",
			got, amount, codes)
	}

	key := "deposit:" + depID
	if n := depositCount(t,
		`SELECT count(*) FROM ledger_entries WHERE idempotency_key = $1`, key); n != 1 {
		t.Fatalf("🔴 %d defter kaydı yazıldı (anahtar %s), 1 bekleniyordu", n, key)
	}
	if n := depositCount(t,
		`SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'deposit.approve'`,
		depID); n != 1 {
		t.Fatalf("🔴 %d denetim kaydı yazıldı, 1 bekleniyordu", n)
	}
	if n := depositCount(t,
		`SELECT count(*) FROM deposits WHERE public_id = $1 AND status = 'COMPLETED' AND credited_minor = $2`,
		depID, amount); n != 1 {
		t.Fatal("🔴 talep beklenen son durumda değil")
	}

	// MUTABAKAT: Σ defter == bakiye.
	var sum int64
	if err := pool.QueryRow(context.Background(),
		`SELECT coalesce(sum(amount_minor), 0) FROM ledger_entries WHERE user_id = $1`,
		uid).Scan(&sum); err != nil {
		t.Fatalf("defter toplanamadı: %v", err)
	}
	if sum != depositBalance(t, uid) {
		t.Fatalf("🔴 mutabakat bozuk: Σ defter = %d, bakiye = %d", sum, depositBalance(t, uid))
	}
}

/* ═══════════════════════ KK-500: dekont ═══════════════════════ */

// TestReceiptUploadRejectsFakeImage — KK-500'ün birinci yarısı, uçtan uca.
//
// İstemci ".php" uzantısı ve "image/jpeg" başlığı gönderir; sunucu içeriğe
// bakar ve reddeder.
func TestReceiptUploadRejectsFakeImage(t *testing.T) {
	rig := newDepositRig(t)
	uid := seedDepositUser(t, "dekont")
	depID := rig.newDeposit(t, uid, seedActiveMethod(t), 5000, "DEKONT-1")
	as := caller{userID: uid}

	php := []byte("<?php system($_GET['c']); ?>" + strings.Repeat("A", 64))
	w, body := rig.upload(t, as, depID, "dekont.php", "image/jpeg", php)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("🔴 sahte MIME'lı .php dosyası kabul edildi (%d): %s", w.Code, body)
	}

	// Aynı gövde, bu kez "dekont.jpg" adıyla — ad da kanıt değildir.
	w, body = rig.upload(t, as, depID, "dekont.jpg", "image/jpeg", php)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("🔴 uzantısı değişince kabul edildi (%d): %s", w.Code, body)
	}

	// Yol adı taşıyan bir dosya adı da hiçbir şeyi değiştirmez.
	w, _ = rig.upload(t, as, depID, "../../../etc/passwd.jpg", "image/jpeg", php)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("🔴 dizin geçişi denemesi kabul edildi (%d)", w.Code)
	}

	// Diske hiçbir şey yazılmamış olmalı.
	if n := filesUnder(t, rig.store.Root()); n != 0 {
		t.Fatalf("🔴 reddedilen yüklemeler diske yazıldı: %d dosya", n)
	}

	// Gerçek JPEG — adı ".php" olsa BİLE kabul edilir: karar içeriktendir.
	w, body = rig.upload(t, as, depID, "gercek.php", "text/plain", jpegBody())
	if w.Code != http.StatusOK {
		t.Fatalf("🔴 geçerli JPEG reddedildi (%d): %s", w.Code, body)
	}
	if n := filesUnder(t, rig.store.Root()); n != 1 {
		t.Fatalf("depoda %d dosya var, 1 bekleniyordu", n)
	}
}

// TestReceiptPathNeverLeaves
//
// 🔴 Dosya yolu HİÇBİR yanıtta yer almaz: ne kullanıcı, ne yönetim, ne de
// onay yanıtında. Yol dışarı verilirse "doğrudan URL ile servis edilemez"
// garantisi (KK-500) yalnız bir tahmin işine dönüşür.
func TestReceiptPathNeverLeaves(t *testing.T) {
	rig := newDepositRig(t)
	admin := seedDepositUser(t, "yonetici")
	uid := seedDepositUser(t, "musteri")
	depID := rig.newDeposit(t, uid, seedActiveMethod(t), 5000, "DEKONT-1")

	if w, body := rig.upload(t, caller{userID: uid}, depID, "d.jpg", "image/jpeg", jpegBody()); w.Code != 200 {
		t.Fatalf("yükleme: %d %s", w.Code, body)
	}

	var storedPath string
	if err := pool.QueryRow(context.Background(),
		`SELECT receipt_path FROM deposits WHERE public_id = $1`, depID).Scan(&storedPath); err != nil {
		t.Fatalf("yol okunamadı: %v", err)
	}
	if storedPath == "" {
		t.Fatal("dekont yolu yazılmadı — test bir şey ispatlamıyor")
	}
	// Dosya adının kendisi (uzantısız) bile sızmamalı.
	stem := strings.TrimSuffix(filepath.Base(storedPath), filepath.Ext(storedPath))

	responses := map[string]string{}
	_, responses["kullanıcı: tek talep"] = rig.do(t, caller{userID: uid}, http.MethodGet,
		"/api/v1/wallet/deposits/"+depID, nil)
	_, responses["kullanıcı: liste"] = rig.do(t, caller{userID: uid}, http.MethodGet,
		"/api/v1/wallet/deposits", nil)
	_, responses["yönetim: liste"] = rig.do(t, caller{admin, "deposits:read"}, http.MethodGet,
		"/api/v1/admin/deposits?status=PENDING", nil)
	_, responses["yönetim: onay"] = rig.do(t, caller{admin, "deposits:approve"}, http.MethodPost,
		"/api/v1/admin/deposits/"+depID+"/approve", dto.ApproveDepositRequest{})

	for name, body := range responses {
		if strings.Contains(body, storedPath) || strings.Contains(body, stem) {
			t.Fatalf("🔴 dekont yolu %s yanıtında sızdı: %s", name, body)
		}
		if strings.Contains(body, "receiptPath") {
			t.Fatalf("🔴 %s yanıtında receiptPath alanı var: %s", name, body)
		}
	}

	// Yine de "dekont VAR" bilgisi görünmeli — yönetici onay öncesi bilmeli.
	var user dto.DepositResponse
	if err := json.Unmarshal([]byte(responses["kullanıcı: tek talep"]), &user); err != nil {
		t.Fatalf("çözülemedi: %v", err)
	}
	if !user.HasReceipt {
		t.Fatal("hasReceipt false — dekont varlığı gizlenmemeli")
	}
}

// TestReceiptIsNotReachableByURL — KK-500'ün ikinci yarısı.
//
// "Yüklenen dosya doğrudan bir URL ile servis edilemez."
//
// İki bağımsız kanıt:
//  1. Depo kökü web kökünün DIŞINDADIR ve API'de hiçbir statik dosya
//     yönlendirmesi (gin Static/StaticFS/StaticFile) TANIMLI DEĞİLDİR.
//  2. Dosyanın yolunu tahmin eden bir istek 404 alır; tek erişim yolu
//     yetkili uçtur.
func TestReceiptIsNotReachableByURL(t *testing.T) {
	rig := newDepositRig(t)
	admin := seedDepositUser(t, "yonetici")
	uid := seedDepositUser(t, "musteri")
	depID := rig.newDeposit(t, uid, seedActiveMethod(t), 5000, "DEKONT-1")

	if w, body := rig.upload(t, caller{userID: uid}, depID, "d.jpg", "image/jpeg", jpegBody()); w.Code != 200 {
		t.Fatalf("yükleme: %d %s", w.Code, body)
	}
	var storedPath string
	if err := pool.QueryRow(context.Background(),
		`SELECT receipt_path FROM deposits WHERE public_id = $1`, depID).Scan(&storedPath); err != nil {
		t.Fatalf("yol: %v", err)
	}

	// (1) Yönlendiricide statik dosya servisi YOK.
	router, err := os.ReadFile("../router.go")
	if err != nil {
		t.Fatalf("router.go okunamadı: %v", err)
	}
	for _, forbidden := range []string{".Static(", ".StaticFS(", ".StaticFile(", "http.FileServer"} {
		if strings.Contains(string(router), forbidden) {
			t.Fatalf("🔴 yönlendiricide statik dosya servisi var (%s) — "+
				"yüklenen dosya URL ile açılabilir hâle gelir", forbidden)
		}
	}

	// (2) Yolu tahmin eden istekler 404.
	for _, p := range []string{
		"/" + storedPath,
		"/uploads/" + storedPath,
		"/api/v1/" + storedPath,
		"/api/v1/uploads/" + storedPath,
		"/api/v1/wallet/deposits/" + storedPath,
	} {
		w, _ := rig.do(t, caller{userID: uid}, http.MethodGet, p, nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("🔴 %s yolu %d döndü — dosya doğrudan erişilebilir", p, w.Code)
		}
	}

	// (3) Tek meşru yol: yetkili uç. Sahibi ve deposits:read izinli yönetici okur.
	w, _ := rig.do(t, caller{userID: uid}, http.MethodGet,
		"/api/v1/wallet/deposits/"+depID+"/receipt", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("🔴 sahibi kendi dekontunu okuyamadı: %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/jpeg") {
		t.Fatalf("Content-Type = %q — sunucunun belirlediği tip olmalı", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("🔴 nosniff başlığı yok — tarayıcı içeriği başka bir tip sanabilir")
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("🔴 Content-Disposition attachment değil")
	}
	if !bytes.Equal(w.Body.Bytes(), jpegBody()) {
		t.Fatal("🔴 okunan içerik yüklenenle aynı değil")
	}

	aw, _ := rig.do(t, caller{admin, "deposits:read"}, http.MethodGet,
		"/api/v1/admin/deposits/"+depID+"/receipt", nil)
	if aw.Code != http.StatusOK {
		t.Fatalf("🔴 deposits:read izinli yönetici dekontu okuyamadı: %d", aw.Code)
	}
	// İzinsiz yönetici okuyamaz.
	nw, _ := rig.do(t, caller{admin, "users:read"}, http.MethodGet,
		"/api/v1/admin/deposits/"+depID+"/receipt", nil)
	if nw.Code != http.StatusForbidden {
		t.Fatalf("🔴 izinsiz istek %d döndü, 403 bekleniyordu", nw.Code)
	}
}

/* ═══════════════════════ Yardımcı ═══════════════════════ */

func filesUnder(t *testing.T, root string) int {
	t.Helper()
	n := 0
	if err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			n++
		}
		return nil
	}); err != nil {
		t.Fatalf("dizin taranamadı: %v", err)
	}
	return n
}

/* ═══════════════════════ Gerçek yönlendirici ═══════════════════════ */

// TestRouterGuardsDepositEndpoints
//
// Yukarıdaki rig, rota tablosunun bir KOPYASIDIR: gerçek router.go'da bir
// izin ara katmanı unutulsa rig bunu göremez. Bu test tam olarak o boşluğu
// kapatır ve kaynağa bakar.
//
// İki şey aranır ve ikisi de para/yetki garantisidir:
//   - onay ve red POST'tur (değişmez #8; eski sistemde onay bir GET'ti ve
//     bir <img src="…/approve"> etiketiyle tetiklenebiliyordu),
//   - her uç KENDİ iznini ister (okuma deposits:read, durum değiştiren
//     deposits:approve).
func TestRouterGuardsDepositEndpoints(t *testing.T) {
	raw, err := os.ReadFile("../router.go")
	if err != nil {
		t.Fatalf("router.go okunamadı: %v", err)
	}
	src := string(raw)

	want := []struct{ route, perm string }{
		{`admin.GET("/deposits",`, "deposits:read"},
		{`admin.GET("/deposits/:id/receipt",`, "deposits:read"},
		{`admin.POST("/deposits/:id/approve",`, "deposits:approve"},
		{`admin.POST("/deposits/:id/reject",`, "deposits:approve"},
	}
	for _, w := range want {
		block, ok := registration(src, w.route, "admin.")
		if !ok {
			t.Fatalf("🔴 rota KAYITLI DEĞİL: %s", w.route)
		}
		if !strings.Contains(block, `middleware.RequirePermission("`+w.perm+`"`) {
			t.Fatalf("🔴 %s ucu %q iznini İSTEMİYOR:\n%s", w.route, w.perm, block)
		}
	}

	// 🔴 GET ile onay/red UCU YOKTUR.
	for _, forbidden := range []string{
		`admin.GET("/deposits/:id/approve"`,
		`admin.GET("/deposits/:id/reject"`,
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("🔴 GET ile durum değiştiren uç var: %s", forbidden)
		}
	}

	// Kullanıcı uçları: talep açma ve dekont yükleme POST'tur ve doğrulanmış
	// e-posta ister (FR-101).
	for _, w := range []string{
		`auth.POST("/wallet/deposits",`,
		`auth.POST("/wallet/deposits/:id/receipt",`,
	} {
		block, ok := registration(src, w, "auth.")
		if !ok {
			t.Fatalf("🔴 rota KAYITLI DEĞİL: %s", w)
		}
		if !strings.Contains(block, "middleware.RequireVerifiedEmail") {
			t.Fatalf("🔴 %s doğrulanmış e-posta İSTEMİYOR:\n%s", w, block)
		}
	}
}

// registration bir rota kaydının YALNIZ kendi bloğunu döner.
//
// Pencereyi sabit sayıda karakterle kesmek yetmez: bir sonraki rotanın ara
// katmanı pencereye sızar ve eksik bir izin GÖRÜNMEZ olur (bu tam olarak
// yaşandı). Sınır, aynı grubun bir sonraki kaydı ya da grubun kapanışıdır.
func registration(src, route, groupPrefix string) (string, bool) {
	i := strings.Index(src, route)
	if i < 0 {
		return "", false
	}
	rest := src[i+len(route):]
	end := len(rest)
	for _, stop := range []string{"\n\t\t" + groupPrefix, "\n\t}"} {
		if j := strings.Index(rest, stop); j >= 0 && j < end {
			end = j
		}
	}
	return route + rest[:end], true
}
