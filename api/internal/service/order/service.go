// Package order sipariş kullanım senaryolarını yürütür.
//
// test: order_integration_test.go#TestProviderFailureRefundsFully
// test: order_integration_test.go#TestConcurrentPurchaseOfSameQuote
//
// EN KRİTİK KURAL: dış çağrı (HTTP) ASLA veritabanı transaction'ı içinde
// yapılmaz. Satın alma üç aşamalıdır (docs/design.md §5.4, ADR-008):
//
//	T1  → kısa transaction: teklifi tüket, bakiyeyi düş
//	HTTP→ sağlayıcıdan numara al (transaction DIŞINDA, yeniden deneme YOK)
//	T2  → kısa transaction: siparişi yaz veya iade et
//
// Eski prototip iki HTTP çağrısını transaction içinde yapıyordu: bağlantı
// havuzu tükeniyor, kilitler birikiyor ve süreç ölürse "sağlayıcıdan numara
// alınmış ama tahsilat yapılmamış" durumu oluşuyordu.
package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
)

// providerTimeout satın alma çağrısı için üst sınır.
//
// Sağlayıcı 10 saniyede yanıt vermezse pes ederiz. Daha uzun beklemek,
// kullanıcının bakiyesi düşülmüş hâlde ekranda asılı kalması demektir.
const providerTimeout = 10 * time.Second

// t2Timeout satın alma sonrası telafi adımlarının üst sınırı.
//
// Bu adımlar (sipariş yazımı, numarayı bırakma, iade) İSTEMCİDEN BAĞIMSIZ
// çalışır — kullanıcı sekmeyi kapatsa da parası geri verilmeli. Kendi
// zaman aşımı vardır çünkü çağıranın bağlamından koparıldı ve süresiz bir
// bağlam takılan bir sorguyu sonsuza kadar bekletirdi.
const t2Timeout = 15 * time.Second

// defaultCancelGrace sağlayıcı süre bildirmezse iptal için beklenecek süre.
//
// Sağlayıcı ilk ~120 saniye iptali reddediyor (minActivationTime). Bu değer
// YALNIZ sağlayıcı bir şey söylemediğinde kullanılır; söylediğinde onunki kazanır.
const defaultCancelGrace = 120 * time.Second

// expirySafetyMargin sağlayıcının expiredAt değerinden düşülen pay.
//
// Sağlayıcı tam o anda kapatırsa aramızda birkaç saniye fark olur; iade
// penceresini kaçırmamak için erken davranırız.
const expirySafetyMargin = 15 * time.Second

type txRunner interface {
	Queries() *db.Queries
	InTx(ctx context.Context, fn func(*db.Queries) error) error
}

// Publisher sipariş olaylarını yayınlar (SSE için).
//
// ARAYÜZ olarak durur çünkü servis Redis'i tanımamalıdır; ayrıca yayın
// başarısız olsa bile sipariş akışı DEVAM ETMELİDİR — bildirim bir kolaylıktır,
// paranın doğruluğu ona bağlı değildir.
type Publisher interface {
	Publish(ctx context.Context, orderPublicID string, event Event) error
}

// Event SSE'ye giden olay.
type Event struct {
	Type      string        `json:"-"` // status | code | cancelled
	OrderID   string        `json:"orderId"`
	Status    string        `json:"status"`
	Messages  []MessageView `json:"messages,omitempty"`
	Refunded  *money.Money  `json:"-"`
	ExpiresAt string        `json:"expiresAt,omitempty"`
}

// MessageView bir SMS mesajının dışa açık görünümü.
type MessageView struct {
	Code       string `json:"code"`
	Body       string `json:"body"`
	Sender     string `json:"sender,omitempty"`
	ReceivedAt string `json:"receivedAt"`
}

// Deps servis bağımlılıkları.
type Deps struct {
	TxRunner  txRunner
	Registry  *provider.Registry
	Secrets   *crypto.SecretBox
	Wallet    *walletsvc.Service
	Publisher Publisher
	Clock     port.Clock
}

