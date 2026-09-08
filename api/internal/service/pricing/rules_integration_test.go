//go:build integration

package pricing_test

// Fiyat kuralı yönetimi (FR-703) ve canlı önizleme.
//
// Buradaki testlerin ölçtüğü asıl şey iki tanedir:
//   1. Marjı değiştirmek sistemi SATILAMAZ hâle getiremez.
//   2. Panelde gösterilen fiyat ile tahsil edilecek fiyat AYNI hesaptan çıkar.
//
// TestMain, pool, setup, numeric ve errIs quote_integration_test.go içindedir.

import (
	"context"
	"strings"
	"testing"
	"time"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
)

type ruleEnv struct {
	*env
	rules *pricingsvc.RuleService
}

func setupRules(t *testing.T) *ruleEnv {
	t.Helper()
	e := setup(t, "40.00", 0)

	// Bu testin yazdığı kurallar, kullanıcı silinmeden ÖNCE temizlenmeli:
	// pricing_rules.created_by_user_id → users(id) yabancı anahtarı var ve
	// ON DELETE kuralı yok. t.Cleanup LIFO çalıştığı için burada kaydedilen
	// temizlik, setup'ın kullanıcı temizliğinden önce koşar.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM pricing_rules WHERE created_by_user_id = $1`, e.userID)
	})

	safety, err := money.RateFromString("1.0")
	if err != nil {
		t.Fatal(err)
	}
	return &ruleEnv{env: e, rules: pricingsvc.NewRuleService(pricingsvc.RuleDeps{
		TxRunner: e.tx, FX: e.fxsvc, Clock: e.clock, FXSafetyMargin: safety,
	})}
}

