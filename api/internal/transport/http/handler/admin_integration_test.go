//go:build integration

package handler_test

// Yönetim uçlarının SIZDIRMAMA garantileri.
//
// Bu dosyadaki testler bir özelliği değil, bir YOKLUĞU doğrular: yanıtlarda
// olmaması gereken şeyler. Böyle bir garanti, testi olmadan sessizce
// kaybolur — kimse "parola özeti hâlâ görünmüyor mu?" diye elle bakmaz.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/testsupport"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/handler"
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

type adminRig struct {
	router  *gin.Engine
	queries *db.Queries
	box     *crypto.SecretBox
}

func newAdminRig(t *testing.T) *adminRig {
	t.Helper()
	gin.SetMode(gin.TestMode)

	box, err := crypto.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("secretbox: %v", err)
	}
	q := db.New(pool)
	a := handler.NewAdmin(handler.AdminDeps{
		Queries: q, Secrets: box,
		Responder: handler.Responder{
			OK:        func(c *gin.Context, b any) { c.JSON(http.StatusOK, b) },
			NoContent: func(c *gin.Context) { c.Status(http.StatusNoContent) },
			Fail: func(c *gin.Context, err error) {
				// 🔴 GERÇEK EŞLEME + ABORT. İki ayrı tuzak:
				//  · Her hatayı 400'e çevirmek 404/409/422 ölçen testleri yanıltır.
				//  · `c.JSON` zinciri DURDURMAZ — handler çalışmaya devam eder ve
				//    "yetkisiz istek reddedildi" sanılan çağrı işi GERÇEKTEN yapar.
				ae, ok := apperr.As(err)
				if !ok {
					ae = apperr.Internal(err)
				}
				c.AbortWithStatusJSON(ae.HTTPStatus(), gin.H{
					"error": gin.H{"code": ae.Code, "message": ae.Message},
				})
			},
			FailField: func(c *gin.Context, errs []dto.FieldError) {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"fields": errs})
			},
		},
	})

	r := gin.New()
	// Yetki ara katmanı BİLEREK yok: burada doğrulanan şey yetki değil,
	// yetkili bir yöneticinin bile göremeyeceği alanlardır.
	r.GET("/admin/providers", a.ListProviders)
	r.PATCH("/admin/providers/:id", a.UpdateProvider)
	r.PUT("/admin/providers/:id/api-key", a.SetProviderAPIKey)
	r.GET("/admin/users", a.ListUsers)

	return &adminRig{router: r, queries: q, box: box}
}

func (rig *adminRig) do(t *testing.T, method, path string, body any) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var rdr *strings.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("gövde kurulamadı: %v", err)
		}
		rdr = strings.NewReader(string(b))
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, req)
	return w, w.Body.String()
}

// seedProvider şifreli anahtarı olan bir sağlayıcı ekler ve temizliğini kurar.
func (rig *adminRig) seedProvider(t *testing.T, plainKey string) (int64, []byte) {
	t.Helper()
	enc, err := rig.box.SealString(plainKey)
	if err != nil {
		t.Fatalf("anahtar şifrelenemedi: %v", err)
	}
	name := "test-saglayici-" + t.Name()
	var id int64
	err = pool.QueryRow(context.Background(), `
		INSERT INTO providers (name, protocol, base_url, api_key_enc, is_active, priority, cost_multiplier)
		VALUES ($1, 'FAKE', 'https://ornek.test', $2, true, 500, 1.0)
		RETURNING id`, name, enc).Scan(&id)
	if err != nil {
		t.Fatalf("sağlayıcı eklenemedi: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM providers WHERE id = $1`, id)
	})
	return id, enc
}

