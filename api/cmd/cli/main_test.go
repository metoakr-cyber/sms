package main

import "testing"

// TestTutarKurusa TL metninin kuruşa TAM çevrildiğini doğrular.
//
// 🔴 ASIL İDDİA KURUŞ KAYBI OLMAMASI. Ölçüldü:
//
//	ParseFloat("19.99") = 19.98999999999999843681 → int64(f*100) = 1998
//
// yani bir kuruş buharlaşır. "1000.10" ise float yolunda DOĞRU çıkar — hata
// her değerde görünmediği için göz kararıyla yakalanmaz; testin varlık sebebi
// budur.
func TestTutarKurusa(t *testing.T) {
	gecerli := map[string]int64{
		"1000":     100_000,
		"1000.10":  100_010,
		"0.01":     1,
		"1000,50":  100_050, // Türkçe virgül
		"-250.50":  -25_050, // eksi: bakiyeden düşer
		" 42 ":     4_200,   // kenar boşlukları
		"19.99":    1_999,   // float yolunda 1998 çıkardı
		"12345.67": 1_234_567,
	}
	for giris, bek := range gecerli {
		m, err := tutarKurusa(giris)
		if err != nil {
			t.Errorf("%q: beklenmedik hata: %v", giris, err)
			continue
		}
		if m.Minor() != bek {
			t.Errorf("%q → %d kuruş, %d bekleniyordu", giris, m.Minor(), bek)
		}
	}

	gecersiz := []string{
		"",                     // boş
		"abc",                  // sayı değil
		"0",                    // sıfır bakiye değiştirmez
		"0.00",                 // sıfırın yazımı da reddedilir
		"1.005",                // kuruştan küçük kesir — SESSİZCE YUVARLANMAZ
		"1.000,50",             // hem nokta hem virgül: belirsiz
		"99999999999999999999", // int64 taşması
	}
	for _, giris := range gecersiz {
		if m, err := tutarKurusa(giris); err == nil {
			t.Errorf("%q kabul edildi (%d kuruş), reddedilmeliydi", giris, m.Minor())
		}
	}
}
