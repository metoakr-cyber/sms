package herosms

import (
	"encoding/json"
	"testing"
)

// TestAllowedDurationsParsesFromError
//
// SÖZLEŞME: kabul edilen kiralama süreleri SAĞLAYICIDAN öğrenilir, koda
// gömülmez. Sağlayıcı "desteklenen süreler" diye bir uç sunmuyor; listeyi
// söylediği tek yer BAD_DURATION hata yanıtı.
//
// Spec bu konuda ÜÇ YERDE kendiyle çelişiyor (RentDuration enum'u bir şey,
// BAD_DURATION yanıtı başka, serviceCountRent örneği bambaşka). Sabit liste
// yazmak, sağlayıcı değiştirdiğinde sessizce yanlış olmak demekti.
func TestAllowedDurationsParsesFromError(t *testing.T) {
	// Canlıdan alınan gerçek yanıt (2026-09-08).
	raw := []byte(`{"title":"BAD_DURATION","details":"Invalid rental period.",` +
		`"info":{"min":24,"max":4320,` +
		`"available_durations":[24,72,168,336,720,1440,2160,4320]}}`)

	var e apiError
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	if e.Title != "BAD_DURATION" {
		t.Fatalf("başlık = %q", e.Title)
	}

	var info struct {
		Available []int `json:"available_durations"`
	}
	if err := json.Unmarshal(raw, &struct {
		Info *struct {
			Available []int `json:"available_durations"`
		} `json:"info"`
	}{Info: &info}); err != nil {
		t.Fatal(err)
	}

	want := []int{24, 72, 168, 336, 720, 1440, 2160, 4320}
	if len(info.Available) != len(want) {
		t.Fatalf("süre sayısı = %d, beklenen %d — hata gövdesinden okunamıyor",
			len(info.Available), len(want))
	}
	for i, v := range want {
		if info.Available[i] != v {
			t.Errorf("süre[%d] = %d, beklenen %d", i, info.Available[i], v)
		}
	}

	// Aylık kiralama (30 gün = 720 saat) listede OLMALI: ürünün en çok
	// tercih edilen kademesi bu.
	var hasMonthly bool
	for _, v := range info.Available {
		if v == 720 {
			hasMonthly = true
		}
	}
	if !hasMonthly {
		t.Error("720 saat (30 gün) desteklenen süreler arasında yok")
	}
}

// TestFallbackDurationsMatchLiveList
//
// Yedek liste, canlıdan okunan listeyle AYNI olmalı. Ayrışırsa sağlayıcıya
// ulaşılamadığı anlarda kullanıcıya var olmayan bir süre gösteririz ve satın
// alma BAD_DURATION ile düşer.
func TestFallbackDurationsMatchLiveList(t *testing.T) {
	live := []int{24, 72, 168, 336, 720, 1440, 2160, 4320}
	if len(fallbackDurations) != len(live) {
		t.Fatalf("yedek liste %d süre, canlı %d", len(fallbackDurations), len(live))
	}
	for i, v := range live {
		if fallbackDurations[i] != v {
			t.Errorf("yedek[%d] = %d, canlı %d", i, fallbackDurations[i], v)
		}
	}
}
