package main

import "testing"

// TestSaglayiciSatiriSenkronDustugundeKirmiziOlur, "satışa hazır mı" raporundaki
// sağlayıcı satırının anahtarın VARLIĞINA değil ÇALIŞTIĞINA baktığını gösterir.
//
// Bu testin sebebi gerçek bir kurulumdur (11 Eylül 2026): katalog senkronu
// BAD_API_KEY ile düştü, rapor iki satır aşağıda "✓ etkin sağlayıcı + API
// anahtarı" bastı ve sonunda "Sistem satışa hazır." dedi. Sistem hazır
// değildi — sağlayıcı bağlantısı kopuktu ve stok, önceki kurulumdan kalan
// bayat veriydi.
func TestSaglayiciSatiriSenkronDustugundeKirmiziOlur(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ad           string
		anahtarVar   bool
		senkronDusen int
		istenen      bool
		neden        string
	}{
		{
			ad:         "anahtar var, senkron başarılı",
			anahtarVar: true, senkronDusen: 0, istenen: true,
			neden: "tek sağlıklı durum budur",
		},
		{
			ad:         "anahtar var ama sağlayıcı REDDETTİ",
			anahtarVar: true, senkronDusen: 1, istenen: false,
			neden: "üretimde tam bu durumda ✓ basıyordu — testin varlık sebebi",
		},
		{
			ad:         "anahtar hiç yok",
			anahtarVar: false, senkronDusen: 0, istenen: false,
			neden: "anahtarsız satış yapılamaz",
		},
		{
			ad:         "ne anahtar ne senkron",
			anahtarVar: false, senkronDusen: 2, istenen: false,
			neden: "iki koşul da sağlanmıyor",
		},
	}

	for _, c := range cases {
		t.Run(c.ad, func(t *testing.T) {
			t.Parallel()
			if got := saglayiciHazirMi(c.anahtarVar, c.senkronDusen); got != c.istenen {
				t.Errorf("saglayiciHazirMi(%v, %d) = %v, istenen %v\n  neden: %s",
					c.anahtarVar, c.senkronDusen, got, c.istenen, c.neden)
			}
		})
	}
}