func activeGlobalRuleID(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM pricing_rules WHERE is_active AND scope = 'GLOBAL'`).Scan(&id); err != nil {
		t.Fatalf("etkin GLOBAL kural bulunamadı: %v", err)
	}
	return id
}

func countActiveGlobal(t *testing.T) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pricing_rules WHERE is_active AND scope = 'GLOBAL'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

/* ═══════════════════ Son GLOBAL kural koruması ═══════════════════ */

// TestDeactivatingLastGlobalRuleIsRefused
//
// 🔴 EN PAHALI SENARYO: yönetici "bu kuralı kaldırayım" der, GLOBAL kural
// kalmaz, ListApplicableRules boş döner, SelectRule ErrNoRule verir ve
// HİÇBİR ürün fiyatlanamaz. Site açık kalır, katalog görünür, ama her teklif
// NO_PRICING_RULE ile düşer — dışarıdan "site bozuldu" gibi görünür ve
// sebebi tek bir tıklamadır.
//
// Test yalnız hatayı değil, hatadan SONRAKİ durumu da ölçer: kural yerinde
// durmalı ve satış devam etmelidir.
func TestDeactivatingLastGlobalRuleIsRefused(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)
	id := activeGlobalRuleID(t)

	err := e.rules.Deactivate(ctx, pricingsvc.DeactivateRuleInput{
		RuleID: id, ActorUserID: e.userID,
	})
	if err == nil {
		t.Fatal("🔴 son GLOBAL kural pasifleştirildi — sistem satılamaz hâle geldi")
	}
	if !errIs(err, pricingsvc.ErrLastGlobalRule) {
		t.Fatalf("beklenen LAST_GLOBAL_RULE, gelen: %v", err)
	}

	// Kural HÂLÂ etkin.
	if n := countActiveGlobal(t); n != 1 {
		t.Fatalf("etkin GLOBAL kural sayısı %d, 1 bekleniyordu", n)
	}

	// Ve satış GERÇEKTEN devam ediyor — testin anlamlı olduğunun kanıtı.
	if _, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU",
	}); err != nil {
		t.Fatalf("reddedilmiş pasifleştirmeden sonra satış durdu: %v", err)
	}
}

// TestNonGlobalRuleCanBeDeactivated
//
// Koruma DAR olmalı: yalnız son GLOBAL kuralı korur. Daha geniş bir koruma
// (örn. "hiçbir kural pasifleştirilemez") yanlış girilmiş bir ülke kuralını
// kalıcı yapardı.
func TestNonGlobalRuleCanBeDeactivated(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)

	v, err := e.rules.Create(ctx, pricingsvc.CreateRuleInput{
		Target:        pricingsvc.RuleTarget{Scope: "COUNTRY", CountryISO: "RU"},
		MarginPercent: "10",
		ActorUserID:   e.userID,
	})
	if err != nil {
		t.Fatalf("ülke kuralı yazılamadı: %v", err)
	}
	if err := e.rules.Deactivate(ctx, pricingsvc.DeactivateRuleInput{
		RuleID: v.ID, ActorUserID: e.userID,
	}); err != nil {
		t.Fatalf("ülke kuralı pasifleştirilemedi: %v", err)
	}
}

/* ═══════════════════ Kural yazma ═══════════════════ */

// TestCreatingGlobalRuleReplacesPrevious
//
// Marj değiştirmenin TEK yolu yeni bir GLOBAL kural yazmaktır. Yeni kural
// eskisini AYNI TRANSACTION içinde devreden çıkarır: iki etkin GLOBAL kural
// asla yan yana durmaz ve arada bir an bile kuralsız kalınmaz.
// test: rules_integration_test.go#TestCreatingGlobalRuleReplacesPrevious
func TestCreatingGlobalRuleReplacesPrevious(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)
	oldID := activeGlobalRuleID(t)

	before, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU",
	})
	if err != nil {
		t.Fatal(err)
	}

	v, err := e.rules.Create(ctx, pricingsvc.CreateRuleInput{
		Target:        pricingsvc.RuleTarget{Scope: "GLOBAL"},
		MarginPercent: "100",
		Note:          "marj iki katına çıkarıldı",
		ActorUserID:   e.userID,
	})
	if err != nil {
		t.Fatalf("GLOBAL kural yazılamadı: %v", err)
	}
	if !v.Replaced {
		t.Error("eski kuralın devreden çıktığı bildirilmedi")
	}
	if n := countActiveGlobal(t); n != 1 {
		t.Fatalf("🔴 etkin GLOBAL kural sayısı %d — tek olmalı", n)
	}
	if v.ID == oldID {
		t.Fatal("yeni kural yazılmamış, eskisi güncellenmiş — geçmiş kayboldu")
	}

	// Eski kural SİLİNMEDİ, pasifleşti: geçmiş tekliflerin dayanağı durmalı.
	var stillThere bool
	if err := pool.QueryRow(ctx,
		`SELECT NOT is_active FROM pricing_rules WHERE id = $1`, oldID).Scan(&stillThere); err != nil {
		t.Fatalf("eski kural kayboldu: %v", err)
	}
	if !stillThere {
		t.Error("eski kural hâlâ etkin")
	}

	// Fiyat GERÇEKTEN değişti: %40 → %100.
	after, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU",
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.SellPrice.Minor() <= before.SellPrice.Minor() {
		t.Fatalf("🔴 marj değişti ama fiyat değişmedi: %d → %d",
			before.SellPrice.Minor(), after.SellPrice.Minor())
	}
}

// TestPricingRuleChangeIsAudited
//
// "Kim, ne zaman, hangi marjı yazdı?" — bir fiyat anlaşmazlığında
// cevaplanabilir olmalı. Kayıt sayısal kimlik değil, okunur kapsam anahtarı
// taşır.
func TestPricingRuleChangeIsAudited(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM audit_logs WHERE actor_user_id = $1`, e.userID)
	})

	if _, err := e.rules.Create(ctx, pricingsvc.CreateRuleInput{
		Target:        pricingsvc.RuleTarget{Scope: "SERVICE_COUNTRY", ServiceCode: "tg", CountryISO: "RU"},
		MarginPercent: "12.50",
		ActorUserID:   e.userID,
	}); err != nil {
		t.Fatal(err)
	}

	var entity, action string
	var after []byte
	if err := pool.QueryRow(ctx, `
		SELECT entity_id, action, after FROM audit_logs
		WHERE actor_user_id = $1 AND entity_type = 'pricing_rule'
		ORDER BY id DESC LIMIT 1`, e.userID).Scan(&entity, &action, &after); err != nil {
		t.Fatalf("🔴 fiyat kuralı değişikliği denetim kaydına yazılmadı: %v", err)
	}
	if action != "pricing.rule.create" {
		t.Errorf("eylem = %q", action)
	}
	if entity != "SERVICE_COUNTRY:tg:RU" {
		t.Errorf("kapsam anahtarı = %q, okunur bir anahtar bekleniyordu", entity)
	}
	if !strings.Contains(string(after), "12.50") {
		t.Errorf("yeni marj kayda geçmemiş: %s", after)
	}
}

