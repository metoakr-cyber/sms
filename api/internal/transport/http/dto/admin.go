package dto

// Yönetim panelinin eksik uçları: fiyat kuralı (FR-703), denetim kaydı
// (FR-705), sağlayıcı ekleme (FR-701) ve katalog senkronu tetikleme.
//
// 🔴 Bu dosyadaki HİÇBİR yanıt sağlayıcı API anahtarını taşımaz — ne düz ne
// şifreli hâliyle, ne de "ekleme isteğinde verilmişti" diye geri yankılanarak.
// test: ../handler/admin_provider_integration_test.go#TestCreateProviderIgnoresAPIKeyInBody

import (
	"strings"
)

/* ═══════════════════════ Fiyat kuralları (FR-703) ═══════════════════════ */

// PricingRuleResponse etkin bir fiyat kuralı.
//
// `id` SAYISALDIR. pricing_rules tablosunda public_id yoktur ve bu kayıt
// kullanıcıya değil yalnız `pricing:read` iznine sahip yöneticiye görünür —
// sağlayıcı kayıtlarıyla aynı istisna (Değişmez #10 kullanıcıya açık
// kaynakları hedefler).
type PricingRuleResponse struct {
	ID              int64  `json:"id"`
	Scope           string `json:"scope"`
	ServiceCode     string `json:"serviceCode,omitempty"`
	CountryISO      string `json:"countryIso,omitempty"`
	DurationMinutes int32  `json:"durationMinutes,omitempty"`
	MarginPercent   string `json:"marginPercent"`
	FixedFee        Money  `json:"fixedFee"`
	MinPrice        Money  `json:"minPrice"`
	Note            string `json:"note"`
	CreatedAt       string `json:"createdAt"`
	// Replaced yalnız oluşturma yanıtında anlamlıdır: aynı kapsamdaki eski
	// kural devreden çıkarıldı mı.
	Replaced bool `json:"replaced,omitempty"`
}

type PricingRuleListResponse struct {
	Items []PricingRuleResponse `json:"items"`
}

// CreatePricingRuleRequest yeni fiyat kuralı.
//
// KAPSAM KODLARLA verilir (serviceCode/countryIso), sayısal kimlikle değil:
// istemcinin bilmediği bir kimliği göndermesi beklenemez ve gönderdiği kimliğe
// güvenilmez (Değişmez #9).
type CreatePricingRuleRequest struct {
	Scope           string `json:"scope"`
	ServiceCode     string `json:"serviceCode"`
	CountryISO      string `json:"countryIso"`
	DurationMinutes int32  `json:"durationMinutes"`
	MarginPercent   string `json:"marginPercent"`
	FixedFeeMinor   int64  `json:"fixedFeeMinor"`
	MinPriceMinor   int64  `json:"minPriceMinor"`
	Note            string `json:"note"`
}

func (r *CreatePricingRuleRequest) Validate() []FieldError {
	var errs []FieldError
	if strings.TrimSpace(r.Scope) == "" {
		errs = append(errs, FieldError{"scope", "Kapsam zorunludur."})
	}
	if strings.TrimSpace(r.MarginPercent) == "" {
		errs = append(errs, FieldError{"marginPercent", "Marj yüzdesi zorunludur."})
	}
	if r.DurationMinutes < 0 {
		errs = append(errs, FieldError{"durationMinutes", "Süre negatif olamaz."})
	}
	return errs
}

// PricingPreviewRequest canlı önizleme isteği.
//
// POST'TUR, GET DEĞİL — ama durum değiştirdiği için değil (değiştirmez):
// gövdesi aday kural taşır ve teklif üretmez. Sözleşme (docs/trd.md) bu yolu
// POST olarak tanımlıyor; okuma izni (`pricing:read`) yeterlidir.
type PricingPreviewRequest struct {
	ServiceCode     string `json:"serviceCode"`
	CountryISO      string `json:"countryIso"`
	DurationMinutes int32  `json:"durationMinutes"`

	// Candidate doluysa HENÜZ KAYDEDİLMEMİŞ kural denenir. Boşsa bugünkü
	// kayıtlı kural uygulanır.
	Candidate *PricingCandidate `json:"candidate"`
}

