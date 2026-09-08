// Package dto HTTP istek/yanıt yapılarını içerir.
//
// Domain modelleri doğrudan serileştirilmez ve istek gövdesi doğrudan
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
// Çıplak ondalık sayı gönderilmez (test: scripts/smoke-auth.sh — bakiye
// yanıtı {minor,currency,formatted}): JavaScript'te float'a dönüşür ve
// 12.50 değeri 12.499999... olabilir (docs/trd.md §9).
type Money struct {
	Minor     int64  `json:"minor"`
	Currency  string `json:"currency"`
	Formatted string `json:"formatted"`
}

type UserResponse struct {
	ID            string   `json:"id"` // public_id (UUID) — sayısal id dışarı verilmez
	Email         string   `json:"email"`
	Username      string   `json:"username"`
	Status        string   `json:"status"`
	EmailVerified bool     `json:"emailVerified"`
	Balance       Money    `json:"balance"`
	Permissions   []string `json:"permissions"`
	CreatedAt     string   `json:"createdAt"`
}

type SessionResponse struct {
	// ID bir HANDLE'dır, ham oturum kimliği DEĞİL. Ham kimlik bir taşıyıcı
	// token'dır ve dışarı verilmez (docs/design.md §10).
	ID         string `json:"id"`
	UserAgent  string `json:"userAgent"`
	IP         string `json:"ip,omitempty"`
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

// ServiceSummaryResponse servis ızgarası için özet.
//
// Fiyat BURADA YOKTUR ve olmayacaktır: fiyat kullanıcıya ÖZELDİR ve yalnız
// teklif (quote) ile verilir. Izgarada bir fiyat göstermek, teklifle
// uyuşmadığında güven kaybettirir.
type ServiceSummaryResponse struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	IconURL      string `json:"iconUrl,omitempty"`
	CountryCount int64  `json:"countryCount"`
}

type BalanceResponse struct {
	Balance Money `json:"balance"`
	// AlreadyApplied true ise bu istek bir TEKRAR'dı ve yeni bir hareket
	// oluşmadı. Bu bilgi olmadan yönetici "işlem geçti mi geçmedi mi"
	// ayrımını yapamaz ve tekrar denemeye yönelir.
	AlreadyApplied bool `json:"alreadyApplied,omitempty"`
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

	// IdempotencyKey ZORUNLUDUR ve İSTEMCİ tarafından üretilir.
	//
	// Sunucu tarafında üretilen bir korelasyon kimliğinden (X-Request-Id)
	// türetilemez: o kimlik her istekte yenilenir, dolayısıyla ağ hatası
	// sonrası tekrar gönderilen aynı düzeltme İKİ KEZ uygulanırdı.
	// Anahtar mantıksal işlemi tanımlamalı, taşıyıcısını değil.
	IdempotencyKey string `json:"idempotencyKey"`
}

