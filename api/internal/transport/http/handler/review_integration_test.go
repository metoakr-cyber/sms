//go:build integration

package handler_test

// Müşteri yorumu uçlarının HTTP katmanı garantileri:
// YETKİ MATRİSİ (401 / 403 / 404) ve E-POSTANIN SİTEYE SIZMAMASI.
//
// TestMain, pool, seedUser ve testResponder bu paketin diğer dosyalarında
// tanımlıdır; hdrUser / hdrPerms başlıkları deposit_integration_test.go'dan
// gelir.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	reviewsvc "github.com/ikmetrik/sms-platform/api/internal/service/review"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

/* ═══════════════════════ Kurulum ═══════════════════════ */

// reviewRig gerçek yönlendiricinin rota + ara katman düzenini taklit eder.
//
// 🔴 HIZ LİMİTİ ARA KATMANI BAĞLANMAZ (ticketRig ile aynı gerekçe): üretimdeki
// 5/dk sınırı bağlansaydı matris testleri rastgele 429 alırdı.
//
// 🔴 `RequireVerifiedEmail` de BAĞLANMAZ: bu dosya YETKİ matrisini ölçer,
// e-posta doğrulamasını değil. Gerçek router'da o ara katman POST /reviews
// yolundadır (router.go) ve orada gerekçesiyle birlikte yazılıdır.
//
// İZİN ARA KATMANI İSE GERÇEĞİDİR: bu dosyanın ölçtüğü şey odur.
type reviewRig struct{ router *gin.Engine }

func newReviewRig(t *testing.T) *reviewRig {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := reviewsvc.New(reviewsvc.Deps{
		TxRunner: postgres.NewTxRunner(pool),
		Clock:    port.RealClock{},
	})
	r := testResponder()
	h := handler.NewReview(svc, r)

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

	v1 := e.Group("/api/v1")
	// SİTE UCU: oturumsuz, auth ara katmanının DIŞINDA.
	v1.GET("/catalog/reviews", h.Public)

	g := v1.Group("", auth)
	g.POST("/reviews", h.Create)
	g.GET("/reviews/mine", h.Mine)
	g.GET("/reviews/:id", h.Get)

	g.GET("/admin/reviews",
		middleware.RequirePermission("reviews:read", r.Fail), h.AdminList)
	g.POST("/admin/reviews/:id/approve",
		middleware.RequirePermission("reviews:moderate", r.Fail), h.Approve)
	g.POST("/admin/reviews/:id/reject",
		middleware.RequirePermission("reviews:moderate", r.Fail), h.Reject)

	return &reviewRig{router: e}
}

type reviewCaller struct {
	userID int64
	perms  string
}

