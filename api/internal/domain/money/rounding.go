package money

import (
	"fmt"
	"math/big"
)

// Rounding yuvarlama yönünü belirtir.
//
// Bu tipin varsayılan değeri YOKTUR: her çağrı yönü açıkça seçmek zorundadır.
// Sessiz bir varsayılan, para hesaplarında en tehlikeli hata kaynağıdır.
type Rounding int

const (
	// RoundUp sonsuza doğru yuvarlar (ceil). Satış fiyatı hesabında kullanılır:
	// yuvarlama farkı her zaman platform lehine olmalıdır.
	RoundUp Rounding = iota + 1
	// RoundDown sıfıra doğru yuvarlar (truncate).
	RoundDown
	// RoundHalfEven bankacı yuvarlaması. İstatistik ve raporlamada sapmayı önler.
	RoundHalfEven
)

func (r Rounding) String() string {
	switch r {
	case RoundUp:
		return "up"
	case RoundDown:
		return "down"
	case RoundHalfEven:
		return "half-even"
	default:
		return "invalid"
	}
}

// roundRat bir rasyonel sayıyı verilen yönde tam sayıya yuvarlar.
func roundRat(r *big.Rat, mode Rounding) (*big.Int, error) {
	num, den := r.Num(), r.Denom()
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Sign() == 0 {
		return q, nil // tam bölünüyor, yuvarlama gerekmiyor
	}

	switch mode {
	case RoundDown:
		// QuoRem zaten sıfıra doğru kırpar.
		return q, nil

	case RoundUp:
		if r.Sign() > 0 {
			return q.Add(q, big.NewInt(1)), nil
		}
		return q, nil // negatifte sıfıra doğru = yukarı

	case RoundHalfEven:
		// |rem| * 2 ile den karşılaştırılır.
		twice := new(big.Int).Abs(rem)
		twice.Lsh(twice, 1)
		cmp := twice.Cmp(new(big.Int).Abs(den))
		step := big.NewInt(1)
		if r.Sign() < 0 {
			step = big.NewInt(-1)
		}
		switch {
		case cmp > 0:
			return q.Add(q, step), nil
		case cmp < 0:
			return q, nil
		default: // tam yarım → çifte yuvarla
			if q.Bit(0) == 0 {
				return q, nil
			}
			return q.Add(q, step), nil
		}

	default:
		return nil, fmt.Errorf("money: yuvarlama yönü belirtilmemiş (mode=%d)", mode)
	}
}