func (r *AdjustBalanceRequest) Validate() []FieldError {
	var errs []FieldError
	if r.AmountMinor == 0 {
		errs = append(errs, FieldError{"amountMinor", "Tutar sıfır olamaz."})
	}
	if len(strings.TrimSpace(r.Note)) < 5 {
		errs = append(errs, FieldError{"note", "Düzeltme sebebi en az 5 karakter olmalıdır."})
	}
	if k := strings.TrimSpace(r.IdempotencyKey); k == "" {
		errs = append(errs, FieldError{"idempotencyKey",
			"Çift işlem koruması için benzersiz bir anahtar gönderilmelidir."})
	} else if len(k) < 8 || len(k) > 64 {
		errs = append(errs, FieldError{"idempotencyKey", "Anahtar 8-64 karakter olmalıdır."})
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

// QuoteResponse fiyat teklifi.
//
// providerId ve maliyet BULUNMAZ — bilinçli. Satın alma yalnız quoteId ile
// yapılır; istemci hangi sağlayıcıdan hangi fiyata alacağını belirleyemez
// (docs/trd.md KK-305).
type QuoteResponse struct {
	QuoteID   string `json:"quoteId"`
	Price     Money  `json:"price"`
	Stock     int    `json:"stock"`
	ExpiresAt string `json:"expiresAt"`
	ExpiresIn int    `json:"expiresIn"` // saniye
}

// ─────────────────────────── Siparişler ───────────────────────────

// CreateOrderRequest satın alma isteği.
//
// GÖVDEDE BAŞKA HİÇBİR ALAN YOKTUR. Fiyat, sağlayıcı ve maliyet istemciden
// gelmez (CLAUDE.md değişmez #9); teklif kimliği hepsini sunucuda belirler.
type CreateOrderRequest struct {
	QuoteID string `json:"quoteId"`
}

func (r *CreateOrderRequest) Validate() []FieldError {
	if strings.TrimSpace(r.QuoteID) == "" {
		return []FieldError{{"quoteId", "Fiyat teklifi seçilmedi."}}
	}
	return nil
}

// OrderMessageResponse bir SMS mesajı.
type OrderMessageResponse struct {
	Code       string `json:"code"`
	Body       string `json:"body"`
	Sender     string `json:"sender,omitempty"`
	ReceivedAt string `json:"receivedAt"`
}

// OrderResponse sipariş — dışa açık görünüm.
//
// Sayısal id, sağlayıcı kimliği, maliyet ve kur BURADA YOKTUR ve olmayacaktır.
type OrderResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	PhoneNumber string `json:"phoneNumber"`
	ServiceCode string `json:"serviceCode"`
	ServiceName string `json:"serviceName"`
	CountryISO2 string `json:"countryIso2"`
	CountryName string `json:"countryName"`
	PhoneCode   string `json:"phoneCode,omitempty"`
	Price       Money  `json:"price"`

	ExpiresAt string `json:"expiresAt"`
	// ExpiresIn sunucunun hesapladığı kalan saniye. İstemci kendi saatiyle
	// hesaplamaz: saat kayması ve donmuş sekme yanlış sonuç verir.
	ExpiresIn     int    `json:"expiresIn"`
	CancellableAt string `json:"cancellableAt"`
	CancellableIn int    `json:"cancellableIn"`
	CreatedAt     string `json:"createdAt"`

	Messages []OrderMessageResponse `json:"messages"`
}

type OrderListResponse struct {
	Items  []OrderResponse `json:"items"`
	Total  int64           `json:"total"`
	Limit  int32           `json:"limit"`
	Offset int32           `json:"offset"`
}

// ─────────────────────────── Kiralama ───────────────────────────

// RentalServiceResponse kiralık ızgarası için servis özeti.
type RentalServiceResponse struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	IconURL       string `json:"iconUrl,omitempty"`
	CountryCount  int64  `json:"countryCount"`
	DurationCount int64  `json:"durationCount"`
}

// RentalDurationResponse bir servis × ülke için kiralanabilir süre.
//
// FİYAT BURADA YOKTUR: fiyat kullanıcıya ÖZELDİR ve yalnız teklif ile verilir.
// Listede bir fiyat göstermek, teklifle uyuşmadığında güven kaybettirir.
type RentalDurationResponse struct {
	Minutes int32  `json:"minutes"`
	Hours   int32  `json:"hours"`
	Days    int32  `json:"days"`
	Label   string `json:"label"`
	InStock bool   `json:"inStock"`
}

// ─────────────────────────── Yönetim ───────────────────────────

// AdminUserResponse yönetim kullanıcı listesi satırı.
//
// 🔴 password_hash BURADA YOKTUR ve olmayacaktır.
type AdminUserResponse struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	Username      string   `json:"username"`
	Status        string   `json:"status"`
	EmailVerified bool     `json:"emailVerified"`
	Balance       Money    `json:"balance"`
	Roles         []string `json:"roles"`
	OrderCount    int64    `json:"orderCount"`
	CreatedAt     string   `json:"createdAt"`
}

type AdminUserListResponse struct {
	Items  []AdminUserResponse `json:"items"`
	Total  int64               `json:"total"`
	Limit  int32               `json:"limit"`
	Offset int32               `json:"offset"`
}

type SetUserStatusRequest struct {
	Status string `json:"status"`
}