func (rig *adminRig) apiKeyEnc(t *testing.T, id int64) []byte {
	t.Helper()
	var enc []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT api_key_enc FROM providers WHERE id = $1`, id).Scan(&enc); err != nil {
		t.Fatalf("anahtar okunamadı: %v", err)
	}
	return enc
}

/* ═══════════════════════ Testler ═══════════════════════ */

// TestSavingProviderSettingsDoesNotEraseAPIKey
//
// 🔴 API anahtarı AYRI BİR UÇTA olduğu için, ayar kaydeden bir istek onu
// silemez. Anahtar `UpdateProviderSettings` ile aynı UPDATE'te olsaydı,
// panelde "kaydet"e basmak — anahtar alanı boş bırakıldığında — sağlayıcıyı
// sessizce çalışmaz hâle getirirdi. Ayrımı koruyan tek şey bu testtir.
func TestSavingProviderSettingsDoesNotEraseAPIKey(t *testing.T) {
	rig := newAdminRig(t)
	const plain = "gizli-saglayici-anahtari-123456"
	id, before := rig.seedProvider(t, plain)

	// Yönetici yalnız ayarları değiştiriyor; anahtara hiç dokunmuyor.
	w, body := rig.do(t, http.MethodPatch, "/admin/providers/"+strconv.FormatInt(id, 10),
		dto.UpdateProviderRequest{
			BaseURL:        "https://yeni-adres.test",
			IsActive:       false,
			Priority:       10,
			CostMultiplier: "1.2500",
		})
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d, 200 bekleniyordu — gövde: %s", w.Code, body)
	}

	// (1) Anahtar YERİNDE ve DEĞİŞMEMİŞ.
	after := rig.apiKeyEnc(t, id)
	if len(after) == 0 {
		t.Fatal("🔴 ayar kaydı API anahtarını SİLDİ — sağlayıcı sessizce çalışmaz hâle geldi")
	}
	if !bytes.Equal(before, after) {
		t.Fatal("🔴 ayar kaydı API anahtarını DEĞİŞTİRDİ — ayar ucu anahtara yazıyor")
	}
	// Çözülüp aynı düz metni vermeli: baytlar eşit ama anlam bozulmuş olmasın.
	got, err := rig.box.OpenString(after)
	if err != nil {
		t.Fatalf("anahtar çözülemedi: %v", err)
	}
	if got != plain {
		t.Fatalf("anahtar bozulmuş: %q", got)
	}

	// (2) Ayarlar GERÇEKTEN değişmiş olmalı — testin kendisi anlamlı olsun.
	var baseURL string
	var active bool
	if err := pool.QueryRow(context.Background(),
		`SELECT base_url, is_active FROM providers WHERE id = $1`, id).Scan(&baseURL, &active); err != nil {
		t.Fatalf("sağlayıcı okunamadı: %v", err)
	}
	if baseURL != "https://yeni-adres.test" || active {
		t.Fatalf("ayarlar uygulanmamış: base_url=%q is_active=%v", baseURL, active)
	}

	// (3) Yanıt anahtarı SIZDIRMASIN — ne düz ne şifreli hâliyle.
	assertNoKeyLeak(t, body, plain, before)
}

// TestEmptyAPIKeyIsRejected
//
// Boş anahtar KABUL EDİLMEZ. Kabul edilseydi, formu boş kaydeden bir
// yönetici anahtarı silerdi — ayrı uç tam da bunu önlemek için var.
func TestEmptyAPIKeyIsRejected(t *testing.T) {
	rig := newAdminRig(t)
	id, before := rig.seedProvider(t, "gizli-anahtar-abcdef")

	for _, val := range []string{"", "   ", "\t\n"} {
		w, body := rig.do(t, http.MethodPut,
			"/admin/providers/"+strconv.FormatInt(id, 10)+"/api-key",
			dto.SetAPIKeyRequest{APIKey: val})
		if w.Code == http.StatusOK {
			t.Fatalf("boş anahtar (%q) KABUL EDİLDİ — gövde: %s", val, body)
		}
		if after := rig.apiKeyEnc(t, id); !bytes.Equal(before, after) {
			t.Fatalf("boş anahtar (%q) mevcut anahtarı bozdu", val)
		}
	}
}

// TestSetAPIKeyReturnsOnlyMask
//
// Anahtar yazıldıktan sonra YANITTA DÖNMEZ. Yöneticinin doğru anahtarı
// yapıştırdığını görebilmesi için yalnız maskeli bir önizleme döner.
func TestSetAPIKeyReturnsOnlyMask(t *testing.T) {
	rig := newAdminRig(t)
	id, _ := rig.seedProvider(t, "eski-anahtar")

	const yeni = "yeni-cok-gizli-anahtar-987654321"
	w, body := rig.do(t, http.MethodPut,
		"/admin/providers/"+strconv.FormatInt(id, 10)+"/api-key",
		dto.SetAPIKeyRequest{APIKey: yeni})
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d, 200 bekleniyordu — gövde: %s", w.Code, body)
	}
	if strings.Contains(body, yeni) {
		t.Fatal("🔴 API anahtarı YANITTA döndü")
	}

	// Yazma gerçekten olmuş olmalı.
	got, err := rig.box.OpenString(rig.apiKeyEnc(t, id))
	if err != nil {
		t.Fatalf("anahtar çözülemedi: %v", err)
	}
	if got != yeni {
		t.Fatalf("anahtar yazılmamış: %q", got)
	}
}

// TestListProvidersNeverReturnsAPIKey
//
// 🔴 Şifreli hâli bile dışarı verilmez: panelde gösterilecek bir şey değil
// ve varlığı/uzunluğu bilgi sızdırır. `hasApiKey` yeterli bilgidir.
func TestListProvidersNeverReturnsAPIKey(t *testing.T) {
	rig := newAdminRig(t)
	const plain = "listede-gorunmemeli-anahtar-42"
	id, enc := rig.seedProvider(t, plain)

	w, body := rig.do(t, http.MethodGet, "/admin/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d — gövde: %s", w.Code, body)
	}
	assertNoKeyLeak(t, body, plain, enc)

	// Alan adının kendisi de olmasın: `apiKey`, `apiKeyEnc`, `api_key_enc`.
	for _, field := range []string{"apiKeyEnc", "api_key_enc", `"apiKey"`} {
		if strings.Contains(body, field) {
			t.Errorf("🔴 yanıtta %s alanı var", field)
		}
	}

	// Ama VARLIK bildirilmeli — aksi hâlde panel anahtarın kurulu olup
	// olmadığını gösteremez.
	var resp struct {
		Items []struct {
			ID        string `json:"id"`
			HasAPIKey bool   `json:"hasApiKey"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("yanıt çözümlenemedi: %v", err)
	}
	found := false
	for _, it := range resp.Items {
		if it.ID == strconv.FormatInt(id, 10) {
			found = true
			if !it.HasAPIKey {
				t.Error("hasApiKey=false — anahtar var ama bildirilmiyor")
			}
		}
	}
	if !found {
		t.Fatal("eklenen sağlayıcı listede yok")
	}
}

