package errors

import "net/http"

// Uygulama genelinde kullanılan hata kataloğu.
// Kullanıcıya gösterilen metinler Türkçedir ve teknik detay içermez.
var (
	// ─── Genel ───
	ErrInternal = New(KindInternal, "INTERNAL",
		"Beklenmeyen bir hata oluştu. Lütfen tekrar deneyin.")
	ErrValidation = NewStatus(KindDomain, "VALIDATION",
		"Gönderilen bilgiler geçersiz.", http.StatusUnprocessableEntity)
	ErrNotFound = New(KindNotFound, "NOT_FOUND",
		"Kayıt bulunamadı.")
	ErrRateLimited = New(KindRateLimit, "RATE_LIMITED",
		"Çok fazla istek gönderdiniz. Lütfen biraz bekleyin.")

	// ─── Kimlik ve yetki ───
	ErrUnauthenticated = New(KindAuth, "UNAUTHENTICATED",
		"Bu işlem için giriş yapmalısınız.")
	ErrForbidden = NewStatus(KindAuth, "FORBIDDEN",
		"Bu işlem için yetkiniz yok.", http.StatusForbidden)
	ErrInvalidCredentials = New(KindAuth, "INVALID_CREDENTIALS",
		"E-posta veya şifre hatalı.")
	ErrAccountSuspended = NewStatus(KindAuth, "ACCOUNT_SUSPENDED",
		"Hesabınız askıya alınmış. Destek ile iletişime geçin.", http.StatusForbidden)
	ErrEmailNotVerified = NewStatus(KindAuth, "EMAIL_NOT_VERIFIED",
		"Bu işlem için e-posta adresinizi doğrulamanız gerekiyor.", http.StatusForbidden)
	ErrEmailTaken = NewStatus(KindDomain, "EMAIL_TAKEN",
		"Bu e-posta adresi zaten kayıtlı.", http.StatusConflict)
	ErrUsernameTaken = NewStatus(KindDomain, "USERNAME_TAKEN",
		"Bu kullanıcı adı zaten alınmış.", http.StatusConflict)
	ErrTokenInvalid = NewStatus(KindAuth, "TOKEN_INVALID",
		"Bağlantı geçersiz veya süresi dolmuş.", http.StatusGone)
	ErrWeakPassword = NewStatus(KindDomain, "WEAK_PASSWORD",
		"Şifreniz yeterince güçlü değil.", http.StatusUnprocessableEntity)
	ErrCaptchaFailed = NewStatus(KindDomain, "CAPTCHA_FAILED",
		"Robot doğrulaması başarısız oldu. Lütfen tekrar deneyin.", http.StatusBadRequest)
	ErrTooManyAttempts = New(KindRateLimit, "TOO_MANY_ATTEMPTS",
		"Çok fazla başarısız deneme. Lütfen 15 dakika sonra tekrar deneyin.")

	// ─── Cüzdan ───
	ErrInsufficientBalance = NewStatus(KindDomain, "INSUFFICIENT_BALANCE",
		"Bakiyeniz yetersiz.", http.StatusConflict)
	ErrLedgerImmutable = New(KindInternal, "LEDGER_IMMUTABLE",
		"Beklenmeyen bir hata oluştu.")

	// ─── Katalog ve fiyat ───
	ErrQuoteExpired = NewStatus(KindDomain, "QUOTE_EXPIRED",
		"Fiyat teklifinin süresi doldu. Lütfen tekrar deneyin.", http.StatusConflict)
	ErrQuoteConsumed = NewStatus(KindDomain, "QUOTE_CONSUMED",
		"Bu teklif zaten kullanılmış.", http.StatusConflict)
	ErrPriceChanged = NewStatus(KindDomain, "PRICE_CHANGED",
		"Fiyat değişti. Lütfen yeni fiyatı görüp tekrar deneyin.", http.StatusConflict)
	ErrOutOfStock = NewStatus(KindDomain, "OUT_OF_STOCK",
		"Seçtiğiniz servis için şu an numara bulunmuyor.", http.StatusConflict)
	ErrFxUnavailable = NewStatus(KindInfra, "FX_UNAVAILABLE",
		"Fiyatlar şu an hesaplanamıyor. Lütfen birazdan tekrar deneyin.",
		http.StatusServiceUnavailable)

	// ─── Sipariş ───
	ErrInvalidStateTransition = NewStatus(KindDomain, "INVALID_STATE",
		"Bu işlem siparişin mevcut durumunda yapılamaz.", http.StatusConflict)
	ErrCancelTooEarly = NewStatus(KindDomain, "CANCEL_TOO_EARLY",
		"Numarayı henüz iptal edemezsiniz. Lütfen biraz bekleyin.", http.StatusConflict)

	// ─── Sağlayıcı ───
	ErrNoProviderAvailable = NewStatus(KindInfra, "NO_PROVIDER_AVAILABLE",
		"Şu an bu servis için numara sağlanamıyor. Lütfen birazdan tekrar deneyin.",
		http.StatusServiceUnavailable)
	ErrProviderUnavailable = NewStatus(KindInfra, "PROVIDER_UNAVAILABLE",
		"Servis sağlayıcıya ulaşılamıyor. Lütfen birazdan tekrar deneyin.",
		http.StatusServiceUnavailable)
)
