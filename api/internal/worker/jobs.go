package worker

// M5 işleri: kur senkronu, süre dolumu, sağlayıcıda kapatma, yetim provizyon.
//
// FR-404 (yoklama) ve FR-406b (iade yeniden deneme) buraya eklenecek; bugünkü
// set, PARANIN ASILI KALMASINI önleyen asgari kümedir.

import (
	"context"
	"log/slog"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	ordersvc "github.com/ikmetrik/sms-platform/api/internal/service/order"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
)

// batchLimit bir turda işlenecek azami kayıt.
//
// SINIRSIZ İŞLEMEK TEHLİKELİDİR: bir birikim (örn. sağlayıcı kesintisi sonrası
// 10.000 sipariş) tek turu dakikalarca sürdürür, sonraki turlar üst üste biner
// ve sağlayıcıya istek fırtınası gider.
const batchLimit = 100

type txRunner interface {
	Queries() *db.Queries
	InTx(ctx context.Context, fn func(*db.Queries) error) error
}

// Deps iş bağımlılıkları.
type Deps struct {
	TxRunner txRunner
	Orders   *ordersvc.Service
	FX       *pricingsvc.FXService
	Wallet   *walletsvc.Service
	Clock    interface{ Now() time.Time }
}

// All M5 iş kümesini döner.
func All(d Deps) []Job {
	return []Job{
		fxSync(d),
		orderExpirer(d),
		activationReaper(d),
		orphanHoldReaper(d),
	}
}

/* ═══════════════════════ Kur senkronu ═══════════════════════ */

// fxSync döviz kurunu tazeler.
//
// SIKLIK: 10 dakika. Kur azami yaşı 30 dakikadır (FX_MAX_AGE); üç deneme
// hakkımız olsun diye üçte biri seçildi. Tek deneme hakkı olsaydı, TCMB'nin
// bir kesintisi doğrudan "site satış yapmıyor" demek olurdu.
//
// AÇILIŞTA HEMEN ÇALIŞIR: sunucu yeniden başladığında kur bayatsa ilk turu
// beklemek 10 dakika satış kaybıdır.
func fxSync(d Deps) Job {
	return Job{
		Name: "fx-sync", Every: 10 * time.Minute, RunAtStart: true,
		Run: func(ctx context.Context) error {
			// Hata YUTULMAZ ama iş DÜŞMEZ: kur çekilemezse son bilinen kur
			// kullanılmaya devam eder ve bayatlarsa Current() satışı durdurur.
			// Burada patlamak, bir sonraki turu da engellerdi.
			if err := d.FX.Refresh(ctx); err != nil {
				slog.Warn("kur tazelenemedi — son bilinen kur geçerli", "err", err)
			}
			return nil
		},
	}
}

/* ═══════════════════════ Süre dolumu ═══════════════════════ */

// orderExpirer süresi dolmuş siparişleri iptal edip iade eder (FR-405).
//
// SIKLIK: 30 saniye. Kullanıcı süresi dolan numaranın parasını dakikalarca
// bekleyemez; öte yandan her saniye taramak boşuna yüktür.
//
// KULLANICI HİÇBİR ŞEY YAPMAZ (KK-405): iade bu iş sayesinde kendiliğinden
// gelir. Bu iş durursa para kullanıcıda değil bizde asılı kalır.
func orderExpirer(d Deps) Job {
	return Job{
		Name: "order-expirer", Every: 30 * time.Second,
		Run: func(ctx context.Context) error {
			now := d.Clock.Now()
			rows, err := d.TxRunner.Queries().ListExpiredPendingOrders(ctx,
				db.ListExpiredPendingOrdersParams{Now: now, Lim: batchLimit})
			if err != nil {
				return err
			}
			for _, o := range rows {
				// TEK SİPARİŞİN HATASI TURU DURDURMAZ: biri kilitliyse veya
				// sağlayıcı yanıt vermiyorsa diğerlerinin iadesi beklemez.
				if err := d.Orders.Expire(ctx, o.ID); err != nil {
					slog.Error("sipariş süresi işlenemedi",
						"order", o.PublicID, "err", err)
				}
			}
			if len(rows) > 0 {
				slog.Info("süresi dolan siparişler işlendi", "count", len(rows))
			}
			return nil
		},
	}
}