type PricingCandidate struct {
	MarginPercent string `json:"marginPercent"`
	FixedFeeMinor int64  `json:"fixedFeeMinor"`
	MinPriceMinor int64  `json:"minPriceMinor"`
}

func (r *PricingPreviewRequest) Validate() []FieldError {
	var errs []FieldError
	if strings.TrimSpace(r.ServiceCode) == "" {
		errs = append(errs, FieldError{"serviceCode", "Servis kodu zorunludur."})
	}
	if strings.TrimSpace(r.CountryISO) == "" {
		errs = append(errs, FieldError{"countryIso", "Ülke zorunludur."})
	}
	if r.DurationMinutes < 0 {
		errs = append(errs, FieldError{"durationMinutes", "Süre negatif olamaz."})
	}
	if r.Candidate != nil && strings.TrimSpace(r.Candidate.MarginPercent) == "" {
		errs = append(errs, FieldError{"candidate.marginPercent", "Marj yüzdesi zorunludur."})
	}
	return errs
}

// PricingPreviewResponse önizleme sonucu ve ara değerleri.
//
// Ara değerler (maliyet, kur, sağlayıcı) YALNIZ YÖNETİCİYE gider; kullanıcı
// teklifinde bunların hiçbiri yoktur (docs/memory.md §3.8).
type PricingPreviewResponse struct {
	SellPrice     Money  `json:"sellPrice"`
	Cost          Money  `json:"cost"`
	CostInTRY     Money  `json:"costInTry"`
	FXRate        string `json:"fxRate"`
	FXFetchedAt   string `json:"fxFetchedAt"`
	ProviderName  string `json:"providerName"`
	Stock         int    `json:"stock"`
	InStock       bool   `json:"inStock"`
	RuleSource    string `json:"ruleSource"` // SAVED | CANDIDATE
	RuleScope     string `json:"ruleScope,omitempty"`
	MarginPercent string `json:"marginPercent"`
	HitMinimum    bool   `json:"hitMinimum"`
	// CostSource önizlemenin hangi maliyete dayandığı. Bugün her zaman
	// "CACHE": önizleme sağlayıcıya canlı istek atmaz, dolayısıyla
	// aktivasyon satın almasında hesaplanan fiyat bundan farklı olabilir.
	CostSource string `json:"costSource"`
}

/* ═══════════════════════ Denetim kaydı (FR-705) ═══════════════════════ */

// AuditLogResponse tek bir denetim kaydı.
//
// 🔴 before/after HAM DÖNMEZ: yalnız izin listesindeki alanlar geçer, geri
// kalanı `redactedFields` altında ADLARIYLA bildirilir. Kayıt yazan her yeni
// çağrı, tanımadığı bir alanı sessizce dışarı sızdıramaz.
// test: ../handler/admin_audit_integration_test.go#TestAuditPayloadIsFieldFiltered
type AuditLogResponse struct {
	ID             int64          `json:"id"`
	Action         string         `json:"action"`
	EntityType     string         `json:"entityType"`
	EntityID       string         `json:"entityId"`
	ActorID        string         `json:"actorId,omitempty"`
	ActorUsername  string         `json:"actorUsername,omitempty"`
	Before         map[string]any `json:"before,omitempty"`
	After          map[string]any `json:"after,omitempty"`
	RedactedFields []string       `json:"redactedFields,omitempty"`
	IP             string         `json:"ip,omitempty"`
	RequestID      string         `json:"requestId,omitempty"`
	CreatedAt      string         `json:"createdAt"`
}

type AuditLogListResponse struct {
	Items  []AuditLogResponse `json:"items"`
	Total  int64              `json:"total"`
	Limit  int32              `json:"limit"`
	Offset int32              `json:"offset"`
}

/* ═══════════════════════ Sağlayıcı ekleme (FR-701) ═══════════════════════ */

