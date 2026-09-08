//go:build integration

package handler_test

// Sağlayıcı EKLEME (FR-701) ve katalog senkronu tetikleme.
//
// Kurulum (adminAuthRig, seedAdminUser, runAuthCases) admin_pricing_integration_test.go'dadır.

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
)

func cleanupProvider(t *testing.T, name string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM providers WHERE name = $1`, name)
	})
}

/* ═══════════════════ Anahtar sızıntısı ═══════════════════ */

// TestCreateProviderIgnoresAPIKeyInBody
//
// 🔴 EKLEME UCU API ANAHTARI ALMAZ.
//
// Neden ayrı uç: ekleme gövdesi tarayıcı ağ sekmesine, ters vekil erişim
// günlüğüne ve hata ayıklama çıktısına düşer. Anahtar orada olsaydı,
// sağlayıcının tüm bakiyesini harcayabilecek bir sır bu üç yere birden
// yazılırdı.
//
// Test gövdeye anahtar KOYAR ve iki şeyi ölçer: yanıtta yankılanmaz,
// veritabanına yazılmaz.
func TestCreateProviderIgnoresAPIKeyInBody(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "saglayici-ekle")
	name := "test-ekleme-" + uuid.NewString()[:8]
	cleanupProvider(t, name)

	const secret = "cok-gizli-saglayici-anahtari-999"
	// DTO'da böyle bir alan YOK; ham JSON ile gönderiyoruz — istemcinin
	// fazladan alan göndermesi engellenemez, sunucunun onu görmezden gelmesi
	// gerekir.
	raw := map[string]any{
		"name": name, "protocol": "FAKE", "baseUrl": "https://ornek.test",
		"priority": 100, "costMultiplier": "1.0",
		"apiKey": secret, "apiKeyEnc": secret, "api_key_enc": secret,
	}
	w, body := rig.do(t, caller{admin, "providers:write"},
		http.MethodPost, "/api/v1/admin/providers", raw)
	if w.Code != http.StatusOK {
		t.Fatalf("sağlayıcı eklenemedi (%d): %s", w.Code, body)
	}
	if strings.Contains(body, secret) {
		t.Fatal("🔴 gönderilen anahtar yanıtta yankılandı")
	}

	var enc []byte
	var hasKey bool
	if err := pool.QueryRow(context.Background(),
		`SELECT api_key_enc, coalesce(length(api_key_enc) > 0, false)
		 FROM providers WHERE name = $1`, name).Scan(&enc, &hasKey); err != nil {
		t.Fatalf("sağlayıcı okunamadı: %v", err)
	}
	if hasKey {
		t.Fatalf("🔴 ekleme ucu API anahtarı YAZDI (%d bayt) — ayrı uç anlamsızlaştı", len(enc))
	}

	var resp dto.AdminProviderResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if resp.HasAPIKey {
		t.Error("anahtarı olmayan sağlayıcı hasApiKey=true bildirdi")
	}
}

// TestNewProviderIsCreatedInactive
//
// Yeni sağlayıcı PASİF DOĞAR; istekteki `isActive` alanına BAKILMAZ.
//
// Etkin doğsaydı, anahtarı ve boyut eşleştirmeleri henüz yokken teklif
// yolunda aday sayılırdı: her teklif isteği ona da sorar, 3 saniyelik zaman
// aşımına düşer ve kullanıcının gördüğü tek şey "site yavaşladı" olurdu.
func TestNewProviderIsCreatedInactive(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "pasif-dogum")
	name := "test-pasif-" + uuid.NewString()[:8]
	cleanupProvider(t, name)

	// İstek AÇIKÇA etkin olmasını istiyor.
	raw := map[string]any{
		"name": name, "protocol": "FAKE", "baseUrl": "https://ornek.test",
		"priority": 5, "costMultiplier": "1.0", "isActive": true,
	}
	w, body := rig.do(t, caller{admin, "providers:write"},
		http.MethodPost, "/api/v1/admin/providers", raw)
	if w.Code != http.StatusOK {
		t.Fatalf("ekleme (%d): %s", w.Code, body)
	}

	var active bool
	if err := pool.QueryRow(context.Background(),
		`SELECT is_active FROM providers WHERE name = $1`, name).Scan(&active); err != nil {
		t.Fatalf("sağlayıcı okunamadı: %v", err)
	}
	if active {
		t.Fatal("🔴 yeni sağlayıcı ETKİN doğdu — anahtarsız sağlayıcı teklif yolunda aday oldu")
	}

	// Aynı ad ikinci kez eklenemez.
	w, body = rig.do(t, caller{admin, "providers:write"},
		http.MethodPost, "/api/v1/admin/providers", raw)
	if w.Code == http.StatusOK {
		t.Fatalf("🔴 aynı adla ikinci sağlayıcı eklendi: %s", body)
	}
}

// TestCreateProviderValidatesProtocol
//
// Bilinmeyen protokol Go'da elenir. Elenmeseydi doğrudan Postgres'e gider,
// 22P02 ile 500 dönerdi: kullanıcı hatası sunucu hatası gibi görünürdü.
func TestCreateProviderValidatesProtocol(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "protokol")

	for _, proto := range []string{"", "HEROSMS", "herosms_v1", "DROP TABLE", "SMSPVA"} {
		w, body := rig.do(t, caller{admin, "providers:write"},
			http.MethodPost, "/api/v1/admin/providers", dto.CreateProviderRequest{
				Name: "gecersiz-" + uuid.NewString()[:6], Protocol: proto,
				Priority: 10, CostMultiplier: "1.0",
			})
		if w.Code == http.StatusOK {
			t.Errorf("🔴 geçersiz protokol kabul edildi: %q", proto)
		}
		if w.Code >= 500 {
			t.Errorf("🔴 geçersiz protokol %d döndürdü (%q): %s", w.Code, proto, body)
		}
	}
}

/* ═══════════════════ Yetki matrisi ═══════════════════ */

// TestProviderEndpointsAuthorizationMatrix
func TestProviderEndpointsAuthorizationMatrix(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "sag-yonetici")
	plain := seedAdminUser(t, "sag-musteri")

	const providers = "/api/v1/admin/providers"
	body := dto.CreateProviderRequest{
		Name: "yetki-" + uuid.NewString()[:6], Protocol: "FAKE",
		Priority: 100, CostMultiplier: "1.0",
	}
	cleanupProvider(t, body.Name)

	runAuthCases(t, rig, []authCase{
		// ── oturumsuz → 401 ──
		{"ekleme: oturumsuz", caller{}, http.MethodPost, providers, body, 401},
		{"senkron: oturumsuz", caller{}, http.MethodPost, providers + "/1/sync", nil, 401},
		{"liste: oturumsuz", caller{}, http.MethodGet, providers, nil, 401},

		// ── yetkisiz → 403 ──
		{"ekleme: izinsiz", caller{plain, ""}, http.MethodPost, providers, body, 403},
		{"ekleme: yalnız okuma izni", caller{admin, "providers:read"},
			http.MethodPost, providers, body, 403},
		{"senkron: yalnız okuma izni", caller{admin, "providers:read"},
			http.MethodPost, providers + "/1/sync", nil, 403},
		{"senkron: yanlış izin", caller{admin, "pricing:write"},
			http.MethodPost, providers + "/1/sync", nil, 403},

		// ── var olmayan kaynak → 404 ──
		{"senkron: var olmayan sağlayıcı", caller{admin, "providers:write"},
			http.MethodPost, providers + "/99999999/sync", nil, 404},
		{"senkron: bozuk kimlik", caller{admin, "providers:write"},
			http.MethodPost, providers + "/bozuk/sync", nil, 404},
	})
}

/* ═══════════════════ Senkron tetikleme ═══════════════════ */

// TestSyncEndpointReturnsImmediatelyAndRefusesSecondRun
//
// Uç işi BAŞLATIR ve döner; koşarken gelen ikinci istek 409 alır.
// Aynı anda iki senkron `provider_offers` üzerinde yarışır ve birinin
// `synced_at`'i diğerinin bayatlık eşiğinin altında kalarak canlı teklifleri
// "yok" işaretler — stokta numara varken satış durur.
func TestSyncEndpointReturnsImmediatelyAndRefusesSecondRun(t *testing.T) {
	admin := seedAdminUser(t, "senkron")

	name := "test-senkron-" + uuid.NewString()[:8]
	cleanupProvider(t, name)
	var id int64
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO providers (name, protocol, base_url, is_active, priority, cost_multiplier)
		VALUES ($1, 'FAKE', 'https://ornek.test', true, 500, 1.0)
		RETURNING id`, name).Scan(&id); err != nil {
		t.Fatalf("sağlayıcı eklenemedi: %v", err)
	}
	path := "/api/v1/admin/providers/" + strconv.FormatInt(id, 10) + "/sync"

	// Koşucuyu bloke ederek "hâlâ çalışıyor" durumunu yakalarız.
	block := make(chan struct{})
	rig := newAdminAuthRigWithSync(t, func(ctx context.Context, providerID int64) error {
		<-block
		return nil
	})

	w, body := rig.do(t, caller{admin, "providers:write"}, http.MethodPost, path, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("senkron başlatılamadı (%d): %s", w.Code, body)
	}
	var resp dto.ProviderSyncResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if !resp.Started || !resp.Sync.Running {
		t.Fatalf("senkron başlatıldı olarak bildirilmedi: %s", body)
	}

	// İkinci istek 409.
	w, body = rig.do(t, caller{admin, "providers:write"}, http.MethodPost, path, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("🔴 koşan senkron üstüne ikincisi %d ile kabul edildi: %s", w.Code, body)
	}
	if !strings.Contains(body, "SYNC_IN_PROGRESS") {
		t.Errorf("hata kodu yanıtta yok: %s", body)
	}

	// Sağlayıcı listesi durumu bildirmeli — panelin ilerlemeyi okuduğu yer.
	w, body = rig.do(t, caller{admin, "providers:read"}, http.MethodGet,
		"/api/v1/admin/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("liste (%d): %s", w.Code, body)
	}
	if !strings.Contains(body, `"running":true`) {
		t.Fatalf("🔴 senkron durumu listede yok — panel ilerlemeyi göremez: %s", body)
	}

	close(block)
}

// TestSyncEndpointRefusesWhenNotConfigured
//
// Koşucu bağlanmamışsa uç SESSİZ KALMAZ: 503 ve açık bir mesaj döner.
// 404 dönseydi yönetici "böyle bir özellik yok" sanır, yapılandırma
// eksikliğini hiç aramazdı.
func TestSyncEndpointRefusesWhenNotConfigured(t *testing.T) {
	admin := seedAdminUser(t, "senkron-yok")
	name := "test-senkron-yok-" + uuid.NewString()[:8]
	cleanupProvider(t, name)
	var id int64
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO providers (name, protocol, base_url, is_active, priority, cost_multiplier)
		VALUES ($1, 'FAKE', '', true, 500, 1.0) RETURNING id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}

	rig := newAdminAuthRigWithSync(t, nil)
	w, body := rig.do(t, caller{admin, "providers:write"}, http.MethodPost,
		"/api/v1/admin/providers/"+strconv.FormatInt(id, 10)+"/sync", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("durum = %d, 503 bekleniyordu: %s", w.Code, body)
	}
	if !strings.Contains(body, "SYNC_UNAVAILABLE") {
		t.Errorf("hata kodu yanıtta yok: %s", body)
	}
}
