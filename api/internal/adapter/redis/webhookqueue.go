package redis

// Webhook kuyruğu.
//
// NEDEN REDIS LİSTESİ: webhook handler'ın 3 saniye bütçesi var ve işin
// kendisi (sağlayıcıdan teyit + veritabanı + yayın) o bütçeye sığmaz.
// Bildirim önce buraya düşer, işçi sonra alır.
//
// NEDEN asynq DEĞİL: asynq bir bağımlılık ve yeniden deneme/ölü mektup
// kutusu gibi özellikleri var — ama bu iş için gerekli olan tek şey
// "sırayla al, işle". Kaybolan bir bildirimin güvenlik ağı zaten
// `order-poller` (30 sn). Kuyruğa kütüphane eklemek, kaybı önlemiyor;
// yalnız kurulumu ağırlaştırıyordu.

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	webhookKey = "webhook:herosms"

	// maxQueueLen kuyruk üst sınırı.
	//
	// Kimliksiz bir uçtan besleniyor. Sınırsız büyümesine izin vermek,
	// gizli yol sızarsa belleği doldurma vektörü olurdu. Sınır aşılırsa
	// EN ESKİ bildirim düşer: yeni bildirimler daha değerlidir ve eskisi
	// için yoklama güvenlik ağı var.
	maxQueueLen = 10_000
)

// WebhookQueue ham webhook gövdelerini tutar.
type WebhookQueue struct {
	client *goredis.Client
}

func NewWebhookQueue(c *goredis.Client) *WebhookQueue { return &WebhookQueue{client: c} }

// Enqueue bildirimi kuyruğa alır.
func (q *WebhookQueue) Enqueue(ctx context.Context, raw []byte) error {
	pipe := q.client.TxPipeline()
	pipe.LPush(ctx, webhookKey, raw)
	// LTRIM her yazımda: ayrı bir temizlik işi olmadan sınır korunur.
	pipe.LTrim(ctx, webhookKey, 0, maxQueueLen-1)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("webhook kuyruğa alınamadı: %w", err)
	}
	return nil
}

// Dequeue bir bildirimi bloklayarak alır.
//
// BLOKLAYAN OKUMA (BRPOP) tercih edildi: yoklamalı bir döngü, boşta bile
// saniyede bir Redis'e gider ve gelen bildirimi ortalama yarım tur geciktirir.
// Kod bekleyen kullanıcı için o gecikme doğrudan görünür.
func (q *WebhookQueue) Dequeue(ctx context.Context, timeout time.Duration) ([]byte, error) {
	// 🔴 REDIS'İN EN KÜÇÜK BLOKLAMA SÜRESİ 1 SANİYEDİR. Daha kısa bir değer
	// sessizce yuvarlanmaz: istemci HER ÇAĞRIDA uyarı basar. Saniyede bir
	// koşan bir tüketicide bu, günde ~86 bin satır gürültü demek ve gerçek
	// olayları boğar. Sınırı arka ucun sahibi olan katman korur.
	// test: webhookqueue_test.go#TestDequeueRespectsRedisMinimumTimeout
	res, err := q.client.BRPop(ctx, clampBlock(timeout), webhookKey).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil // zaman aşımı — kuyruk boş, hata değil
		}
		return nil, err
	}
	if len(res) != 2 {
		return nil, nil
	}
	return []byte(res[1]), nil
}

// clampBlock bloklama süresini Redis'in alt sınırına çeker.
func clampBlock(d time.Duration) time.Duration {
	if d < time.Second {
		return time.Second
	}
	return d
}

// Len kuyruk uzunluğu — izleme için.
func (q *WebhookQueue) Len(ctx context.Context) (int64, error) {
	return q.client.LLen(ctx, webhookKey).Result()
}
