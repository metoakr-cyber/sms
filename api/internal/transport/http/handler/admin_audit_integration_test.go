//go:build integration

package handler_test

// Denetim kaydı okuma (FR-705).
//
// Kurulum (adminAuthRig, seedAdminUser, runAuthCases) admin_pricing_integration_test.go'dadır.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
)

// seedAuditLog denetim kaydı satırı ekler ve temizliğini kurar.
func seedAuditLog(t *testing.T, actor int64, action, entityType, entityID, before, after string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id,
		                        before, after, ip, user_agent, request_id)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,'203.0.113.9','test-ua','req-123')
		RETURNING id`, actor, action, entityType, entityID, before, after).Scan(&id)
	if err != nil {
		t.Fatalf("denetim kaydı eklenemedi: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE id = $1`, id)
	})
	return id
}

/* ═══════════════════ Alan süzgeci ═══════════════════ */

// TestAuditPayloadIsFieldFiltered
//
// 🔴 before/after JSONB'si HAM DÖNMEZ.
//
// Denetim kaydını yazan çağrılar zamanla çoğalır ve her biri kendi alanlarını
// seçer. Yanıt gövdeyi olduğu gibi geçseydi, yarın eklenen bir `email`,
// `phone` veya `receiptPath` alanı — kimse fark etmeden — denetim listesinde
// dışarı akardı. Süzgeç bir İZİN LİSTESİDİR: tanınmayan alan geçmez.
//
// Ama sessizce de yutulmaz: saklanan alanların ADLARI `redactedFields`
// içinde bildirilir, yoksa denetçi eksik bir kaydı tam sanır.
func TestAuditPayloadIsFieldFiltered(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "denetci")

	entity := "deposit-" + uuid.NewString()
	seedAuditLog(t, admin, "deposit.approve", "deposit", entity,
		`{"status":"PENDING","amountMinor":50000}`,
		`{"status":"COMPLETED","amountMinor":50000,"creditedMinor":50000,
		  "email":"kurban@ornek.test","receiptPath":"/veri/dekont/gizli.jpg",
		  "phone":"+905551112233","txHash":"0xabc123"}`)

	w, body := rig.do(t, caller{admin, "audit:read"}, http.MethodGet,
		"/api/v1/admin/audit-logs?entityId="+entity, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d: %s", w.Code, body)
	}

	// (1) Hassas değerlerin HİÇBİRİ gövdede olmamalı.
	for _, leak := range []string{
		"kurban@ornek.test", "/veri/dekont/gizli.jpg", "+905551112233", "0xabc123",
	} {
		if strings.Contains(body, leak) {
			t.Fatalf("🔴 hassas alan denetim yanıtında sızdı: %q\n%s", leak, body)
		}
	}

	var resp dto.AuditLogListResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("yanıt çözülemedi: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("kayıt sayısı = %d, 1 bekleniyordu: %s", len(resp.Items), body)
	}
	item := resp.Items[0]

	// (2) İzinli alanlar GEÇMELİ — süzgeç her şeyi silmiyor olmalı, yoksa
	// denetim kaydı işe yaramaz.
	if item.After["status"] != "COMPLETED" {
		t.Fatalf("izinli alan süzüldü: %+v", item.After)
	}
	if item.Before["status"] != "PENDING" {
		t.Fatalf("öncesi kaybolmuş: %+v", item.Before)
	}

	// (3) Saklananların ADI bildirilmeli.
	got := strings.Join(item.RedactedFields, ",")
	for _, name := range []string{"email", "phone", "receiptPath", "txHash"} {
		if !strings.Contains(got, name) {
			t.Errorf("🔴 saklanan alan bildirilmedi: %s (bildirilen: %s)", name, got)
		}
	}

	// (4) Aktör public_id ile bildirilir; sayısal kimlik ve e-posta yok.
	if item.ActorID == "" {
		t.Error("aktör kimliği yok")
	}
	if _, err := uuid.Parse(item.ActorID); err != nil {
		t.Errorf("🔴 aktör kimliği UUID değil (%q) — sayısal id sızmış", item.ActorID)
	}
	if strings.Contains(body, "@ornek.test") {
		t.Error("🔴 aktör e-postası denetim listesinde")
	}
	// FR-705 IP'yi açıkça ister.
	if item.IP != "203.0.113.9" {
		t.Errorf("IP bildirilmedi: %q", item.IP)
	}
}

// TestAuditUnknownPayloadIsNotEchoed
//
// Tanınmayan bir gövde biçimi (JSON nesnesi olmayan) HİÇ GÖSTERİLMEZ:
// ne olduğu bilinmeyen bir metni panele basmak en iyi ihtimalle gürültü,
// en kötüsü sızıntıdır.
func TestAuditUnknownPayloadIsNotEchoed(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "denetci-bozuk")

	entity := "weird-" + uuid.NewString()
	seedAuditLog(t, admin, "weird.action", "weird", entity,
		`"cok-gizli-duz-metin"`, `["gizli-dizi-elemani"]`)

	w, body := rig.do(t, caller{admin, "audit:read"}, http.MethodGet,
		"/api/v1/admin/audit-logs?entityId="+entity, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d: %s", w.Code, body)
	}
	for _, leak := range []string{"cok-gizli-duz-metin", "gizli-dizi-elemani"} {
		if strings.Contains(body, leak) {
			t.Fatalf("🔴 nesne olmayan gövde olduğu gibi yankılandı: %s", body)
		}
	}
}

