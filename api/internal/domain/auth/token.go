package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// TokenBytes üretilen ham token'ın bayt uzunluğu. 32 bayt = 256 bit entropi.
const TokenBytes = 32

// Token e-posta doğrulama ve şifre sıfırlama için tek kullanımlık bir sırdır.
type Token struct {
	// Plain kullanıcıya e-posta ile gönderilen değer. ASLA saklanmaz.
	Plain string
	// Hash veritabanında saklanan SHA-256 özeti.
	// Veritabanı sızarsa token'lar kullanılamaz olsun diye ham değer tutulmaz.
	Hash []byte
}

// NewToken kriptografik olarak güvenli yeni bir token üretir.
func NewToken() (Token, error) {
	b := make([]byte, TokenBytes)
	if _, err := rand.Read(b); err != nil {
		return Token{}, fmt.Errorf("auth: token üretilemedi: %w", err)
	}
	plain := base64.RawURLEncoding.EncodeToString(b)
	return Token{Plain: plain, Hash: HashToken(plain)}, nil
}

// HashToken bir token'ın saklanacak özetini üretir.
//
// Burada argon2 KULLANILMAZ: token zaten 256 bit rastgeledir, kaba kuvvetle
// tahmin edilemez. Yavaş bir özet yalnız doğrulamayı yavaşlatırdı.
func HashToken(plain string) []byte {
	sum := sha256.Sum256([]byte(plain))
	return sum[:]
}

// TokenMatches sabit zamanlı karşılaştırma yapar.
func TokenMatches(plain string, storedHash []byte) bool {
	got := HashToken(plain)
	return subtle.ConstantTimeCompare(got, storedHash) == 1
}

// NewSessionID opak bir oturum kimliği üretir.
func NewSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: oturum kimliği üretilemedi: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
