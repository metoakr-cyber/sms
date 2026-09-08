// Package postgres PostgreSQL bağlantı havuzunu ve depo uygulamalarını içerir.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool bir bağlantı havuzu açar ve erişilebilirliği DOĞRULAR.
// Bağlanamazsa hata döner — süreç yarı çalışır durumda başlamaz.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres: bağlantı dizesi ayrıştırılamadı: %w", err)
	}

	// Havuz sınırları açıkça belirlenir. Varsayılana bırakılırsa yoğunlukta
	// bağlantı tükenmesi teşhis edilmesi zor bir şekilde ortaya çıkar.
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.HealthCheckPeriod = time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: havuz oluşturulamadı: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: erişilemiyor: %w", err)
	}
	return pool, nil
}
