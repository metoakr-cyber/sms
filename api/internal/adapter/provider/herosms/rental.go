package herosms

// Kiralık numara desteği.
//
// AKIŞ HİBRİTTİR (docs/provider-herosms.md §8):
//   · Satın alma  → MODERN uç, `POST /activations` + `duration` (subtype: 2)
//   · Fiyat/stok  → LEGACY uç, `?action=serviceCountRent` — modern `offers`
//                   şemasında `duration` boyutu YOK
//   · Uzatma      → `POST /activations/{id}/prolong`
//
// Bu ayrım bir tercih değil, sağlayıcının durumu: aynı ürünün iki yüzeyi var
// ve fiyatı yalnız eski yüzeyde duruyor.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

var _ port.RentalProvider = (*Provider)(nil)

// fallbackDurations sağlayıcı süre listesini vermezse kullanılır.
//
// SABİT LİSTE SON ÇAREDİR. Değerler BAD_DURATION yanıtından canlı olarak
// okundu (2026-09-08): sağlayıcı reddederken kabul ettiği listeyi söylüyor.
// Spec'teki RentDuration enum'u ile de örtüşüyor — ama spec üç yerde
// kendiyle çelişiyor, bu yüzden önce her zaman canlıya sorulur.
//
// test: rental_test.go#TestFallbackDurationsMatchLiveList
var fallbackDurations = []int{24, 72, 168, 336, 720, 1440, 2160, 4320}

// AllowedDurations desteklenen kiralama sürelerini (saat) döner.
//
// SAĞLAYICIYA BİLEREK GEÇERSİZ BİR SÜRE GÖNDERİLİR: `BAD_DURATION` yanıtı
// `info.available_durations` alanında kabul edilen listeyi taşır. Bu, listeyi
// öğrenmenin tek yolu — sağlayıcı "desteklenen süreler" diye bir uç sunmuyor.
//
// Tuhaf ama dürüst: alternatif, koda sabit liste yazıp sağlayıcı değiştirdiğinde
// sessizce yanlış olmaktı.
//
// test: rental_test.go#TestAllowedDurationsParsesFromError
func (p *Provider) AllowedDurations(ctx context.Context, c port.Creds) ([]int, error) {
	body, err := p.legacyGetRaw(ctx, c, "getRentServicesAndCountries", url.Values{
		// 1 saat kabul edilen bir değer değil — kasıtlı.
		"duration": {"1"},
	})
	if err != nil {
		return append([]int(nil), fallbackDurations...), nil
	}

	var e apiError
	if err := json.Unmarshal(body, &e); err == nil && e.Title == "BAD_DURATION" {
		var info struct {
			Available []int `json:"available_durations"`
		}
		if err := json.Unmarshal(body, &struct {
			Info *struct {
				Available []int `json:"available_durations"`
			} `json:"info"`
		}{Info: &info}); err == nil && len(info.Available) > 0 {
			sort.Ints(info.Available)
			return info.Available, nil
		}
	}
	return append([]int(nil), fallbackDurations...), nil
}

// rentEntry serviceCountRent yanıtındaki tek bir kayıt.
type rentEntry struct {
	Count       int     `json:"count"`
	Price       float64 `json:"price"`
	RetailPrice float64 `json:"retail_price"`
}

