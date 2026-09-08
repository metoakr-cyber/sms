package auth

import (
	"strings"
	"unicode"
)

// MinPasswordLength — docs/trd.md FR-100.
// Karmaşıklık kuralı (büyük harf + rakam + sembol zorunluluğu) BİLİNÇLİ olarak
// yoktur: kullanıcıyı "Parola1!" gibi tahmin edilebilir kalıplara iter.
// NIST SP 800-63B uzunluk + yaygın-şifre kontrolünü önerir; onu uyguluyoruz.
const MinPasswordLength = 10

// PasswordProblem şifrenin neden reddedildiğini söyler.
type PasswordProblem int

const (
	PasswordOK PasswordProblem = iota
	PasswordTooShort
	PasswordTooCommon
	PasswordTooSimple        // tek karakter tekrarı veya ardışık dizi
	PasswordContainsIdentity // e-posta veya kullanıcı adını içeriyor
)

// Message kullanıcıya gösterilecek Türkçe açıklamayı döner.
func (p PasswordProblem) Message() string {
	switch p {
	case PasswordTooShort:
		return "Şifreniz en az 10 karakter olmalı."
	case PasswordTooCommon:
		return "Bu şifre çok yaygın kullanılıyor, lütfen başka bir şifre seçin."
	case PasswordTooSimple:
		return "Şifreniz çok basit bir örüntü içeriyor, lütfen başka bir şifre seçin."
	case PasswordContainsIdentity:
		return "Şifreniz e-posta adresinizi veya kullanıcı adınızı içeremez."
	default:
		return ""
	}
}

// CheckPasswordStrength şifreyi doğrular. identity alanları (e-posta, kullanıcı adı)
// şifrenin içinde geçmemelidir.
func CheckPasswordStrength(password string, identity ...string) PasswordProblem {
	if len([]rune(password)) < MinPasswordLength {
		return PasswordTooShort
	}

	lower := strings.ToLower(password)

	if isCommonPassword(lower) {
		return PasswordTooCommon
	}
	if isTooSimple(lower) {
		return PasswordTooSimple
	}
	for _, id := range identity {
		if id == "" {
			continue
		}
		id = strings.ToLower(id)
		// e-postanın yalnız yerel kısmı anlamlıdır: "ali@gmail.com" için "ali"
		if i := strings.IndexByte(id, '@'); i > 0 {
			id = id[:i]
		}
		if len(id) >= 4 && strings.Contains(lower, id) {
			return PasswordContainsIdentity
		}
	}
	return PasswordOK
}

// isTooSimple tek karakter tekrarı, ardışık dizi ve klavye yürüyüşü tespit eder.
func isTooSimple(s string) bool {
	// tek karakterden oluşuyor: "aaaaaaaaaa"
	allSame := true
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return true
	}

	// tamamen ardışık: "1234567890", "abcdefghij" (artan veya azalan)
	if len(s) >= MinPasswordLength {
		asc, desc := true, true
		for i := 1; i < len(s); i++ {
			if s[i] != s[i-1]+1 {
				asc = false
			}
			if s[i] != s[i-1]-1 {
				desc = false
			}
			if !asc && !desc {
				break
			}
		}
		if asc || desc {
			return true
		}
	}

	// klavye yürüyüşü
	for _, row := range keyboardRows {
		if len(s) >= 8 && strings.Contains(row, s) {
			return true
		}
	}

	// yalnız rakam ve 10-12 hane: telefon/tarih benzeri, düşük entropi
	onlyDigits := true
	for _, r := range s {
		if !unicode.IsDigit(r) {
			onlyDigits = false
			break
		}
	}
	return onlyDigits
}

var keyboardRows = []string{
	"qwertyuiop", "asdfghjkl", "zxcvbnm",
	"1234567890",
	"qwertyuıopğü", "asdfghjklşi", "zxcvbnmöç", // TR klavye
}
