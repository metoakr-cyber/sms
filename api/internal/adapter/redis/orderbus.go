package redis

// Sipariş olay yolu — SSE'nin arkasındaki taşıma.
//
// NEDEN REDIS PUB/SUB: kod webhook ile bir sunucuya, kullanıcının SSE bağlantısı
// BAŞKA bir sunucuya düşebilir. Süreç içi bir kanal, tek sunucuda çalışır ve
// ikinci sunucu eklendiği gün sessizce bozulur — "kod gelmiyor" şikâyeti olarak.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// OrderBus sipariş olaylarını yayınlar ve dinler.
type OrderBus struct {
	client *goredis.Client
}

func NewOrderBus(c *goredis.Client) *OrderBus { return &OrderBus{client: c} }

// channel bir siparişin kanal adı.
//
// public_id kullanılır, sayısal id DEĞİL: kanal adı log'lara ve hata
// mesajlarına düşer; sayısal kimlik sızdırmak sipariş sayımızı ele verir.
func channel(orderPublicID string) string { return "order:" + orderPublicID }

// Publish olayı yayınlar.
//
// ABONE YOKSA HATA DEĞİLDİR: kullanıcı sayfayı kapatmış olabilir. Redis
// PUBLISH sıfır abone için 0 döner ve bu normaldir.
func (b *OrderBus) Publish(ctx context.Context, orderPublicID string, payload any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("orderbus: olay kodlanamadı: %w", err)
	}
	if err := b.client.Publish(ctx, channel(orderPublicID), buf).Err(); err != nil {
		return fmt.Errorf("orderbus: yayınlanamadı: %w", err)
	}
	return nil
}

// Subscribe bir siparişin olaylarını dinler.
//
// Dönen kanal, ctx iptal edilene kadar açık kalır. Çağıran kapatmaz —
// aboneliğin ömrü ctx'e bağlıdır ve `Close` burada yapılır.
func (b *OrderBus) Subscribe(ctx context.Context, orderPublicID string) (<-chan []byte, error) {
	sub := b.client.Subscribe(ctx, channel(orderPublicID))

	// İlk onayı BEKLERİZ: abonelik kurulmadan "dinliyorsun" demek, ilk olayın
	// kaybolmasına ve kullanıcının kodu görmemesine yol açar.
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := sub.Receive(waitCtx); err != nil {
		_ = sub.Close()
		return nil, fmt.Errorf("orderbus: abone olunamadı: %w", err)
	}

	out := make(chan []byte, 16)
	go func() {
		defer close(out)
		defer func() { _ = sub.Close() }()
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				select {
				case out <- []byte(msg.Payload):
				case <-ctx.Done():
					return
				default:
					// Tampon dolu: yavaş bir istemci yüzünden tüm yayını
					// bloklamayız. Kaybolan olay, istemcinin yoklama yedeğiyle
					// telafi edilir (FR-404).
				}
			}
		}
	}()
	return out, nil
}
