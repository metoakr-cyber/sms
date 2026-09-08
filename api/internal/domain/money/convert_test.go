package money_test

import (
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

// TestGoldenPriceChain — docs/trd.md KK-304'ün birebir ispatı.
//
//	cost = 0.35 USD · multiplier = 1.0 · fx = 43.20 · margin = %40
//	fixedFee = 0 · minPrice = 0   →   TAM OLARAK 2117 kuruş (21,17 ₺)
//
// Ara hesap: 0.35 × 43.20 = 15.12 TRY;  15.12 × 1.40 = 21.168 TRY
// Yukarı yuvarlama (platform lehine): 21.17 ₺ = 2117 kuruş.
func TestGoldenPriceChain(t *testing.T) {
	// Maliyet mikro-dolar cinsinden saklanır (ADR-026): 0.35 USD = 350000 mikro.
	cost := money.New(350_000, money.USD)

	multiplier := mustRate(t, "1.0")
	fx := mustRate(t, "43.20")
	margin, err := money.MarginRate("40")
	if err != nil {
		t.Fatal(err)
	}

	// 1) sağlayıcıya özel maliyet düzeltmesi (USD alanında kalır, yuvarlama yok)
	adjusted, err := cost.MulRate(multiplier, money.RoundUp)
	if err != nil {
		t.Fatal(err)
	}
	if adjusted.Minor() != 350_000 {
		t.Fatalf("düzeltilmiş maliyet = %d mikro-USD, beklenen 350000", adjusted.Minor())
	}

	// 2) TRY'ye çevir  +  3) marjı uygula — TEK zincirde, ara yuvarlama YOK.
	//    Ara yuvarlama yapılsaydı 15.12 × 1.40 yerine round(15.12) × 1.40 hesaplanırdı.
	sell, err := adjusted.Convert(money.TRY, fx.Mul(margin), money.RoundUp)
	if err != nil {
		t.Fatal(err)
	}

	const want = 2117
	if sell.Minor() != want {
		t.Fatalf("ALTIN TEST BAŞARISIZ: satış fiyatı = %d kuruş, beklenen %d (%s)",
			sell.Minor(), want, sell)
	}
	if sell.Currency() != money.TRY {
		t.Fatalf("satış para birimi = %v, beklenen TRY", sell.Currency())
	}
	t.Logf("0,35 USD × 43,20 × 1,40 = %s ✓", sell)
}

// Yuvarlamanın HER ZAMAN platform lehine olduğunu ispatlar (ürün ilkesi §5).
func TestRoundingAlwaysFavoursPlatform(t *testing.T) {
	fx := mustRate(t, "43.20")
	// Kuruşun altında kalan her artık yukarı yuvarlanmalı.
	for _, micro := range []int64{1, 999, 100_001, 350_001} {
		cost := money.New(micro, money.USD)
		up, err := cost.Convert(money.TRY, fx, money.RoundUp)
		if err != nil {
			t.Fatal(err)
		}
		down, err := cost.Convert(money.TRY, fx, money.RoundDown)
		if err != nil {
			t.Fatal(err)
		}
		if up.Minor() < down.Minor() {
			t.Fatalf("micro=%d: RoundUp(%d) < RoundDown(%d)", micro, up.Minor(), down.Minor())
		}
		if up.Minor()-down.Minor() > 1 {
			t.Fatalf("micro=%d: yuvarlama farkı 1 kuruştan büyük (%d)", micro, up.Minor()-down.Minor())
		}
	}
}

// Kur değişimi tahsil edilen tutarı etkilememelidir — teklif fiyatı sözleşmedir.
// Bu test hesaplamanın DETERMİNİSTİK olduğunu gösterir: aynı girdi → aynı çıktı.
func TestPriceCalculationIsDeterministic(t *testing.T) {
	cost := money.New(350_000, money.USD)
	fx := mustRate(t, "43.204512")
	margin, _ := money.MarginRate("37.5")

	var first int64
	for i := 0; i < 100; i++ {
		got, err := cost.Convert(money.TRY, fx.Mul(margin), money.RoundUp)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = got.Minor()
			continue
		}
		if got.Minor() != first {
			t.Fatalf("tur %d: %d, ilk tur %d — hesap deterministik değil", i, got.Minor(), first)
		}
	}
	t.Logf("100 turda sabit: %d kuruş", first)
}

func TestConvertRejectsInvalidTarget(t *testing.T) {
	m := money.New(1, money.USD)
	if _, err := m.Convert(money.Currency{}, money.RateOne(), money.RoundUp); err == nil {
		t.Fatal("geçersiz hedef para birimi reddedilmeliydi")
	}
}

// USD mikro ölçeğinin gerçekten gerekli olduğunu gösterir (ADR-026).
// HeroSMS MaxPrice.minimum = 0.0067 USD — sent (2 hane) ölçeğinde bu 0 veya 1 olurdu.
func TestMicroScalePreservesProviderPrecision(t *testing.T) {
	// 0.0067 USD = 6700 mikro-dolar
	tiny := money.New(6_700, money.USD)
	if tiny.String() != "0,006700 USD" {
		t.Fatalf("mikro ölçek kaybı: %s", tiny)
	}
	// Sent ölçeğinde saklansaydı: round(0.0067 × 100) = 1 sent = 0.01 USD → %49 sapma.
	fx := mustRate(t, "43.20")
	got, err := tiny.Convert(money.TRY, fx, money.RoundUp)
	if err != nil {
		t.Fatal(err)
	}
	// 0.0067 × 43.20 = 0.28944 TRY → yukarı → 0,29 ₺
	if got.Minor() != 29 {
		t.Fatalf("0,0067 USD → %d kuruş, beklenen 29", got.Minor())
	}
}
