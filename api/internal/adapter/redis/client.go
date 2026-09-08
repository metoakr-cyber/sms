// Package redis oturum deposu, önbellek ve kuyruk için Redis istemcisini sağlar.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client = redis.Client

// New bir Redis istemcisi açar ve erişilebilirliği DOĞRULAR.
func New(ctx context.Context, url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: bağlantı dizesi ayrıştırılamadı: %w", err)
	}
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 3 * time.Second
	opt.WriteTimeout = 3 * time.Second
	opt.PoolSize = 20

	c := redis.NewClient(opt)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := c.Ping(pingCtx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis: erişilemiyor: %w", err)
	}
	return c, nil
}
