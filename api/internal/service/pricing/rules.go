package pricing

// Fiyat kuralı yönetimi (FR-703) ve canlı önizleme.
//
// NEDEN VAR: marj bir İŞ KARARIDIR ve bugüne kadar yalnız migration'da
// tohumlanmış tek bir %40 GLOBAL kuralla yaşıyordu. Onu değiştirmenin tek yolu
// üretim veritabanında elle SQL yazmaktı — para sisteminde yapılabilecek en
// tehlikeli operasyon.
//
// 🔴 FİYAT HESABI BURADA TEKRARLANMAZ. Hem teklif (QuoteService.Create) hem
// önizleme aynı domain fonksiyonunu (dompricing.Calculate), aynı kural
// seçicisini (selectRule) ve aynı sağlayıcı sıralamasını (cheapestCached)
// kullanır. İkinci bir hesap, panelde görülen fiyat ile tahsil edilen fiyatın
// sessizce ayrışması demektir.
// test: rules_integration_test.go#TestPreviewMatchesQuotePriceForCachedCost

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	dompricing "github.com/ikmetrik/sms-platform/api/internal/domain/pricing"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	auditsvc "github.com/ikmetrik/sms-platform/api/internal/service/audit"
)

// maxRuleAmountMinor sabit bedel ve taban fiyat için üst sınır (1.000.000,00 ₺).
//
// Şemada böyle bir sınır YOK (yalnız negatiflik yasak). Sınır burada var çünkü
// asıl risk aşırı büyük bir değer değil, FAZLADAN İKİ SIFIR: 5,00 ₺ yerine
// 500,00 ₺ taban fiyat yazmak tüm ürünleri satılamaz yapar ve bunu ilk fark
// eden kullanıcı olur.
const maxRuleAmountMinor int64 = 100_000_000

// ErrLastGlobalRule son GLOBAL kuralın pasifleştirilmesi girişimi.
//
// GLOBAL kural kalmazsa ListApplicableRules boş döner, SelectRule ErrNoRule
// verir ve HİÇBİR ürün fiyatlanamaz: sistem tümüyle satılamaz hâle gelir.
// Tek bir düğme tıklamasının bunu yapabilmesi kabul edilemez.
// test: rules_integration_test.go#TestDeactivatingLastGlobalRuleIsRefused
var ErrLastGlobalRule = apperr.NewStatus(apperr.KindDomain, "LAST_GLOBAL_RULE",
	"Son GLOBAL fiyat kuralı pasifleştirilemez — sistem satış yapamaz hâle gelir. "+
		"Marjı değiştirmek için yeni bir GLOBAL kural oluşturun; eskisi otomatik olarak devreden çıkar.",
	409)

// ErrRuleConflict aynı kapsamda eşzamanlı ikinci bir kural yazımı.
var ErrRuleConflict = apperr.NewStatus(apperr.KindDomain, "PRICING_RULE_CONFLICT",
	"Bu kapsamda az önce başka bir kural oluşturuldu. Listeyi yenileyip tekrar deneyin.", 409)

// RuleService fiyat kurallarını okur, yazar ve önizleme üretir.
type RuleService struct {
	tx     *postgres.TxRunner
	fx     *FXService
	clock  port.Clock
	safety money.Rate
}

type RuleDeps struct {
	TxRunner       *postgres.TxRunner
	FX             *FXService
	Clock          port.Clock
	FXSafetyMargin money.Rate
}

func NewRuleService(d RuleDeps) *RuleService {
	if d.Clock == nil {
		d.Clock = port.RealClock{}
	}
	safety := d.FXSafetyMargin
	if safety.IsZero() {
		safety = money.RateOne()
	}
	return &RuleService{tx: d.TxRunner, fx: d.FX, clock: d.Clock, safety: safety}
}

/* ═══════════════════════════ Girdi tipleri ═══════════════════════════ */

// RuleTarget kuralın hangi kapsama yazılacağı.
//
// KAPSAM SAYISAL KİMLİKLE DEĞİL, KODLARLA anlatılır: "whatsapp × TR".
// pricing_rules ve products tablolarında public_id yoktur; servis kodu ile
// ülke ISO'su hem kalıcı hem de yöneticinin zaten düşündüğü dil.
type RuleTarget struct {
	Scope           string
	ServiceCode     string
	CountryISO      string
	DurationMinutes int32 // yalnız PRODUCT kapsamı; >0 ise kiralık ürün
}

