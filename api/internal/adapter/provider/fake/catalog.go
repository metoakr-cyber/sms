package fake

import (
	"context"
	"sort"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// seedCatalog gerçekçi bir başlangıç kataloğu kurar.
//
// Fiyatlar ve stoklar HeroSMS'te gözlemlenen gerçek değerlerden esinlenmiştir:
// WhatsApp/TR pahalı ve STOKSUZ (canlıda physicalCount=0 gözlendi), Telegram
// ucuz ve bol. Böylece "stok yok" dalı yerel geliştirmede de gerçekten oluşur.
func (p *Provider) seedCatalog() {
	seed := []struct {
		service   string
		country   string
		costMicro int64
		stock     int
	}{
		{"wa", "TR", 1_440_000, 0}, // WhatsApp/TR: pahalı, STOKSUZ (gerçek gözlem)
		{"wa", "RU", 420_000, 1_240},
		{"wa", "UA", 380_000, 860},
		{"tg", "TR", 180_000, 3_412},
		{"tg", "RU", 95_000, 12_800},
		{"tg", "UA", 88_000, 9_100},
		{"ig", "TR", 260_000, 540},
		{"ig", "RU", 150_000, 4_300},
		{"go", "TR", 310_000, 210},
		{"go", "RU", 175_000, 2_900},
		{"fb", "TR", 240_000, 1_100},
		{"am", "US", 620_000, 75},
	}
	for _, s := range seed {
		p.catalog[key{s.service, s.country, port.VerifySMS}] = &entry{
			costMicro: s.costMicro, stock: s.stock,
		}
		// 'call' doğrulaması AYRI bir eksendir: farklı fiyat, farklı stok.
		p.catalog[key{s.service, s.country, port.VerifyCall}] = &entry{
			costMicro: s.costMicro * 2, stock: s.stock / 10,
		}
	}
}

var fakeCountries = map[string]struct {
	name, nameTR, phoneCode string
	rent                    bool
}{
	"TR": {"Turkey", "Türkiye", "90", true},
	"RU": {"Russia", "Rusya", "7", true},
	"UA": {"Ukraine", "Ukrayna", "380", true},
	"US": {"United States", "Amerika Birleşik Devletleri", "1", false},
	"KZ": {"Kazakhstan", "Kazakistan", "7", true},
}

var fakeServices = map[string]string{
	"wa": "Whatsapp",
	"tg": "Telegram",
	"ig": "Instagram",
	"go": "Google",
	"fb": "Facebook",
	"am": "Amazon",
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
