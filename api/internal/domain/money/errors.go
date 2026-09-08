package money

import "errors"

var (
	// ErrCurrencyMismatch farklı para birimlerinde aritmetik denendiğinde döner.
	ErrCurrencyMismatch = errors.New("money: para birimleri uyuşmuyor")
	// ErrInvalidCurrency tanımsız veya bozuk bir Currency kullanıldığında döner.
	ErrInvalidCurrency = errors.New("money: geçersiz para birimi")
	// ErrUnsupportedCurrency desteklenmeyen bir ISO kodu için döner.
	ErrUnsupportedCurrency = errors.New("money: desteklenmeyen para birimi")
	// ErrOverflow int64 taşması olduğunda döner. Sessizce sarmalamak yerine hata veririz.
	ErrOverflow = errors.New("money: tam sayı taşması")
	// ErrNegativeRate negatif bir oran verildiğinde döner.
	ErrNegativeRate = errors.New("money: oran negatif olamaz")
)