// CreateProviderRequest yeni sağlayıcı.
//
// 🔴 API ANAHTARI ALANI YOKTUR ve eklenmeyecektir. Anahtar ayrı bir uçtan
// (PUT /admin/providers/:id/api-key) yazılır; aynı gövdede taşınsaydı,
// sağlayıcı ekleme isteğinin gövdesi log'a, hata ayıklama çıktısına ve
// tarayıcı ağ sekmesine anahtarla birlikte düşerdi.
type CreateProviderRequest struct {
	Name           string   `json:"name"`
	Protocol       string   `json:"protocol"`
	BaseURL        string   `json:"baseUrl"`
	Priority       int32    `json:"priority"`
	CostMultiplier string   `json:"costMultiplier"`
	Capabilities   []string `json:"capabilities"`
}

// ProviderProtocols desteklenen protokoller — şemadaki provider_protocol
// enum'uyla AYNI olmalıdır.
//
// Doğrulama Go'da da yapılır: bilinmeyen bir değer doğrudan Postgres'e
// giderse 22P02 ile 500 döner ve kullanıcı hatası sunucu hatası gibi görünür.
var ProviderProtocols = []string{"FAKE", "HEROSMS_V1", "FIVE_SIM"}

func (r *CreateProviderRequest) Validate() []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(r.Name)
	switch {
	case name == "":
		errs = append(errs, FieldError{"name", "Ad zorunludur."})
	case len(name) > 60:
		errs = append(errs, FieldError{"name", "Ad en fazla 60 karakter olabilir."})
	}

	known := false
	for _, p := range ProviderProtocols {
		if r.Protocol == p {
			known = true
		}
	}
	if !known {
		errs = append(errs, FieldError{"protocol",
			"Protokol şunlardan biri olmalıdır: " + strings.Join(ProviderProtocols, ", ")})
	}

	base := strings.TrimSpace(r.BaseURL)
	if base != "" && !strings.HasPrefix(base, "https://") && !strings.HasPrefix(base, "http://") {
		errs = append(errs, FieldError{"baseUrl", "Adres http:// veya https:// ile başlamalıdır."})
	}
	if len(base) > 300 {
		errs = append(errs, FieldError{"baseUrl", "Adres en fazla 300 karakter olabilir."})
	}

	if r.Priority < 0 || r.Priority > 10000 {
		errs = append(errs, FieldError{"priority", "Öncelik 0-10000 arasında olmalıdır."})
	}
	if strings.TrimSpace(r.CostMultiplier) == "" {
		errs = append(errs, FieldError{"costMultiplier", "Çarpan zorunludur."})
	}
	for _, c := range r.Capabilities {
		if c != "SMS_ACTIVATION" && c != "SMS_RENTAL" {
			errs = append(errs, FieldError{"capabilities",
				"Yetenek SMS_ACTIVATION veya SMS_RENTAL olmalıdır."})
			break
		}
	}
	return errs
}

/* ═══════════════════════ Katalog senkronu ═══════════════════════ */

// ProviderSyncResponse senkronun O ANKİ durumu.
//
// Senkron istek içinde KOŞMAZ: gerçek katalogda dakikalarca sürer, istemci
// zaman aşımına uğrar ve iş yarıda kalır. Uç yalnız işi başlatır ve durumu
// döner; ilerleme `GET /admin/providers` yanıtındaki `sync` alanından okunur.
// test: ../handler/admin_sync_test.go#TestSyncRunsInBackgroundAndIsSingleFlight
type ProviderSyncResponse struct {
	ProviderID string          `json:"providerId"`
	Started    bool            `json:"started"`
	Sync       ProviderSyncDTO `json:"sync"`
}

// ProviderSyncDTO tek bir sağlayıcının senkron durumu.
type ProviderSyncDTO struct {
	Running    bool   `json:"running"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	// Failed son turun başarısız olduğunu söyler. HAM HATA METNİ TAŞIMAZ
	// (Değişmez #12): sebep sunucu günlüğündedir.
	Failed bool `json:"failed"`
}
