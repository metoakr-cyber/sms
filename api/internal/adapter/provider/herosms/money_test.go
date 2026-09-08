package herosms

import (
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

// TestUsdFromFloatIsExact sağlayıcının float fiyatının mikro-dolara TAM
// çevrildiğini gösterir.
//
// Sağlayıcı JSON'da float gönderiyor; seçme şansımız yok. Çeviri tek bir yerde
// yapılır ve hemen tam sayıya geçilir — float aritmetiği sistemin geri kalanına
// sızmaz. Bu test o sınırın kanıtıdır.
func TestUsdFromFloatIsExact(t *testing.T) {
	cases := []struct {
		in   float64
		want int64 // mikro-dolar
	}{
		{0, 0},
		{0.0067, 6_700}, // spec'teki MaxPrice.minimum
		{0.045, 45_000}, // legacy TopCountriesOneService'te görülen fiyat
		{0.05, 50_000},
		{0.9600, 960_000}, // 4 ondalıklı — canlı offers yanıtından
		{1.0800, 1_080_000},
		{1.3625, 1_362_500},
		{1.4694, 1_469_400},
		{1.5, 1_500_000}, // wa × DE, canlı
		{2.6283, 2_628_300},
		{37.5, 37_500_000},
	}
	for _, c := range cases {
		got, err := usdFromFloat(c.in)
		if err != nil {
			t.Fatalf("usdFromFloat(%v): %v", c.in, err)
		}
		if got.Minor() != c.want {
			t.Errorf("usdFromFloat(%v) = %d mikro, beklenen %d", c.in, got.Minor(), c.want)
		}
		if got.Currency() != money.USD {
			t.Errorf("usdFromFloat(%v) para birimi = %v, beklenen USD", c.in, got.Currency())
		}
	}
}

// TestUsdFromFloatDoesNotDriftDown float artıklarının BİR MİKRO-DOLAR aşağı
// kaymasını engellediğimizi gösterir.
//
// 1.0800 IEEE-754'te 1.0799999999999998... olarak temsil edilir. Doğrudan
// int64(v*1e6) yazsaydık 1_079_999 çıkardı: her fiyatta bir mikro-dolar
// eksik hesaplar, binlerce siparişte sistematik zarar ederdik.
func TestUsdFromFloatDoesNotDriftDown(t *testing.T) {
	// Doğrudan çarpımın gerçekten kaydığı değerler.
	risky := []struct {
		in   float64
		want int64
	}{
		{1.08, 1_080_000},
		{2.05, 2_050_000},
		{8.03, 8_030_000},
		{0.29, 290_000},
	}
	for _, c := range risky {
		naive := int64(c.in * 1_000_000) // yuvarlamasız — hatalı yöntem
		got, err := usdFromFloat(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if got.Minor() != c.want {
			t.Errorf("usdFromFloat(%v) = %d, beklenen %d (yuvarlamasız: %d)",
				c.in, got.Minor(), c.want, naive)
		}
	}
}

// TestUsdFromFloatRejectsNegative negatif fiyat kabul edilmez: bir sağlayıcı
// hatası bize "para kazandıran" bir maliyet olarak geçmemelidir.
func TestUsdFromFloatRejectsNegative(t *testing.T) {
	if _, err := usdFromFloat(-0.5); err == nil {
		t.Fatal("negatif fiyat kabul edildi")
	}
}
