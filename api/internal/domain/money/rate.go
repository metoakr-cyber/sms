package money

import (
	"fmt"
	"math/big"
	"strings"
)

// Rate bir çarpanı TAM olarak temsil eder (kur, kâr marjı, düzeltme katsayısı).
//
// test: money_test.go#TestNoFloatDrift
// float64 kullanılmaz: 1.1 gibi bir değer ikili tabanda tam temsil edilemez ve
// zincirleme çarpımda hata birikir. big.Rat ile tüm ara hesaplar kayıpsızdır;
// yuvarlama yalnızca en sonda, açıkça belirtilen yönde bir kez yapılır.
type Rate struct{ r *big.Rat }

// NewRate bir pay/payda ikilisinden oran üretir.
func NewRate(num, den int64) (Rate, error) {
	if den == 0 {
		return Rate{}, fmt.Errorf("money: oran paydası sıfır olamaz")
	}
	return Rate{r: new(big.Rat).SetFrac64(num, den)}, nil
}

// RateFromString "43.204512" gibi ondalık bir metinden oran üretir.
// Veritabanından gelen NUMERIC değerleri için birincil giriş noktasıdır.
func RateFromString(s string) (Rate, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return Rate{}, fmt.Errorf("money: boş oran")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return Rate{}, fmt.Errorf("money: oran ayrıştırılamadı: %q", s)
	}
	return Rate{r: r}, nil
}

// RateOne 1.0 oranını döner (değişiklik yok).
func RateOne() Rate { return Rate{r: new(big.Rat).SetInt64(1)} }

// MarginRate yüzde marjdan çarpan üretir: %40 → 1.40
func MarginRate(percent string) (Rate, error) {
	p, err := RateFromString(percent)
	if err != nil {
		return Rate{}, err
	}
	if p.r.Sign() < 0 {
		return Rate{}, fmt.Errorf("%w: marj %%%s", ErrNegativeRate, percent)
	}
	hundredth := new(big.Rat).Quo(p.r, new(big.Rat).SetInt64(100))
	return Rate{r: new(big.Rat).Add(new(big.Rat).SetInt64(1), hundredth)}, nil
}

// Mul iki oranı çarpar.
func (r Rate) Mul(o Rate) Rate {
	return Rate{r: new(big.Rat).Mul(r.rat(), o.rat())}
}

// IsNegative oran negatifse true döner.
func (r Rate) IsNegative() bool { return r.rat().Sign() < 0 }

// IsZero oran sıfırsa true döner.
func (r Rate) IsZero() bool { return r.rat().Sign() == 0 }

func (r Rate) rat() *big.Rat {
	if r.r == nil {
		return new(big.Rat) // sıfır değeri = 0
	}
	return r.r
}

// String hata ayıklama ve log için ondalık gösterim döner.
func (r Rate) String() string { return r.rat().FloatString(8) }