// AdminProviderResponse sağlayıcı — API ANAHTARI YOK.
//
// Anahtarın şifreli hâli bile dönmez: panelde gösterilecek bir şey değil ve
// varlığı/uzunluğu bilgi sızdırır. Yöneticinin bilmesi gereken tek şey
// anahtarın TANIMLI OLUP OLMADIĞI.
type AdminProviderResponse struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Protocol       string   `json:"protocol"`
	BaseURL        string   `json:"baseUrl"`
	IsActive       bool     `json:"isActive"`
	Priority       int32    `json:"priority"`
	HasAPIKey      bool     `json:"hasApiKey"`
	CostMultiplier string   `json:"costMultiplier"`
	Capabilities   []string `json:"capabilities"`
	Balance        Money    `json:"balance"`
}

type UpdateProviderRequest struct {
	BaseURL        string `json:"baseUrl"`
	IsActive       bool   `json:"isActive"`
	Priority       int32  `json:"priority"`
	CostMultiplier string `json:"costMultiplier"`
}

func (r *UpdateProviderRequest) Validate() []FieldError {
	var errs []FieldError
	if r.Priority < 0 || r.Priority > 10000 {
		errs = append(errs, FieldError{"priority", "Öncelik 0-10000 arasında olmalıdır."})
	}
	if strings.TrimSpace(r.CostMultiplier) == "" {
		errs = append(errs, FieldError{"costMultiplier", "Çarpan zorunludur."})
	}
	return errs
}

// SetAPIKeyRequest sağlayıcı API anahtarı.
//
// AYRI BİR İSTEK: diğer ayarlarla birlikte gönderilseydi, ayar değiştiren
// her kayıt anahtarı da yazardı ve boş bir alan onu SİLERDİ.
type SetAPIKeyRequest struct {
	APIKey string `json:"apiKey"`
}

type SetActiveRequest struct {
	IsActive bool `json:"isActive"`
}

// DepositMethodResponse ödeme yöntemi.
type DepositMethodResponse struct {
	ID           string            `json:"id"`
	Code         string            `json:"code"`
	Kind         string            `json:"kind"`
	Name         string            `json:"name"`
	Instructions string            `json:"instructions"`
	Config       map[string]string `json:"config"`
	MinAmount    Money             `json:"minAmount"`
	MaxAmount    Money             `json:"maxAmount"`
	IsActive     bool              `json:"isActive"`
	SortOrder    int32             `json:"sortOrder"`
	// MissingFields aktifleştirmeyi engelleyen boş alanlar. Yöneticiye
	// "neden aktifleştiremiyorum?" sorusunu sordurmadan cevap verir.
	MissingFields []string `json:"missingFields,omitempty"`
}

type DepositMethodRequest struct {
	Code           string            `json:"code"`
	Kind           string            `json:"kind"`
	Name           string            `json:"name"`
	Instructions   string            `json:"instructions"`
	Config         map[string]string `json:"config"`
	MinAmountMinor int64             `json:"minAmountMinor"`
	MaxAmountMinor int64             `json:"maxAmountMinor"`
	SortOrder      int32             `json:"sortOrder"`
}

func (r *DepositMethodRequest) Validate(isCreate bool) []FieldError {
	var errs []FieldError
	if isCreate {
		code := strings.TrimSpace(r.Code)
		if code == "" {
			errs = append(errs, FieldError{"code", "Kod zorunludur."})
		} else if len(code) > 40 {
			errs = append(errs, FieldError{"code", "Kod en fazla 40 karakter olabilir."})
		}
		if r.Kind != "BANK_TRANSFER" && r.Kind != "CRYPTO" {
			errs = append(errs, FieldError{"kind", "Yöntem tipi BANK_TRANSFER veya CRYPTO olmalıdır."})
		}
	}
	if strings.TrimSpace(r.Name) == "" {
		errs = append(errs, FieldError{"name", "Ad zorunludur."})
	}
	if r.MinAmountMinor < 0 {
		errs = append(errs, FieldError{"minAmountMinor", "En az tutar negatif olamaz."})
	}
	if r.MaxAmountMinor < 0 {
		errs = append(errs, FieldError{"maxAmountMinor", "En çok tutar negatif olamaz."})
	}
	if r.MaxAmountMinor > 0 && r.MaxAmountMinor < r.MinAmountMinor {
		errs = append(errs, FieldError{"maxAmountMinor", "En çok tutar, en az tutardan küçük olamaz."})
	}
	return errs
}
