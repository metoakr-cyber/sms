package money

import "fmt"

// Currency bir para birimini ve onun SAKLAMA ÖLÇEĞİNİ tanımlar.
//
// Ölçek (Scale) = en küçük birimdeki ondalık hane sayısı.
// TRY için 2 (kuruş) yeterlidir; kullanıcıya dönük tüm tutarlar bu ölçektedir.
// USD için 6 (mikro-dolar) kullanılır: sağlayıcı fiyatları 4 ondalıklı gelebiliyor
// (HeroSMS MaxPrice.minimum = 0.0067) ve sent'e (2 hane) yuvarlamak birim başına
// 0,0021 USD'ye kadar kırpma yapardı. Bkz. docs/design.md ADR-026.
type Currency struct {
	Code  string
	Scale int32
}

var (
	// TRY — kullanıcı cüzdanının para birimi. Ölçek: kuruş.
	TRY = Currency{Code: "TRY", Scale: 2}
	// USD — sağlayıcı maliyetlerinin para birimi. Ölçek: mikro-dolar (ADR-026).
	USD = Currency{Code: "USD", Scale: 6}
)

// ISO 4217 sayısal kodları. HeroSMS bu kodları kullanır (Currency.enum = [840, 978, 156]).
const (
	ISOUSD = 840
	ISOEUR = 978
	ISOCNY = 156
	ISOTRY = 949
)

func (c Currency) String() string { return c.Code }

func (c Currency) valid() bool { return c.Code != "" && c.Scale >= 0 && c.Scale <= 9 }

// FromISO bir ISO 4217 sayısal kodunu bilinen bir Currency'ye çevirir.
func FromISO(code int) (Currency, error) {
	switch code {
	case ISOUSD:
		return USD, nil
	case ISOTRY:
		return TRY, nil
	default:
		return Currency{}, fmt.Errorf("%w: ISO %d", ErrUnsupportedCurrency, code)
	}
}
