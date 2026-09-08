package config

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// decodeKey base64 (standart / ham / URL güvenli) veya hex kodlu bir anahtarı çözer.
//
// Kodlamalar BELİRSİZDİR: yalnızca [a-f0-9] içeren bir hex dizesi aynı zamanda
// geçerli base64'tür ve farklı bir bayt dizisi üretir. Örneğin 64 karakterlik
// "abab..." hex olarak 32 bayt, base64 olarak 48 bayt verir.
//
// Belirsizliği BEKLENEN UZUNLUKLA çözeriz: hangi kodlama tam olarak wantBytes
// bayt üretiyorsa o doğrudur. Hiçbiri üretmiyorsa hata döneriz — sessizce
// yanlış uzunlukta bir anahtar kabul etmeyiz.
func decodeKey(s string, wantBytes int) ([]byte, error) {
	decoders := []struct {
		name string
		fn   func(string) ([]byte, error)
	}{
		{"hex", hex.DecodeString},
		{"base64", base64.StdEncoding.DecodeString},
		{"base64-raw", base64.RawStdEncoding.DecodeString},
		{"base64-url", base64.URLEncoding.DecodeString},
		{"base64-url-raw", base64.RawURLEncoding.DecodeString},
	}

	var sawAny bool
	for _, d := range decoders {
		b, err := d.fn(s)
		if err != nil {
			continue
		}
		sawAny = true
		if len(b) == wantBytes {
			return b, nil
		}
	}
	if !sawAny {
		return nil, fmt.Errorf("base64 veya hex olarak çözülemedi")
	}
	return nil, fmt.Errorf("çözüldü ama %d bayt üretmedi", wantBytes)
}
