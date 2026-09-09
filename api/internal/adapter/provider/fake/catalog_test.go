package fake_test

// Sahte kataloğun TOHUM VERİSİ garantileri.
//
// Buradaki testler sağlayıcı davranışını değil, tohumun kendisini ölçer:
// "16 servis var" demek yetmez; o servislerin kodları logolarla ve ülke
// referansıyla GERÇEKTEN eşleşmiyorsa katalog yine logosuz ve eksik açılır.
// Kök sebep tam olarak buydu — ön yüz doğruydu, veri yanlıştı.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider/fake"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// repoRoot depo kökünü verir: bu paket api/internal/adapter/provider/fake.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("kök yol: %v", err)
	}
	return root
}

func seedServiceCodes(t *testing.T) []string {
	t.Helper()
	p := fake.New(nil)
	rows, err := p.ListServices(context.Background(), port.Creds{})
	if err != nil {
		t.Fatalf("servis listesi: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.RemoteCode)
	}
	return out
}

// TestSeedServiceCodesHaveLogoFiles her tohum servis kodunun bir logo dosyası
// olduğunu doğrular.
//
// Logo eşleştirmesi KODA göre yapılır (`web/scripts/servis-logolari.sql`:
// `WHERE code = 'wa'`). Uydurma bir kod eklendiğinde ön yüz sessizce harf
// rozetine düşer ve kimse fark etmez; bu test o sessizliği bozar.
func TestSeedServiceCodesHaveLogoFiles(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "web", "public", "servis-logolari")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("logo dizini okunamadı: %v", err)
	}
	// taban ad → var  ("wa.svg" ve "am.png" ikisi de "wa"/"am" sayılır.)
	have := make(map[string]bool, len(entries))
	for _, e := range entries {
		name := e.Name()
		ext := filepath.Ext(name)
		if ext != ".svg" && ext != ".png" {
			continue
		}
		have[strings.TrimSuffix(name, ext)] = true
	}

	for _, code := range seedServiceCodes(t) {
		if !have[code] {
			t.Errorf("servis %q için logo dosyası yok (web/public/servis-logolari/%s.svg|.png); "+
				"ya logoyu ekleyin ya da tohumdan çıkarın", code, code)
		}
	}
}

// countryRefKeys 00007_country_reference.sql içindeki name_key değerlerini okur.
//
// Veritabanına GİTMEZ: bu bir birim testidir ve migration dosyası zaten tek
// gerçek kaynaktır.
func countryRefKeys(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "api", "migrations", "00007_country_reference.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ülke referansı okunamadı: %v", err)
	}
	// ('turkey', 'TR', 'Turkey', 'Türkiye', '90'),
	re := regexp.MustCompile(`\(\s*'([^']+)'\s*,\s*'([A-Z]{2})'`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		out[m[1]] = m[2]
	}
	if len(out) < 50 {
		t.Fatalf("ülke referansı ayrıştırılamadı (%d kayıt)", len(out))
	}
	return out
}

// TestSeedCountryNamesAreResolvable tohumdaki her ülkenin İngilizce adının
// `country_reference` üzerinden DOĞRU ISO2'ye çözüldüğünü doğrular.
//
// Senkron ISO2'yi sağlayıcı kodundan değil bu addan türetir
// (service/catalog/sync.go, interpretCountry) ve çözemediği ülkeyi SESSİZCE
// ATLAR. "Almanya eklendi ama katalogda yok" hatası tam olarak böyle oluşur.
func TestSeedCountryNamesAreResolvable(t *testing.T) {
	refs := countryRefKeys(t)

	p := fake.New(nil)
	rows, err := p.ListCountries(context.Background(), port.Creds{})
	if err != nil {
		t.Fatalf("ülke listesi: %v", err)
	}
	if len(rows) < 10 {
		t.Fatalf("tohumda %d ülke var, en az 10 bekleniyordu", len(rows))
	}
	for _, r := range rows {
		iso, ok := refs[strings.ToLower(r.Name)]
		if !ok {
			t.Errorf("ülke adı %q country_reference'ta yok — senkron bu ülkeyi ATLAR", r.Name)
			continue
		}
		if iso != r.RemoteCode {
			t.Errorf("ülke %q: referans %s diyor, tohum kodu %s", r.Name, iso, r.RemoteCode)
		}
	}
}

// TestSeedHasEnoughStockedServices ana sayfada en az 12 servis GÖSTERİLEBİLİR
// olduğunu doğrular.
//
// Kullanıcının şikâyeti buydu: "hâlâ 6 servis görünüyor". Ön yüz stoksuz
// servisi göstermez (`/catalog/services-in-stock`), yani sayı tohumdaki
// servis sayısı değil, EN AZ BİR ÜLKEDE STOKLU servis sayısıdır.
func TestSeedHasEnoughStockedServices(t *testing.T) {
	ctx := context.Background()
	p := fake.New(nil)

	offers, err := p.ListOffers(ctx, port.Creds{}, port.VerifySMS)
	if err != nil {
		t.Fatalf("teklifler: %v", err)
	}
	stocked := map[string]bool{}
	for _, o := range offers {
		if o.Stock > 0 {
			stocked[o.ServiceCode] = true
		}
	}
	if len(stocked) < 12 {
		t.Fatalf("stoklu servis sayısı %d, en az 12 olmalı", len(stocked))
	}
}

// TestWhatsAppTurkeyStaysOutOfStock gerçek gözlemi koruyan tohum kuralıdır.
//
// WhatsApp × TR canlıda physicalCount=0 idi. Tohumu "daha güzel görünsün" diye
// stoklu yapmak, "stok yok" dalını yerel geliştirmede HİÇ çalıştırmamak
// demektir; o dal ilk kez üretimde çalışırsa hata da orada bulunur.
func TestWhatsAppTurkeyStaysOutOfStock(t *testing.T) {
	ctx := context.Background()
	p := fake.New(nil)

	res, err := p.GetPriceAndStock(ctx, port.Creds{}, port.PriceQuery{
		ServiceCode: "wa", CountryCode: "TR", VerificationType: port.VerifySMS,
	})
	if err != nil {
		t.Fatalf("fiyat/stok: %v", err)
	}
	if res.Stock != 0 {
		t.Fatalf("WhatsApp × TR stoğu %d, 0 olmalıydı (gerçek gözlem)", res.Stock)
	}

	// Ama WhatsApp TÜMÜYLE görünmez olmamalı: stoklu bir ülkesi bulunmalı,
	// yoksa kullanıcı en çok aradığı servisi katalogda hiç göremez.
	res, err = p.GetPriceAndStock(ctx, port.Creds{}, port.PriceQuery{
		ServiceCode: "wa", CountryCode: "RU", VerificationType: port.VerifySMS,
	})
	if err != nil {
		t.Fatalf("fiyat/stok (RU): %v", err)
	}
	if res.Stock <= 0 {
		t.Fatal("WhatsApp hiçbir ülkede stoklu değil — katalogda hiç görünmez")
	}
}