// CreateRuleInput yeni kural.
//
// Kural GÜNCELLEME diye bir işlem yoktur: aynı kapsamdaki etkin kural
// pasifleştirilir ve yenisi eklenir. Böylece eski kural tarihte kalır ve
// "o gün hangi marjla satmışız?" sorusu cevaplanabilir.
type CreateRuleInput struct {
	Target        RuleTarget
	MarginPercent string
	FixedFeeMinor int64
	MinPriceMinor int64
	Note          string
	ActorUserID   int64
	Audit         auditsvc.Meta
}

// DeactivateRuleInput kuralı devreden çıkarma.
type DeactivateRuleInput struct {
	RuleID      int64
	ActorUserID int64
	Audit       auditsvc.Meta
}

// CandidateRule HENÜZ KAYDEDİLMEMİŞ kural — yalnız önizleme için.
type CandidateRule struct {
	MarginPercent string
	FixedFeeMinor int64
	MinPriceMinor int64
}

// PreviewInput önizleme isteği.
type PreviewInput struct {
	ServiceCode     string
	CountryISO      string
	DurationMinutes int32
	// Candidate nil ise KAYITLI kurallar uygulanır (bugünkü fiyat);
	// doluysa aday kural denenir (değişiklik sonrası fiyat).
	Candidate *CandidateRule
}

/* ═══════════════════════════ Çıktı tipleri ═══════════════════════════ */

// RuleView bir fiyat kuralının okunur hâli.
type RuleView struct {
	ID              int64
	Scope           string
	ServiceCode     string
	CountryISO      string
	DurationMinutes int32
	MarginPercent   string
	FixedFee        money.Money
	MinPrice        money.Money
	Note            string
	CreatedAt       time.Time
	// Replaced: bu kural yazılırken aynı kapsamdaki eski kural devreden
	// çıkarıldı mı. Yönetici "kaydettim ama iki kural mı oldu?" diye
	// sormasın diye açıkça bildirilir.
	Replaced bool
}

// PreviewResult önizleme çıktısı — ara değerlerle birlikte.
//
// Ara değerler DÖNER çünkü "bu fiyat nasıl oluştu" sorusu panelde
// cevaplanabilir olmalı: maliyet mi arttı, kur mu, marj mı.
type PreviewResult struct {
	SellPrice     money.Money
	Cost          money.Money // sağlayıcı maliyeti (USD)
	CostInTRY     money.Money
	FXRate        string
	FXFetchedAt   time.Time
	ProviderName  string
	Stock         int
	RuleSource    string // "SAVED" | "CANDIDATE"
	RuleScope     string // yalnız SAVED
	MarginPercent string
	HitMinimum    bool
}

/* ═══════════════════════════ Okuma ═══════════════════════════ */

