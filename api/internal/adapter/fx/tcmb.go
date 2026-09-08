// Package fx döviz kuru sağlayıcılarını içerir.
package fx

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// TCMB Türkiye Cumhuriyet Merkez Bankası günlük kur servisi.
//
// Seçilme gerekçesi: resmî, ücretsiz, anahtar gerektirmiyor ve Türkiye'deki
// muhasebe pratiğinde referans alınan kaynak.
//
// ⚠️ SINIRI: günde bir kez (iş günlerinde ~15:30) güncellenir. Gün içi
// dalgalanmayı yakalamaz. Bu yüzden kur tamponu (FX_SAFETY_MARGIN_PCT)
// zorunludur — kur riski bizde (docs/intent.md §5).
type TCMB struct {
	client *http.Client
	url    string
}

func NewTCMB() *TCMB {
	return &TCMB{
		client: &http.Client{Timeout: 10 * time.Second},
		url:    "https://www.tcmb.gov.tr/kurlar/today.xml",
	}
}

func (t *TCMB) Source() string { return "tcmb" }

type tcmbEnvelope struct {
	XMLName    xml.Name       `xml:"Tarih_Date"`
	Date       string         `xml:"Tarih,attr"`
	Currencies []tcmbCurrency `xml:"Currency"`
}

type tcmbCurrency struct {
	Code            string `xml:"CurrencyCode,attr"`
	ForexSelling    string `xml:"ForexSelling"`
	BanknoteSelling string `xml:"BanknoteSelling"`
}

func (t *TCMB) Fetch(ctx context.Context, base, quote money.Currency) (port.FXQuote, error) {
	// TCMB yalnız TRY karşılığı yayınlar.
	if quote.Code != "TRY" {
		return port.FXQuote{}, fmt.Errorf("fx: TCMB yalnız TRY karşılığı verir (istenen: %s)", quote.Code)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
	if err != nil {
		return port.FXQuote{}, err
	}
	req.Header.Set("User-Agent", "sms-platform/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return port.FXQuote{}, fmt.Errorf("fx: TCMB'ye ulaşılamadı: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return port.FXQuote{}, fmt.Errorf("fx: TCMB HTTP %d", resp.StatusCode)
	}

	var env tcmbEnvelope
	if err := xml.NewDecoder(resp.Body).Decode(&env); err != nil {
		return port.FXQuote{}, fmt.Errorf("fx: TCMB yanıtı çözülemedi: %w", err)
	}

	for _, c := range env.Currencies {
		if !strings.EqualFold(c.Code, base.Code) {
			continue
		}
		// ForexSelling (döviz satış) kullanılır: biz döviz cinsinden mal alıyoruz,
		// yani dövizi "satın alıyoruz". Alış kuru kullanmak maliyeti olduğundan
		// düşük gösterir ve marjı sessizce yer.
		raw := strings.TrimSpace(c.ForexSelling)
		if raw == "" {
			raw = strings.TrimSpace(c.BanknoteSelling)
		}
		if raw == "" {
			return port.FXQuote{}, fmt.Errorf("fx: TCMB %s için satış kuru boş", base.Code)
		}
		rate, err := money.RateFromString(raw)
		if err != nil {
			return port.FXQuote{}, fmt.Errorf("fx: kur ayrıştırılamadı %q: %w", raw, err)
		}
		if rate.IsZero() || rate.IsNegative() {
			return port.FXQuote{}, fmt.Errorf("fx: geçersiz kur %q", raw)
		}
		return port.FXQuote{
			Base: base, Quote: quote, Rate: rate,
			Source: t.Source(), FetchedAt: time.Now(),
		}, nil
	}
	return port.FXQuote{}, fmt.Errorf("fx: TCMB yanıtında %s bulunamadı", base.Code)
}

// Static sabit kur döndürür. YALNIZ test ve yerel geliştirme içindir;
// config paketi üretimde 'manual' sağlayıcıyı da kabul eder ama o durumda
// kur elle güncellenmek zorundadır ve bayatlama kontrolü yine çalışır.
type Static struct {
	Rate money.Rate
	At   time.Time
}

func (s *Static) Source() string { return "static" }

func (s *Static) Fetch(_ context.Context, base, quote money.Currency) (port.FXQuote, error) {
	at := s.At
	if at.IsZero() {
		at = time.Now()
	}
	return port.FXQuote{Base: base, Quote: quote, Rate: s.Rate, Source: s.Source(), FetchedAt: at}, nil
}
