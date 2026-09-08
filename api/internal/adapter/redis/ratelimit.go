package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// RateLimiter sabit pencereli sayaç.
//
// Kaydırmalı pencere (sliding log) daha adil olurdu ama her istek için bir
// sıralı küme girdisi tutar. Bizim kullanım amacımız (kaba kuvvet ve kötüye
// kullanım engelleme) için sabit pencere yeterlidir ve tek bir INCR + EXPIRE ile
// atomik olarak çalışır.
type RateLimiter struct{ rdb *goredis.Client }

func NewRateLimiter(rdb *goredis.Client) *RateLimiter { return &RateLimiter{rdb: rdb} }

var _ port.RateLimiter = (*RateLimiter)(nil)

func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	k := "rl:" + key

	// INCR ve EXPIRE tek turda; ilk artışta pencere başlatılır.
	pipe := r.rdb.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.ExpireNX(ctx, k, window) // yalnız TTL yoksa ayarla — pencere kaymasın
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, fmt.Errorf("redis: hız limiti sayacı: %w", err)
	}

	count := incr.Val()
	if count <= int64(limit) {
		return true, 0, nil
	}

	ttl, err := r.rdb.TTL(ctx, k).Result()
	if err != nil || ttl < 0 {
		ttl = window
	}
	return false, ttl, nil
}

func (r *RateLimiter) Reset(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, "rl:"+key).Err()
}