// List etkin fiyat kurallarını döner.
func (s *RuleService) List(ctx context.Context) ([]RuleView, error) {
	rows, err := s.tx.Queries().ListPricingRulesForAdmin(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	out := make([]RuleView, 0, len(rows))
	for _, r := range rows {
		v := RuleView{
			ID:            r.ID,
			Scope:         string(r.Scope),
			MarginPercent: numericText(r.MarginPercent),
			FixedFee:      money.New(r.FixedFeeMinor, money.TRY),
			MinPrice:      money.New(r.MinPriceMinor, money.TRY),
			Note:          r.Note,
			CreatedAt:     r.CreatedAt,
		}
		if r.ServiceCode != nil {
			v.ServiceCode = *r.ServiceCode
		}
		if r.CountryIso2 != nil {
			v.CountryISO = *r.CountryIso2
		}
		if r.ProductDurationMinutes != nil {
			v.DurationMinutes = *r.ProductDurationMinutes
		}
		out = append(out, v)
	}
	return out, nil
}

/* ═══════════════════════════ Yazma ═══════════════════════════ */

// Create yeni bir fiyat kuralı yazar; aynı kapsamdaki eskisini devreden çıkarır.
//
// Tümü TEK TRANSACTION içinde olur: eski kural pasifleşip yenisi yazılamazsa
// o kapsam kuralsız kalır ve fiyat sessizce daha genel bir kurala (en kötü
// hâlde GLOBAL'e) düşer.
func (s *RuleService) Create(ctx context.Context, in CreateRuleInput) (RuleView, error) {
	margin, marginText, err := parseMarginPercent(in.MarginPercent)
	if err != nil {
		return RuleView{}, err
	}
	if err := checkAmount("sabit bedel", in.FixedFeeMinor); err != nil {
		return RuleView{}, err
	}
	if err := checkAmount("taban fiyat", in.MinPriceMinor); err != nil {
		return RuleView{}, err
	}
	note := strings.TrimSpace(in.Note)
	if len(note) > 500 {
		return RuleView{}, apperr.ErrValidation.WithMessage("Not en fazla 500 karakter olabilir.")
	}

	var view RuleView
	err = s.tx.InTx(ctx, func(q *db.Queries) error {
		target, err := s.resolve(ctx, q, in.Target)
		if err != nil {
			return err
		}

		// ── Aynı kapsamdaki etkin kuralı KİLİTLE ve devreden çıkar ──
		var before any
		prev, err := q.LockActiveRuleForScope(ctx, db.LockActiveRuleForScopeParams{
			Scope:     db.PricingScope(target.scope.String()),
			ServiceID: target.serviceID,
			CountryID: target.countryID,
			ProductID: target.productID,
		})
		switch {
		case err == nil:
			before = ruleSnapshot(prev)
			if err := q.DeactivatePricingRule(ctx, prev.ID); err != nil {
				return apperr.Internal(err)
			}
			view.Replaced = true
		case errors.Is(err, pgx.ErrNoRows):
			// Bu kapsamda kural yoktu — ilk kez yazılıyor.
		default:
			return apperr.Internal(err)
		}

		actor := in.ActorUserID
		row, err := q.CreatePricingRule(ctx, db.CreatePricingRuleParams{
			Scope:           db.PricingScope(target.scope.String()),
			ServiceID:       target.serviceID,
			CountryID:       target.countryID,
			ProductID:       target.productID,
			MarginPercent:   margin,
			FixedFeeMinor:   in.FixedFeeMinor,
			MinPriceMinor:   in.MinPriceMinor,
			Note:            note,
			CreatedByUserID: &actor,
		})
		if err != nil {
			// Kısmi tekil indeks: aynı kapsamda ikinci bir ETKİN kural.
			// Kilide rağmen buraya düşülürse başka bir transaction araya
			// girmiştir. HAM KISIT ADI KULLANICIYA GÖSTERİLMEZ.
			// test: ../../transport/http/handler/admin_pricing_integration_test.go#TestScopeMismatchGivesReadableError
			if isUniqueViolation(err) {
				return ErrRuleConflict.Wrap(err)
			}
			// Kapsam tutarlılığı CHECK'i Go tarafında da doğrulanıyor;
			// buraya düşmek programlama hatasıdır, kullanıcı hatası değil.
			return apperr.Internal(err)
		}

		if err := auditsvc.Record(ctx, q, auditsvc.Entry{
			ActorUserID: &actor,
			Action:      "pricing.rule.create",
			EntityType:  "pricing_rule",
			EntityID:    target.key,
			Before:      before,
			After:       ruleSnapshot(row),
			Meta:        in.Audit,
		}); err != nil {
			return apperr.Internal(err)
		}

		view.ID = row.ID
		view.Scope = string(row.Scope)
		view.ServiceCode = in.Target.ServiceCode
		view.CountryISO = in.Target.CountryISO
		view.DurationMinutes = in.Target.DurationMinutes
		view.MarginPercent = marginText
		view.FixedFee = money.New(row.FixedFeeMinor, money.TRY)
		view.MinPrice = money.New(row.MinPriceMinor, money.TRY)
		view.Note = row.Note
		view.CreatedAt = row.CreatedAt
		return nil
	})
	if err != nil {
		return RuleView{}, err
	}
	return view, nil
}

// Deactivate bir kuralı devreden çıkarır.
//
// SON GLOBAL KURAL PASİFLEŞTİRİLEMEZ (ErrLastGlobalRule): fiyatlandırma
// kuralsız kalır ve tüm satış durur.
// test: rules_integration_test.go#TestDeactivatingLastGlobalRuleIsRefused
func (s *RuleService) Deactivate(ctx context.Context, in DeactivateRuleInput) error {
	return s.tx.InTx(ctx, func(q *db.Queries) error {
		rule, err := q.GetPricingRule(ctx, in.RuleID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrNotFound.WithMessage("Fiyat kuralı bulunamadı.")
		}
		if err != nil {
			return apperr.Internal(err)
		}
		if !rule.IsActive {
			return apperr.ErrValidation.WithMessage("Bu kural zaten pasif.")
		}

		if rule.Scope == db.PricingScopeGLOBAL {
			n, err := q.CountActiveRulesByScope(ctx, db.PricingScopeGLOBAL)
			if err != nil {
				return apperr.Internal(err)
			}
			// Kısmi tekil indeks etkin GLOBAL kuralı zaten ≤1 ile sınırlar;
			// yani bu koşul bugün DAİMA doğrudur. Yine de sayarak yazılır:
			// indeks bir gün gevşetilirse kural kendiliğinden doğru kalır.
			if n <= 1 {
				return ErrLastGlobalRule
			}
		}

		if err := q.DeactivatePricingRule(ctx, rule.ID); err != nil {
			return apperr.Internal(err)
		}

		actor := in.ActorUserID
		after := ruleSnapshot(rule)
		after["isActive"] = false
		if err := auditsvc.Record(ctx, q, auditsvc.Entry{
			ActorUserID: &actor,
			Action:      "pricing.rule.deactivate",
			EntityType:  "pricing_rule",
			EntityID:    scopeKeyFromRule(ctx, q, rule.ID, rule.Scope),
			Before:      ruleSnapshot(rule),
			After:       after,
			Meta:        in.Audit,
		}); err != nil {
			return apperr.Internal(err)
		}
		return nil
	})
}

/* ═══════════════════════════ Önizleme ═══════════════════════════ */

// Preview seçilen ürün için mevcut maliyetle satış fiyatını hesaplar.
//
// SAĞLAYICIYA CANLI İSTEK ATILMAZ: önizleme yönetim panelinde her tuş
// vuruşunda çağrılabilir ve her çağrının sağlayıcıya gitmesi hem hız limitini
// yer hem de satış yolundaki teklifleri yavaşlatır. Önbellekteki maliyet
// kullanılır — teklif yolunda AKTİVASYON için canlı maliyet sorulduğundan
// gerçek satış fiyatı bundan farklı çıkabilir; kiralıkta ikisi aynıdır.
// test: rules_integration_test.go#TestPreviewMatchesQuotePriceForCachedCost
func (s *RuleService) Preview(ctx context.Context, in PreviewInput) (PreviewResult, error) {
	q := s.tx.Queries()

	fxq, err := s.fx.Current(ctx)
	if err != nil {
		return PreviewResult{}, err // ErrFxUnavailable — fiyat hesaplanamaz
	}

	svc, err := q.GetServiceByCode(ctx, strings.TrimSpace(in.ServiceCode))
	if err != nil {
		return PreviewResult{}, apperr.ErrNotFound.WithMessage("Servis bulunamadı.")
	}
	ctry, err := q.GetCountryByISO(ctx, strings.ToUpper(strings.TrimSpace(in.CountryISO)))
	if err != nil {
		return PreviewResult{}, apperr.ErrNotFound.WithMessage("Ülke bulunamadı.")
	}
	prod, err := findProduct(ctx, q, svc.ID, ctry.ID, in.DurationMinutes)
	if err != nil {
		return PreviewResult{}, err
	}

	rows, err := q.ListOffersForProductAnyStock(ctx, prod.ID)
	if err != nil {
		return PreviewResult{}, apperr.Internal(err)
	}
	best, err := cheapestCached(anyStockRows(rows), false)
	if err != nil {
		// Hiç teklif yoksa gösterilecek maliyet de yoktur.
		return PreviewResult{}, apperr.ErrOutOfStock.WithMessage(
			"Bu ürün için hiçbir sağlayıcıda fiyat kaydı yok; önizleme yapılamıyor.")
	}

	var rule dompricing.Rule
	source := "SAVED"
	if in.Candidate != nil {
		source = "CANDIDATE"
		_, marginText, err := parseMarginPercent(in.Candidate.MarginPercent)
		if err != nil {
			return PreviewResult{}, err
		}
		if err := checkAmount("sabit bedel", in.Candidate.FixedFeeMinor); err != nil {
			return PreviewResult{}, err
		}
		if err := checkAmount("taban fiyat", in.Candidate.MinPriceMinor); err != nil {
			return PreviewResult{}, err
		}
		// Kapsam yalnız Calculate'in "kural var mı" kontrolünü geçmek için
		// doludur; aday kural henüz bir kapsama yazılmamıştır ve çıktıda
		// kapsam olarak RAPOR EDİLMEZ.
		rule = dompricing.Rule{
			Scope:         dompricing.ScopeProduct,
			MarginPercent: marginText,
			FixedFee:      money.New(in.Candidate.FixedFeeMinor, money.TRY),
			MinPrice:      money.New(in.Candidate.MinPriceMinor, money.TRY),
		}
	} else {
		applicable, err := q.ListApplicableRules(ctx, db.ListApplicableRulesParams{
			CountryID: &ctry.ID, ServiceID: &svc.ID, ProductID: &prod.ID,
		})
		if err != nil {
			return PreviewResult{}, apperr.Internal(err)
		}
		rule, err = selectRule(applicable)
		if err != nil {
			return PreviewResult{}, apperr.ErrNoPricingRule.Wrap(err)
		}
	}

	multiplier, err := numericToRate(best.CostMultiplier)
	if err != nil {
		multiplier = money.RateOne()
	}
	calc, err := dompricing.Calculate(dompricing.Input{
		Cost:               best.Cost,
		ProviderMultiplier: multiplier,
		FXRate:             fxq.Rate,
		FXSafetyMargin:     s.safety,
		Rule:               rule,
	})
	if err != nil {
		return PreviewResult{}, apperr.Internal(err)
	}

	res := PreviewResult{
		SellPrice:     calc.SellPrice,
		Cost:          best.Cost,
		CostInTRY:     calc.CostInTRY,
		FXRate:        fxq.Rate.String(),
		FXFetchedAt:   fxq.FetchedAt,
		ProviderName:  best.ProviderName,
		Stock:         best.Stock,
		RuleSource:    source,
		MarginPercent: calc.MarginApplied,
		HitMinimum:    calc.HitMinimum,
	}
	if source == "SAVED" {
		res.RuleScope = rule.Scope.String()
	}
	return res, nil
}

/* ═══════════════════════════ Yardımcılar ═══════════════════════════ */

// resolvedTarget kapsamın çözülmüş hâli.
type resolvedTarget struct {
	scope     dompricing.Scope
	serviceID *int64
	countryID *int64
	productID *int64
	key       string // denetim kaydı için okunur kapsam anahtarı
}

// resolve kapsamı doğrular ve kimliklere çevirir.
//
// KAPSAM TUTARLILIĞI GO'DA DA DOĞRULANIR. Şemada `pricing_scope_consistent`
// CHECK'i var ve son savunma odur; ama ona bırakılırsa kullanıcı
// "pricing_rules_check" gibi bir kısıt adı görür (Değişmez #12). Burada
// hangi alanın neden gerektiği Türkçe söylenir.
// test: ../../transport/http/handler/admin_pricing_integration_test.go#TestScopeMismatchGivesReadableError
func (s *RuleService) resolve(ctx context.Context, q *db.Queries, t RuleTarget) (resolvedTarget, error) {
	scope, err := dompricing.ParseScope(strings.ToUpper(strings.TrimSpace(t.Scope)))
	if err != nil {
		return resolvedTarget{}, apperr.ErrValidation.WithMessage(
			"Geçersiz kapsam. Geçerli değerler: GLOBAL, COUNTRY, SERVICE, SERVICE_COUNTRY, PRODUCT.")
	}

	code := strings.TrimSpace(t.ServiceCode)
	iso := strings.ToUpper(strings.TrimSpace(t.CountryISO))

	needService := scope == dompricing.ScopeService ||
		scope == dompricing.ScopeServiceCountry || scope == dompricing.ScopeProduct
	needCountry := scope == dompricing.ScopeCountry ||
		scope == dompricing.ScopeServiceCountry || scope == dompricing.ScopeProduct

	if needService && code == "" {
		return resolvedTarget{}, apperr.ErrValidation.WithMessage(
			scope.String() + " kapsamı için servis kodu zorunludur.")
	}
	if !needService && code != "" {
		return resolvedTarget{}, apperr.ErrValidation.WithMessage(
			scope.String() + " kapsamında servis seçilemez.")
	}
	if needCountry && iso == "" {
		return resolvedTarget{}, apperr.ErrValidation.WithMessage(
			scope.String() + " kapsamı için ülke zorunludur.")
	}
	if !needCountry && iso != "" {
		return resolvedTarget{}, apperr.ErrValidation.WithMessage(
			scope.String() + " kapsamında ülke seçilemez.")
	}
	if scope != dompricing.ScopeProduct && t.DurationMinutes != 0 {
		return resolvedTarget{}, apperr.ErrValidation.WithMessage(
			"Süre yalnız PRODUCT kapsamında verilebilir.")
	}

	out := resolvedTarget{scope: scope, key: scope.String()}

	var svcID, ctryID int64
	if code != "" {
		svc, err := q.GetServiceByCode(ctx, code)
		if err != nil {
			return resolvedTarget{}, apperr.ErrNotFound.WithMessage("Servis bulunamadı: " + code)
		}
		svcID = svc.ID
		out.key += ":" + svc.Code
	}
	if iso != "" {
		ctry, err := q.GetCountryByISO(ctx, iso)
		if err != nil {
			return resolvedTarget{}, apperr.ErrNotFound.WithMessage("Ülke bulunamadı: " + iso)
		}
		ctryID = ctry.ID
		out.key += ":" + ctry.Iso2
	}

	switch scope {
	case dompricing.ScopeCountry:
		out.countryID = &ctryID
	case dompricing.ScopeService:
		out.serviceID = &svcID
	case dompricing.ScopeServiceCountry:
		out.serviceID = &svcID
		out.countryID = &ctryID
	case dompricing.ScopeProduct:
		// PRODUCT kapsamında ÜRÜN dışındaki alanlar NULL kalmalıdır
		// (pricing_scope_consistent yalnız product_id NOT NULL ister).
		prod, err := findProduct(ctx, q, svcID, ctryID, t.DurationMinutes)
		if err != nil {
			return resolvedTarget{}, err
		}
		id := prod.ID
		out.productID = &id
		out.key += fmt.Sprintf(":%d", t.DurationMinutes)
	}
	return out, nil
}

// findProduct servis × ülke (× süre) kombinasyonunun ürününü bulur.
func findProduct(ctx context.Context, q *db.Queries, serviceID, countryID int64, duration int32) (db.Product, error) {
	if duration > 0 {
		d := duration
		p, err := q.GetProductForRental(ctx, db.GetProductForRentalParams{
			ServiceID: &serviceID, CountryID: &countryID, DurationMinutes: &d,
		})
		if err != nil {
			return db.Product{}, apperr.ErrNotFound.WithMessage(
				"Bu servis, ülke ve süre için kiralık ürün tanımlı değil.")
		}
		return p, nil
	}
	p, err := q.GetProductForActivation(ctx, db.GetProductForActivationParams{
		ServiceID: &serviceID, CountryID: &countryID, VerificationType: db.VerificationTypeSms,
	})
	if err != nil {
		return db.Product{}, apperr.ErrNotFound.WithMessage(
			"Bu servis ve ülke için ürün tanımlı değil.")
	}
	return p, nil
}

// marginPattern marj metninin izin verilen biçimi: en çok 4 tam, 2 ondalık hane.
//
// NUMERIC(7,2) ile UYUMLU olmalı. Serbest bir metni doğrudan Scan'e vermek
// "1e5" veya "1/3" gibi değerleri kabul eder; ilki taşma, ikincisi sessiz
// yuvarlama üretir.
var marginPattern = regexp.MustCompile(`^\d{1,4}(\.\d{1,2})?$`)

// parseMarginPercent marj metnini doğrular ve NUMERIC'e çevirir.
func parseMarginPercent(s string) (pgtype.Numeric, string, error) {
	s = strings.TrimSpace(s)
	// Türkçe klavyeden ondalık ayırıcı virgül gelir; reddetmek yerine
	// çeviririz — yönetici "40,5" yazdığında hata görmesi anlamsız.
	s = strings.ReplaceAll(s, ",", ".")
	if !marginPattern.MatchString(s) {
		return pgtype.Numeric{}, "", apperr.ErrValidation.WithMessage(
			"Marj yüzdesi sayı olmalıdır (örn. 40 veya 40.50).")
	}
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		return pgtype.Numeric{}, "", apperr.ErrValidation.WithMessage(
			"Marj yüzdesi okunamadı.")
	}
	if v.Cmp(new(big.Rat).SetInt64(1000)) > 0 {
		return pgtype.Numeric{}, "", apperr.ErrValidation.WithMessage(
			"Marj yüzdesi en fazla 1000 olabilir.")
	}
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, "", apperr.ErrValidation.WithMessage(
			"Marj yüzdesi okunamadı.")
	}
	// domain/pricing marjı big.Rat ile ayrıştırır; burada da geçtiğini
	// doğrularız, yoksa hata satın alma anında ortaya çıkardı.
	if _, err := money.MarginRate(s); err != nil {
		return pgtype.Numeric{}, "", apperr.ErrValidation.WithMessage(
			"Marj yüzdesi okunamadı.")
	}
	return n, s, nil
}

