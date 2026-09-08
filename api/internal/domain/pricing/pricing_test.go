package pricing_test

import (
	"errors"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/domain/pricing"
)

func rate(t *testing.T, s string) money.Rate {
	t.Helper()
	r, err := money.RateFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func globalRule(margin string) pricing.Rule {
	return pricing.Rule{ID: 1, Scope: pricing.ScopeGlobal, MarginPercent: margin,
		FixedFee: money.Zero(money.TRY), MinPrice: money.Zero(money.TRY)}
}

// docs/trd.md KK-304 — ALTIN TEST.
// Bu değer değişirse fiyatlandırma bozulmuş demektir.
func TestKK304_GoldenPrice(t *testing.T) {
	res, err := pricing.Calculate(pricing.Input{
		Cost:               money.New(350_000, money.USD), // 0,35 USD (mikro)
		ProviderMultiplier: rate(t, "1.0"),
		FXRate:             rate(t, "43.20"),
		FXSafetyMargin:     money.RateOne(), // tampon yok
		Rule:               globalRule("40"),
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = 2117 // 21,17 ₺
	if res.SellPrice.Minor() != want {
		t.Fatalf("ALTIN TEST BAŞARISIZ: %d kuruş, beklenen %d (%s)",
			res.SellPrice.Minor(), want, res.SellPrice)
	}
	t.Logf("0,35 USD × 43,20 × 1,40 = %s ✓", res.SellPrice)
}

// Kapsam önceliği: en SPESİFİK kural kazanır.
func TestScopePriority(t *testing.T) {
	all := []pricing.Rule{
		{ID: 1, Scope: pricing.ScopeGlobal, MarginPercent: "10"},
		{ID: 2, Scope: pricing.ScopeCountry, MarginPercent: "20"},
		{ID: 3, Scope: pricing.ScopeService, MarginPercent: "30"},
		{ID: 4, Scope: pricing.ScopeServiceCountry, MarginPercent: "40"},
		{ID: 5, Scope: pricing.ScopeProduct, MarginPercent: "50"},
	}
	// Her alt küme için en spesifik olan seçilmeli.
	tests := []struct {
		name  string
		rules []pricing.Rule
		want  int64
	}{
		{"yalnız global", all[:1], 1},
		{"global + ülke", all[:2], 2},
		{"global + ülke + servis", all[:3], 3},
		{"+ servis-ülke", all[:4], 4},
		{"hepsi", all, 5},
		{"ters sıra", []pricing.Rule{all[4], all[0], all[2]}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pricing.SelectRule(tt.rules)
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tt.want {
				t.Fatalf("seçilen kural = %d (%s), beklenen %d", got.ID, got.Scope, tt.want)
			}
		})
	}
}

// Kural yoksa SATIŞ YAPILMAZ. Sessiz varsayılan yok.
func TestNoRuleIsAnError(t *testing.T) {
	if _, err := pricing.SelectRule(nil); !errors.Is(err, pricing.ErrNoRule) {
		t.Fatalf("hata = %v, ErrNoRule bekleniyordu", err)
	}
	_, err := pricing.Calculate(pricing.Input{
		Cost: money.New(350_000, money.USD), FXRate: rate(t, "43.20"),
	})
	if !errors.Is(err, pricing.ErrNoRule) {
		t.Fatalf("kuralsız hesap = %v, ErrNoRule bekleniyordu", err)
	}
}

func TestFixedFeeAndMinimum(t *testing.T) {
	base := pricing.Input{
		Cost: money.New(10_000, money.USD), // 0,01 USD — çok ucuz
		ProviderMultiplier: money.RateOne(),
		FXRate:             rate(t, "43.20"),
		FXSafetyMargin:     money.RateOne(),
	}

	// Sabit bedelsiz: 0,01 × 43,20 × 1,40 = 0,60480 → 0,61 ₺
	r := base
	r.Rule = globalRule("40")
	got, err := pricing.Calculate(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.SellPrice.Minor() != 61 {
		t.Fatalf("sabit bedelsiz = %d kuruş, beklenen 61", got.SellPrice.Minor())
	}

	// Sabit bedelli: + 2,00 ₺ = 2,61 ₺
	r.Rule = pricing.Rule{ID: 1, Scope: pricing.ScopeGlobal, MarginPercent: "40",
		FixedFee: money.New(200, money.TRY), MinPrice: money.Zero(money.TRY)}
	got, err = pricing.Calculate(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.SellPrice.Minor() != 261 {
		t.Fatalf("sabit bedelli = %d kuruş, beklenen 261", got.SellPrice.Minor())
	}
	if got.HitMinimum {
		t.Error("taban fiyata takılmadığı halde HitMinimum true")
	}

	// Taban fiyat: 5,00 ₺ — hesap 0,61 ₺ olsa da taban uygulanır.
	r.Rule = pricing.Rule{ID: 1, Scope: pricing.ScopeGlobal, MarginPercent: "40",
		FixedFee: money.Zero(money.TRY), MinPrice: money.New(500, money.TRY)}
	got, err = pricing.Calculate(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.SellPrice.Minor() != 500 {
		t.Fatalf("taban fiyatlı = %d kuruş, beklenen 500", got.SellPrice.Minor())
	}
	if !got.HitMinimum {
		t.Error("taban uygulandığı halde HitMinimum false — rapor yanıltıcı olur")
	}
}

// Sağlayıcı çarpanı ve kur tamponu maliyet tarafına uygulanır.
func TestMultiplierAndSafetyMargin(t *testing.T) {
	base := pricing.Input{
		Cost: money.New(350_000, money.USD), FXRate: rate(t, "43.20"),
		Rule: globalRule("40"),
	}
	plain := base
	plain.ProviderMultiplier = money.RateOne()
	plain.FXSafetyMargin = money.RateOne()
	a, _ := pricing.Calculate(plain)

	withMult := base
	withMult.ProviderMultiplier = rate(t, "1.10") // sağlayıcı %10 komisyon
	withMult.FXSafetyMargin = money.RateOne()
	b, _ := pricing.Calculate(withMult)
	if b.SellPrice.Minor() <= a.SellPrice.Minor() {
		t.Fatalf("çarpan fiyatı artırmadı: %d vs %d", b.SellPrice.Minor(), a.SellPrice.Minor())
	}

	withSafety := base
	withSafety.ProviderMultiplier = money.RateOne()
	withSafety.FXSafetyMargin = rate(t, "1.02") // %2 kur tamponu
	c, _ := pricing.Calculate(withSafety)
	if c.SellPrice.Minor() <= a.SellPrice.Minor() {
		t.Fatalf("kur tamponu fiyatı artırmadı: %d vs %d", c.SellPrice.Minor(), a.SellPrice.Minor())
	}
}

// Ara yuvarlama YAPILMAMALI. Yapılsaydı zincirin her adımında kuruş kaybedilirdi.
func TestNoIntermediateRounding(t *testing.T) {
	// Kasten "kirli" değerler: her ara adım ondalık artık üretir.
	in := pricing.Input{
		Cost:               money.New(333_333, money.USD),
		ProviderMultiplier: rate(t, "1.037"),
		FXRate:             rate(t, "43.217891"),
		FXSafetyMargin:     rate(t, "1.023"),
		Rule:               globalRule("37.5"),
	}
	got, err := pricing.Calculate(in)
	if err != nil {
		t.Fatal(err)
	}

	// Beklenen: 0.333333 × 1.037 × 43.217891 × 1.023 × 1.375 = 21.0155... ₺
	// Ara yuvarlama olsaydı sonuç birkaç kuruş SAPARDI.
	const want = 2102 // tavana yuvarlanmış
	if got.SellPrice.Minor() != want {
		t.Fatalf("= %d kuruş, beklenen %d — ara yuvarlama yapılıyor olabilir",
			got.SellPrice.Minor(), want)
	}
}

func TestDeterminism(t *testing.T) {
	in := pricing.Input{
		Cost: money.New(350_000, money.USD), ProviderMultiplier: rate(t, "1.037"),
		FXRate: rate(t, "43.204512"), FXSafetyMargin: rate(t, "1.02"),
		Rule: globalRule("40"),
	}
	first, err := pricing.Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		got, err := pricing.Calculate(in)
		if err != nil {
			t.Fatal(err)
		}
		if got.SellPrice.Minor() != first.SellPrice.Minor() {
			t.Fatalf("%d. turda farklı sonuç: %d vs %d", i, got.SellPrice.Minor(), first.SellPrice.Minor())
		}
	}
}

func TestInvalidInputRejected(t *testing.T) {
	valid := pricing.Input{
		Cost: money.New(350_000, money.USD), ProviderMultiplier: money.RateOne(),
		FXRate: rate(t, "43.20"), FXSafetyMargin: money.RateOne(), Rule: globalRule("40"),
	}
	tests := map[string]func(*pricing.Input){
		"TRY maliyet":    func(i *pricing.Input) { i.Cost = money.New(100, money.TRY) },
		"sıfır maliyet":  func(i *pricing.Input) { i.Cost = money.Zero(money.USD) },
		"negatif maliyet": func(i *pricing.Input) { i.Cost = money.New(-100, money.USD) },
		"sıfır kur":      func(i *pricing.Input) { i.FXRate = money.Rate{} },
		"bozuk marj":     func(i *pricing.Input) { i.Rule.MarginPercent = "abc" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			in := valid
			mutate(&in)
			if _, err := pricing.Calculate(in); err == nil {
				t.Fatal("geçersiz girdi kabul edildi")
			}
		})
	}
}

func TestScopeParsing(t *testing.T) {
	for _, s := range []pricing.Scope{
		pricing.ScopeGlobal, pricing.ScopeCountry, pricing.ScopeService,
		pricing.ScopeServiceCountry, pricing.ScopeProduct,
	} {
		got, err := pricing.ParseScope(s.String())
		if err != nil || got != s {
			t.Errorf("ParseScope(%q) = %v, %v", s.String(), got, err)
		}
	}
	if _, err := pricing.ParseScope("SACMA"); err == nil {
		t.Error("bilinmeyen kapsam kabul edildi")
	}
}
