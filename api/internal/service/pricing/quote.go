package pricing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/adapter/provider"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	dompricing "github.com/ikmetrik/sms-platform/api/internal/domain/pricing"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// QuoteTTL teklifin geçerlilik süresi.
//
// Kısa tutulur çünkü kur riski bizde: uzun bir teklif süresi, kur yükseldiğinde
// zararına satış demektir. 120 saniye kullanıcının seçim yapıp satın almasına
// yeter (docs/trd.md FR-305).
const QuoteTTL = 120 * time.Second

// providerTimeout tek bir sağlayıcıya tanınan süre.
// Yavaş bir sağlayıcı tüm teklifi geciktirmemeli — elenir.
const providerTimeout = 3 * time.Second

type QuoteService struct {
	tx       *postgres.TxRunner
	registry *provider.Registry
	secrets  *crypto.SecretBox
	fx       *FXService
	clock    port.Clock
	safety   money.Rate
}

type QuoteDeps struct {
	TxRunner       *postgres.TxRunner
	Registry       *provider.Registry
	Secrets        *crypto.SecretBox
	FX             *FXService
	Clock          port.Clock
	FXSafetyMargin money.Rate
}

func NewQuoteService(d QuoteDeps) *QuoteService {
	if d.Clock == nil {
		d.Clock = port.RealClock{}
	}
	safety := d.FXSafetyMargin
	if safety.IsZero() {
		safety = money.RateOne()
	}
	return &QuoteService{
		tx: d.TxRunner, registry: d.Registry, secrets: d.Secrets,
		fx: d.FX, clock: d.Clock, safety: safety,
	}
}

// QuoteRequest kullanıcının teklif isteği.
type QuoteRequest struct {
	UserID      int64
	ServiceCode string
	CountryISO  string
}

// Quote istemciye dönen teklif.
//
// providerID ve maliyet BULUNMAZ. Bu bilinçlidir: eski prototipte istemci
// hangi sağlayıcıdan hangi fiyata alacağını gövdede belirleyebiliyordu
// (docs/memory.md §3.8). Satın alma yalnız QuoteID ile yapılır.
type Quote struct {
	QuoteID   uuid.UUID
	SellPrice money.Money
	Stock     int
	ExpiresAt time.Time
}

