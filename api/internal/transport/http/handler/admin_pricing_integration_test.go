//go:build integration

package handler_test

// Yeni yönetim uçlarının YETKİ MATRİSİ ve hata görünürlüğü.
//
// CLAUDE.md "Yeni uç nokta ekleme" §7 her uç için üç durumu zorunlu kılar:
//
//	oturumsuz              → 401
//	yetkisiz (yanlış izin) → 403
//	var olmayan kaynak     → 404
//
// Sıra ÖNEMLİDİR: oturumsuz istek 403 değil 401 almalıdır — kullanıcıya
// "giriş yap" mı "yetkin yok" mu denileceği farklı bir eylemdir.
//
// TestMain, pool, seedUser, caller, hdrUser, hdrPerms ve depositResponder
// diğer test dosyalarındadır.

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

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/fx"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

/* ═══════════════════════ Kurulum ═══════════════════════ */

type adminAuthRig struct {
	router *gin.Engine
	sync   *handler.SyncRunner
}

// newAdminAuthRig gerçek yönlendiricinin izin ara katmanlarıyla aynı
// dizilimi kurar.
//
// 🔴 HIZ LİMİTİ BAĞLANMAZ: üretimde yönetim uçları 60 istek/dk ile
// sınırlıdır; matris testinde bağlansaydı istekler 429 alır ve 401/403/404
// ayrımı ölçülemezdi.
func newAdminAuthRig(t *testing.T) *adminAuthRig {
	t.Helper()
	return newAdminAuthRigWithSync(t, func(ctx context.Context, providerID int64) error { return nil })
}

// newAdminAuthRigWithSync senkron işini testin belirlemesine izin verir.
// run nil ise koşucu HİÇ bağlanmaz — yapılandırılmamış sunucu hâli.
func newAdminAuthRigWithSync(t *testing.T, run func(context.Context, int64) error) *adminAuthRig {
	t.Helper()
	gin.SetMode(gin.TestMode)

	box, err := crypto.New(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatalf("secretbox: %v", err)
	}
	tx := postgres.NewTxRunner(pool)
	fxsvc := pricingsvc.NewFXService(tx, &fx.Static{Rate: money.RateOne()},
		port.RealClock{}, 0)
	rules := pricingsvc.NewRuleService(pricingsvc.RuleDeps{
		TxRunner: tx, FX: fxsvc, Clock: port.RealClock{},
	})
	var syncer handler.CatalogSyncer
	var runner *handler.SyncRunner
	if run != nil {
		runner = handler.NewSyncRunner(run)
		syncer = runner
	}

	r := depositResponder()
	a := handler.NewAdmin(handler.AdminDeps{
		Queries: db.New(pool), Secrets: box,
		Rules: rules, Sync: syncer, Responder: r,
	})

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

	// router.go ile AYNI yol/izin eşlemesi. Farklı olursa test gerçek
	// dünyayı ölçmez; bu eşleme elle senkron tutulur.
	g := e.Group("/api/v1/admin", auth)
	g.GET("/pricing-rules", middleware.RequirePermission("pricing:read", r.Fail), a.ListPricingRules)
	g.POST("/pricing-rules", middleware.RequirePermission("pricing:write", r.Fail), a.CreatePricingRule)
	g.DELETE("/pricing-rules/:id", middleware.RequirePermission("pricing:write", r.Fail), a.DeactivatePricingRule)
	g.POST("/pricing-rules/preview", middleware.RequirePermission("pricing:read", r.Fail), a.PreviewPricing)
	g.GET("/audit-logs", middleware.RequirePermission("audit:read", r.Fail), a.ListAuditLogs)
	g.POST("/providers", middleware.RequirePermission("providers:write", r.Fail), a.CreateProvider)
	g.POST("/providers/:id/sync", middleware.RequirePermission("providers:write", r.Fail), a.SyncProvider)
	g.GET("/providers", middleware.RequirePermission("providers:read", r.Fail), a.ListProviders)

	return &adminAuthRig{router: e, sync: runner}
}

