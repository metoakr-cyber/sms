// Package pricing satış fiyatını hesaplar. Saf ve yan etkisizdir: G/Ç yapmaz,
// veritabanı ve saat bilmez. Tüm girdiler açıkça verilir.
//
// Bkz. docs/design.md §9, docs/trd.md FR-303, FR-304.
package pricing

import (
	"fmt"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

// Scope bir fiyat kuralının kapsamı. Değerler ÖNCELİK SIRASINDADIR:
// büyük olan daha spesifiktir ve kazanır.
type Scope int

const (
	ScopeGlobal Scope = iota + 1
	ScopeCountry
	ScopeService
	ScopeServiceCountry
	ScopeProduct
)

func (s Scope) String() string {
	switch s {
	case ScopeGlobal:
		return "GLOBAL"
	case ScopeCountry:
		return "COUNTRY"
	case ScopeService:
		return "SERVICE"
	case ScopeServiceCountry:
		return "SERVICE_COUNTRY"
	case ScopeProduct:
		return "PRODUCT"
	default:
		return "UNKNOWN"
	}
}

// ParseScope metin kapsamı çözer.
func ParseScope(s string) (Scope, error) {
	switch s {
	case "GLOBAL":
		return ScopeGlobal, nil
	case "COUNTRY":
		return ScopeCountry, nil
	case "SERVICE":
		return ScopeService, nil
	case "SERVICE_COUNTRY":
		return ScopeServiceCountry, nil
	case "PRODUCT":
		return ScopeProduct, nil
	default:
		return 0, fmt.Errorf("pricing: bilinmeyen kapsam %q", s)
	}
}

// Rule bir fiyatlandırma kuralı.
type Rule struct {
	ID            int64
	Scope         Scope
	MarginPercent string // ondalık metin; big.Rat ile tam ayrıştırılır
	FixedFee      money.Money
	MinPrice      money.Money
}

// SelectRule uygulanabilir kurallardan EN SPESİFİK olanı seçer.
//
// Kapsam önceliği burada, TEK BİR YERDE tanımlıdır. Dağıtılmış bir öncelik
// mantığı (her sorguya serpiştirilmiş ORDER BY) sessizce tutarsızlaşır.
func SelectRule(rules []Rule) (Rule, error) {
	if len(rules) == 0 {
		// "Kural yoksa maliyetine sat" KABUL EDİLEMEZ: eski prototip tam olarak
		// bunu yapıyordu ve platform hiç kâr etmiyordu (docs/memory.md §3.5).
		return Rule{}, ErrNoRule
	}
	best := rules[0]
	for _, r := range rules[1:] {
		if r.Scope > best.Scope {
			best = r
		}
	}
	return best, nil
}

// Input fiyat hesabının girdileri.
type Input struct {
	// Cost sağlayıcının maliyeti (mikro-USD).
	Cost money.Money
	// ProviderMultiplier sağlayıcıya özel maliyet düzeltmesi. Kâr marjı DEĞİL.
	ProviderMultiplier money.Rate
	// FXRate USD → TRY kuru.
	FXRate money.Rate
	// FXSafetyMargin kur tamponu (örn. "1.02" = %2). Kur riski bizde olduğu
	// için maliyet tarafına küçük bir emniyet payı eklenir.
	FXSafetyMargin money.Rate
	// Rule uygulanacak fiyat kuralı.
	Rule Rule
}

// Result hesabın çıktısı ve ara değerleri.
//
// Ara değerler DÖNDÜRÜLÜR çünkü kâr raporu, denetim ve destek için
// "bu fiyat nasıl oluştu" sorusunun cevaplanabilir olması gerekir.
type Result struct {
	SellPrice     money.Money // TRY kuruş — SÖZLEŞME
	CostInTRY     money.Money // maliyetin TRY karşılığı (rapor için)
	AppliedRule   Rule
	MarginApplied string
	HitMinimum    bool // taban fiyata takıldı mı
}

// Calculate satış fiyatını hesaplar.
//
//	sell = tavana_yuvarla( maliyet × çarpan × kur × tampon × (1 + marj/100) + sabit_bedel )
//	sell = max(sell, taban_fiyat)
//
// TÜM ara hesaplar big.Rat ile TAM yapılır; yuvarlama YALNIZ SONDA bir kez
// ve YUKARI yönde uygulanır. Ara yuvarlama yapılsaydı her adımda kuruş
// kaybeder ve altın test tutmazdı.
//
// Yuvarlama her zaman platform lehinedir (ürün ilkesi §5).
func Calculate(in Input) (Result, error) {
	if in.Cost.Currency() != money.USD {
		return Result{}, fmt.Errorf("%w: maliyet %s cinsinden", ErrInvalidInput, in.Cost.Currency())
	}
	if !in.Cost.IsPositive() {
		return Result{}, fmt.Errorf("%w: maliyet sıfır veya negatif", ErrInvalidInput)
	}
	if in.FXRate.IsZero() || in.FXRate.IsNegative() {
		return Result{}, fmt.Errorf("%w: kur geçersiz", ErrInvalidInput)
	}
	if in.Rule.Scope == 0 {
		return Result{}, ErrNoRule
	}

	multiplier := in.ProviderMultiplier
	if multiplier.IsZero() {
		multiplier = money.RateOne()
	}
	safety := in.FXSafetyMargin
	if safety.IsZero() {
		safety = money.RateOne()
	}

	margin, err := money.MarginRate(in.Rule.MarginPercent)
	if err != nil {
		return Result{}, fmt.Errorf("%w: marj %q", ErrInvalidInput, in.Rule.MarginPercent)
	}

	// Maliyetin TRY karşılığı — rapor için, ara yuvarlama YAPILMADAN önce
	// ayrıca hesaplanır (bu değer sadece gösterim amaçlıdır).
	costTRY, err := in.Cost.Convert(money.TRY, in.FXRate.Mul(multiplier).Mul(safety), money.RoundHalfEven)
	if err != nil {
		return Result{}, err
	}

	// TEK ZİNCİR, TEK YUVARLAMA: maliyet × çarpan × kur × tampon × marj
	chain := in.FXRate.Mul(multiplier).Mul(safety).Mul(margin)
	sell, err := in.Cost.Convert(money.TRY, chain, money.RoundUp)
	if err != nil {
		return Result{}, err
	}

	if !in.Rule.FixedFee.IsZero() {
		sell, err = sell.Add(in.Rule.FixedFee)
		if err != nil {
			return Result{}, err
		}
	}

	hitMin := false
	if !in.Rule.MinPrice.IsZero() {
		if c, err := sell.Cmp(in.Rule.MinPrice); err == nil && c < 0 {
			sell = in.Rule.MinPrice
			hitMin = true
		}
	}

	return Result{
		SellPrice:     sell,
		CostInTRY:     costTRY,
		AppliedRule:   in.Rule,
		MarginApplied: in.Rule.MarginPercent,
		HitMinimum:    hitMin,
	}, nil
}