func checkAmount(label string, minor int64) error {
	if minor < 0 {
		return apperr.ErrValidation.WithMessage(
			strings.ToUpper(label[:1]) + label[1:] + " negatif olamaz.")
	}
	if minor > maxRuleAmountMinor {
		return apperr.ErrValidation.WithMessage(
			strings.ToUpper(label[:1]) + label[1:] + " çok yüksek (en fazla 1.000.000,00 ₺).")
	}
	return nil
}

// ruleSnapshot denetim kaydına yazılacak alan değişimi.
//
// 🔴 KİŞİSEL VERİ YOKTUR: yalnız kuralın sayısal/kategorik alanları.
func ruleSnapshot(r db.PricingRule) map[string]any {
	return map[string]any{
		"scope":         string(r.Scope),
		"marginPercent": numericText(r.MarginPercent),
		"fixedFeeMinor": r.FixedFeeMinor,
		"minPriceMinor": r.MinPriceMinor,
		"isActive":      r.IsActive,
	}
}

// scopeKeyFromRule kaydedilmiş bir kuralın okunur kapsam anahtarını üretir.
//
// Sayısal kimlikler denetim kaydına da girmez (Değişmez #10); kod/ISO
// çözülemezse kapsam adıyla yetinilir.
func scopeKeyFromRule(ctx context.Context, q *db.Queries, ruleID int64, scope db.PricingScope) string {
	key := string(scope)
	lbl, err := q.GetPricingRuleLabels(ctx, ruleID)
	if err != nil {
		return key
	}
	if lbl.ServiceCode != nil {
		key += ":" + *lbl.ServiceCode
	}
	if lbl.CountryIso2 != nil {
		key += ":" + *lbl.CountryIso2
	}
	if scope == db.PricingScopePRODUCT {
		d := int32(0)
		if lbl.ProductDurationMinutes != nil {
			d = *lbl.ProductDurationMinutes
		}
		key += fmt.Sprintf(":%d", d)
	}
	return key
}

// numericText pgtype.Numeric'i metne çevirir; float64'e UĞRAMAZ.
func numericText(n pgtype.Numeric) string {
	v, err := n.Value()
	if err != nil {
		return "0"
	}
	s, ok := v.(string)
	if !ok {
		return "0"
	}
	return s
}

// isUniqueViolation Postgres 23505 hatasını tanır.
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