func (rig *adminAuthRig) do(t *testing.T, as caller, method, path string, body any) (*httptest.ResponseRecorder, string) {
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

// seedAdminUser denetim ve kural kayıtlarını kullanıcıdan ÖNCE temizler.
func seedAdminUser(t *testing.T, suffix string) int64 {
	t.Helper()
	id, _ := seedUser(t, suffix)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM pricing_rules WHERE created_by_user_id = $1`, id)
	})
	return id
}

type authCase struct {
	name         string
	as           caller
	method, path string
	body         any
	want         int
}

func runAuthCases(t *testing.T, rig *adminAuthRig, cases []authCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, body := rig.do(t, tc.as, tc.method, tc.path, tc.body)
			if w.Code != tc.want {
				t.Fatalf("durum = %d, %d bekleniyordu — gövde: %s", w.Code, tc.want, body)
			}
		})
	}
}

/* ═══════════════════ Fiyat kuralı yetki matrisi ═══════════════════ */

// TestPricingRulesAuthorizationMatrix
//
// OKUMA İLE YAZMA AYRI İZİNLERDİR. `pricing:read` taşıyan destek personeli
// marjı değiştirememelidir: marj doğrudan gelirdir ve tek bir yanlış rakam
// her satışta zarar ettirir.
func TestPricingRulesAuthorizationMatrix(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "fiyat-yonetici")
	plain := seedAdminUser(t, "fiyat-musteri")

	const list = "/api/v1/admin/pricing-rules"
	const preview = "/api/v1/admin/pricing-rules/preview"
	create := dto.CreatePricingRuleRequest{Scope: "COUNTRY", CountryISO: "RU", MarginPercent: "40"}
	prev := dto.PricingPreviewRequest{ServiceCode: "tg", CountryISO: "RU"}

	runAuthCases(t, rig, []authCase{
		// ── oturumsuz → 401 ──
		{"liste: oturumsuz", caller{}, http.MethodGet, list, nil, 401},
		{"ekleme: oturumsuz", caller{}, http.MethodPost, list, create, 401},
		{"önizleme: oturumsuz", caller{}, http.MethodPost, preview, prev, 401},
		{"pasifleştirme: oturumsuz", caller{}, http.MethodDelete, list + "/1", nil, 401},

		// ── yetkisiz → 403 ──
		{"liste: izinsiz", caller{plain, ""}, http.MethodGet, list, nil, 403},
		{"liste: yanlış izin", caller{plain, "users:read"}, http.MethodGet, list, nil, 403},
		{"ekleme: yalnız okuma izni", caller{admin, "pricing:read"}, http.MethodPost, list, create, 403},
		{"pasifleştirme: yalnız okuma izni", caller{admin, "pricing:read"}, http.MethodDelete, list + "/1", nil, 403},
		{"önizleme: izinsiz", caller{plain, ""}, http.MethodPost, preview, prev, 403},
		{"önizleme: yazma izni okumaya yetmez",
			caller{admin, "pricing:write"}, http.MethodPost, preview, prev, 403},

		// ── var olmayan kaynak → 404 ──
		{"pasifleştirme: var olmayan kural", caller{admin, "pricing:write"},
			http.MethodDelete, list + "/99999999", nil, 404},
		{"pasifleştirme: bozuk kimlik", caller{admin, "pricing:write"},
			http.MethodDelete, list + "/bozuk", nil, 404},
		{"önizleme: var olmayan servis", caller{admin, "pricing:read"}, http.MethodPost, preview,
			dto.PricingPreviewRequest{ServiceCode: "boyle-bir-servis-yok", CountryISO: "RU"}, 404},
	})
}

// TestScopeMismatchGivesReadableError
//
// 🔴 HAM VERİTABANI KISIT HATASI KULLANICIYA GÖSTERİLMEZ (Değişmez #12).
//
// Şemadaki `pricing_scope_consistent` son savunmadır; ona bırakılsaydı
// yönetici "ERROR: new row for relation \"pricing_rules\" violates check
// constraint \"pricing_scope_consistent\"" görür ve neyi yanlış yaptığını
// anlamazdı. Doğrulama Go tarafında da yapılır ve Türkçe konuşur.
func TestScopeMismatchGivesReadableError(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "kapsam")

	cases := []dto.CreatePricingRuleRequest{
		{Scope: "GLOBAL", ServiceCode: "tg", MarginPercent: "40"},
		{Scope: "COUNTRY", MarginPercent: "40"},
		{Scope: "SERVICE_COUNTRY", ServiceCode: "tg", MarginPercent: "40"},
		{Scope: "HERKESE", MarginPercent: "40"},
	}
	for _, req := range cases {
		w, body := rig.do(t, caller{admin, "pricing:write"},
			http.MethodPost, "/api/v1/admin/pricing-rules", req)
		if w.Code == http.StatusOK {
			t.Fatalf("🔴 tutarsız kapsam kabul edildi: %+v", req)
		}
		if w.Code >= 500 {
			t.Fatalf("🔴 kullanıcı hatası %d olarak döndü (%+v): %s", w.Code, req, body)
		}
		for _, leak := range []string{
			"pricing_scope_consistent", "SQLSTATE", "violates check constraint",
			"pgconn", "pgx", "relation \"pricing_rules\"",
		} {
			if strings.Contains(body, leak) {
				t.Fatalf("🔴 ham veritabanı hatası yanıta sızdı (%q): %s", leak, body)
			}
		}
		if !strings.Contains(body, "message") && !strings.Contains(body, "fields") {
			t.Fatalf("hata mesajı yok: %s", body)
		}
	}
}

// TestCreateAndListPricingRuleOverHTTP
//
// Uçların gerçekten çalıştığının kanıtı — yetki matrisi tek başına
// "her şey 403 dönüyor" hâlini de geçerdi.
func TestCreateAndListPricingRuleOverHTTP(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "kural-yazan")

	seedServiceAndCountry(t, "tstsvc", "QQ")

	w, body := rig.do(t, caller{admin, "pricing:write"}, http.MethodPost,
		"/api/v1/admin/pricing-rules", dto.CreatePricingRuleRequest{
			Scope: "SERVICE_COUNTRY", ServiceCode: "tstsvc", CountryISO: "QQ",
			MarginPercent: "33.25", Note: "http testi",
		})
	if w.Code != http.StatusOK {
		t.Fatalf("kural yazılamadı (%d): %s", w.Code, body)
	}
	var created dto.PricingRuleResponse
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if created.MarginPercent != "33.25" {
		t.Errorf("marj = %q", created.MarginPercent)
	}
	// Para alanları {minor,currency,formatted} biçiminde olmalı (Değişmez #1).
	if created.FixedFee.Currency != "TRY" || created.MinPrice.Currency != "TRY" {
		t.Errorf("para alanları biçimsiz: %+v / %+v", created.FixedFee, created.MinPrice)
	}

	w, body = rig.do(t, caller{admin, "pricing:read"}, http.MethodGet,
		"/api/v1/admin/pricing-rules", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("liste (%d): %s", w.Code, body)
	}
	if !strings.Contains(body, "tstsvc") || !strings.Contains(body, `"QQ"`) {
		t.Fatalf("yazılan kural listede okunur kapsamıyla yok: %s", body)
	}

	// Pasifleştirme geçer ve kural listeden düşer.
	w, body = rig.do(t, caller{admin, "pricing:write"}, http.MethodDelete,
		"/api/v1/admin/pricing-rules/"+strconv.FormatInt(created.ID, 10), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("pasifleştirme (%d): %s", w.Code, body)
	}
	_, body = rig.do(t, caller{admin, "pricing:read"}, http.MethodGet,
		"/api/v1/admin/pricing-rules", nil)
	if strings.Contains(body, "tstsvc") {
		t.Fatalf("pasifleştirilen kural hâlâ listede: %s", body)
	}
}

// TestLastGlobalRuleIsProtectedOverHTTP
//
// Servis katmanındaki koruma HTTP'den de görünmeli ve 409 dönmeli.
func TestLastGlobalRuleIsProtectedOverHTTP(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "global-koruma")

	// Etkin GLOBAL kural her zaman VARDIR (migration tohumu).
	var id int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM pricing_rules WHERE is_active AND scope='GLOBAL'`).Scan(&id); err != nil {
		t.Fatalf("etkin GLOBAL kural yok — sistem zaten satış yapamaz durumda: %v", err)
	}

	w, body := rig.do(t, caller{admin, "pricing:write"}, http.MethodDelete,
		"/api/v1/admin/pricing-rules/"+strconv.FormatInt(id, 10), nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("🔴 son GLOBAL kuralın pasifleştirilmesi %d döndü, 409 bekleniyordu: %s",
			w.Code, body)
	}
	if !strings.Contains(body, "LAST_GLOBAL_RULE") {
		t.Errorf("hata kodu yanıtta yok: %s", body)
	}

	var stillActive bool
	if err := pool.QueryRow(context.Background(),
		`SELECT is_active FROM pricing_rules WHERE id=$1`, id).Scan(&stillActive); err != nil {
		t.Fatal(err)
	}
	if !stillActive {
		t.Fatal("🔴 reddedilen istek kuralı yine de pasifleştirdi")
	}
}

// seedServiceAndCountry testin kendi ürün boyutlarını kurar.
func seedServiceAndCountry(t *testing.T, code, iso string) {
	t.Helper()
	ctx := context.Background()
	var svcID, ctryID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO services (code, name, name_tr) VALUES ($1, $1, $1)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, code).Scan(&svcID); err != nil {
		t.Fatalf("servis: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO countries (iso2, name, name_tr, phone_code) VALUES ($1, $1, $1, '+999')
		ON CONFLICT (iso2) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`, iso).Scan(&ctryID); err != nil {
		t.Fatalf("ülke: %v", err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM pricing_rules WHERE service_id=$1 OR country_id=$2`, svcID, ctryID)
		_, _ = pool.Exec(c, `DELETE FROM services WHERE id=$1`, svcID)
		_, _ = pool.Exec(c, `DELETE FROM countries WHERE id=$1`, ctryID)
	})
}