func (rig *reviewRig) do(t *testing.T, as reviewCaller, method, path string, body any) (*httptest.ResponseRecorder, string) {
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

// seedReviewUser kullanıcı ekler ve yorum kayıtlarının temizliğini kurar.
//
// seedUser'ın kendi temizliği kullanıcıyı siler; reviews.user_id yabancı
// anahtarı yüzünden yorumlar ÖNCE silinmelidir. t.Cleanup LIFO çalıştığı için
// burada kaydedilen temizlik önce koşar.
func seedReviewUser(t *testing.T, suffix string) int64 {
	t.Helper()
	id, _ := seedUser(t, suffix)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM reviews WHERE user_id = $1`, id)
		_, _ = pool.Exec(ctx, `UPDATE reviews SET reviewed_by_user_id = NULL
		                       WHERE reviewed_by_user_id = $1`, id)
	})
	return id
}

func (rig *reviewRig) newReview(t *testing.T, userID int64, body string) string {
	t.Helper()
	w, raw := rig.do(t, reviewCaller{userID: userID}, http.MethodPost, "/api/v1/reviews",
		dto.CreateReviewRequest{Rating: 5, Body: body})
	if w.Code != http.StatusOK {
		t.Fatalf("yorum oluşturulamadı: %d %s", w.Code, raw)
	}
	var resp dto.ReviewResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	return resp.ID
}

/* ═══════════════════════ Yetki matrisi ═══════════════════════ */

// TestReviewAuthMatrix her uç için üç senaryoyu sınar:
// oturumsuz (401) · yetkisiz (403) · başkasının kaynağı (404).
//
// Üçünün AYRI kodlar döndürmesi gerekir: hepsini 403 yapmak, oturumu düşmüş
// kullanıcıya "yetkiniz yok" dedirtir ve giriş ekranına yönlendirilmez.
func TestReviewAuthMatrix(t *testing.T) {
	rig := newReviewRig(t)
	sahip := seedReviewUser(t, "matrixsahip")
	yabanci := seedReviewUser(t, "matrixyabanci")
	moderator := seedReviewUser(t, "matrixmod")

	id := rig.newReview(t, sahip, "Numara hemen geldi, kod da anında düştü. Memnunum.")

	cases := []struct {
		ad     string
		as     reviewCaller
		method string
		path   string
		body   any
		want   int
	}{
		// ── OTURUMSUZ → 401 ──
		{"oturumsuz: yorum gönder", reviewCaller{}, http.MethodPost,
			"/api/v1/reviews", dto.CreateReviewRequest{Rating: 5, Body: "Yeterince uzun bir yorum."},
			http.StatusUnauthorized},
		{"oturumsuz: kendi yorumlarım", reviewCaller{}, http.MethodGet,
			"/api/v1/reviews/mine", nil, http.StatusUnauthorized},
		{"oturumsuz: yönetim listesi", reviewCaller{}, http.MethodGet,
			"/api/v1/admin/reviews", nil, http.StatusUnauthorized},
		{"oturumsuz: onay", reviewCaller{}, http.MethodPost,
			"/api/v1/admin/reviews/" + id + "/approve", nil, http.StatusUnauthorized},

		// ── OTURUM VAR, İZİN YOK → 403 ──
		{"izinsiz: yönetim listesi", reviewCaller{userID: yabanci}, http.MethodGet,
			"/api/v1/admin/reviews", nil, http.StatusForbidden},
		{"izinsiz: onay", reviewCaller{userID: yabanci}, http.MethodPost,
			"/api/v1/admin/reviews/" + id + "/approve", nil, http.StatusForbidden},
		{"izinsiz: red", reviewCaller{userID: yabanci}, http.MethodPost,
			"/api/v1/admin/reviews/" + id + "/reject",
			dto.RejectReviewRequest{Reason: "Reklam."}, http.StatusForbidden},
		// 🔴 YALNIZ OKUMA İZNİ MODERASYONA YETMEZ: destek personelinin
		// kuyruğu görmesi, sitede ne yayımlanacağına karar verebilmesi
		// anlamına gelmez.
		{"yalnız okuma izni: onay", reviewCaller{userID: moderator, perms: "reviews:read"},
			http.MethodPost, "/api/v1/admin/reviews/" + id + "/approve", nil,
			http.StatusForbidden},

		// ── BAŞKASININ KAYNAĞI → 404 (403 DEĞİL) ──
		// "Bu yorum başkasının" demek geçerli kimliklerin varlığını sızdırır.
		{"başkasının yorumu", reviewCaller{userID: yabanci}, http.MethodGet,
			"/api/v1/reviews/" + id, nil, http.StatusNotFound},
		{"var olmayan yorum", reviewCaller{userID: sahip}, http.MethodGet,
			"/api/v1/reviews/" + uuid.NewString(), nil, http.StatusNotFound},
		{"bozuk kimlik", reviewCaller{userID: sahip}, http.MethodGet,
			"/api/v1/reviews/bu-uuid-degil", nil, http.StatusNotFound},

		// ── SAHİBİ KENDİ YORUMUNU GÖRÜR ──
		{"sahibi okur", reviewCaller{userID: sahip}, http.MethodGet,
			"/api/v1/reviews/" + id, nil, http.StatusOK},
		// ── DOĞRU İZİNLE YÖNETİM LİSTESİ AÇILIR ──
		{"okuma izniyle liste", reviewCaller{userID: moderator, perms: "reviews:read"},
			http.MethodGet, "/api/v1/admin/reviews", nil, http.StatusOK},
	}

	for _, c := range cases {
		t.Run(c.ad, func(t *testing.T) {
			w, body := rig.do(t, c.as, c.method, c.path, c.body)
			if w.Code != c.want {
				t.Fatalf("%s %s → %d, beklenen %d\n%s", c.method, c.path, w.Code, c.want, body)
			}
		})
	}
}

/* ═══════════════════════ Gizlilik ═══════════════════════ */

// TestPublicReviewsNeverExposeEmail sitedeki yanıtta kullanıcı e-postasının
// BULUNMADIĞINI doğrular.
//
// Bu tek bir alan meselesi değil: yorum bölümü sitenin OTURUMSUZ görülen
// kısmıdır ve oraya düşen bir e-posta, arama motoru tarafından indekslenir.
// Test yanıtın TAMAMINI ham metin olarak da tarar — DTO'ya sonradan eklenen
// bir alan (ya da bir `gin.H` sarmalayıcısı) sessizce geçemesin.
func TestPublicReviewsNeverExposeEmail(t *testing.T) {
	ctx := context.Background()
	rig := newReviewRig(t)
	sahip := seedReviewUser(t, "gizlilik")
	staff := seedReviewUser(t, "gizlilikstaff")

	id := rig.newReview(t, sahip, "Panelden numarayı aldım, kod 8 saniyede geldi.")

	// Kullanıcının e-postası ve kullanıcı adı.
	var email, username string
	if err := pool.QueryRow(ctx,
		`SELECT email::text, username::text FROM users WHERE id = $1`, sahip).
		Scan(&email, &username); err != nil {
		t.Fatalf("kullanıcı okunamadı: %v", err)
	}

	// Onayla — sitede görünsün.
	w, body := rig.do(t, reviewCaller{userID: staff, perms: "reviews:moderate"},
		http.MethodPost, "/api/v1/admin/reviews/"+id+"/approve", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("onay başarısız: %d %s", w.Code, body)
	}

	w, raw := rig.do(t, reviewCaller{}, http.MethodGet, "/api/v1/catalog/reviews", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("site listesi: %d %s", w.Code, raw)
	}

	// (1) Ham gövdede e-posta GEÇMEMELİ.
	if strings.Contains(strings.ToLower(raw), strings.ToLower(email)) {
		t.Fatalf("site yanıtında kullanıcı e-postası var: %s", raw)
	}
	// (2) "@" işareti hiç olmamalı: kısmi bir e-posta da sızıntıdır.
	if strings.Contains(raw, "@") {
		t.Fatalf("site yanıtında '@' geçiyor, e-posta sızmış olabilir: %s", raw)
	}

	// (3) Yorum GERÇEKTEN listede olmalı — boş bir liste bu testi bedavaya
	//     geçirirdi.
	var resp dto.PublicReviewListResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	var bulundu bool
	for _, it := range resp.Items {
		if it.ID == id {
			bulundu = true
			if it.AuthorName != username {
				t.Errorf("yazar adı %q, kullanıcı adı %q olmalıydı", it.AuthorName, username)
			}
		}
	}
	if !bulundu {
		t.Fatal("onaylanan yorum sitede görünmüyor — test bedavaya geçmiş olurdu")
	}
	if resp.Total < 1 {
		t.Fatalf("özet toplamı %d", resp.Total)
	}
}

// TestUserCannotSeeOthersReview sahiplik kısıtının SORGUDA olduğunu ve
// başkasının yorumunun 404 döndüğünü doğrular.
func TestUserCannotSeeOthersReview(t *testing.T) {
	rig := newReviewRig(t)
	sahip := seedReviewUser(t, "sahiplik")
	yabanci := seedReviewUser(t, "sahiplikyabanci")

	id := rig.newReview(t, sahip, "Kod gelmeyince para hemen geri yüklendi, iyi.")

	w, body := rig.do(t, reviewCaller{userID: yabanci}, http.MethodGet, "/api/v1/reviews/"+id, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("başkasının yorumu %d döndü, 404 olmalıydı: %s", w.Code, body)
	}

	// Kendi listesinde de görünmemeli.
	w, raw := rig.do(t, reviewCaller{userID: yabanci}, http.MethodGet, "/api/v1/reviews/mine", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("kendi listesi: %d %s", w.Code, raw)
	}
	if strings.Contains(raw, id) {
		t.Fatalf("başkasının yorumu 'yorumlarım' listesinde: %s", raw)
	}
}

/* ═══════════════════════ Moderasyon ═══════════════════════ */

// TestRejectRequiresReason gerekçesiz reddin 422 ile durdurulduğunu doğrular.
//
// İki katman: DTO doğrulaması (bu test) ve domain (Decide). Yalnız istemci
// tarafında zorlamak, doğrudan API çağıran bir betiğe kapıyı açık bırakırdı.
func TestRejectRequiresReason(t *testing.T) {
	rig := newReviewRig(t)
	sahip := seedReviewUser(t, "redgerekce")
	staff := seedReviewUser(t, "redgerekcestaff")
	id := rig.newReview(t, sahip, "Bu yorum reddedilecek, yeterince uzun bir metin.")

	as := reviewCaller{userID: staff, perms: "reviews:moderate"}

	w, body := rig.do(t, as, http.MethodPost, "/api/v1/admin/reviews/"+id+"/reject",
		dto.RejectReviewRequest{Reason: "   "})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("boş gerekçe %d döndü, 422 olmalıydı: %s", w.Code, body)
	}

	// Yorum HÂLÂ kuyrukta olmalı: reddedilmemiş bir yorum reddedilmiş
	// görünmemeli.
	var status string
	if err := pool.QueryRow(context.Background(),
		`SELECT status::text FROM reviews WHERE public_id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("durum: %v", err)
	}
	if status != "PENDING" {
		t.Fatalf("başarısız red sonrası durum %s", status)
	}

	w, body = rig.do(t, as, http.MethodPost, "/api/v1/admin/reviews/"+id+"/reject",
		dto.RejectReviewRequest{Reason: "Yorum reklam bağlantısı içeriyor."})
	if w.Code != http.StatusOK {
		t.Fatalf("gerekçeli red %d döndü: %s", w.Code, body)
	}

	// Kullanıcı KENDİ red gerekçesini görebilmeli.
	w, raw := rig.do(t, reviewCaller{userID: sahip}, http.MethodGet, "/api/v1/reviews/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("kendi yorumu: %d %s", w.Code, raw)
	}
	if !strings.Contains(raw, "reklam bağlantısı") {
		t.Fatalf("red gerekçesi kullanıcıya gösterilmiyor: %s", raw)
	}
}

