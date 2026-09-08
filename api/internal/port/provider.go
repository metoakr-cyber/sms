package port

import (
	"context"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

// ─────────────────────────── Değer tipleri ───────────────────────────

// ProductKind satılabilir ürün tipi.
type ProductKind string

const (
	KindSMSActivation ProductKind = "SMS_ACTIVATION"
	KindSMSRental     ProductKind = "SMS_RENTAL"
)

// VerificationType doğrulama kanalı. AYRI bir fiyat/stok eksenidir.
type VerificationType string

const (
	VerifySMS  VerificationType = "sms"
	VerifyCall VerificationType = "call"
)

// Dimension eşleştirilebilir boyutlar. Yeni bir boyut eklemek bu listeye
// değer eklemektir; yeni tablo gerekmez (docs/design.md §4).
type Dimension string

const (
	DimService  Dimension = "service"
	DimCountry  Dimension = "country"
	DimOperator Dimension = "operator"
)

// RemoteDimension sağlayıcının bir boyut değeri.
type RemoteDimension struct {
	RemoteCode string
	Name       string
	// Extra sağlayıcıya özgü bayraklar (örn. HeroSMS ülkelerinde "rent").
	Extra map[string]any
}

// Creds bir sağlayıcı çağrısının kimlik ve uç nokta bilgisi.
// Şifresi çözülmüş API anahtarı YALNIZ burada, bellekte taşınır.
type Creds struct {
	BaseURL string
	APIKey  string
}

// PriceQuery fiyat/stok sorgusu.
type PriceQuery struct {
	ServiceCode      string
	CountryCode      string
	OperatorCode     string // "" veya "any"
	VerificationType VerificationType
	DurationHours    int // 0 = tek seferlik
}

// PriceResult tek bir kombinasyonun fiyat ve stok bilgisi.
type PriceResult struct {
	// Cost sağlayıcının maliyeti. MİKRO-birimde taşınır (ADR-026).
	Cost money.Money
	// Stock GERÇEK stok. Sağlayıcının "havuz" sayacı değil.
	Stock int
}

// OfferSnapshot toplu katalog senkronunun tek satırı.
type OfferSnapshot struct {
	ServiceCode      string
	CountryCode      string
	VerificationType VerificationType
	Cost             money.Money
	Stock            int
}

// PurchaseCmd satın alma isteği.
type PurchaseCmd struct {
	ServiceCode      string
	CountryCode      string
	OperatorCode     string
	VerificationType VerificationType
	DurationHours    int // 0 = tek seferlik

	// MaxCost ödemeyi kabul ettiğimiz AZAMİ maliyet.
	// Fiyat bunun üstündeyse satın alma GERÇEKLEŞMEZ — fiyat garantisi
	// sağlayıcı sınırında zorlanır (ADR-018).
	// test: contract/contract.go — "MaxCost aşılırsa satın alma YAPILMAZ"
	MaxCost money.Money

	// ClientRef siparişimizi sağlayıcı tarafında etiketler (yetim provizyon
	// korelasyonu, FR-408). Anlamsız bir kimlik olmalı — kişisel veri değil.
	ClientRef string
}

// PurchaseResult başarılı satın alma.
type PurchaseResult struct {
	RemoteOrderID string
	PhoneNumber   string
	CountryCode   string
	OperatorCode  string
	// Cost sağlayıcının FİİLEN tahsil ettiği maliyet.
	Cost money.Money
	// ExpiresAt numaranın geçerlilik sonu. Sağlayıcıdan gelir, koda gömülmez.
	ExpiresAt time.Time
	Subtype   ProductKind
}

// RemoteOrderState sağlayıcı tarafındaki normalize durum.
//
// Sağlayıcının kendi durum kodları BURAYA EŞLENMEZ, çevrilir: HeroSMS'in
// [1,2,3,4,6,7,8,10] enum'unda 1,2,3,4,7'nin anlamı belgelenmemiş.
// Bilinmeyen değer StateWaiting'e düşer — savunmacı (ADR-025).
type RemoteOrderState string

const (
	StateWaiting   RemoteOrderState = "WAITING"   // kod bekleniyor
	StateCompleted RemoteOrderState = "COMPLETED" // kod geldi
	StateCancelled RemoteOrderState = "CANCELLED"
	StateRefunded  RemoteOrderState = "REFUNDED"
	StateUnknown   RemoteOrderState = "UNKNOWN"
)

// RemoteStatus sipariş durumu.
type RemoteStatus struct {
	State    RemoteOrderState
	Messages []RemoteMessage
}

// RemoteMessage gelen bir SMS/çağrı.
//
// Alan adları NORMALİZEDİR. HeroSMS aynı veriyi üç farklı isimle döndürüyor:
// modern {smsCode,smsText,receivedAt} · legacy {code,text,date} ·
// webhook {code,text,receivedAt} — iki farklı tarih formatıyla.
// Adaptörün görevi bu farkı YUTMAKTIR (docs/provider-herosms.md §7.3).
type RemoteMessage struct {
	RemoteID   string
	Code       string // boş olabilir: 'call' tipinde veya kod ayrıştırılamadığında
	Body       string
	Sender     string
	ReceivedAt time.Time
}

// ─────────────────────────── Arayüz ───────────────────────────

// ProviderPort bir SMS sağlayıcısının sözleşmesi.
//
// Adaptörün görevi ÇEVİRİDİR, karar vermek değil: ham yanıtları normalize
// tiplere ve hataları tipli hatalara çevirir. "Hangi sağlayıcıdan alalım",
// "iade edelim mi" gibi kararlar servis katmanına aittir.
type ProviderPort interface {
	Protocol() string
	Capabilities() []ProductKind

	// ListCountries / ListServices katalog senkronu için boyut listeleri.
	ListCountries(ctx context.Context, c Creds) ([]RemoteDimension, error)
	ListServices(ctx context.Context, c Creds) ([]RemoteDimension, error)

	// GetPriceAndStock tek kombinasyon için canlı fiyat. Satın alma öncesi
	// her zaman çağrılır.
	GetPriceAndStock(ctx context.Context, c Creds, q PriceQuery) (*PriceResult, error)

	// ListOffers TOPLU fiyat/stok anlık görüntüsü (katalog önbelleği).
	// HeroSMS'te tek çağrıda ~20.000 kombinasyon dönüyor; ülke başına
	// döngü kurmak gereksiz ve yavaştır.
	ListOffers(ctx context.Context, c Creds, vt VerificationType) ([]OfferSnapshot, error)

	// Purchase numara satın alır. İDEMPOTENT DEĞİLDİR: yeniden denenmemelidir.
	// Bu bir SÖZLEŞME kuralıdır; çağıranın uyması gerekir (CLAUDE.md değişmez #6).
	Purchase(ctx context.Context, c Creds, cmd PurchaseCmd) (*PurchaseResult, error)

	// GetStatus sipariş durumunu ve gelen mesajları döner.
	GetStatus(ctx context.Context, c Creds, remoteOrderID string) (*RemoteStatus, error)

	// Cancel iptal eder ve İADE TALEP EDER.
	// Finish başarıyla kapatır, İADE YOKTUR.
	// İkisi AYNI ŞEY DEĞİLDİR (ADR-023): kod teslim edilmiş bir siparişi
	// Cancel ile kapatmak sağlayıcıya yanlış sinyal verir.
	Cancel(ctx context.Context, c Creds, remoteOrderID string) error
	Finish(ctx context.Context, c Creds, remoteOrderID string) error

	// GetBalance sağlayıcıdaki bakiyemiz.
	GetBalance(ctx context.Context, c Creds) (money.Money, error)
}

// RentalProvider kiralık numara yetenekleri (v1.1).
//
// Ayrı bir arayüz: her sağlayıcı kiralık desteklemez ve ProviderPort'u
// desteklemeyen metotlarla şişirmek "uygulanmamış metot" tuzağı yaratır.
type RentalProvider interface {
	// Extend süreyi uzatır. time.Duration DEĞİL: sağlayıcılar SABİT süre
	// kümesi kabul ediyor (HeroSMS: 24|72|168|336|720|1440|2160|4320 saat).
	Extend(ctx context.Context, c Creds, remoteOrderID string, hours int) error

	// AllowedDurations desteklenen süreler (saat).
	//
	// SAĞLAYICIDAN ÖĞRENİLİR, koda gömülmez: HeroSMS spec'i süreler konusunda
	// ÜÇ YERDE kendiyle çelişiyor (RentDuration enum'u bir şey, BAD_DURATION
	// yanıtı başka, serviceCountRent örneği bambaşka). Sabit bir liste
	// yazmak, sağlayıcı listeyi değiştirdiğinde sessizce yanlış olur.
	AllowedDurations(ctx context.Context, c Creds) ([]int, error)

	// ListRentOffers bir servisin kiralık fiyat ve stoklarını döner.
	//
	// SERVİS BAZINDA çağrılır çünkü sağlayıcı toplu bir kiralık katalog
	// sunmuyor: `getRentServicesAndCountries` servis listesini BOŞ döndürüyor.
	// Aktivasyon tarafındaki tek istekli toplu uç burada YOK.
	ListRentOffers(ctx context.Context, c Creds, serviceCode string) ([]RentOffer, error)
}

// RentOffer kiralık fiyat/stok anlık görüntüsü.
type RentOffer struct {
	ServiceCode   string
	CountryCode   string
	DurationHours int
	Cost          money.Money
	Stock         int
}