// Service sipariş servisi.
type Service struct {
	tx       txRunner
	registry *provider.Registry
	secrets  *crypto.SecretBox
	wallet   *walletsvc.Service
	pub      Publisher
	clock    port.Clock
}

func New(d Deps) *Service {
	return &Service{
		tx: d.TxRunner, registry: d.Registry, secrets: d.Secrets,
		wallet: d.Wallet, pub: d.Publisher, clock: d.Clock,
	}
}

/* ═══════════════════════════ Satın alma ═══════════════════════════ */

// CreateInput satın alma isteği.
//
// GÖVDEDE YALNIZ quoteId VARDIR. Fiyat, sağlayıcı ve maliyet istemciden
// GELMEZ (CLAUDE.md değişmez #9): eski prototipte kullanıcı istediği fiyatı
// gövdede gönderebiliyordu.
type CreateInput struct {
	UserID  int64
	QuoteID uuid.UUID
}

// Create numara satın alır.
func (s *Service) Create(ctx context.Context, in CreateInput) (db.Order, error) {
	// ── T1: teklifi tüket + bakiyeyi düş ──
	hold, err := s.reserve(ctx, in)
	if err != nil {
		return db.Order{}, err
	}

	// ── Sağlayıcı çağrısı: TRANSACTION DIŞINDA ──
	//
	// 🔴 YENİDEN DENEME YOK. Purchase idempotent değildir: toplu bir uçtur ve
	// bir tekrar 10 numaraya kadar çift alım yapabilir.
	pctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()

	res, perr := s.callProvider(pctx, hold)

	// 🔴 T2 ARTIK İSTEMCİNİN BAĞLANTISINA BAĞLI DEĞİL.
	//
	// Para T1'de ZATEN DÜŞÜLDÜ. Bundan sonraki telafi adımları (siparişi yaz,
	// numarayı bırak, parayı iade et) çağıranın bağlamını kullanırsa,
	// kullanıcının sekmeyi kapatması ya da mobilde ağın düşmesi iadeyi
	// engeller: net/http `c.Request.Context()`'i iptal eder, `InTx` içindeki
	// `pool.Begin(ctx)` "context canceled" ile düşer ve defterde HİÇ iade
	// kaydı oluşmaz.
	//
	// Ölçüldü: bakiye 10000 → 7500, REFUND kaydı 0.
	//
	// `orphan-hold-reaper` bunu 2-3 dakikada toplar (ve o güvenlik ağı bu
	// turda ayrıca onarıldı), ama kullanıcının parasını dakikalarca askıda
	// bırakmanın ve numarayı sağlayıcıda açık bırakmanın gerekçesi yok:
	// kullanıcının kontrolündeki bir olay para kaybına dönüşmemeli.
	//
	// Sağlayıcı çağrısı BİLEREK dışarıda kaldı: kullanıcı vazgeçtiyse yeni
	// numara almaya devam etmenin anlamı yok.
	//
	// test: order_integration_test.go#TestClientDisconnectStillRefunds
	t2, t2cancel := context.WithTimeout(context.WithoutCancel(ctx), t2Timeout)
	defer t2cancel()

	// ── T2: siparişi yaz veya parayı geri ver ──
	if perr != nil {
		if refundErr := s.refundHold(t2, hold, perr); refundErr != nil {
			// İade YAZILAMADI. Bu, kullanıcının parasının askıda kalması
			// demektir — sessiz geçilemez. `orphan-hold-reaper` ikinci
			// şanstır ama olay burada da yüksek sesle kaydedilir.
			slog.Error("satın alma başarısız VE iade yazılamadı — yetim provizyon",
				"quote", hold.QuotePublicID, "user", in.UserID,
				"provider_err", perr, "refund_err", refundErr)
		}
		return db.Order{}, mapProviderError(perr)
	}

	ord, err := s.persist(t2, hold, res)
	if err != nil {
		// Numara ALINDI ama sipariş YAZILAMADI. Numarayı sağlayıcıda açıkta
		// bırakmayız: hemen iptal etmeye çalışırız, sonra parayı iade ederiz.
		s.abandonRemote(t2, hold, res.RemoteOrderID)
		if refundErr := s.refundHold(t2, hold, err); refundErr != nil {
			slog.Error("sipariş yazılamadı VE iade yazılamadı",
				"quote", hold.QuotePublicID, "err", err, "refund_err", refundErr)
		}
		return db.Order{}, err
	}

	s.publish(t2, ord, "status", nil)
	return ord, nil
}

