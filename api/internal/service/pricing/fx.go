// Package pricing fiyat teklifi üretir ve kur yönetir.
package pricing

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// FXService kuru çeker, saklar ve bayatlığını denetler.
type FXService struct {
	tx       txRunner
	provider port.FXProvider
	clock    port.Clock
	maxAge   time.Duration
}

type txRunner interface {
	Queries() *db.Queries
}

func NewFXService(tx txRunner, p port.FXProvider, clock port.Clock, maxAge time.Duration) *FXService {
	if clock == nil {
		clock = port.RealClock{}
	}
	if maxAge <= 0 {
		maxAge = 30 * time.Minute
	}
	return &FXService{tx: tx, provider: p, clock: clock, maxAge: maxAge}
}

// Refresh kuru sağlayıcıdan çeker ve kaydeder. Zamanlanmış iş bunu çağırır.
func (s *FXService) Refresh(ctx context.Context) error {
	q, err := s.provider.Fetch(ctx, money.USD, money.TRY)
	if err != nil {
		// Çekilemedi: hizmet HEMEN durmaz, son bilinen kur kullanılmaya devam
		// eder. Ama kur bayatlarsa Current() satışı durdurur.
		slog.Error("kur çekilemedi — son bilinen kur kullanılacak",
			"source", s.provider.Source(), "err", err)
		return apperr.ErrFxUnavailable.Wrap(err)
	}

	var rate pgtype.Numeric
	if err := rate.Scan(q.Rate.String()); err != nil {
		return apperr.Internal(err)
	}
	if _, err := s.tx.Queries().InsertFXRate(ctx, db.InsertFXRateParams{
		Base: db.CurrencyCodeUSD, Quote: db.CurrencyCodeTRY,
		Rate: rate, Source: q.Source, FetchedAt: q.FetchedAt,
	}); err != nil {
		return apperr.Internal(err)
	}

	slog.Info("kur güncellendi", "source", q.Source, "usd_try", q.Rate.String())
	return nil
}

// Current geçerli kuru döner.
//
// Kur MaxAge'den eskiyse HATA döner ve satış durur. Bu bilinçli bir tercihtir:
// bayat bir kurla satış yapmak, kur yükselmişse her işlemde zarar etmek
// demektir. Hizmeti durdurmak, sessizce zarar etmekten iyidir
// (docs/trd.md FR-302, KK-302).
func (s *FXService) Current(ctx context.Context) (port.FXQuote, error) {
	row, err := s.tx.Queries().GetLatestFXRate(ctx, db.GetLatestFXRateParams{
		Base: db.CurrencyCodeUSD, Quote: db.CurrencyCodeTRY,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Error("hiç kur kaydı yok — satış yapılamaz")
		return port.FXQuote{}, apperr.ErrFxUnavailable
	}
	if err != nil {
		return port.FXQuote{}, apperr.Internal(err)
	}

	rate, err := numericToRate(row.Rate)
	if err != nil {
		return port.FXQuote{}, apperr.Internal(err)
	}

	q := port.FXQuote{
		Base: money.USD, Quote: money.TRY, Rate: rate,
		Source: row.Source, FetchedAt: row.FetchedAt,
	}

	if age := q.Age(s.clock.Now()); age > s.maxAge {
		slog.Error("KUR BAYAT — satış durduruldu",
			"age", age.Round(time.Second), "max_age", s.maxAge, "source", row.Source)
		return port.FXQuote{}, apperr.ErrFxUnavailable
	}
	return q, nil
}

// numericToRate pgtype.Numeric'i tam bir orana çevirir.
//
// float64'e uğramaz: NUMERIC(18,8) değerleri float64'te tam temsil edilemez
// ve kur hatası doğrudan fiyat hatasıdır.
func numericToRate(n pgtype.Numeric) (money.Rate, error) {
	v, err := n.Value() // driver.Value: string
	if err != nil {
		return money.Rate{}, err
	}
	s, ok := v.(string)
	if !ok {
		return money.Rate{}, errors.New("pricing: kur değeri metne çevrilemedi")
	}
	return money.RateFromString(s)
}