/* ═══════════════════ Süzgeç ve sayfalama ═══════════════════ */

// TestAuditFiltersAndPaging
func TestAuditFiltersAndPaging(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "denetci-suzgec")
	other := seedAdminUser(t, "denetci-diger")

	var otherPublic string
	if err := pool.QueryRow(context.Background(),
		`SELECT public_id::text FROM users WHERE id = $1`, other).Scan(&otherPublic); err != nil {
		t.Fatal(err)
	}

	tag := uuid.NewString()
	seedAuditLog(t, admin, "pricing.rule.create", "pricing_rule", "GLOBAL-"+tag,
		`{"marginPercent":"40.00"}`, `{"marginPercent":"55.00"}`)
	seedAuditLog(t, other, "pricing.rule.create", "pricing_rule", "COUNTRY-"+tag,
		`{"marginPercent":"10.00"}`, `{"marginPercent":"20.00"}`)

	// Aktöre göre süzme — public_id ile.
	w, body := rig.do(t, caller{admin, "audit:read"}, http.MethodGet,
		"/api/v1/admin/audit-logs?actorId="+otherPublic+"&entityType=pricing_rule", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d: %s", w.Code, body)
	}
	if strings.Contains(body, "GLOBAL-"+tag) {
		t.Fatalf("🔴 aktör süzgeci başka aktörün kaydını sızdırdı: %s", body)
	}
	if !strings.Contains(body, "COUNTRY-"+tag) {
		t.Fatalf("aktör süzgeci kendi kaydını da eledi: %s", body)
	}

	// Bozuk aktör kimliği 422 — 500 DEĞİL.
	w, body = rig.do(t, caller{admin, "audit:read"}, http.MethodGet,
		"/api/v1/admin/audit-logs?actorId=bozuk-uuid", nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bozuk aktör kimliği %d döndü, 422 bekleniyordu: %s", w.Code, body)
	}

	// Tarih süzgeci YALNIZ RFC 3339 (Değişmez #19).
	for _, bad := range []string{"2026-01-31 00:00:00", "31/01/2026", "dun"} {
		w, body = rig.do(t, caller{admin, "audit:read"}, http.MethodGet,
			"/api/v1/admin/audit-logs?from="+url.QueryEscape(bad), nil)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("🔴 RFC 3339 olmayan tarih (%q) kabul edildi: %d %s", bad, w.Code, body)
		}
	}

	// Geleceğe bakan aralık boş dönmeli — süzgecin gerçekten uygulandığının
	// kanıtı.
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	w, body = rig.do(t, caller{admin, "audit:read"}, http.MethodGet,
		"/api/v1/admin/audit-logs?entityType=pricing_rule&from="+url.QueryEscape(future), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d: %s", w.Code, body)
	}
	var resp dto.AuditLogListResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 0 || resp.Total != 0 {
		t.Fatalf("🔴 tarih süzgeci uygulanmadı: %d kayıt döndü", len(resp.Items))
	}

	// Sayfa boyutu tavanı: limit=9999 istense de 100'ü aşmaz.
	w, body = rig.do(t, caller{admin, "audit:read"}, http.MethodGet,
		"/api/v1/admin/audit-logs?limit=9999", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("durum = %d: %s", w.Code, body)
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Limit > 100 {
		t.Errorf("🔴 sayfa boyutu tavanı yok: limit=%d", resp.Limit)
	}
}

/* ═══════════════════ Yetki matrisi ═══════════════════ */

// TestAuditLogsAuthorizationMatrix
//
// Denetim kaydı `audit:read` ister. Başka bir yönetim izni (users:read,
// deposits:read) YETMEZ: kayıt tüm yönetim işlemlerinin izidir ve onu
// okuyabilen, kimin neyi ne zaman yaptığını görür.
func TestAuditLogsAuthorizationMatrix(t *testing.T) {
	rig := newAdminAuthRig(t)
	admin := seedAdminUser(t, "denetim-yetki")
	plain := seedAdminUser(t, "denetim-musteri")
	const path = "/api/v1/admin/audit-logs"

	runAuthCases(t, rig, []authCase{
		{"oturumsuz", caller{}, http.MethodGet, path, nil, 401},
		{"izinsiz", caller{plain, ""}, http.MethodGet, path, nil, 403},
		{"users:read yetmez", caller{admin, "users:read"}, http.MethodGet, path, nil, 403},
		{"deposits:read yetmez", caller{admin, "deposits:read"}, http.MethodGet, path, nil, 403},
		{"pricing:read yetmez", caller{admin, "pricing:read"}, http.MethodGet, path, nil, 403},
		// Var olmayan kayıt 404 değil BOŞ LİSTE döner: liste uçlarında
		// "sonuç yok" bir hata değildir.
		{"var olmayan varlık: boş liste", caller{admin, "audit:read"}, http.MethodGet,
			path + "?entityId=" + uuid.NewString(), nil, 200},
	})
}
