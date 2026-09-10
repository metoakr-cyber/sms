// Package herosms HeroSMS sağlayıcı adaptörüdür.
//
// İKİ YÜZEY, TEK ANAHTAR (docs/provider-herosms.md §1):
//
//	Modern REST  https://hero-sms.com/api/v1        Authorization: ApiKey <token>
//	Legacy       .../stubs/handler_api.php          ?api_key=<token>
//
// Legacy'ye MECBURUZ: ülke ve servis adları ile bakiye modern uçlarda hiç yok.
//
// 🔴 LEGACY'DE HTTP DURUMU HATA GÖSTERGESİ DEĞİLDİR. `?action=getCountries`
// yalnız 200 tanımlar; `NO_KEY` / `BAD_KEY` gibi hatalar 200 gövdesinin İÇİNDE
// düz metin olarak gelir. Bu yüzden legacy yolunda `if status == 200 { başarılı }`
// YASAKTIR — gövde her zaman ayrıştırılır.
package herosms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

const (
	defaultBaseURL = "https://hero-sms.com"
	modernPath     = "/api/v1"
	legacyPath     = "/stubs/handler_api.php"

	// Yanıt gövdesi için üst sınır. Sınırsız okumak, bozuk veya düşmanca bir
	// yanıtın belleği tüketmesine izin verir.
	maxBody = 24 << 20 // 24 MiB — offers yanıtı büyük (tüm servis × ülke matrisi)
)

// Provider HeroSMS adaptörü.
type Provider struct {
	client *http.Client
}

func New() *Provider {
	return &Provider{client: &http.Client{Timeout: 45 * time.Second}}
}

var _ port.ProviderPort = (*Provider)(nil)

func (p *Provider) Protocol() string { return "HEROSMS_V1" }

func (p *Provider) Capabilities() []port.ProductKind {
	return []port.ProductKind{port.KindSMSActivation, port.KindSMSRental}
}

/* ─────────────────────────── HTTP yardımcıları ─────────────────────────── */

func baseOf(c port.Creds) string {
	if s := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/"); s != "" {
		return s
	}
	return defaultBaseURL
}

// legacyGet legacy uca istek atar ve HAM gövdeyi döner.
//
// 🔴 URL LOG'A YAZILMAZ: API anahtarı sorgu dizesinde gider ve vekil/erişim
// log'larına düşer (docs/design.md §12). Hata mesajlarında da yalnız action adı
// geçer, tam URL değil.
//
// test: leak_test.go#TestLegacyCallNeverLogsAPIKey
func (p *Provider) legacyGet(ctx context.Context, c port.Creds, action string, extra url.Values) ([]byte, error) {
	q := url.Values{"api_key": {c.APIKey}, "action": {action}}
	for k, v := range extra {
		q[k] = v
	}
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("%w: legacy %s gövdesi okunamadı: %v", port.ErrUnavailable, action, err)
	}
	// Durum kodu YİNE DE kontrol edilir — legacy 200 dışını belgelemiyor ama
	// vekil/CDN katmanı 502/504 üretebilir ve bunlar JSON değildir.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: legacy %s HTTP %d", port.ErrUnavailable, action, resp.StatusCode)
	}
	if err := legacyBodyError(body); err != nil {
		return nil, err
	}
	return body, nil
}

// legacyBodyError 200 gövdesinin içindeki düz metin hata kodlarını yakalar.
//
// Bu fonksiyon olmadan `BAD_KEY` yanıtı geçerli veri sanılır ve JSON
// ayrıştırma hatası olarak raporlanır — asıl sebep (yanlış anahtar) kaybolur.
func legacyBodyError(body []byte) error {
	s := strings.TrimSpace(string(body))
	// Hata kodları kısa ve düz metindir; JSON gövde zaten '{' veya '[' ile başlar.
	if len(s) == 0 {
		return fmt.Errorf("%w: legacy boş gövde", port.ErrUnavailable)
	}
	if s[0] == '{' || s[0] == '[' {
		return nil
	}
	head := s
	if i := strings.IndexAny(head, " \n\r\t:"); i > 0 {
		head = head[:i]
	}
	switch strings.ToUpper(head) {
	case "NO_KEY", "BAD_KEY", "BAD_ACTION", "ERROR_SQL":
		return fmt.Errorf("%w: legacy %s", port.ErrProviderAuth, head)
	case "NO_BALANCE":
		return port.ErrProviderNoBalance
	case "NO_NUMBERS":
		return port.ErrOutOfStock
	case "ACCESS_BALANCE", "ACCESS_NUMBER", "ACCESS_READY", "STATUS_OK", "STATUS_WAIT_CODE":
		return nil // beklenen düz metin başarı yanıtları
	}
	return fmt.Errorf("%w: legacy beklenmeyen yanıt: %.60s", port.ErrUnavailable, s)
}