// TestScopeInconsistencyIsRejectedBeforeDatabase
//
// Kapsam tutarlılığı Go'da doğrulanır; ham kısıt hatası KULLANICIYA GÖSTERİLMEZ.
// test: rules_integration_test.go#TestScopeInconsistencyIsRejectedBeforeDatabase
func TestScopeInconsistencyIsRejectedBeforeDatabase(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)

	cases := []struct {
		name   string
		target pricingsvc.RuleTarget
	}{
		{"GLOBAL + servis", pricingsvc.RuleTarget{Scope: "GLOBAL", ServiceCode: "tg"}},
		{"GLOBAL + ülke", pricingsvc.RuleTarget{Scope: "GLOBAL", CountryISO: "RU"}},
		{"COUNTRY: ülke yok", pricingsvc.RuleTarget{Scope: "COUNTRY"}},
		{"COUNTRY + servis", pricingsvc.RuleTarget{Scope: "COUNTRY", CountryISO: "RU", ServiceCode: "tg"}},
		{"SERVICE: servis yok", pricingsvc.RuleTarget{Scope: "SERVICE"}},
		{"SERVICE_COUNTRY: yarım", pricingsvc.RuleTarget{Scope: "SERVICE_COUNTRY", ServiceCode: "tg"}},
		{"bilinmeyen kapsam", pricingsvc.RuleTarget{Scope: "HERKESE"}},
		{"süre GLOBAL'de", pricingsvc.RuleTarget{Scope: "GLOBAL", DurationMinutes: 60}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.rules.Create(ctx, pricingsvc.CreateRuleInput{
				Target: tc.target, MarginPercent: "40", ActorUserID: e.userID,
			})
			if err == nil {
				t.Fatal("🔴 tutarsız kapsam kabul edildi")
			}
			ae, ok := apperr.As(err)
			if !ok {
				t.Fatalf("tipsiz hata: %v", err)
			}
			// Kullanıcıya giden metin ham kısıt adı OLMAMALI.
			for _, leak := range []string{"pricing_scope_consistent", "SQLSTATE", "violates", "pgconn"} {
				if strings.Contains(ae.Message, leak) {
					t.Fatalf("🔴 ham veritabanı hatası kullanıcıya gitti: %q", ae.Message)
				}
			}
			if ae.Message == "" {
				t.Fatal("boş hata mesajı")
			}
		})
	}
}

// TestMarginInputIsValidated
//
// NUMERIC(7,2) sınırları ve serbest metin tuzakları.
func TestMarginInputIsValidated(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)

	bad := []string{"", "abc", "-10", "1e5", "1/3", "2000", "40.123", "  "}
	for _, m := range bad {
		_, err := e.rules.Create(ctx, pricingsvc.CreateRuleInput{
			Target:        pricingsvc.RuleTarget{Scope: "COUNTRY", CountryISO: "UA"},
			MarginPercent: m, ActorUserID: e.userID,
		})
		if err == nil {
			t.Errorf("🔴 geçersiz marj kabul edildi: %q", m)
		}
	}

	// Türkçe klavyeden gelen virgüllü ondalık KABUL EDİLİR.
	if _, err := e.rules.Create(ctx, pricingsvc.CreateRuleInput{
		Target:        pricingsvc.RuleTarget{Scope: "COUNTRY", CountryISO: "UA"},
		MarginPercent: "40,5", ActorUserID: e.userID,
	}); err != nil {
		t.Fatalf("virgüllü marj reddedildi: %v", err)
	}

	// Taban fiyat ve sabit bedel üst sınırı (fazladan iki sıfır tuzağı).
	if _, err := e.rules.Create(ctx, pricingsvc.CreateRuleInput{
		Target:        pricingsvc.RuleTarget{Scope: "COUNTRY", CountryISO: "UA"},
		MarginPercent: "40", MinPriceMinor: 999_999_999_999, ActorUserID: e.userID,
	}); err == nil {
		t.Error("🔴 saçma yükseklikte taban fiyat kabul edildi")
	}
}

/* ═══════════════════ Önizleme ═══════════════════ */

