// Package crypto sağlayıcı API anahtarları gibi sırların veritabanında
// şifreli saklanması için AES-GCM sarmalayıcısı sağlar.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

var (
	ErrKeySize    = errors.New("crypto: anahtar 32 bayt olmalı")
	ErrCiphertext = errors.New("crypto: şifreli metin bozuk veya anahtar yanlış")
)

// SecretBox AES-256-GCM ile şifreleme/çözme.
//
// GCM seçildi çünkü KİMLİK DOĞRULAMALI şifrelemedir: bozulmuş veya
// kurcalanmış bir şifreli metin çözülmeye çalışıldığında hata verir,
// sessizce çöp üretmez. Düz AES-CBC bunu yapmazdı.
type SecretBox struct{ aead cipher.AEAD }

// New 32 baytlık bir anahtarla kutu oluşturur.
func New(key []byte) (*SecretBox, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("%w (verilen: %d)", ErrKeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: şifre bloğu oluşturulamadı: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: GCM oluşturulamadı: %w", err)
	}
	return &SecretBox{aead: aead}, nil
}

// Seal düz metni şifreler. Çıktı: nonce || şifreli metin || etiket.
//
// Nonce HER ÇAĞRIDA rastgele üretilir ve çıktının başına eklenir. Sabit veya
// tekrar eden bir nonce GCM'de felakettir: aynı anahtarla iki mesaj
// şifrelenirse ikisi de çözülebilir hale gelir.
func (s *SecretBox) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce üretilemedi: %w", err)
	}
	return s.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// SealString metni şifreler.
func (s *SecretBox) SealString(plaintext string) ([]byte, error) {
	return s.Seal([]byte(plaintext))
}

// Open şifreli metni çözer.
func (s *SecretBox) Open(sealed []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n+s.aead.Overhead() {
		return nil, ErrCiphertext
	}
	nonce, ct := sealed[:n], sealed[n:]
	out, err := s.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		// Ham hata sızdırılmaz: "kimlik doğrulama başarısız" mı "anahtar yanlış"
		// mı ayrımı saldırgana bilgi verir.
		return nil, ErrCiphertext
	}
	return out, nil
}

// OpenString şifreli metni çözüp dizgi döner.
func (s *SecretBox) OpenString(sealed []byte) (string, error) {
	b, err := s.Open(sealed)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Mask bir sırrı log ve arayüz için maskeler: "9cA5…e04c"
func Mask(secret string) string {
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "…" + secret[len(secret)-4:]
}
