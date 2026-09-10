package herosms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// TestStokVarsayilanFiyatSayacindanGelir stok ölçütünün `counts.defaultPrice`
// olduğunu ve diğer iki sayacın KULLANILMADIĞINI gösterir.
//
// Bu testin varlık sebebi bir üretim hatasıdır: ölçüt `counts.physical` iken
// Türkiye'nin 123 kombinasyonunun HİÇBİRİ satılabilir görünmüyordu ve katalogun
// ~%28'i sessizce kapalıydı. "Sessizce" kilit kelime — hiçbir hata log'lanmıyor,
// kullanıcı yalnız "numara bulunmuyor" görüyordu.
//
// Vakalar 10 Eylül 2026'da canlı `GET /activations/offers/sms` yanıtından
// alındı; uydurulmadı.
func TestStokVarsayilanFiyatSayacindanGelir(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ad                        string
		total, physical, varsayil int
		istenen                   int
		neden                     string
	}{
		{
			ad: "WhatsApp × Türkiye — physical sıfır ama numara VAR",
			// Canlı: prices.default 1.20 USD. Eski ölçüt burada 0 diyordu.
			total: 265_358, physical: 0, varsayil: 9_929,
			istenen: 9_929,
			neden:   "physical=0 satış engellememeli; 9.929 numara varsayılan fiyattan alınabilir",
		},
		{
			ad: "hu × 1 — physical pozitif ama hepsi tavanımızın ÜSTÜNDE",
			// Canlı: default 0,075 · merdivenin en ucuz basamağı 0,0751.
			// `maxPrice`'ımızla alınamazlar, dolayısıyla stok 0 DOĞRU cevaptır.
			total: 5, physical: 5, varsayil: 0,
			istenen: 0,
			neden:   "maxPrice=default ile alınamayan numara stok sayılmaz",
		},
		{
			ad:    "gerçekten boş",
			total: 0, physical: 0, varsayil: 0,
			istenen: 0,
			neden:   "üç sayaç da sıfırsa stok yoktur",
		},
		{
			ad: "sağlıklı teklif — üçü de pozitif",
			// docs/provider-herosms.md §3'teki örnek yanıt.
			total: 2_219, physical: 1_953, varsayil: 2_219,
			istenen: 2_219,
			neden:   "physical daha küçükken bile ölçüt defaultPrice'tır",
		},
		{
			ad: "total ASLA kullanılmaz",
			// Merdivenin tepesi 37,50 USD'ye kadar çıkıyor; `total` o havuzun
			// tamamı. Eski prototip bunu kullanıyordu ve para çekip boş dönüyordu.
			total: 1_382_789, physical: 0, varsayil: 0,
			istenen: 0,
			neden:   "1,38 milyonluk total stok değildir — fiyat tavanı yok",
		},
	}

	for _, c := range cases {
		t.Run(c.ad, func(t *testing.T) {
			t.Parallel()
			var o offerEntry
			o.Counts.Total = c.total
			o.Counts.Physical = c.physical
			o.Counts.DefaultPrice = c.varsayil

			if got := o.stok(); got != c.istenen {
				t.Errorf("stok() = %d, istenen %d\n  neden: %s\n  sayaçlar: total=%d physical=%d defaultPrice=%d",
					got, c.istenen, c.neden, c.total, c.physical, c.varsayil)
			}
		})
	}
}

// TestListOffersStoguYanittanOkur ölçütün JSON ayrıştırmasından satın alma
// katmanına kadar TAŞINDIĞINI gösterir.
//
// `stok()`'u tek başına sınamak yetmez: alan adı `defaultPrice` yanlış
// etiketlenirse birim testi yine geçer (sıfır değer okunur) ama üretimde her
// teklif stoksuz görünür. Bu test gerçek yanıt gövdesinden geçer.
func TestListOffersStoguYanittanOkur(t *testing.T) {
	t.Parallel()

	// Canlı yanıtın birebir şekli — WhatsApp × Türkiye.
	const govde = `{"data":{"wa":{"62":{
		"prices":{"default":1.2,"retail":1.44,"min":1.08},
		"counts":{"total":265358,"physical":0,"defaultPrice":9929}
	}}}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(govde))
	}))
	defer srv.Close()

	p := New()
	c := port.Creds{APIKey: "sahte-api-anahtari-yalnizca-test", BaseURL: srv.URL}

	offers, err := p.ListOffers(context.Background(), c, port.VerifySMS)
	if err != nil {
		t.Fatalf("ListOffers hata verdi: %v", err)
	}
	if len(offers) != 1 {
		t.Fatalf("teklif sayısı = %d, istenen 1", len(offers))
	}
	o := offers[0]
	if o.ServiceCode != "wa" || o.CountryCode != "62" {
		t.Errorf("eşleşme yanlış: %s × %s", o.ServiceCode, o.CountryCode)
	}
	if o.Stock != 9929 {
		t.Errorf("🔴 Stock = %d, istenen 9929 — `defaultPrice` alanı okunamıyor olabilir", o.Stock)
	}
	if o.Cost.Minor() != 1_200_000 {
		t.Errorf("Cost = %d mikro-dolar, istenen 1200000", o.Cost.Minor())
	}
}

// TestOfferEntryAlanEtiketleri sayaç etiketlerinin sağlayıcının gönderdiği
// adlarla eşleştiğini gösterir.
//
// Bir etiket hatası sessizdir: `encoding/json` bilinmeyen alanı atlar, sayaç
// sıfır kalır ve katalog boş görünür. Hata mesajı çıkmaz.
func TestOfferEntryAlanEtiketleri(t *testing.T) {
	t.Parallel()

	var o offerEntry
	if err := json.Unmarshal([]byte(
		`{"prices":{"default":0.5},"counts":{"total":7,"physical":3,"defaultPrice":5}}`), &o); err != nil {
		t.Fatalf("ayrıştırma hatası: %v", err)
	}
	if o.Counts.DefaultPrice != 5 {
		t.Errorf("🔴 defaultPrice = %d, istenen 5 — JSON etiketi yanlış", o.Counts.DefaultPrice)
	}
	if o.Counts.Physical != 3 || o.Counts.Total != 7 {
		t.Errorf("diğer sayaçlar da okunmalı: total=%d physical=%d", o.Counts.Total, o.Counts.Physical)
	}
	if o.Prices.Default != 0.5 {
		t.Errorf("prices.default = %v, istenen 0.5", o.Prices.Default)
	}
}
