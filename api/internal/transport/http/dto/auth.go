// Package dto HTTP istek/yanıt yapılarını içerir.
//
// Domain modelleri ASLA doğrudan serileştirilmez ve istek gövdesi ASLA doğrudan
// bir domain/db yapısına bağlanmaz (mass assignment koruması — docs/design.md §15).
package dto

import (
	"net/mail"
	"strings"
	"unicode"
)

// FieldError tek bir alanın doğrulama hatası.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ─────────────────────────── Kayıt ───────────────────────────

type RegisterRequest struct {
	Email        string `json:"email"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	CaptchaToken string `json:"captchaToken"`
	AcceptTerms  bool   `json:"acceptTerms"`
}

func (r *RegisterRequest) Validate() []FieldError {
	var errs []FieldError
	if e := validateEmail(r.Email); e != "" {
		errs = append(errs, FieldError{"email", e})
	}
	if e := validateUsername(r.Username); e != "" {
		errs = append(errs, FieldError{"username", e})
	}
	if strings.TrimSpace(r.Password) == "" {
		errs = append(errs, FieldError{"password", "Şifre zorunludur."})
	}
	if !r.AcceptTerms {
		errs = append(errs, FieldError{"acceptTerms", "Kullanım şartlarını kabul etmelisiniz."})
	}
	return errs
}

// ─────────────────────────── Giriş ───────────────────────────

type LoginRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	CaptchaToken string `json:"captchaToken"`
}

func (r *LoginRequest) Validate() []FieldError {
	var errs []FieldError
	if strings.TrimSpace(r.Email) == "" {
		errs = append(errs, FieldError{"email", "E-posta zorunludur."})
	}
	if r.Password == "" {
		errs = append(errs, FieldError{"password", "Şifre zorunludur."})
	}
	return errs
}

// ─────────────────────────── Token'lı işlemler ───────────────────────────

type TokenRequest struct {
	Token string `json:"token"`
}

func (r *TokenRequest) Validate() []FieldError {
	if strings.TrimSpace(r.Token) == "" {
		return []FieldError{{"token", "Bağlantı geçersiz."}}
	}
	return nil
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

func (r *ForgotPasswordRequest) Validate() []FieldError {
	if e := validateEmail(r.Email); e != "" {
		return []FieldError{{"email", e}}
	}
	return nil
}

type ResetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (r *ResetPasswordRequest) Validate() []FieldError {
	var errs []FieldError
	if strings.TrimSpace(r.Token) == "" {
		errs = append(errs, FieldError{"token", "Bağlantı geçersiz."})
	}
	if strings.TrimSpace(r.Password) == "" {
		errs = append(errs, FieldError{"password", "Şifre zorunludur."})
	}
	return errs
}

// ─────────────────────────── Yanıtlar ───────────────────────────

// Money para değerlerinin DEĞİŞMEZ gösterimi.
//
// Çıplak ondalık sayı ASLA gönderilmez: JavaScript'te float'a dönüşür ve
// 12.50 değeri 12.499999... olabilir (docs/trd.md §9).
type Money struct {
	Minor     int64  `json:"minor"`
	Currency  string `json:"currency"`
	Formatted string `json:"formatted"`
}

type UserResponse struct {
	ID            string  `json:"id"` // public_id (UUID) — sayısal id dışarı verilmez
	Email         string  `json:"email"`
	Username      string  `json:"username"`
	Status        string  `json:"status"`
	EmailVerified bool    `json:"emailVerified"`
	Balance       Money   `json:"balance"`
	Permissions   []string `json:"permissions"`
	CreatedAt     string  `json:"createdAt"`
}

type SessionResponse struct {
	ID         string `json:"id"`
	UserAgent  string `json:"userAgent"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
	Current    bool   `json:"current"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

// ─────────────────────────── Doğrulama yardımcıları ───────────────────────────

func validateEmail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "E-posta zorunludur."
	}
	if len(s) > 254 {
		return "E-posta adresi çok uzun."
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || !strings.Contains(s, ".") {
		return "Geçerli bir e-posta adresi giriniz."
	}
	return ""
}

func validateUsername(s string) string {
	s = strings.TrimSpace(s)
	n := len([]rune(s))
	if n == 0 {
		return "Kullanıcı adı zorunludur."
	}
	if n < 3 || n > 32 {
		return "Kullanıcı adı 3-32 karakter olmalıdır."
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return "Kullanıcı adı yalnız harf, rakam ve alt çizgi içerebilir."
		}
		if r > unicode.MaxASCII {
			return "Kullanıcı adı yalnız İngilizce harf ve rakam içerebilir."
		}
	}
	return ""
}

// ─────────────────────────── Cüzdan ───────────────────────────

type BalanceResponse struct {
	Balance Money `json:"balance"`
}

type LedgerEntryResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	TypeLabel    string `json:"typeLabel"` // Türkçe, kullanıcıya gösterilir
	Amount       Money  `json:"amount"`
	BalanceAfter Money  `json:"balanceAfter"`
	Reference    string `json:"reference,omitempty"`
	Note         string `json:"note,omitempty"`
	CreatedAt    string `json:"createdAt"`
}

type StatementResponse struct {
	Items  []LedgerEntryResponse `json:"items"`
	Total  int64                 `json:"total"`
	Limit  int32                 `json:"limit"`
	Offset int32                 `json:"offset"`
}

type AdjustBalanceRequest struct {
	// AmountMinor kuruş cinsinden; pozitif ekler, negatif düşer.
	// Çıplak ondalık sayı KABUL EDİLMEZ (docs/trd.md §9).
	AmountMinor int64  `json:"amountMinor"`
	Note        string `json:"note"`
}

func (r *AdjustBalanceRequest) Validate() []FieldError {
	var errs []FieldError
	if r.AmountMinor == 0 {
		errs = append(errs, FieldError{"amountMinor", "Tutar sıfır olamaz."})
	}
	if len(strings.TrimSpace(r.Note)) < 5 {
		errs = append(errs, FieldError{"note", "Düzeltme sebebi en az 5 karakter olmalıdır."})
	}
	return errs
}

// ─────────────────────────── Katalog ───────────────────────────

type ServiceResponse struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	IconURL string `json:"iconUrl,omitempty"`
}

type CountryResponse struct {
	ISO2      string `json:"iso2"`
	Name      string `json:"name"`
	PhoneCode string `json:"phoneCode"`
}

// CatalogItemResponse bir servis×ülke kombinasyonunun listeleme görünümü.
//
// FİYAT BURADA YOKTUR. Kullanıcıya gösterilecek fiyat, teklif (quote) uç
// noktasından alınır ve o teklif bir SÖZLEŞMEDİR. Listede "yaklaşık fiyat"
// göstermek, satın alma anında farklı tutar çıkması demektir.
type CatalogItemResponse struct {
	ServiceCode string `json:"serviceCode"`
	ServiceName string `json:"serviceName"`
	IconURL     string `json:"iconUrl,omitempty"`
	CountryISO2 string `json:"countryIso2"`
	CountryName string `json:"countryName"`
	PhoneCode   string `json:"phoneCode"`
	InStock     bool   `json:"inStock"`
}

type CatalogResponse struct {
	Items []CatalogItemResponse `json:"items"`
}