// ListRentOffers bir servisin tüm ülke × süre fiyat ve stoklarını döner.
//
// YANIT ŞEKLİ: {"<ülkeId>": {"<saat>": {count, price, retail_price}}}
//
//	{"1": {"24": {"count":8,"price":1,"retail_price":1.2}, "72": {...}}}
//
// `duration` parametresi ZORUNLUDUR ama yanıtı SINIRLAMAZ: sağlayıcı hangi
// süreyi gönderirsek gönderelim tüm süreleri döndürüyor. Yine de göndeririz,
// yoksa istek doğrulamadan geçmiyor.
func (p *Provider) ListRentOffers(ctx context.Context, c port.Creds, serviceCode string) ([]port.RentOffer, error) {
	body, err := p.legacyGetRaw(ctx, c, "serviceCountRent", url.Values{
		"service":  {serviceCode},
		"duration": {"720"}, // doğrulamadan geçmek için; yanıtı sınırlamıyor
	})
	if err != nil {
		return nil, err
	}

	// Hata gövdesi de 200 ile gelebilir (legacy yolu) — önce onu eleriz.
	var e apiError
	if err := json.Unmarshal(body, &e); err == nil && e.Title != "" {
		switch e.Title {
		case "UNPROCESSABLE_ENTITY", "BAD_SERVICE", "WRONG_SERVICE":
			// Bu servis kiralık desteklemiyor — hata DEĞİL, boş sonuç.
			return nil, nil
		case "BAD_DURATION":
			return nil, fmt.Errorf("%w: kiralama süresi reddedildi", port.ErrUnsupported)
		}
		return nil, fmt.Errorf("%w: serviceCountRent: %s", port.ErrUnavailable, e.Title)
	}

	var raw map[string]map[string]rentEntry
	if err := json.Unmarshal(body, &raw); err != nil {
		// Beklenmeyen şekil: servis kiralık desteklemiyor olabilir.
		return nil, nil
	}

	out := make([]port.RentOffer, 0, len(raw)*4)
	for country, byDuration := range raw {
		for hoursStr, entry := range byDuration {
			hours, err := strconv.Atoi(hoursStr)
			if err != nil || hours <= 0 {
				continue
			}
			// FİYAT: `price` alanı kullanılır, `retail_price` DEĞİL.
			//
			// Aktivasyon tarafındaki `prices.default` ile aynı mantık: bize
			// kesilen tutar budur. `retail_price` sağlayıcının perakende
			// listesidir ve bizim maliyetimiz değildir; onu kullanmak kâr
			// marjını yanlış hesaplatır.
			cost, err := usdFromFloat(entry.Price)
			if err != nil {
				continue
			}
			out = append(out, port.RentOffer{
				ServiceCode:   serviceCode,
				CountryCode:   country,
				DurationHours: hours,
				Cost:          cost,
				// ⚠️ AÇIK SORU: kiralık `count` alanının gerçek stok mu yoksa
				// aktivasyon tarafındaki `total` gibi bir havuz sayacı mı
				// olduğu spec'te BELİRTİLMEMİŞ. Aktivasyonda `physical`
				// ayrımı vardı, kiralıkta öyle bir alan yok. Şimdilik olduğu
				// gibi alınıyor; ilk stoksuz satın alma bunu ortaya çıkarır.
				// docs/memory.md §6'ya açık soru olarak yazıldı.
				Stock: entry.Count,
			})
		}
	}
	return out, nil
}

// Extend kiralık numaranın süresini uzatır.
//
// 🔴 `prolong` ≠ `reactivate` ≠ `rent` — ÜÇ AYRI mekanizma. Sağlayıcı yanlış
// olanı çağırdığımızda `ACTION_NOT_AVAILABLE: "Try '/reactivate' method."`
// diyor. Buradaki uç yalnız SÜRE UZATMA içindir.
func (p *Provider) Extend(ctx context.Context, c port.Creds, remoteOrderID string, hours int) error {
	if hours <= 0 {
		return fmt.Errorf("%w: uzatma süresi pozitif olmalı", port.ErrUnsupported)
	}
	status, body, err := p.do(ctx, c, http.MethodPost,
		"/activations/"+url.PathEscape(remoteOrderID)+"/prolong",
		map[string]any{"duration": hours})
	if err != nil {
		return err
	}
	if status == http.StatusNoContent || status == http.StatusOK {
		return nil
	}
	// 425 TOO_EARLY: uzatma için henüz erken; sağlayıcı ne zaman
	// denenebileceğini söylüyor.
	return mapLifecycleError(status, body, remoteOrderID)
}

// legacyGetRaw legacy uca istek atar ve HAM gövdeyi döner — hata ayrıştırmadan.
//
// `legacyGet` düz metin hata kodlarını yakalayıp hata döndürür; kiralık
// uçları ise hataları JSON olarak gönderiyor ve o JSON'un İÇİNDEKİ bilgi
// (available_durations) bize lazım. Bu yüzden ayrı bir yol.
func (p *Provider) legacyGetRaw(ctx context.Context, c port.Creds, action string, extra url.Values) ([]byte, error) {
	q := url.Values{"api_key": {c.APIKey}, "action": {action}}
	for k, v := range extra {
		q[k] = v
	}
	// 🔴 URL LOG'A YAZILMAZ: API anahtarı sorgu dizesinde gider.
	u := baseOf(c) + legacyPath + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("herosms: istek kurulamadı (%s): %w", action, err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: legacy %s: %v", port.ErrUnavailable, action, err)
	}
	defer func() { _ = resp.Body.Close() }()

	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if len(buf) > maxBody {
				return nil, fmt.Errorf("%w: legacy %s yanıtı çok büyük", port.ErrUnavailable, action)
			}
		}
		if rerr != nil {
			break
		}
	}
	return buf, nil
}
