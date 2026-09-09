package fake

import (
	"context"
	"sort"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// ═══════════════════════ SAHTE KATALOG ═══════════════════════
//
// Bu katalog GERÇEK HeroSMS servis kodlarını kullanır (`wa`, `tg`, `lf`, `mt`…),
// uydurulmuş kodları değil. Sebep tek başına estetik değil: `services.code`
// alanı hem logo eşleştirmesinin (`web/scripts/servis-logolari.sql`) hem de
// sağlayıcı boyut eşleştirmesinin anahtarıdır. Uydurma bir kodla geliştirme
// yaparsak ön yüz logosuz çalışır ve gerçek sağlayıcıya geçince "neden şimdi
// bozuldu?" sorusu üretilir.
//
// Kod → marka eşleşmesi `web/public/servis-logolari/<kod>.svg` dosyalarındaki
// `aria-label` değerlerinden DOĞRULANMIŞTIR; tahminle yazılmadı.
// test: catalog_test.go#TestSeedServiceCodesHaveLogoFiles

// fakeCountries sahte sağlayıcının ülke boyutu.
//
// 🔴 `name` alanı SERBEST BİR ETİKET DEĞİLDİR: senkron ISO2 kodunu bu ada
// bakarak `country_reference` tablosundan çözer (service/catalog/sync.go,
// interpretCountry). Burada "Deutschland" yazsaydık ülke çözülemez ve senkron
// onu sessizce ATLARDI.
// test: catalog_test.go#TestSeedCountryNamesAreResolvable
var fakeCountries = map[string]struct {
	name, nameTR, phoneCode string
	rent                    bool
}{
	"TR": {"Turkey", "Türkiye", "90", true},
	"RU": {"Russia", "Rusya", "7", true},
	"UA": {"Ukraine", "Ukrayna", "380", true},
	"US": {"United States", "Amerika Birleşik Devletleri", "1", false},
	"KZ": {"Kazakhstan", "Kazakistan", "7", true},
	"DE": {"Germany", "Almanya", "49", true},
	"GB": {"United Kingdom", "Birleşik Krallık", "44", false},
	"FR": {"France", "Fransa", "33", true},
	"IN": {"India", "Hindistan", "91", true},
	"BR": {"Brazil", "Brezilya", "55", false},
}

// fakeServices sahte sağlayıcının servis boyutu.
//
// Ad İNGİLİZCEDİR çünkü senkron `name_tr` alanını BOŞ bırakır (sync.go) ve
// arayüz Türkçe ad yoksa bu adı gösterir. Marka adları zaten çevrilmez.
var fakeServices = map[string]string{
	"wa":  "Whatsapp",
	"tg":  "Telegram",
	"ig":  "Instagram",
	"go":  "Google",
	"fb":  "Facebook",
	"tw":  "X (Twitter)",
	"ds":  "Discord",
	"lf":  "TikTok",
	"am":  "Amazon",
	"mm":  "Microsoft",
	"nf":  "Netflix",
	"ub":  "Uber",
	"vi":  "Viber",
	"mt":  "Steam",
	"ya":  "Yandex",
	"bkd": "Sahibinden",
}

// seedRow tek bir servis × ülke kombinasyonu.
type seedRow struct {
	service   string
	country   string
	costMicro int64 // mikro-USD
	stock     int
}

// seedRows açıkça yazılmıştır, türetilmez.
//
// Bir formülle ("taban fiyat × ülke çarpanı") üretmek daha kısa olurdu ama
// stok ve fiyat GÖZLEME dayalı: WhatsApp Türkiye'de pahalı ve stoksuz,
// Telegram Rusya'da ucuz ve bol. Formül bunları düzleştirir ve sahte
// sağlayıcının var oluş sebebini (gerçek davranışı taklit etmek) yok eder.
var seedRows = []seedRow{
	// 🔴 WhatsApp × TR BİLEREK STOKSUZ — canlıda gözlenen durum
	// (physicalCount=0). "Stok yok" dalı yerel geliştirmede de gerçekten
	// oluşsun diye korunur; sözleşme testi de bu kombinasyona dayanır
	// (fake_test.go#TestContract, EmptyService/EmptyCountry).
	{"wa", "TR", 1_440_000, 0},
	{"wa", "RU", 420_000, 1_240},
	{"wa", "UA", 380_000, 860},
	{"wa", "KZ", 460_000, 430},
	{"wa", "IN", 310_000, 2_150},
	{"wa", "BR", 520_000, 610},

	{"tg", "TR", 180_000, 3_412},
	{"tg", "RU", 95_000, 12_800}, // sözleşme testinin STOKLU kombinasyonu
	{"tg", "UA", 88_000, 9_100},
	{"tg", "KZ", 120_000, 5_400},
	{"tg", "DE", 240_000, 1_180},
	{"tg", "IN", 105_000, 7_900},

	{"ig", "TR", 260_000, 540},
	{"ig", "RU", 150_000, 4_300},
	{"ig", "UA", 140_000, 3_100},
	{"ig", "BR", 190_000, 2_400},
	{"ig", "IN", 130_000, 6_200},

	{"go", "TR", 310_000, 210},
	{"go", "RU", 175_000, 2_900},
	{"go", "UA", 165_000, 1_950},
	{"go", "US", 540_000, 180},
	{"go", "DE", 420_000, 760},
	{"go", "IN", 150_000, 4_800},

	{"fb", "TR", 240_000, 1_100},
	{"fb", "RU", 130_000, 3_600},
	{"fb", "UA", 125_000, 2_800},
	{"fb", "BR", 160_000, 1_900},
	{"fb", "IN", 110_000, 5_100},

	{"tw", "TR", 350_000, 320},
	{"tw", "RU", 210_000, 1_450},
	{"tw", "UA", 195_000, 980},
	{"tw", "US", 610_000, 120},
	{"tw", "DE", 480_000, 340},

	{"ds", "TR", 290_000, 460},
	{"ds", "RU", 165_000, 2_100},
	{"ds", "UA", 155_000, 1_500},
	{"ds", "US", 520_000, 240},
	{"ds", "GB", 470_000, 310},

	{"lf", "TR", 280_000, 880},
	{"lf", "RU", 155_000, 3_200},
	{"lf", "UA", 145_000, 2_300},
	{"lf", "BR", 185_000, 1_700},
	{"lf", "IN", 125_000, 4_400},

	{"am", "TR", 520_000, 95},
	{"am", "US", 620_000, 75},
	{"am", "GB", 590_000, 110},
	{"am", "DE", 560_000, 140},
	{"am", "FR", 545_000, 125},

	{"mm", "TR", 330_000, 280},
	{"mm", "RU", 190_000, 1_250},
	{"mm", "US", 570_000, 160},
	{"mm", "DE", 450_000, 420},
	{"mm", "GB", 465_000, 380},

	{"nf", "TR", 610_000, 48},
	{"nf", "US", 740_000, 32},
	{"nf", "GB", 700_000, 55},
	{"nf", "BR", 480_000, 90},
	{"nf", "DE", 660_000, 64},

	{"ub", "TR", 420_000, 150},
	{"ub", "RU", 240_000, 620},
	{"ub", "US", 580_000, 95},
	{"ub", "BR", 300_000, 410},
	{"ub", "FR", 500_000, 130},

	{"vi", "TR", 210_000, 1_450},
	{"vi", "RU", 120_000, 5_200},
	{"vi", "UA", 110_000, 4_100},
	{"vi", "KZ", 140_000, 2_300},

	{"mt", "TR", 300_000, 380},
	{"mt", "RU", 170_000, 1_800},
	{"mt", "UA", 160_000, 1_200},
	{"mt", "DE", 430_000, 520},
	{"mt", "US", 550_000, 210},

	{"ya", "TR", 200_000, 760},
	{"ya", "RU", 90_000, 8_400},
	{"ya", "UA", 105_000, 3_600},
	{"ya", "KZ", 115_000, 2_900},

	// Yalnız Türkiye'de anlamı olan bir servis: katalogda "her servis her
	// ülkede var" yanılsaması oluşmasın.
	{"bkd", "TR", 250_000, 640},
}

// seedCatalog gerçekçi bir başlangıç kataloğu kurar.
//
// Fiyatlar ve stoklar HeroSMS'te gözlemlenen gerçek değerlerden esinlenmiştir:
// WhatsApp/TR pahalı ve STOKSUZ, Telegram ucuz ve bol. Böylece "stok yok" dalı
// yerel geliştirmede de gerçekten oluşur.
func (p *Provider) seedCatalog() {
	for _, s := range seedRows {
		p.catalog[key{s.service, s.country, port.VerifySMS}] = &entry{
			costMicro: s.costMicro, stock: s.stock,
		}
		// 'call' doğrulaması AYRI bir eksendir: farklı fiyat, farklı stok.
		p.catalog[key{s.service, s.country, port.VerifyCall}] = &entry{
			costMicro: s.costMicro * 2, stock: s.stock / 10,
		}
	}
}

func (p *Provider) ListCountries(ctx context.Context, _ port.Creds) ([]port.RemoteDimension, error) {
	if err := p.delay(ctx); err != nil {
		return nil, err
	}
	out := make([]port.RemoteDimension, 0, len(fakeCountries))
	for iso, c := range fakeCountries {
		out = append(out, port.RemoteDimension{
			RemoteCode: iso,
			Name:       c.name,
			Extra: map[string]any{
				"nameTR":    c.nameTR,
				"phoneCode": c.phoneCode,
				"rent":      c.rent,
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RemoteCode < out[j].RemoteCode })
	return out, nil
}

func (p *Provider) ListServices(ctx context.Context, _ port.Creds) ([]port.RemoteDimension, error) {
	if err := p.delay(ctx); err != nil {
		return nil, err
	}
	out := make([]port.RemoteDimension, 0, len(fakeServices))
	for code, name := range fakeServices {
		out = append(out, port.RemoteDimension{RemoteCode: code, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RemoteCode < out[j].RemoteCode })
	return out, nil
}
