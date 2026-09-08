package port

import (
	"context"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

// FXQuote bir kur değeri ve ne zaman alındığı.
type FXQuote struct {
	Base      money.Currency
	Quote     money.Currency
	Rate      money.Rate
	Source    string
	FetchedAt time.Time
}

// Age kurun yaşı.
func (q FXQuote) Age(now time.Time) time.Duration { return now.Sub(q.FetchedAt) }

// FXProvider kur kaynağı.
type FXProvider interface {
	// Fetch güncel kuru çeker.
	Fetch(ctx context.Context, base, quote money.Currency) (FXQuote, error)
	Source() string
}