// modernGet modern REST ucuna istek atar.
func (p *Provider) modernGet(ctx context.Context, c port.Creds, path string, q url.Values) ([]byte, error) {
	u := baseOf(c) + modernPath + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("herosms: istek kurulamadı (%s): %w", path, err)
	}
	// "ApiKey", "Bearer" DEĞİL. Bearer 401 döner (canlı doğrulandı).
	req.Header.Set("Authorization", "ApiKey "+c.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: modern %s: %v", port.ErrUnavailable, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("%w: modern %s gövdesi okunamadı: %v", port.ErrUnavailable, path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, modernStatusError(resp, body, path)
	}
	return body, nil
}

// modernStatusError modern uçtaki HTTP durumunu alan hatasına çevirir.
func modernStatusError(resp *http.Response, body []byte, path string) error {
	var e struct {
		Title   string `json:"title"`
		Details string `json:"details"`
		Info    struct {
			RetryAfterSeconds int `json:"retry_after_seconds"`
		} `json:"info"`
	}
	_ = json.Unmarshal(body, &e)

	retryAfter := time.Duration(e.Info.RetryAfterSeconds) * time.Second
	if h := resp.Header.Get("Retry-After"); h != "" {
		if secs, err := strconv.Atoi(h); err == nil {
			retryAfter = time.Duration(secs) * time.Second
		}
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: modern %s: %s", port.ErrProviderAuth, path, e.Title)
	case http.StatusNotFound:
		return port.ErrOutOfStock
	case http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: modern %s: %s", port.ErrMappingMissing, path, e.Title)
	case http.StatusTooManyRequests, http.StatusTooEarly:
		if retryAfter <= 0 {
			retryAfter = 30 * time.Second
		}
		return port.NewRetryAfter(e.Title, retryAfter, port.ErrUnavailable)
	}
	return fmt.Errorf("%w: modern %s HTTP %d: %s", port.ErrUnavailable, path, resp.StatusCode, e.Title)
}

/* ───────────────────────────── Katalog ───────────────────────────── */