// TestRejectedReviewNeverReachesTheSite reddedilen yorumun site listesinde
// BULUNMADIĞINI doğrular.
//
// Sitedeki listenin tek süzgeci `status = 'APPROVED'`; bu test onun gerçekten
// uygulandığını ölçer. Kaçırılması en pahalı hata budur.
func TestRejectedReviewNeverReachesTheSite(t *testing.T) {
	rig := newReviewRig(t)
	sahip := seedReviewUser(t, "redsite")
	staff := seedReviewUser(t, "redsitestaff")
	id := rig.newReview(t, sahip, "Reddedilecek yorum, sitede asla görünmemeli.")

	// Henüz karar verilmedi: sitede olmamalı.
	_, raw := rig.do(t, reviewCaller{}, http.MethodGet, "/api/v1/catalog/reviews", nil)
	if strings.Contains(raw, id) {
		t.Fatal("onay bekleyen yorum sitede görünüyor")
	}

	w, body := rig.do(t, reviewCaller{userID: staff, perms: "reviews:moderate"},
		http.MethodPost, "/api/v1/admin/reviews/"+id+"/reject",
		dto.RejectReviewRequest{Reason: "Konuyla ilgisiz."})
	if w.Code != http.StatusOK {
		t.Fatalf("red: %d %s", w.Code, body)
	}

	_, raw = rig.do(t, reviewCaller{}, http.MethodGet, "/api/v1/catalog/reviews", nil)
	if strings.Contains(raw, id) {
		t.Fatal("REDDEDİLEN yorum sitede görünüyor")
	}
}

