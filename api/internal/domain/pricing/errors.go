package pricing

import "errors"

var (
	// ErrNoRule hiçbir fiyat kuralı bulunamadı.
	//
	// test: quote_integration_test.go#TestNoFXRateStopsSelling
// Bu durumda satış YAPILMAZ. "Kural yoksa maliyetine sat" veya
	// "varsayılan %X uygula" davranışı kabul edilemez: ilki zarar ettirir,
	// ikincisi sessizce yanlış fiyat üretir. Doğru davranış hizmeti
	// durdurmak ve alarm üretmektir.
	ErrNoRule = errors.New("pricing: uygulanabilir fiyat kuralı yok")

	// ErrInvalidInput hesaplama girdileri geçersiz.
	ErrInvalidInput = errors.New("pricing: geçersiz girdi")
)