// hold T1 sonrası elde kalan bilgi.
type hold struct {
	QuoteID        int64
	QuotePublicID  uuid.UUID
	UserID         int64
	ProviderID     int64
	ProductID      int64
	SellPriceMinor int64
	CostMicro      int64
	FXRate         string

	// DurationHours > 0 ise KİRALIK sipariş. Sağlayıcıya `duration` olarak
	// gider ve yanıtta `subtype: 2` bekleriz.
	DurationHours int

	ServiceCode, ServiceName string
	CountryISO2, CountryName string
	PhoneCode                string

	RemoteServiceCode string
	RemoteCountryCode string
}

// reserve T1: teklifi kilitler, tüketir ve bakiyeyi düşer.
//
// SIRA ÖNEMLİDİR: teklif ÖNCE kilitlenir, sonra tüketilir, sonra para düşülür.
// Para önce düşülseydi, geçersiz bir teklifte iade gerekirdi.
func (s *Service) reserve(ctx context.Context, in CreateInput) (hold, error) {
	var h hold
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		// Teklifi KİLİTLE. Sahiplik sorgunun parçasıdır: başkasının teklifiyle
		// satın alma denemesi burada sıfır satır döner.
		quote, err := q.LockQuoteForConsumption(ctx, db.LockQuoteForConsumptionParams{
			PublicID: in.QuoteID, UserID: in.UserID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.ErrQuoteNotFound
			}
			return apperr.Internal(err)
		}
		now := s.clock.Now()
		if quote.ConsumedAt != nil {
			return apperr.ErrQuoteConsumed
		}
		if !now.Before(quote.ExpiresAt) {
			return apperr.ErrQuoteExpired
		}
		// ConsumeQuote `WHERE consumed_at IS NULL` içerir: yarış durumunda
		// ikinci çağrı SIFIR satır döner. Kilit + koşullu güncelleme = çift savunma.
		if _, err := q.ConsumeQuote(ctx, db.ConsumeQuoteParams{ID: quote.ID, ConsumedAt: &now}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.ErrQuoteConsumed
			}
			return apperr.Internal(err)
		}

		// Bakiye YALNIZ cüzdan servisi üzerinden değişir (değişmez #2).
		// Anahtar DETERMİNİSTİKTİR: aynı teklif iki kez satın alınamaz.
		if _, err := s.wallet.Apply(ctx, q, walletsvc.Input{
			UserID:         in.UserID,
			Amount:         money.New(-quote.SellPriceMinor, money.TRY),
			Type:           db.LedgerTypePURCHASE,
			IdempotencyKey: "order:" + in.QuoteID.String(),
			ReferenceType:  "quote",
			ReferenceID:    in.QuoteID.String(),
			Note:           "Numara satın alma",
		}); err != nil {
			return err
		}

		info, err := q.GetQuoteContext(ctx, quote.ID)
		if err != nil {
			return apperr.Internal(err)
		}
		codes, err := q.GetProviderRemoteCodes(ctx, db.GetProviderRemoteCodesParams{
			ProviderID: quote.ProviderID, ProductID: quote.ProductID,
		})
		if err != nil {
			// Eşleştirme yoksa sağlayıcıya SORAMAYIZ. Uydurma bir kodla
			// istek atmak, yanlış ülkeden numara almakla sonuçlanabilir.
			return apperr.ErrMappingMissing
		}

		h = hold{
			QuoteID: quote.ID, QuotePublicID: in.QuoteID, UserID: in.UserID,
			ProviderID: quote.ProviderID, ProductID: quote.ProductID,
			SellPriceMinor: quote.SellPriceMinor, CostMicro: quote.CostMicro,
			FXRate:      numericToString(quote.FxRate),
			ServiceCode: info.ServiceCode, ServiceName: displayName(info.ServiceNameTr, info.ServiceName),
			CountryISO2: info.CountryIso2, CountryName: displayName(info.CountryNameTr, info.CountryName),
			PhoneCode:         info.PhoneCode,
			RemoteServiceCode: codes.ServiceRemoteCode,
			RemoteCountryCode: codes.CountryRemoteCode,
		}
		// Ürün kiralıksa süreyi taşırız. Dakikadan saate çevrim TEK YERDE:
		// iki birim arasında gidip gelmek er geç 60 kat hataya yol açar.
		if prodRow, err := q.GetProductByID(ctx, quote.ProductID); err == nil {
			if prodRow.Kind == db.ProductKindSMSRENTAL && prodRow.DurationMinutes != nil {
				h.DurationHours = int(*prodRow.DurationMinutes / 60)
			}
		}
		return nil
	})
	return h, err
}