/* ═══════════════════════ Sağlayıcıda kapatma ═══════════════════════ */

// activationReaper terminal ama sağlayıcıda kapatılmamış siparişleri kapatır (FR-412).
//
// SIKLIK: 60 saniye. Kapatma normalde satın alma/iptal anında senkron denenir;
// bu iş yalnız BAŞARISIZ olanları toplar.
//
// KK-412: uzun vadede `provider_closed_at IS NULL` olan terminal sipariş
// SAYISI SIFIR olmalıdır. Sıfır değilse ya bu iş çalışmıyordur ya da sağlayıcı
// kalıcı olarak reddediyordur — ikisi de görünür olmalı.
func activationReaper(d Deps) Job {
	return Job{
		Name: "activation-reaper", Every: 60 * time.Second,
		Run: func(ctx context.Context) error {
			rows, err := d.TxRunner.Queries().ListUnclosedTerminalOrders(ctx, batchLimit)
			if err != nil {
				return err
			}
			var closed, failed int
			for _, o := range rows {
				if err := d.Orders.CloseAtProvider(ctx, o.ID); err != nil {
					failed++
					continue
				}
				closed++
			}
			if len(rows) > 0 {
				slog.Info("sağlayıcıda kapatma turu",
					"aday", len(rows), "kapatildi", closed, "basarisiz", failed)
			}
			return nil
		},
	}
}

/* ═══════════════════════ Yetim provizyon ═══════════════════════ */

// orphanHoldGrace bir teklifin "yetim" sayılması için geçmesi gereken süre.
//
// Satın alma T1 → sağlayıcı → T2 zinciri birkaç saniye sürer. Hemen yetim ilan
// etmek, HÂLÂ ÇALIŞAN bir satın almayı iade eder ve kullanıcıya hem numara hem
// para verir. Sağlayıcı zaman aşımı 10 saniye; iki katı güvenli.
const orphanHoldGrace = 2 * time.Minute

// orphanHoldReaper T1 ile T2 arasında ölen satın almaların parasını iade eder (FR-408).
//
// SENARYO: teklif tüketildi, bakiye düşüldü, sonra süreç öldü. Kullanıcının
// parası gitti ama ne numarası var ne siparişi. Bu iş o durumu bulup parayı
// geri verir.
//
// İdempotency anahtarı satın alma yolundakiyle AYNIDIR
// (`order:{quoteId}:refund`): satın alma yolu iadeyi zaten yazdıysa bu iş
// ikinci kez yazmaz.
func orphanHoldReaper(d Deps) Job {
	return Job{
		Name: "orphan-hold-reaper", Every: 60 * time.Second,
		Run: func(ctx context.Context) error {
			cutoff := d.Clock.Now().Add(-orphanHoldGrace)
			rows, err := d.TxRunner.Queries().ListOrphanHolds(ctx,
				db.ListOrphanHoldsParams{OlderThan: &cutoff, Lim: batchLimit})
			if err != nil {
				return err
			}
			for _, q := range rows {
				quoteID := q.PublicID.String()
				err := d.TxRunner.InTx(ctx, func(qq *db.Queries) error {
					_, err := d.Wallet.Apply(ctx, qq, walletsvc.Input{
						UserID:         q.UserID,
						Amount:         money.New(q.SellPriceMinor, money.TRY),
						Type:           db.LedgerTypeREFUND,
						IdempotencyKey: "order:" + quoteID + ":refund",
						ReferenceType:  "quote",
						ReferenceID:    quoteID,
						Note:           "Satın alma tamamlanamadı — otomatik iade",
					})
					return err
				})
				if err != nil {
					slog.Error("yetim provizyon iadesi yazılamadı",
						"quote", quoteID, "err", err)
					continue
				}
				// Bu log SESSİZ KALMAMALI: yetim provizyon, T1–T2 arasında bir
				// çökme demektir ve sıklığı artıyorsa altta bir sorun vardır.
				slog.Warn("yetim provizyon iade edildi — T1/T2 arasında kesinti olmuş",
					"quote", quoteID, "user", q.UserID, "amount_minor", q.SellPriceMinor)
			}
			return nil
		},
	}
}