// TestPreviewMatchesQuotePriceForCachedCost
//
// 🔴 PANELDE GÖRÜLEN FİYAT = TAHSİL EDİLECEK FİYAT.
//
// Önizleme kendi hesabını yazsaydı (yuvarlama yönü, tampon, çarpan sırası)
// yönetici %40 marjla 55,00 ₺ görür, kullanıcı 54,99 ₺ öderdi ve fark aylarca
// fark edilmezdi. İki yol da domain/pricing.Calculate'i çağırır; bu test o
// bağı sabitler.
func TestPreviewMatchesQuotePriceForCachedCost(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)

	prev, err := e.rules.Preview(ctx, pricingsvc.PreviewInput{
		ServiceCode: "tg", CountryISO: "RU",
	})
	if err != nil {
		t.Fatalf("önizleme: %v", err)
	}
	quote, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU",
	})
	if err != nil {
		t.Fatalf("teklif: %v", err)
	}
	if prev.SellPrice.Minor() != quote.SellPrice.Minor() {
		t.Fatalf("🔴 önizleme %d kuruş, teklif %d kuruş — iki ayrı fiyat hesabı var",
			prev.SellPrice.Minor(), quote.SellPrice.Minor())
	}
	if prev.RuleSource != "SAVED" || prev.RuleScope != "GLOBAL" {
		t.Errorf("kural kaynağı/kapsamı = %q/%q", prev.RuleSource, prev.RuleScope)
	}

	// Aday kural, kaydedilmeden farklı bir fiyat göstermeli.
	cand, err := e.rules.Preview(ctx, pricingsvc.PreviewInput{
		ServiceCode: "tg", CountryISO: "RU",
		Candidate: &pricingsvc.CandidateRule{MarginPercent: "100"},
	})
	if err != nil {
		t.Fatalf("aday önizleme: %v", err)
	}
	if cand.SellPrice.Minor() <= prev.SellPrice.Minor() {
		t.Fatalf("aday marj (%%100) fiyatı yükseltmedi: %d vs %d",
			cand.SellPrice.Minor(), prev.SellPrice.Minor())
	}
	if cand.RuleSource != "CANDIDATE" {
		t.Errorf("aday kural SAVED olarak bildirildi")
	}

	// 🔴 ADAY KURAL KAYDEDİLMEZ: önizleme bir yazma işlemi değildir.
	if n := countActiveGlobal(t); n != 1 {
		t.Fatalf("önizleme kural yazdı — etkin GLOBAL sayısı %d", n)
	}
	after, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "tg", CountryISO: "RU",
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.SellPrice.Minor() != quote.SellPrice.Minor() {
		t.Fatalf("🔴 önizleme gerçek fiyatı değiştirdi: %d → %d",
			quote.SellPrice.Minor(), after.SellPrice.Minor())
	}
}

// TestPreviewWorksWhenOutOfStock
//
// Stoğu tükenmiş ürünün MALİYETİ bilinir, dolayısıyla FİYATI da hesaplanabilir.
// Satış yolu aynı ürünü reddeder — ikisi farklı sorulara cevap verir:
// "bu ürünü şimdi satabilir miyim?" ile "bu marjla kaça satardım?".
//
// (Sahte sağlayıcının gerçek gözleme dayanan kaydı: wa × TR stoksuzdur.)
func TestPreviewWorksWhenOutOfStock(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)

	// Satış yolu: stok yok.
	if _, err := e.quotes.Create(ctx, pricingsvc.QuoteRequest{
		UserID: e.userID, ServiceCode: "wa", CountryISO: "TR",
	}); !errIs(err, apperr.ErrOutOfStock) {
		t.Fatalf("stoksuz ürün için teklif hatası = %v, OUT_OF_STOCK bekleniyordu", err)
	}

	// Önizleme: fiyat yine de hesaplanır.
	prev, err := e.rules.Preview(ctx, pricingsvc.PreviewInput{
		ServiceCode: "wa", CountryISO: "TR",
	})
	if err != nil {
		t.Fatalf("🔴 stoksuz üründe önizleme yapılamadı: %v", err)
	}
	if !prev.SellPrice.IsPositive() {
		t.Fatal("önizleme sıfır fiyat döndü")
	}
	if prev.Stock != 0 {
		t.Errorf("stok = %d, 0 bekleniyordu", prev.Stock)
	}
	if !prev.Cost.IsPositive() {
		t.Error("maliyet bildirilmedi")
	}
}