// ListCountries legacy `?action=getCountries`.
//
// Yanıt bir DİZİ DEĞİL, anahtarları ülke kimliği olan bir NESNEDİR:
//
//	{"1":{"id":1,"eng":"Ukraine","rus":"...","rent":1,"visible":1}, ...}
func (p *Provider) ListCountries(ctx context.Context, c port.Creds) ([]port.RemoteDimension, error) {
	body, err := p.legacyGet(ctx, c, "getCountries", nil)
	if err != nil {
		return nil, err
	}
	var raw map[string]struct {
		ID      int    `json:"id"`
		Eng     string `json:"eng"`
		Rus     string `json:"rus"`
		Visible int    `json:"visible"`
		Rent    int    `json:"rent"`
		Retry   int    `json:"retry"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("herosms: getCountries ayrıştırılamadı: %w", err)
	}

	out := make([]port.RemoteDimension, 0, len(raw))
	for key, v := range raw {
		if v.Visible == 0 {
			// Görünmeyen ülke satın alınamaz; katalogda göstermek kullanıcıyı
			// stoksuz bir akışa sokar.
			continue
		}
		// Kimlik ÖNCELİKLE gövdedeki `id` alanından alınır; harita anahtarı
		// yedektir. İkisi ayrışırsa gövde doğrudur.
		code := strconv.Itoa(v.ID)
		if v.ID == 0 {
			code = key
		}
		out = append(out, port.RemoteDimension{
			RemoteCode: code,
			Name:       v.Eng,
			Extra: map[string]any{
				"rent":  v.Rent == 1,
				"retry": v.Retry == 1,
			},
		})
	}
	return out, nil
}

// ListServices legacy `?action=getServicesList&lang=tr`.
//
// `lang=tr` destekleniyor ama pratikte adlar İngilizce dönüyor ("Whatsapp",
// "facebook"). Yine de gönderilir: sağlayıcı Türkçeleştirdiğinde bedava
// kazanç olur. Görünen ad zaten yerel `services.name_tr` ile ezilebilir.
func (p *Provider) ListServices(ctx context.Context, c port.Creds) ([]port.RemoteDimension, error) {
	body, err := p.legacyGet(ctx, c, "getServicesList", url.Values{"lang": {"tr"}})
	if err != nil {
		return nil, err
	}
	var raw struct {
		Status   string `json:"status"`
		Services []struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"services"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("herosms: getServicesList ayrıştırılamadı: %w", err)
	}
	if raw.Status != "" && raw.Status != "success" {
		return nil, fmt.Errorf("%w: getServicesList status=%s", port.ErrUnavailable, raw.Status)
	}

	out := make([]port.RemoteDimension, 0, len(raw.Services))
	for _, s := range raw.Services {
		code := strings.TrimSpace(s.Code)
		if code == "" {
			continue
		}
		// "full" = tüm numaranın kiralanması; aktivasyon değil. Yeteneklerimizde
		// kiralama ilan etmediğimiz için katalogda yeri yok.
		if code == "full" {
			continue
		}
		name := strings.TrimSpace(s.Name)
		if name == "" {
			name = code
		}
		out = append(out, port.RemoteDimension{RemoteCode: code, Name: name})
	}
	return out, nil
}

// offersResponse modern `GET /activations/offers/{sms|call}` yanıtı.
//
//	{"data": {"<servis>": {"<ülkeId>": {"prices":{...}, "counts":{...}}}}}
type offersResponse struct {
	Data map[string]map[string]offerEntry `json:"data"`
}

// offerEntry tek bir servis × ülke teklifidir.
type offerEntry struct {
	Prices struct {
		Default float64 `json:"default"`
		Retail  float64 `json:"retail"`
		Min     float64 `json:"min"`
	} `json:"prices"`
	Counts struct {
		Total        int `json:"total"`
		Physical     int `json:"physical"`
		DefaultPrice int `json:"defaultPrice"`
	} `json:"counts"`
}

// stok, bu teklifte BİZİM ödemeye razı olduğumuz fiyattan alınabilecek numara
// adedini verir.
//
// ══════════════════════════════════════════════════════════════════════════
// NEDEN `defaultPrice`, NEDEN `physical` DEĞİL
// ══════════════════════════════════════════════════════════════════════════
// Sağlayıcı üç sayaç döndürür ve spec ÜÇÜNÜ DE açıklamaz (docs/provider-herosms.md
// ❓H16). Seçim ölçümle yapıldı, tahminle değil.
//
//	counts.total        fiyat tavanı olmayan havuzun tamamı
//	counts.physical     fiziksel SIM havuzu
//	counts.defaultPrice `prices.default` ve altındaki numaralar
//
// `total` KULLANILAMAZ: merdivenin tepesi 37,50 USD'ye kadar çıkıyor. Eski
// prototip `total` kullanıyordu — kullanıcıdan para çekiliyor, sağlayıcı boş
// dönüyordu (docs/trd.md FR-306).
//
// `physical` de KULLANILAMAZ, ve bu daha sinsi bir hatadır: sessizce satış
// engeller. 10 Eylül 2026'da canlı katalogun tamamı ölçüldü (20.788 kombinasyon):
//
//	physical > 0        9.930  (%47,8)   ← eski ölçütümüz
//	defaultPrice > 0   15.840  (%76,2)
//	ikisi de 0          4.740  (%22,8)   ← gerçekten boş olanlar
//
// Yani katalogun ~%28'i satılabilirken "numara bulunmuyor" diyordu. TÜRKİYE'DE
// ise `physical` 123 kombinasyonun HİÇBİRİNDE pozitif değil — ülke tamamen
// kapalıydı; oysa 70'i varsayılan fiyattan alınabilir durumda (WhatsApp × TR:
// physical 0, defaultPrice 9.929, 1,20 USD).
//
// Doğru sayaç `defaultPrice`'tır ÇÜNKÜ satın alırken `maxPrice = prices.default`
// gönderiyoruz (Değişmez 21). Ölçüt, ödeyeceğimiz fiyata bağlıdır — havuzun
// fiziksel/sanal ayrımına değil.
//
// `physical > 0` iken `defaultPrice == 0` olan 208 kombinasyon ÖLÇÜLDÜ: hepsinde
// merdivenin en ucuz basamağı varsayılan fiyatın hemen ÜSTÜNDE (örn. hu×1:
// default 0,075 · en ucuz basamak 0,0751). Onlar `maxPrice`'ımızla zaten
// alınamaz; 0 doğru cevaptır.
//
// test: herosms_stok_test.go#TestStokVarsayilanFiyatSayacindanGelir
func (o offerEntry) stok() int {
	return o.Counts.DefaultPrice
}

// ListOffers tüm servis × ülke fiyat ve stok anlık görüntüsünü çeker.
//
// 🔴 Stok `offerEntry.stok()` ile hesaplanır — hangi sayacın kullanıldığı ve
// diğer ikisinin neden yanlış olduğu orada yazılı.
func (p *Provider) ListOffers(ctx context.Context, c port.Creds, vt port.VerificationType) ([]port.OfferSnapshot, error) {
	kind := string(vt)
	if kind != string(port.VerifySMS) && kind != string(port.VerifyCall) {
		return nil, fmt.Errorf("%w: verificationType=%q", port.ErrUnsupported, vt)
	}

	body, err := p.modernGet(ctx, c, "/activations/offers/"+kind, nil)
	if err != nil {
		return nil, err
	}
	var raw offersResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("herosms: offers ayrıştırılamadı: %w", err)
	}

	out := make([]port.OfferSnapshot, 0, 4096)
	for service, byCountry := range raw.Data {
		for country, o := range byCountry {
			cost, err := usdFromFloat(o.Prices.Default)
			if err != nil {
				// Tek bir bozuk fiyat tüm senkronu düşürmemeli; o satır atlanır.
				continue
			}
			out = append(out, port.OfferSnapshot{
				ServiceCode:      service,
				CountryCode:      country,
				VerificationType: vt,
				Cost:             cost,
				Stock:            o.stok(),
			})
		}
	}
	return out, nil
}

// GetPriceAndStock tek bir servis × ülke için fiyat ve stok.
//
// `offers` ucu filtre kabul ettiği için tüm matrisi çekmeyiz.
func (p *Provider) GetPriceAndStock(ctx context.Context, c port.Creds, q port.PriceQuery) (*port.PriceResult, error) {
	vt := q.VerificationType
	if vt == "" {
		vt = port.VerifySMS
	}
	body, err := p.modernGet(ctx, c, "/activations/offers/"+string(vt), url.Values{
		"services":  {q.ServiceCode},
		"countries": {q.CountryCode},
	})
	if err != nil {
		return nil, err
	}
	var raw offersResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("herosms: offers ayrıştırılamadı: %w", err)
	}
	o, ok := raw.Data[q.ServiceCode][q.CountryCode]
	if !ok {
		// Sağlayıcı bu kombinasyonu tanımıyor: eşleştirme sorunu mu, geçici
		// stoksuzluk mu ayırt edemeyiz. Stok yok saymak güvenli taraftır.
		return &port.PriceResult{Stock: 0}, nil
	}
	cost, err := usdFromFloat(o.Prices.Default)
	if err != nil {
		return nil, err
	}
	return &port.PriceResult{Cost: cost, Stock: o.stok()}, nil
}

// GetBalance legacy `?action=getBalance` → düz metin `ACCESS_BALANCE:30.3133`.
func (p *Provider) GetBalance(ctx context.Context, c port.Creds) (money.Money, error) {
	body, err := p.legacyGet(ctx, c, "getBalance", nil)
	if err != nil {
		return money.Money{}, err
	}
	s := strings.TrimSpace(string(body))
	_, amount, found := strings.Cut(s, ":")
	if !found {
		return money.Money{}, fmt.Errorf("%w: getBalance biçimi beklenmedik: %.40s", port.ErrUnavailable, s)
	}
	// Birim spec'te belirtilmiyor; USD olduğu §2'de çıkarıldı (docs/provider-herosms.md).
	v, err := strconv.ParseFloat(strings.TrimSpace(amount), 64)
	if err != nil {
		return money.Money{}, fmt.Errorf("%w: getBalance sayı değil: %.40s", port.ErrUnavailable, amount)
	}
	return usdFromFloat(v)
}

/* ───────────────────────────── Para ───────────────────────────── */

// usdFromFloat sağlayıcının float fiyatını mikro-dolara çevirir.
//
// test: money_test.go#TestUsdFromFloatIsExact
// test: money_test.go#TestUsdFromFloatDoesNotDriftDown
//
// Sağlayıcı JSON'da float gönderiyor; seçme şansımız yok. Çeviriyi TEK BİR
// YERDE yapıp hemen tam sayıya geçeriz — float aritmetiği sistemin geri
// kalanına SIZMAZ. 4 ondalıklı fiyatlar (0.9600, 1.3625) mikro ölçekte
// (10^6) tam temsil edilir.
func usdFromFloat(v float64) (money.Money, error) {
	if v < 0 {
		return money.Money{}, errors.New("herosms: negatif fiyat")
	}
	// +0.5 ile yuvarlama: 1.0799999... gibi float artıklarının bir mikro-dolar
	// aşağı kaymasını engeller.
	// test: money_test.go#TestUsdFromFloatDoesNotDriftDown
	micro := int64(v*1_000_000 + 0.5)
	return money.New(micro, money.USD), nil
}