// Create bir fiyat teklifi üretir.
//
// Akış (docs/design.md §8.1):
//  1. Kur oku — bayatsa DUR
//  2. Ürünü bul
//  3. Sağlayıcılara PARALEL fiyat/stok sor (yavaş olan elenir)
//  4. Stoklu olanlar arasında en ucuzu seç
//  5. En spesifik fiyat kuralını uygula
//  6. Teklifi kaydet
func (s *QuoteService) Create(ctx context.Context, req QuoteRequest) (Quote, error) {
	q := s.tx.Queries()

	// ── 1. Kur ──
	fxq, err := s.fx.Current(ctx)
	if err != nil {
		return Quote{}, err // ErrFxUnavailable — satış durur
	}

	// ── 2. Ürün ──
	svc, err := q.GetServiceByCode(ctx, req.ServiceCode)
	if err != nil {
		return Quote{}, apperr.ErrNotFound.WithMessage("Servis bulunamadı.")
	}
	ctry, err := q.GetCountryByISO(ctx, req.CountryISO)
	if err != nil {
		return Quote{}, apperr.ErrNotFound.WithMessage("Ülke bulunamadı.")
	}
	prod, err := q.GetProductForActivation(ctx, db.GetProductForActivationParams{
		ServiceID: &svc.ID, CountryID: &ctry.ID, VerificationType: db.VerificationTypeSms,
	})
	if err != nil {
		return Quote{}, apperr.ErrOutOfStock
	}

	// ── 3-4. Sağlayıcı seçimi ──
	best, err := s.cheapestOffer(ctx, prod.ID, svc.Code, ctry.Iso2)
	if err != nil {
		return Quote{}, err
	}

	// ── 5. Fiyat kuralı ──
	rules, err := q.ListApplicableRules(ctx, db.ListApplicableRulesParams{
		CountryID: &ctry.ID, ServiceID: &svc.ID, ProductID: &prod.ID,
	})
	if err != nil {
		return Quote{}, apperr.Internal(err)
	}
	rule, err := selectRule(rules)
	if err != nil {
		// Kural yoksa satış YAPILMAZ. Sessizce maliyetine satmak,
		// eski prototipin hatasıydı.
		slog.Error("FİYAT KURALI YOK — satış yapılamıyor",
			"service", svc.Code, "country", ctry.Iso2)
		return Quote{}, apperr.ErrFxUnavailable.WithMessage(
			"Fiyatlar şu an hesaplanamıyor. Lütfen birazdan tekrar deneyin.")
	}

	multiplier, err := numericToRate(best.CostMultiplier)
	if err != nil {
		return Quote{}, apperr.Internal(err)
	}

	calc, err := dompricing.Calculate(dompricing.Input{
		Cost:               best.Cost,
		ProviderMultiplier: multiplier,
		FXRate:             fxq.Rate,
		FXSafetyMargin:     s.safety,
		Rule:               rule,
	})
	if err != nil {
		return Quote{}, apperr.Internal(err)
	}

	// ── 6. Kayıt ──
	var fxNum, marginNum pgtype.Numeric
	if err := fxNum.Scan(fxq.Rate.String()); err != nil {
		return Quote{}, apperr.Internal(err)
	}
	if err := marginNum.Scan(rule.MarginPercent); err != nil {
		return Quote{}, apperr.Internal(err)
	}

	now := s.clock.Now()
	row, err := q.CreateQuote(ctx, db.CreateQuoteParams{
		UserID: req.UserID, ProductID: prod.ID, ProviderID: best.ProviderID,
		CostMicro: best.Cost.Minor(), CostCurrency: db.CurrencyCodeUSD,
		FxRate: fxNum, MarginPercent: marginNum,
		PricingRuleID:  &rule.ID,
		SellPriceMinor: calc.SellPrice.Minor(),
		StockAtQuote:   int32(best.Stock),
		ExpiresAt:      now.Add(QuoteTTL),
	})
	if err != nil {
		return Quote{}, apperr.Internal(err)
	}

	return Quote{
		QuoteID:   row.PublicID,
		SellPrice: calc.SellPrice,
		Stock:     best.Stock,
		ExpiresAt: row.ExpiresAt,
	}, nil
}

// offer bir sağlayıcının canlı teklifi.
type offer struct {
	ProviderID     int64
	ProviderName   string
	Cost           money.Money
	Stock          int
	Priority       int32
	CostMultiplier pgtype.Numeric
	adjusted       int64 // maliyet × çarpan — sıralama için
}

// cheapestOffer aktif sağlayıcılara PARALEL sorar ve en ucuzunu seçer.
//
// Yavaş sağlayıcı BEKLENMEZ: 3 saniyede yanıt vermeyen elenir. Tek bir yavaş
// sağlayıcının tüm teklifi geciktirmesi, kullanıcı için hizmetin çökmesiyle
// aynı şeydir (docs/trd.md KK-306).
func (s *QuoteService) cheapestOffer(ctx context.Context, productID int64, serviceCode, countryISO string) (offer, error) {
	rows, err := s.tx.Queries().ListOffersForProduct(ctx, productID)
	if err != nil {
		return offer{}, apperr.Internal(err)
	}
	if len(rows) == 0 {
		return offer{}, apperr.ErrOutOfStock
	}

	results := make([]offer, len(rows))
	g, gctx := errgroup.WithContext(ctx)

	for i, r := range rows {
		i, r := i, r
		g.Go(func() error {
			adapter, err := s.registry.Resolve(string(r.Protocol))
			if err != nil {
				slog.Warn("adaptör yok", "protocol", r.Protocol)
				return nil // bu sağlayıcı elenir, diğerleri devam
			}
			prov, err := s.tx.Queries().GetProvider(gctx, r.ProviderID)
			if err != nil {
				return nil
			}
			creds, err := s.credsFor(prov)
			if err != nil {
				slog.Error("sağlayıcı anahtarı çözülemedi", "provider", prov.Name)
				return nil
			}

			cctx, cancel := context.WithTimeout(gctx, providerTimeout)
			defer cancel()

			live, err := adapter.GetPriceAndStock(cctx, creds, port.PriceQuery{
				ServiceCode: serviceCode, CountryCode: countryISO,
				VerificationType: port.VerifySMS,
			})
			if err != nil || live == nil {
				slog.Warn("sağlayıcı fiyat vermedi", "provider", prov.Name, "err", err)
				return nil
			}
			// Stok = GERÇEK stok. Sıfırsa bu sağlayıcı aday değildir.
			if live.Stock <= 0 {
				return nil
			}

			mult, err := numericToRate(r.CostMultiplier)
			if err != nil {
				mult = money.RateOne()
			}
			adj, err := live.Cost.MulRate(mult, money.RoundUp)
			if err != nil {
				return nil
			}

			results[i] = offer{
				ProviderID: r.ProviderID, ProviderName: r.ProviderName,
				Cost: live.Cost, Stock: live.Stock, Priority: r.Priority,
				CostMultiplier: r.CostMultiplier, adjusted: adj.Minor(),
			}
			return nil
		})
	}
	_ = g.Wait() // tekil sağlayıcı hataları yutulur; hepsi elenirse aşağıda yakalanır

	valid := results[:0]
	for _, o := range results {
		if o.ProviderID != 0 {
			valid = append(valid, o)
		}
	}
	if len(valid) == 0 {
		return offer{}, apperr.ErrNoProviderAvailable
	}

	// En ucuz; eşitlikte öncelik sırası.
	sort.SliceStable(valid, func(a, b int) bool {
		if valid[a].adjusted != valid[b].adjusted {
			return valid[a].adjusted < valid[b].adjusted
		}
		return valid[a].Priority < valid[b].Priority
	})
	return valid[0], nil
}