// TestPreviewStopsWhenFXIsStale
//
// Kur bayatsa satış durur (KK-302). Önizleme de durmalıdır: bayat kurla
// hesaplanmış bir fiyatı panelde göstermek, yöneticiyi yanlış bir marj
// kararına iter.
func TestPreviewStopsWhenFXIsStale(t *testing.T) {
	ctx := context.Background()
	e := setup(t, "40.00", 30*time.Minute)
	safety, _ := money.RateFromString("1.0")
	rules := pricingsvc.NewRuleService(pricingsvc.RuleDeps{
		TxRunner: e.tx, FX: e.fxsvc, Clock: e.clock, FXSafetyMargin: safety,
	})

	if _, err := rules.Preview(ctx, pricingsvc.PreviewInput{
		ServiceCode: "tg", CountryISO: "RU"}); err != nil {
		t.Fatalf("taze kurla önizleme başarısız: %v", err)
	}
	e.clock.Advance(31 * time.Minute)
	if _, err := rules.Preview(ctx, pricingsvc.PreviewInput{
		ServiceCode: "tg", CountryISO: "RU"}); !errIs(err, apperr.ErrFxUnavailable) {
		t.Fatalf("🔴 bayat kurla önizleme yapıldı: %v", err)
	}
}

// TestPreviewRejectsUnknownProduct
func TestPreviewRejectsUnknownProduct(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)

	for _, tc := range []pricingsvc.PreviewInput{
		{ServiceCode: "yok-boyle-servis", CountryISO: "RU"},
		{ServiceCode: "tg", CountryISO: "ZZ"},
		{ServiceCode: "tg", CountryISO: "RU", DurationMinutes: 999},
	} {
		if _, err := e.rules.Preview(ctx, tc); err == nil {
			t.Errorf("🔴 tanımsız ürün için önizleme üretildi: %+v", tc)
		}
	}
}

// TestSpecificRuleWinsInPreview
//
// Kapsam önceliği önizlemede de geçerlidir: SERVICE_COUNTRY, GLOBAL'i yener.
// Aksi hâlde yönetici özel bir kural yazar, önizlemede GLOBAL fiyatı görür ve
// kuralın işlemediğini sanır.
func TestSpecificRuleWinsInPreview(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)

	base, err := e.rules.Preview(ctx, pricingsvc.PreviewInput{ServiceCode: "tg", CountryISO: "RU"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.rules.Create(ctx, pricingsvc.CreateRuleInput{
		Target:        pricingsvc.RuleTarget{Scope: "SERVICE_COUNTRY", ServiceCode: "tg", CountryISO: "RU"},
		MarginPercent: "300",
		ActorUserID:   e.userID,
	}); err != nil {
		t.Fatal(err)
	}
	after, err := e.rules.Preview(ctx, pricingsvc.PreviewInput{ServiceCode: "tg", CountryISO: "RU"})
	if err != nil {
		t.Fatal(err)
	}
	if after.RuleScope != "SERVICE_COUNTRY" {
		t.Fatalf("🔴 uygulanan kapsam %q — özel kural yenilmiş", after.RuleScope)
	}
	if after.SellPrice.Minor() <= base.SellPrice.Minor() {
		t.Fatalf("özel kural fiyata yansımadı: %d → %d",
			base.SellPrice.Minor(), after.SellPrice.Minor())
	}
}

// TestListReturnsReadableScope
//
// Panelde "service_id 42" değil "tg × RU" yazmalı.
func TestListReturnsReadableScope(t *testing.T) {
	ctx := context.Background()
	e := setupRules(t)

	if _, err := e.rules.Create(ctx, pricingsvc.CreateRuleInput{
		Target:        pricingsvc.RuleTarget{Scope: "SERVICE_COUNTRY", ServiceCode: "tg", CountryISO: "RU"},
		MarginPercent: "55",
		ActorUserID:   e.userID,
	}); err != nil {
		t.Fatal(err)
	}
	views, err := e.rules.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range views {
		if v.Scope == "SERVICE_COUNTRY" {
			found = true
			if v.ServiceCode != "tg" || v.CountryISO != "RU" {
				t.Fatalf("kapsam çözülmedi: %+v", v)
			}
			if v.MarginPercent != "55.00" && v.MarginPercent != "55" {
				t.Errorf("marj = %q", v.MarginPercent)
			}
		}
	}
	if !found {
		t.Fatal("yazılan kural listede yok")
	}
	// GLOBAL kural her zaman listede olmalı — yoksa satış yapılamıyordur.
	globalSeen := false
	for _, v := range views {
		if v.Scope == "GLOBAL" {
			globalSeen = true
		}
	}
	if !globalSeen {
		t.Fatal("etkin GLOBAL kural listede yok")
	}
}
