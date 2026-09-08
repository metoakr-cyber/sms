package order

// webhook-ingest: sağlayıcı bildirimini işler.
//
// HANDLER'DAN AYRI ÇALIŞIR çünkü handler'ın 3 saniye bütçesi var ve buradaki
// iş (sağlayıcıdan teyit + veritabanı yazımı + yayın) o bütçeye sığmaz.
//
// İŞ AT-LEAST-ONCE ÇALIŞIR: aynı bildirim sekiz kez gelebilir. Her adım
// idempotenttir.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// webhookPayload sağlayıcının gönderdiği gövde.
//
// `code` ve `text` `required` DEĞİL ve ikisi de nullable — `call` tipi
// doğrulamada veya kodsuz bir SMS'te boş gelirler. Zorunlu string varsayan
// bir ayrıştırma orada çöker.
type webhookPayload struct {
	ActivationID json.Number `json:"activationId"`
	// ID spec'in `required` listesinde AMA `properties` içinde TANIMSIZ —
	// tipi bilinmiyor. Ham JSON olarak alıp esnek çözeriz.
	ID         json.RawMessage `json:"id"`
	PhoneFrom  string          `json:"phoneFrom"`
	Service    string          `json:"service"`
	Text       *string         `json:"text"`
	Code       *string         `json:"code"`
	Country    json.Number     `json:"country"`
	ReceivedAt string          `json:"receivedAt"`
}

// idString `id` alanını metne çevirir — string de sayı da olabilir.
func (p webhookPayload) idString() string {
	if len(p.ID) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(p.ID, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(p.ID, &n); err == nil {
		return n.String()
	}
	return ""
}

// HandleWebhook ham webhook gövdesini işler.
//
// SIRA (ADR-024) — her adım bir öncekinin sonucuna bağlı:
//  1. Gövdeyi ayrıştır
//  2. Sipariş BİZİM Mİ? Değilse sessizce bırak (webhook slotları hesap
//     genelidir; başka bir ortamın bildirimi bize düşebilir)
//  3. SAĞLAYICIDAN TEYİT AL — gövdedeki koda GÜVENİLMEZ
//  4. Teyit boşsa DUR: durum değiştirilmez, sipariş PENDING kalır
//  5. Mesajı yaz, durumu güncelle, yayınla
//
// test: webhook_integration_test.go#TestFakeWebhookCannotCompleteOrder
func (s *Service) HandleWebhook(ctx context.Context, providerID int64, raw []byte) error {
	var p webhookPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		// Ayrıştırılamayan gövde: log'la ve BIRAK. Yeniden denemenin anlamı
		// yok — aynı gövde yine ayrıştırılamaz.
		slog.Warn("webhook gövdesi ayrıştırılamadı", "err", err, "size", len(raw))
		return nil
	}

	remoteID := p.ActivationID.String()
	if remoteID == "" {
		slog.Warn("webhook'ta activationId yok")
		return nil
	}

	// ── (2) Bizim siparişimiz mi? ──
	ord, err := s.tx.Queries().GetOrderByRemote(ctx, db.GetOrderByRemoteParams{
		ProviderID: providerID, RemoteOrderID: remoteID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// NORMAL BİR DURUM, hata değil.
			//
			// Webhook URL slotları HESAP GENELİDİR (en fazla 3 HTTPS URL,
			// panelden elle). Prod ve staging aynı hesabı kullanıyorsa her
			// ortam tüm hesabın olaylarını alır. Alarm üretmek, gerçek
			// sorunları gürültüde boğardı.
			slog.Debug("webhook bize ait olmayan aktivasyon için", "remote_id", remoteID)
			return nil
		}
		return apperr.Internal(err)
	}

	// Terminal sipariş: yeni mesaj kaydedilir ama durum değişmez (FR-415).
	// Yine de teyide gideriz — ikinci kod da kullanıcıya ulaşmalı.

	// ── (3) SAĞLAYICIDAN TEYİT ──
	//
	// 🔴 GÖVDEDEKİ `code` KULLANILMAZ. HeroSMS imza sunmuyor; URL'i bilen
	// herkes sahte bir "kod geldi" gönderebilir. Kod, sağlayıcının kendi
	// ucundan doğrulanarak alınır.
	//
	// test: webhook_integration_test.go#TestWebhookBodyCodeIsIgnored
	adapter, creds, err := s.providerFor(ctx, providerID)
	if err != nil {
		return err
	}
	vctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()

	status, err := adapter.GetStatus(vctx, creds, remoteID)
	if err != nil {
		if errors.Is(err, port.ErrOrderClosed) {
			// Sipariş kapanmış; geçmiş mesaj okunamaz. Elimizdekiler zaten
			// `order_messages` içinde kalıcı.
			return nil
		}
		// Teyit alınamadı: iş yeniden denenmeli.
		return fmt.Errorf("webhook teyidi başarısız (sipariş %s): %w", ord.PublicID, err)
	}

	// ── (4) TEYİT BOŞSA DUR ──
	//
	// Sahte bir webhook tam burada durur: sağlayıcıda kod yoksa durum
	// DEĞİŞMEZ. Bu metrik sahte webhook göstergesidir.
	if len(status.Messages) == 0 {
		slog.Warn("webhook geldi ama sağlayıcıda kod YOK — durum değiştirilmedi",
			"order", ord.PublicID, "remote_id", remoteID,
			"metric", "provider_webhook_verify_failed_total")
		return nil
	}

	// Kimliksiz mesajlara webhook'taki `id`yi yedek olarak veririz; o da
	// yoksa DeliverMessages içerikten türetir.
	fallbackID := p.idString()
	msgs := make([]port.RemoteMessage, 0, len(status.Messages))
	for i, m := range status.Messages {
		if m.RemoteID == "" && fallbackID != "" {
			m.RemoteID = fallbackID
		}
		if m.ReceivedAt.IsZero() {
			m.ReceivedAt = parseWebhookTime(p.ReceivedAt, s.clock.Now())
		}
		_ = i
		msgs = append(msgs, m)
	}

	// ── (5) Yaz, güncelle, yayınla ──
	return s.DeliverMessages(ctx, ord.ID, msgs)
}

// parseWebhookTime gövdedeki zamanı okur; olmazsa şimdiyi kullanır.
//
// YALNIZ RFC 3339 kabul edilir. Boşluklu bir biçim gelirse ayrıştırmaya
// çalışmayız — yanlış saat dilimiyle kaydedilen bir mesaj, sıralamayı ve
// "en son mesaj" seçimini sessizce bozar.
func parseWebhookTime(s string, fallback time.Time) time.Time {
	if s == "" {
		return fallback
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	// Unix saniye olarak da gelebilir.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 1_000_000_000 {
		return time.Unix(n, 0).UTC()
	}
	return fallback
}
