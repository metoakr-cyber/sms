// Package auth kimlik doğrulamanın saf iş kurallarını içerir:
// şifre özetleme, güç kontrolü ve token üretimi. G/Ç yapmaz.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parametreleri — OWASP 2024 önerisiyle uyumlu.
// bcrypt yerine argon2id: bellek-zor olduğu için GPU ile kaba kuvvet
// saldırısına belirgin şekilde dirençlidir.
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024 // 64 MiB
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

var (
	ErrHashFormat  = errors.New("auth: şifre özeti biçimi tanınmıyor")
	ErrHashVersion = errors.New("auth: desteklenmeyen argon2 sürümü")
)

// HashPassword şifreyi argon2id ile özetler ve PHC biçiminde döner:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
//
// Parametreler özetin İÇİNDE saklanır; ileride maliyeti artırdığımızda eski
// özetler doğrulanmaya devam eder.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: tuz üretilemedi: %w", err)
	}
	p := parallelism()
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, p, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, p,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword bir şifreyi özetle karşılaştırır.
//
// Karşılaştırma SABİT ZAMANLIDIR: normal bayt karşılaştırması ilk farklı baytta
// döneceği için saldırgana özet hakkında zamanlama bilgisi sızdırırdı.
func VerifyPassword(password, encoded string) (bool, error) {
	params, salt, want, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt,
		params.time, params.memory, params.parallelism, uint32(len(want)))

	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NeedsRehash özetin güncel parametrelerle üretilip üretilmediğini söyler.
// Maliyeti artırdığımızda kullanıcı bir sonraki girişinde sessizce yükseltilir.
func NeedsRehash(encoded string) bool {
	params, _, _, err := decodeHash(encoded)
	if err != nil {
		return true
	}
	return params.memory != argonMemory || params.time != argonTime
}

type hashParams struct {
	memory      uint32
	time        uint32
	parallelism uint8
}

func decodeHash(encoded string) (hashParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return hashParams{}, nil, nil, ErrHashFormat
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return hashParams{}, nil, nil, ErrHashFormat
	}
	if version != argon2.Version {
		return hashParams{}, nil, nil, ErrHashVersion
	}

	var p hashParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.parallelism); err != nil {
		return hashParams{}, nil, nil, ErrHashFormat
	}
	if p.memory == 0 || p.time == 0 || p.parallelism == 0 {
		return hashParams{}, nil, nil, ErrHashFormat
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return hashParams{}, nil, nil, ErrHashFormat
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) == 0 {
		return hashParams{}, nil, nil, ErrHashFormat
	}
	return p, salt, hash, nil
}

func parallelism() uint8 {
	n := runtime.NumCPU()
	if n > 4 {
		n = 4
	}
	if n < 1 {
		n = 1
	}
	return uint8(n)
}