// TestListUsersNeverReturnsPasswordHash
//
// 🔴 Parola özeti hiçbir yanıtta yer almaz. Sızan bir özet çevrimdışı
// kırma saldırısına açıktır ve kullanıcı aynı parolayı başka yerde
// kullanıyorsa kayıp bizim sistemimizle sınırlı kalmaz.
func TestListUsersNeverReturnsPasswordHash(t *testing.T) {
	rig := newAdminRig(t)

	// Tanınabilir bir özetle kullanıcı ekle.
	const hash = "$2a$10$TESTTESTTESTTESTTESTTESTTESTTESTTESTTESTTESTTESTTESTTE"
	email := "admin-sizinti-testi@ornek.test"
	var uid int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, username, password_hash, status)
		VALUES ($1, $2, $3, 'ACTIVE') RETURNING id`, email, "admin_sizinti_testi", hash).Scan(&uid)
	if err != nil {
		t.Fatalf("kullanıcı eklenemedi: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, uid) })

	w, body := rig.do(t, http.MethodGet, "/admin/users?q="+email, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d — gövde: %s", w.Code, body)
	}
	if !strings.Contains(body, email) {
		t.Fatalf("kullanıcı listede yok — test bir şey doğrulamıyor: %s", body)
	}
	if strings.Contains(body, hash) {
		t.Fatal("🔴 password_hash YANITTA döndü")
	}
	for _, field := range []string{"passwordHash", "password_hash"} {
		if strings.Contains(body, field) {
			t.Errorf("🔴 yanıtta %s alanı var", field)
		}
	}
	// Sayısal veritabanı kimliği de dışarı verilmez (Değişmez #10).
	if strings.Contains(body, `"id":"`+strconv.FormatInt(uid, 10)+`"`) {
		t.Error("🔴 sayısal kullanıcı kimliği yanıtta — public_id kullanılmalı")
	}
}

// assertNoKeyLeak yanıtta anahtarın hiçbir hâlinin bulunmadığını doğrular.
func assertNoKeyLeak(t *testing.T, body, plain string, enc []byte) {
	t.Helper()
	if strings.Contains(body, plain) {
		t.Error("🔴 API anahtarı DÜZ METİN olarak yanıtta")
	}
	// Şifreli hâl: hem ham hem base64 hem onaltılık gösterimiyle aranır —
	// JSON'da []byte varsayılan olarak base64 serileşir.
	if b64, _ := json.Marshal(enc); len(b64) > 4 && strings.Contains(body, strings.Trim(string(b64), `"`)) {
		t.Error("🔴 şifreli API anahtarı yanıtta (base64)")
	}
	if strings.Contains(body, fmt.Sprintf("%x", enc)) {
		t.Error("🔴 şifreli API anahtarı yanıtta (onaltılık)")
	}
}