// TestCreateRejectsShortBodyAndBadRating sunucu tarafı doğrulamasını sınar.
//
// İstemci doğrulaması bir KOLAYLIKTIR, savunma değil: doğrudan API çağıran
// bir betik onu hiç görmez.
func TestCreateRejectsShortBodyAndBadRating(t *testing.T) {
	rig := newReviewRig(t)
	user := seedReviewUser(t, "dogrulama")

	cases := []struct {
		ad  string
		req dto.CreateReviewRequest
	}{
		{"çok kısa metin", dto.CreateReviewRequest{Rating: 5, Body: "iyi"}},
		{"boş metin", dto.CreateReviewRequest{Rating: 5, Body: "   "}},
		{"puan 0", dto.CreateReviewRequest{Rating: 0, Body: "Yeterince uzun bir yorum metni."}},
		{"puan 6", dto.CreateReviewRequest{Rating: 6, Body: "Yeterince uzun bir yorum metni."}},
		{"puan -1", dto.CreateReviewRequest{Rating: -1, Body: "Yeterince uzun bir yorum metni."}},
		{"1001 karakter", dto.CreateReviewRequest{
			Rating: 5, Body: strings.Repeat("a", 1001)}},
	}
	for _, c := range cases {
		t.Run(c.ad, func(t *testing.T) {
			w, body := rig.do(t, reviewCaller{userID: user}, http.MethodPost,
				"/api/v1/reviews", c.req)
			if w.Code == http.StatusOK {
				t.Fatalf("geçersiz yorum kabul edildi: %s", body)
			}
		})
	}

	// 🔴 1000 TÜRKÇE KARAKTER GEÇMELİ: sınır rune ile ölçülür, bayt ile değil.
	// Bayt sayılsaydı bu istek ~500 karakterde reddedilirdi.
	w, body := rig.do(t, reviewCaller{userID: user}, http.MethodPost, "/api/v1/reviews",
		dto.CreateReviewRequest{Rating: 5, Body: strings.Repeat("ş", 1000)})
	if w.Code != http.StatusOK {
		t.Fatalf("1000 Türkçe karakterlik yorum reddedildi (bayt sayılıyor olabilir): %d %s",
			w.Code, body)
	}
}

// TestClientCannotSubmitApprovedReview istemcinin durum belirleyemediğini
// doğrular.
//
// Gövdeye `"status":"APPROVED"` koymak moderasyonu tümüyle atlatırdı.
// DTO'da böyle bir alan YOKTUR; bu test alanın bir gün eklenmesine karşı
// bekçidir.
func TestClientCannotSubmitApprovedReview(t *testing.T) {
	rig := newReviewRig(t)
	user := seedReviewUser(t, "durumzorlama")

	w, body := rig.do(t, reviewCaller{userID: user}, http.MethodPost, "/api/v1/reviews",
		map[string]any{
			"rating": 5,
			"body":   "Durumu zorla APPROVED yapmayı deneyen bir yorum.",
			"status": "APPROVED",
		})
	if w.Code != http.StatusOK {
		t.Fatalf("yorum gönderilemedi: %d %s", w.Code, body)
	}
	var resp dto.ReviewResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("yanıt: %v", err)
	}
	if resp.Status != "PENDING" {
		t.Fatalf("istemci durumu belirleyebildi: %s", resp.Status)
	}
}