func (s *QuoteService) credsFor(p db.Provider) (port.Creds, error) {
	c := port.Creds{BaseURL: p.BaseUrl}
	if len(p.ApiKeyEnc) == 0 {
		return c, nil
	}
	key, err := s.secrets.OpenString(p.ApiKeyEnc)
	if err != nil {
		return port.Creds{}, err
	}
	c.APIKey = key
	return c, nil
}

// selectRule veritabanı satırlarını domain kuralına çevirip en spesifiği seçer.
func selectRule(rows []db.PricingRule) (dompricing.Rule, error) {
	if len(rows) == 0 {
		return dompricing.Rule{}, dompricing.ErrNoRule
	}
	rules := make([]dompricing.Rule, 0, len(rows))
	for _, r := range rows {
		scope, err := dompricing.ParseScope(string(r.Scope))
		if err != nil {
			continue
		}
		margin, err := r.MarginPercent.Value()
		if err != nil {
			continue
		}
		ms, _ := margin.(string)
		rules = append(rules, dompricing.Rule{
			ID: r.ID, Scope: scope, MarginPercent: ms,
			FixedFee: money.New(r.FixedFeeMinor, money.TRY),
			MinPrice: money.New(r.MinPriceMinor, money.TRY),
		})
	}
	return dompricing.SelectRule(rules)
}

// Consume teklifi tüketir ve satın alma için doğrular.
//
// ÇAĞIRANIN TRANSACTION'I İÇİNDE çalışır: teklifin tüketilmesi ile bakiyenin
// düşülmesi atomik olmalıdır (docs/design.md §8.2, T1 adımı).
func (s *QuoteService) Consume(ctx context.Context, q *db.Queries, userID int64, quoteID uuid.UUID) (db.PriceQuote, error) {
	row, err := q.LockQuoteForConsumption(ctx, db.LockQuoteForConsumptionParams{
		PublicID: quoteID, UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Başkasının teklifi ile var olmayan teklif AYNI hatayı döner:
		// teklif kimliğinin varlığı sızdırılmaz.
		return db.PriceQuote{}, apperr.ErrNotFound
	}
	if err != nil {
		return db.PriceQuote{}, apperr.Internal(err)
	}

	now := s.clock.Now()
	if row.ConsumedAt != nil {
		return db.PriceQuote{}, apperr.ErrQuoteConsumed
	}
	if now.After(row.ExpiresAt) {
		return db.PriceQuote{}, apperr.ErrQuoteExpired
	}

	consumed, err := q.ConsumeQuote(ctx, db.ConsumeQuoteParams{ID: row.ID, ConsumedAt: &now})
	if errors.Is(err, pgx.ErrNoRows) {
		// Kilide rağmen buraya düşülürse başka bir yol tüketmiş demektir.
		return db.PriceQuote{}, apperr.ErrQuoteConsumed
	}
	if err != nil {
		return db.PriceQuote{}, apperr.Internal(err)
	}
	return consumed, nil
}

var _ = fmt.Sprintf
