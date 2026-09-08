package order

// Sağlayıcı iade mutabakatı (FR-406b).
//
// KULLANICI İADESİYLE KARIŞTIRILMAMALI: kullanıcıya iade koşulsuz ve anında
// yapılır (FR-406) ve burada deftere HİÇBİR kayıt yazılmaz. Buradaki konu,
// sağlayıcıdan bizim paramızı geri alıp alamadığımızdır — alamadığımız her
// tutar BİZİM giderimizdir ve ölçülebilir olmalıdır (KK-406b).
//
// ⚠️ Değişmez #6 ile çelişmez: `Purchase` asla yeniden denenmez çünkü
// idempotent değildir ve bir tekrar ikinci bir numara satın alır. Bu iş
// `Purchase`'a hiç dokunmaz; yeniden denediği çağrı `Cancel`/`Finish`'tir —
// sağlayıcı tarafında idempotenttir (kapalı siparişte 204/nil döner) ve
// hiçbir tekrarında yeni bir kaynak yaratmaz.
//
// test: refund_retry_integration_test.go#TestUserRefundIsUnaffectedByProviderRefundOutcome

import (
	"errors"
	"log/slog"

	"context"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// maxRefundAttempts sağlayıcı iadesi için deneme üst sınırı.
//
// SONSUZ DENEME KABUL EDİLMEZ: sağlayıcı kalıcı olarak ağ hatası veriyorsa
// aday satır her turda yeniden çekilir ve iki tarafı da yorar.
//
// Sayı neden 8: `refund_attempts` SAF bir deneme sayacı DEĞİLDİR —
// `SetProviderRefundStatus` her yazımda +1 yapar, başarı yazımlarında da, ve
// sipariş kuyruğa zaten attempts=1 ile girer (transitionAndRefund). Sınır bu
// kirliliği hesaba katarak cömert seçildi: ~7 gerçek deneme × ≥2 dk, yani
// sağlayıcının 20 dakikalık ücretsiz iptal penceresinin ötesi. O pencereden
// sonra sağlayıcı zaten FREE_CANCELLATION_EXPIRED ile kalıcı reddeder.
//
// test: refund_retry_integration_test.go#TestRefundRetryStopsAtCeiling
const maxRefundAttempts = 8

// RefundRetryReport bir iade turunun özeti.
type RefundRetryReport struct {
	Scanned   int // aday satır
	Attempted int // sağlayıcıya gidilen satır
	Refunded  int // sağlayıcı iade etti
	Denied    int // üst sınır aşıldı → gider
	Settled   int // zaten kapalıydı, iade ekseni kapatıldı
}

// RetryProviderRefunds bekleyen sağlayıcı iadelerini yeniden dener (FR-406b).
func (s *Service) RetryProviderRefunds(ctx context.Context, limit int32) (RefundRetryReport, error) {
	var rep RefundRetryReport

	now := s.clock.Now()
	q := s.tx.Queries()
	// DİKKAT: bu sorgunun Now alanı *time.Time'dır (nullable kolonla
	// karşılaştırma). nil geçilirse iş sessizce yalnız PENDING satırları
	// işler ve zamanlanmış iadeler hiç denenmez.
	rows, err := q.ListRefundRetryOrders(ctx, db.ListRefundRetryOrdersParams{Now: &now, Lim: limit})
	if err != nil {
		return rep, apperr.Internal(err)
	}
	rep.Scanned = len(rows)

	for _, ord := range rows {
		// (a) Sağlayıcıda ZATEN kapatılmış ama iade ekseni açık kalmış satır.
		// Bir daha istek göndermenin anlamı yok; eksen kapatılır.
		if ord.ProviderClosedAt != nil {
			if err := s.settleRefundAxis(ctx, ord.ID, db.RefundStatusNOTAPPLICABLE); err != nil {
				slog.Error("iade ekseni kapatılamadı", "order", ord.PublicID, "err", err)
				continue
			}
			rep.Settled++
			continue
		}

		// (b) Üst sınır: iade ekseni KAPANIR ve sağlayıcıya bir daha istek
		// gönderilmez. `provider_closed_at`e DOKUNULMAZ — kapatma ekseni
		// (FR-412) activation-reaper'ındır ve orada asla pes edilmez.
		// test: refund_retry_integration_test.go#TestRefundRetryStopsAtCeiling
		if ord.RefundAttempts >= maxRefundAttempts {
			if err := s.settleRefundAxis(ctx, ord.ID, db.RefundStatusDENIED); err != nil {
				slog.Error("iade ekseni kapatılamadı", "order", ord.PublicID, "err", err)
				continue
			}
			rep.Denied++
			// GİDER KALEMİ: sıfırdan farklıysa sağlayıcı davranışı gözden
			// geçirilmelidir.
			slog.Warn("sağlayıcı iadesi alınamadı — GİDER yazıldı",
				"order", ord.PublicID, "attempts", ord.RefundAttempts,
				"amount_micro", ord.CostMicro, "metric", "provider_refund_denied_total")
			continue
		}

		// (c) Deneme. Sahiplenme, sağlayıcı çağrısından hemen önce
		// CloseAtProvider içinde yapılır: iki işçi aynı satıra düşse bile
		// sağlayıcıya tek çağrı gider.
		rep.Attempted++
		if err := s.CloseAtProvider(ctx, ord.ID); err != nil {
			// SINIFLANDIRMA BURADA YAPILMAZ: hata tipine göre durum yazımı
			// recordCloseFailure içindedir ve tek yerde kalmalıdır.
			// test: refund_retry_integration_test.go#TestRefundRetrySucceedsAfterProviderRetryAfter
			switch {
			case isRetryAfter(err):
				slog.Info("sağlayıcı iadesi ertelendi", "order", ord.PublicID, "err", err)
			case errors.Is(err, port.ErrCancelDenied):
				slog.Warn("sağlayıcı iadesi kalıcı reddedildi — GİDER",
					"order", ord.PublicID, "metric", "provider_refund_denied_total")
			default:
				slog.Error("sağlayıcı iadesi denenemedi", "order", ord.PublicID, "err", err)
			}
			continue
		}
		rep.Refunded++
	}
	return rep, nil
}

func isRetryAfter(err error) bool {
	_, ok := port.AsRetryAfter(err)
	return ok
}

// settleRefundAxis iade eksenini kapatır.
//
// DEFTERE HİÇBİR KAYIT YAZILMAZ. Sağlayıcının iadeyi reddetmesi bizim
// giderimizdir; kullanıcının bakiyesiyle ilgisi yoktur ve kullanıcı iadesini
// çoktan almıştır.
// test: refund_retry_integration_test.go#TestUserRefundIsUnaffectedByProviderRefundOutcome
func (s *Service) settleRefundAxis(ctx context.Context, orderID int64, st db.RefundStatus) error {
	if err := s.tx.Queries().SettleProviderRefund(ctx, db.SettleProviderRefundParams{
		ID: orderID, ProviderRefundStatus: st,
	}); err != nil {
		return apperr.Internal(err)
	}
	return nil
}
