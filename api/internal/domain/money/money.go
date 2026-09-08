// Package money para değerlerini TAM SAYI en küçük birim olarak temsil eder.
//
// Temel kural: para asla float64 ile taşınmaz veya hesaplanmaz.
// test: money_test.go#TestNoFloatDrift
// Kayan noktalı aritmetik ikili tabanda 0.1 gibi ondalıkları temsil edemez;
// bir ödeme sisteminde bu birikimli hata ve mutabakat sapması demektir.
//
// Bkz. docs/design.md §5.1, ADR-005, ADR-026.
package money

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

// Money belirli bir para birimindeki bir tutarı, o birimin en küçük biriminde
// tam sayı olarak tutar. Sıfır değeri (Money{}) geçersizdir; New veya Zero kullanın.
type Money struct {
	minor    int64
	currency Currency
}

// New verilen en küçük birim tutarıyla bir Money üretir.
//
//	money.New(1250, money.TRY) // 12,50 ₺
func New(minor int64, c Currency) Money {
	if !c.valid() {
		panic("money.New: geçersiz para birimi — programlama hatası")
	}
	return Money{minor: minor, currency: c}
}

// Zero verilen para biriminde sıfır tutar döner.
func Zero(c Currency) Money { return New(0, c) }

// Minor tutarı en küçük birimde döner. Veritabanına bu değer yazılır.
func (m Money) Minor() int64 { return m.minor }

// Currency tutarın para birimini döner.
func (m Money) Currency() Currency { return m.currency }

// IsZero tutar sıfırsa true döner.
func (m Money) IsZero() bool { return m.minor == 0 }

// IsNegative tutar sıfırdan küçükse true döner.
func (m Money) IsNegative() bool { return m.minor < 0 }

// IsPositive tutar sıfırdan büyükse true döner.
func (m Money) IsPositive() bool { return m.minor > 0 }

// sameCurrency iki tutarın aynı para biriminde olduğunu doğrular.
func (m Money) sameCurrency(o Money) error {
	if m.currency != o.currency {
		return fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.currency, o.currency)
	}
	return nil
}

// Add iki tutarı toplar. Para birimleri farklıysa veya taşma olursa hata döner.
func (m Money) Add(o Money) (Money, error) {
	if err := m.sameCurrency(o); err != nil {
		return Money{}, err
	}
	sum := m.minor + o.minor
	// Taşma tespiti: işaretler aynıysa ve sonucun işareti değiştiyse taşmıştır.
	if (m.minor > 0 && o.minor > 0 && sum < 0) || (m.minor < 0 && o.minor < 0 && sum >= 0) {
		return Money{}, fmt.Errorf("%w: %d + %d", ErrOverflow, m.minor, o.minor)
	}
	return Money{minor: sum, currency: m.currency}, nil
}

// Sub iki tutarı çıkarır.
func (m Money) Sub(o Money) (Money, error) {
	neg, err := o.Neg()
	if err != nil {
		return Money{}, err
	}
	return m.Add(neg)
}

// Neg tutarın işaretini çevirir.
func (m Money) Neg() (Money, error) {
	if m.minor == math.MinInt64 {
		return Money{}, fmt.Errorf("%w: -(%d)", ErrOverflow, m.minor)
	}
	return Money{minor: -m.minor, currency: m.currency}, nil
}

// Cmp karşılaştırma yapar: -1 (küçük), 0 (eşit), 1 (büyük).
func (m Money) Cmp(o Money) (int, error) {
	if err := m.sameCurrency(o); err != nil {
		return 0, err
	}
	switch {
	case m.minor < o.minor:
		return -1, nil
	case m.minor > o.minor:
		return 1, nil
	default:
		return 0, nil
	}
}

// Max iki tutardan büyüğünü döner.
func Max(a, b Money) (Money, error) {
	c, err := a.Cmp(b)
	if err != nil {
		return Money{}, err
	}
	if c >= 0 {
		return a, nil
	}
	return b, nil
}

// rat tutarı tam bir rasyonel sayıya çevirir (birim cinsinden, ölçek uygulanmış).
func (m Money) rat() *big.Rat {
	return new(big.Rat).SetFrac(
		big.NewInt(m.minor),
		pow10(m.currency.Scale),
	)
}

// fromRat bir rasyonel sayıyı verilen para biriminde Money'ye çevirir.
// Yuvarlama yönü AÇIKÇA belirtilir — varsayılan yoktur.
func fromRat(r *big.Rat, c Currency, mode Rounding) (Money, error) {
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(pow10(c.Scale)))
	i, err := roundRat(scaled, mode)
	if err != nil {
		return Money{}, err
	}
	if !i.IsInt64() {
		return Money{}, fmt.Errorf("%w: %s", ErrOverflow, i.String())
	}
	return Money{minor: i.Int64(), currency: c}, nil
}

// Convert tutarı başka bir para birimine, verilen oranla çevirir.
// Oran "1 birim kaynak = X birim hedef" anlamındadır.
//
//	usd.Convert(money.TRY, rate) // 1 USD = 43.20 TRY
func (m Money) Convert(to Currency, rate Rate, mode Rounding) (Money, error) {
	if !to.valid() {
		return Money{}, ErrInvalidCurrency
	}
	if rate.IsNegative() {
		return Money{}, ErrNegativeRate
	}
	product := new(big.Rat).Mul(m.rat(), rate.rat())
	return fromRat(product, to, mode)
}

// MulRate tutarı bir oranla çarpar, para birimi değişmez.
// Kâr marjı uygulamak için kullanılır: MulRate(1.40) = %40 marj.
func (m Money) MulRate(rate Rate, mode Rounding) (Money, error) {
	if rate.IsNegative() {
		return Money{}, ErrNegativeRate
	}
	product := new(big.Rat).Mul(m.rat(), rate.rat())
	return fromRat(product, m.currency, mode)
}

// String hata ayıklama içindir; kullanıcıya gösterilmez.
// Kullanıcıya dönük biçimleme sunum katmanının işidir.
func (m Money) String() string {
	if m.currency.Code == "" {
		return "Money(<geçersiz>)"
	}
	neg := m.minor < 0
	v := m.minor
	if neg {
		v = -v
	}
	div := pow10(m.currency.Scale).Int64()
	whole := v / div
	frac := v % div
	var sb strings.Builder
	if neg {
		sb.WriteByte('-')
	}
	fmt.Fprintf(&sb, "%d", whole)
	if m.currency.Scale > 0 {
		fmt.Fprintf(&sb, ",%0*d", m.currency.Scale, frac)
	}
	sb.WriteByte(' ')
	sb.WriteString(m.currency.Code)
	return sb.String()
}

func pow10(n int32) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}