// callProvider sağlayıcıdan numara alır.
func (s *Service) callProvider(ctx context.Context, h hold) (*port.PurchaseResult, error) {
	prov, err := s.tx.Queries().GetProvider(ctx, h.ProviderID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	adapter, err := s.registry.Resolve(string(prov.Protocol))
	if err != nil {
		return nil, apperr.Internal(err)
	}
	creds := port.Creds{BaseURL: prov.BaseUrl}
	if len(prov.ApiKeyEnc) > 0 {
		key, err := s.secrets.OpenString(prov.ApiKeyEnc)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		creds.APIKey = key
	}

	// ⭐ maxPrice = TEKLİFTEKİ maliyet (ADR-018).
	// test: order_integration_test.go#TestMaxPriceIsAlwaysSent
	// Fiyat yükselmişse satın alma sağlayıcı sınırında engellenir; beklenmedik
	// tahsilat olmaz. fixedPrice GÖNDERİLMEZ (ADR-027).
	return adapter.Purchase(ctx, creds, port.PurchaseCmd{
		ServiceCode:      h.RemoteServiceCode,
		CountryCode:      h.RemoteCountryCode,
		OperatorCode:     "any",
		VerificationType: port.VerifySMS,
		DurationHours:    h.DurationHours,
		MaxCost:          money.New(h.CostMicro, money.USD),
		ClientRef:        h.QuotePublicID.String(),
	})
}

// persist T2: sipariş satırını yazar.
func (s *Service) persist(ctx context.Context, h hold, res *port.PurchaseResult) (db.Order, error) {
	var out db.Order
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		now := s.clock.Now()

		// TTL SAĞLAYICIDAN gelir, koda gömülmez. Güvenlik payı düşeriz:
		// sağlayıcı tam o anda kapatırsa iade penceresini kaçırmayalım.
		expires := res.ExpiresAt.Add(-expirySafetyMargin)
		if !expires.After(now) {
			// Sağlayıcı zaten dolmuş bir TTL döndürdü. Bu bir sağlayıcı
			// hatasıdır; siparişi yazmayız, çağıran iade eder.
			return fmt.Errorf("%w: sağlayıcı geçmiş bir expiredAt döndürdü (%s)",
				port.ErrUnavailable, res.ExpiresAt.Format(time.RFC3339))
		}

		var activationID *int64
		if n, err := parseInt64(res.RemoteOrderID); err == nil {
			activationID = &n
		}
		productID := h.ProductID
		quoteID := h.QuoteID

		ord, err := q.CreateOrder(ctx, db.CreateOrderParams{
			UserID: h.UserID, ProviderID: h.ProviderID,
			RemoteOrderID: res.RemoteOrderID, ProviderActivationID: activationID,
			PhoneNumber: res.PhoneNumber, VerificationType: db.VerificationTypeSms,
			ProductID:   &productID,
			ServiceCode: h.ServiceCode, ServiceName: h.ServiceName,
			CountryIso2: h.CountryISO2, CountryName: h.CountryName, PhoneCode: h.PhoneCode,
			QuoteID:        &quoteID,
			PricePaidMinor: h.SellPriceMinor, CostMicro: res.Cost.Minor(),
			FxRate:        mustNumeric(h.FXRate),
			ExpiresAt:     expires,
			CancellableAt: now.Add(defaultCancelGrace),
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = ord
		return nil
	})
	return out, err
}

// refundHold satın alma başarısızsa parayı geri verir.
//
// İade anahtarı DETERMİNİSTİKTİR (`order:{quoteId}:refund`): aynı başarısızlık
// iki kez işlenirse para iki kez geri verilmez.
func (s *Service) refundHold(ctx context.Context, h hold, cause error) error {
	return s.tx.InTx(ctx, func(q *db.Queries) error {
		_, err := s.wallet.Apply(ctx, q, walletsvc.Input{
			UserID:         h.UserID,
			Amount:         money.New(h.SellPriceMinor, money.TRY),
			Type:           db.LedgerTypeREFUND,
			IdempotencyKey: "order:" + h.QuotePublicID.String() + ":refund",
			ReferenceType:  "quote",
			ReferenceID:    h.QuotePublicID.String(),
			Note:           "Numara alınamadı — otomatik iade",
		})
		return err
	})
}

// abandonRemote sipariş yazılamadığında sağlayıcıdaki numarayı kapatmaya çalışır.
//
// BAŞARISIZLIĞI YUTULUR: bu noktada zaten bir hata yolundayız ve asıl iş
// kullanıcının parasını geri vermektir. Kapatılamayan aktivasyonu
// `activation-reaper` toplayamaz (sipariş kaydı yok), bu yüzden olay
// log'a yazılır — insan bakabilsin.
func (s *Service) abandonRemote(ctx context.Context, h hold, remoteID string) {
	prov, err := s.tx.Queries().GetProvider(ctx, h.ProviderID)
	if err != nil {
		return
	}
	adapter, err := s.registry.Resolve(string(prov.Protocol))
	if err != nil {
		return
	}
	creds := port.Creds{BaseURL: prov.BaseUrl}
	if len(prov.ApiKeyEnc) > 0 {
		if key, err := s.secrets.OpenString(prov.ApiKeyEnc); err == nil {
			creds.APIKey = key
		}
	}
	cctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	if err := adapter.Cancel(cctx, creds, remoteID); err != nil {
		slog.Error("sipariş yazılamadı ve sağlayıcıdaki numara kapatılamadı — ELLE BAKILMALI",
			"provider", prov.Name, "remote_order_id", remoteID, "err", err)
	}
}

// mapProviderError sağlayıcı hatasını kullanıcıya gösterilecek hataya çevirir.
func mapProviderError(err error) error {
	switch {
	case errors.Is(err, port.ErrOutOfStock):
		return apperr.ErrOutOfStock
	case errors.Is(err, port.ErrPriceChanged):
		return apperr.ErrPriceChanged
	case errors.Is(err, port.ErrProviderNoBalance),
		errors.Is(err, port.ErrProviderAuth),
		errors.Is(err, port.ErrMappingMissing):
		// Bunlar BİZİM sorunumuzdur; kullanıcıya "sistem hatası" deriz ama
		// log'da gerçek sebep durur ve admin alarmı üretilir.
		slog.Error("sağlayıcı yapılandırma hatası — ADMIN", "err", err)
		return apperr.ErrProviderUnavailable.Wrap(err)
	}
	return apperr.ErrProviderUnavailable.Wrap(err)
}

func (s *Service) publish(ctx context.Context, ord db.Order, typ string, msgs []MessageView) {
	if s.pub == nil {
		return
	}
	ev := Event{
		Type: typ, OrderID: ord.PublicID.String(), Status: string(ord.Status),
		Messages: msgs, ExpiresAt: ord.ExpiresAt.Format(time.RFC3339),
	}
	if err := s.pub.Publish(ctx, ord.PublicID.String(), ev); err != nil {
		// Yayın başarısız olsa da sipariş akışı bozulmaz: istemcinin
		// yoklama yedeği var (FR-404). Bildirim bir kolaylıktır.
		slog.Warn("sipariş olayı yayınlanamadı", "order", ord.PublicID, "err", err)
	}
}
