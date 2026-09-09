package worker

// M5 işleri: kur senkronu, süre dolumu, yoklama, sağlayıcıda kapatma, iade
// mutabakatı, yetim provizyon, katalog tazeleme, saklama politikası.
//
// SİPARİŞ EKSENİNDE ROL AYRIMI — üç iş aynı satırlara bakar, kümeleri
// KESİŞMEZ:
//   - `order-poller`          → PENDING, süresi dolmamış (kod arar)
//   - `provider-refund-retry` → iade ekseni AÇIK (PENDING|RETRY_SCHEDULED)
//   - `activation-reaper`     → iade ekseni KAPALI ama sağlayıcıda açık
//
// Ayrım sorgularda zorlanır (queries/orders.sql): iki iş aynı siparişe aynı
// anda Cancel() gönderirse sağlayıcının verdiği Retry-After süresi çiğnenir.
// test: ../service/order/refund_retry_integration_test.go#TestReaperIgnoresScheduledRefundOrders

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	catalogsvc "github.com/ikmetrik/sms-platform/api/internal/service/catalog"
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
	Catalog  *catalogsvc.Service
	Clock    interface{ Now() time.Time }
}

// All M5 iş kümesini döner.
func All(d Deps) []Job {
	return []Job{
		fxSync(d),
		orderExpirer(d),
		orderPoller(d),
		activationReaper(d),
		refundRetry(d),
		orphanHoldReaper(d),
		offerSync(d),
		rentalSync(d),
		dataRetention(d),
	}
}

/* ═══════════════════════ Kiralık katalog ═══════════════════════ */

// rentalSync kiralık fiyat ve stoklarını tazeler.
//
// SIKLIK: 6 saat. Aktivasyon senkronundan ÇOK DAHA SEYREK, çünkü sağlayıcı
// toplu kiralık katalog sunmuyor: her tur 810 ayrı istek demek. Kiralık
// fiyatlar da aktivasyon kadar oynak değil — bir numara 30 gün kiralanıyorsa
// fiyatı dakikalık değişmiyor.
//
// AÇILIŞTA ÇALIŞMAZ: 810 istek, sunucunun ilk saniyelerinde atılacak en kötü
// şeydir. İlk tur altı saat sonra; o zamana kadar önceki turun verisi geçerli.
func rentalSync(d Deps) Job {
	if d.Catalog == nil {
		// Katalog servisi verilmemişse iş hiç kurulmaz — sessizce çalışmayan
		// bir iş, çalıştığını sandığımız bir iştir.
		return Job{Name: "rental-sync", Every: 0}
	}
	return Job{
		Name: "rental-sync", Every: 6 * time.Hour,
		Run: func(ctx context.Context) error {
			provs, err := d.TxRunner.Queries().ListActiveProviders(ctx)
			if err != nil {
				return err
			}
			for _, p := range provs {
				rep, err := d.Catalog.SyncRentals(ctx, p.ID)
				if err != nil {
					slog.Error("kiralık senkronu başarısız", "provider", p.Name, "err", err)
					continue
				}
				if len(rep.Errors) > 0 {
					slog.Warn("kiralık senkronu uyarılarla bitti",
						"provider", p.Name, "errors", rep.Errors)
				}
			}
			return nil
		},
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

/* ═══════════════════════ Webhook işleme ═══════════════════════ */

// WebhookDequeuer kuyruktan ham bildirim alır.
type WebhookDequeuer interface {
	Dequeue(ctx context.Context, timeout time.Duration) ([]byte, error)
}

// webhookIngest kuyruktaki bildirimleri işler (ADR-024).
//
// SIKLIK 1 SANİYE ama gerçekte BLOKLAYARAK bekler: her tur kuyruktan
// bloklayan bir okuma yapar ve bildirim gelene kadar orada durur. Yoklamalı
// bir döngü, kod bekleyen kullanıcıya ortalama yarım tur gecikme bindirirdi.
//
// Tur başına SINIRLI sayıda bildirim işlenir: bir birikim tek turu
// dakikalarca sürdürmemeli.
// blockingRead kuyruktan bloklayan okumanın süresi.
//
// Redis'in alt sınırı 1 saniye. 2 saniye seçildi: bildirim geldiği anda okuma
// zaten döner, bekleme yalnız kuyruk BOŞKEN yaşanır — uzun tutmak gecikme
// eklemez, tur sayısını (ve log gürültüsünü) yarıya indirir.
const blockingRead = 2 * time.Second

func webhookIngest(d Deps, q WebhookDequeuer, providerName string) Job {
	if q == nil || d.Orders == nil {
		return Job{Name: "webhook-ingest", Every: 0}
	}
	return Job{
		Name: "webhook-ingest", Every: time.Second,
		Run: func(ctx context.Context) error {
			prov, err := d.TxRunner.Queries().GetProviderByName(ctx, providerName)
			if err != nil {
				// Sağlayıcı henüz eklenmemiş olabilir — bildirim de gelmez.
				return nil
			}
			for i := 0; i < 50; i++ {
				raw, err := q.Dequeue(ctx, blockingRead)
				if err != nil {
					return err
				}
				if raw == nil {
					return nil // kuyruk boş
				}
				if err := d.Orders.HandleWebhook(ctx, prov.ID, raw); err != nil {
					// İşlenemeyen bildirim KAYBOLUR. Yeniden kuyruğa koymak
					// sonsuz döngü riski taşır (aynı gövde yine düşer).
					// Güvenlik ağı order-poller: kod en geç 30 saniyede gelir.
					slog.Error("webhook işlenemedi — yoklama devreye girecek", "err", err)
				}
			}
			return nil
		},
	}
}

// AllWithWebhook webhook işçisiyle birlikte tüm işleri döner.
func AllWithWebhook(d Deps, q WebhookDequeuer, providerName string) []Job {
	return append(All(d), webhookIngest(d, q, providerName))
}

/* ═══════════════════════ Sunucu tarafı yoklama ═══════════════════════ */

// orderPoller webhook'u gelmemiş siparişleri toplu yoklar (FR-404).
//
// SIKLIK: 30 saniye. Üç dokümanda farklı yazıyordu; docs/memory.md'de 30
// saniyede birleştirildi (design.md §13, trd.md FR-404).
//
// GÜVENLİK AĞI: birincil yol webhook'tur (ADR-020) ama teslim garantisi
// yoktur, imzası yoktur ve URL'i panelden elle kaydedilir. `webhook-ingest`
// işlenemeyen bildirimi DÜŞÜRÜR ve devamını bu işe emanet eder — o satırdaki
// "kod en geç 30 saniyede gelir" sözünün karşılığı budur.
// test: ../service/order/poller_integration_test.go#TestPollerDeliversCodeWithoutWebhook
//
// SAĞLAYICI YÜKÜ SİPARİŞ SAYISIYLA ORANTILI DEĞİLDİR: toplu uç kullanılır ve
// sayfa sayısı bekleyen sipariş sayısından türer — 100 bekleyen sipariş için
// tur başına en fazla 4 istek (KK-404).
// test: ../service/order/poller_integration_test.go#TestPollerUsesAtMostFourRequestsFor100Orders
//
// AÇILIŞTA ÇALIŞMAZ: ilk tur 30 saniye sonra; sunucunun ilk saniyelerinde
// sağlayıcıya toplu istek atmanın kazancı yok.
func orderPoller(d Deps) Job {
	if d.Orders == nil {
		// Sessizce çalışmayan bir iş, çalıştığını sandığımız bir iştir.
		return Job{Name: "order-poller", Every: 0}
	}
	return Job{
		Name: "order-poller", Every: 30 * time.Second,
		Run: func(ctx context.Context) error {
			rep, err := d.Orders.PollPending(ctx, batchLimit)
			if err != nil {
				return err
			}
			if rep.Scanned > 0 {
				slog.Info("yoklama turu",
					"bekleyen", rep.Scanned, "eslesen", rep.Matched,
					"teslim", rep.Delivered, "saglayici_istegi", rep.ProviderCalls)
			}
			return nil
		},
	}
}

/* ═══════════════════════ Sağlayıcı iade mutabakatı ═══════════════════════ */

// refundRetry sağlayıcıdan iade taleplerini yeniden dener (FR-406b).
//
// SIKLIK: 2 dakika (design.md §13). Sağlayıcının EARLY_CANCEL_DENIED için
// verdiği süre tipik 120 saniyedir; daha sık denemek aynı reddi tekrar tekrar
// sormaktır.
//
// KULLANICIYI ETKİLEMEZ: kullanıcı iadesini çoktan almıştır (FR-406). Burada
// alınamayan tutar BİZİM giderimizdir; `provider_refund_denied_total` log
// alanıyla ölçülür.
//
// SONSUZ DÖNGÜ YOKTUR: üst sınıra ulaşan satır DENIED yazılır ve sağlayıcıya
// bir daha istek gönderilmez.
// test: ../service/order/refund_retry_integration_test.go#TestRefundRetryStopsAtCeiling
//
// ⚠️ Değişmez #6 ile çelişmez: yeniden denenen çağrı `Purchase` değil
// `Cancel`/`Finish`'tir. `Purchase` idempotent olmadığı için asla
// tekrarlanmaz; kapatma çağrıları sağlayıcı tarafında idempotenttir.
func refundRetry(d Deps) Job {
	if d.Orders == nil {
		return Job{Name: "provider-refund-retry", Every: 0}
	}
	return Job{
		Name: "provider-refund-retry", Every: 2 * time.Minute,
		Run: func(ctx context.Context) error {
			rep, err := d.Orders.RetryProviderRefunds(ctx, batchLimit)
			if err != nil {
				return err
			}
			if rep.Scanned > 0 {
				slog.Info("sağlayıcı iade turu",
					"aday", rep.Scanned, "denendi", rep.Attempted,
					"iade", rep.Refunded, "gider", rep.Denied, "kapatildi", rep.Settled)
			}
			return nil
		},
	}
}

/* ═══════════════════════ Aktivasyon kataloğu ═══════════════════════ */

// offerSync aktivasyon fiyat ve stoklarını tazeler.
//
// NEDEN VAR: bugüne kadar YALNIZ `rental-sync` (6 saat) vardı; aktivasyon
// teklifleri hiç tazelenmiyordu. Teklif satırı bayatladığında iki yönlü zarar
// oluşur: (a) düşmüş bir maliyetle değil, yükselmiş maliyetin ALTINDA fiyatla
// satarız — her satış zarar; (b) sağlayıcıda tükenmiş stoku satışa açık
// gösteririz — satın alma sağlayıcı sınırında düşer, kullanıcı hata görür.
//
// SIKLIK: 30 dakika (design.md §13 `offers-sync`).
// SAĞLAYICI YÜKÜ ÖLÇÜLDÜ: `ListOffers` TEK çağrıdır ve ~20.000 kombinasyonu
// birlikte döner (canlı ölçüm 1,09 MB / ~600 ms). Yani tur maliyeti = aktif
// sağlayıcı sayısı kadar istek; tek sağlayıcıda günde 48 istek. Daha sık
// (örn. 5 dk) senkron 288 isteğe çıkar ve her turda ~20.000 satır upsert eder
// — sağlayıcıyı da veritabanını da fiyat oynaklığının hak ettiğinden fazla
// yorar. Teklif geçerlilik süresi zaten 120 saniyedir: satın alma anındaki
// güvence tekliftir, bu iş yalnız vitrini tazeler.
//
// AÇILIŞTA ÇALIŞMAZ: yeniden başlatma fırtınasında (deploy, çökme döngüsü)
// her açılış sağlayıcıya tam bir katalog isteği atardı. Vitrin, en fazla bir
// tur boyunca bir öncekinin verisiyle görünür.
func offerSync(d Deps) Job {
	if d.Catalog == nil {
		return Job{Name: "offer-sync", Every: 0}
	}
	return Job{
		Name: "offer-sync", Every: 30 * time.Minute,
		Run: func(ctx context.Context) error {
			provs, err := d.TxRunner.Queries().ListActiveProviders(ctx)
			if err != nil {
				return err
			}
			for _, p := range provs {
				rep, err := d.Catalog.SyncOffers(ctx, p.ID)
				if err != nil {
					// TEK SAĞLAYICININ HATASI TURU DURDURMAZ.
					slog.Error("teklif senkronu başarısız", "provider", p.Name, "err", err)
					continue
				}
				if len(rep.Errors) > 0 {
					slog.Warn("teklif senkronu uyarılarla bitti",
						"provider", p.Name, "offers", rep.Offers, "errors", rep.Errors)
				}
			}
			return nil
		},
	}
}

/* ═══════════════════════ Saklama politikası ═══════════════════════ */

// Saklama süreleri (9 Eylül 2026 kararı). Gizlilik metninde ilan edilen
// sayılarla BİREBİR aynı olmalıdır: metin bir söz verir, buradaki sabitler o
// sözün tek karşılığıdır. Süreyi değiştiren kişi metni de değiştirmelidir.
//
// SQL karşılıkları ve her birinin gerekçesi: queries/retention.sql
const (
	// smsBodyRetentionDays — SMS içeriği 90 gün sonra boşaltılır.
	smsBodyRetentionDays = 90
	// auditLogRetentionYears — denetim kaydı 2 yıl sonra silinir.
	auditLogRetentionYears = 2
)

// retentionBatch tek SQL ifadesinin dokunacağı azami satır.
//
// `batchLimit` (100) burada kullanılmadı: o sabit sağlayıcıya İSTEK ATAN işler
// için seçilmişti, temizlik ise saf veritabanı işidir ve ağ turu yoktur.
// 1.000 satır, tek ifadenin kilit süresini milisaniyelerde tutacak kadar küçük,
// birikmiş bir kuyruğu makul turda eritecek kadar büyüktür.
const retentionBatch = 1000

// retentionMaxBatches bir turda bir tablo için atılacak azami parti.
//
// ÜST SINIR VAR çünkü ilk koşu bir BİRİKİMLE karşılaşır: iş bugüne kadar hiç
// çalışmadı, dolayısıyla ilk tur o güne kadarki tüm eski veriyi görür. Sınırsız
// bir döngü o turu saatlerce sürdürebilir ve tabloyu sürekli meşgul ederdi.
// 50 × 1.000 = tur başına tablo başına 50.000 satır; kalanı bir sonraki tura
// devreder.
const retentionMaxBatches = 50

// dataRetention ilan edilen saklama sürelerini uygular.
//
// SIKLIK: 1 saat. Günlük seçilseydi, günde birden fazla yeniden başlatılan bir
// sunucuda (dağıtım, çökme döngüsü) iş HİÇ çalışmayabilirdi — `Every` sayacı
// süreçle birlikte sıfırlanır. Saatlik tur ilk koşudan sonra neredeyse
// bedeldir: eşleşen satır kalmayınca her adım tek sorguda 0 döner.
//
// AÇILIŞTA ÇALIŞMAZ: dağıtım anında beş tabloyu birden taramanın kazancı yok;
// en fazla bir saat gecikir.
//
// TUR SIRASINDA BİR ADIMIN HATASI DİĞERLERİNİ DURDURMAZ: denetim kaydı
// tablosundaki bir kilit, SMS gövdelerinin boşaltılmasını erteleyemez.
//
// LOG'A YALNIZ SAYI YAZILIR. Silinen satırın içeriği log'a geçseydi, tam da
// veritabanından kaldırdığımız kişisel veriyi log dosyasına kopyalamış olurduk.
// test: retention_integration_test.go#TestRetentionRedactsOldSmsBodyKeepsRow
func dataRetention(d Deps) Job {
	return Job{
		Name: "data-retention", Every: time.Hour,
		Run: func(ctx context.Context) error {
			q := d.TxRunner.Queries()
			now := d.Clock.Now()

			// AddDate takvim aritmetiği yapar: "2 yıl" 730 gün değil, iki
			// takvim yılıdır. İlan edilen süre insan takvimindedir.
			smsCutoff := now.AddDate(0, 0, -smsBodyRetentionDays)
			auditCutoff := now.AddDate(-auditLogRetentionYears, 0, 0)

			steps := []retentionStep{
				{"sms_govdesi", func(c context.Context, lim int32) (int64, error) {
					return q.RedactOldOrderMessages(c, db.RedactOldOrderMessagesParams{
						OlderThan: smsCutoff, Lim: lim})
				}},
				{"oturum", func(c context.Context, lim int32) (int64, error) {
					return q.DeleteExpiredSessions(c, db.DeleteExpiredSessionsParams{
						OlderThan: now, Lim: lim})
				}},
				{"token", func(c context.Context, lim int32) (int64, error) {
					return q.DeleteExpiredAuthTokens(c, db.DeleteExpiredAuthTokensParams{
						OlderThan: now, Lim: lim})
				}},
				{"teklif", func(c context.Context, lim int32) (int64, error) {
					return q.DeleteExpiredQuotes(c, db.DeleteExpiredQuotesParams{
						OlderThan: now, Lim: lim})
				}},
				{"denetim_kaydi", func(c context.Context, lim int32) (int64, error) {
					return q.DeleteOldAuditLogs(c, db.DeleteOldAuditLogsParams{
						OlderThan: auditCutoff, Lim: lim})
				}},
			}

			var firstErr error
			fields := make([]any, 0, 2*len(steps))
			for _, s := range steps {
				n, err := drainBatches(ctx, s.run)
				if err != nil {
					// Adım adı ve o ana kadar işlenen SAYI yazılır; satırın
					// kendisi yazılmaz.
					slog.Error("saklama temizliği adımı yarım kaldı",
						"adim", s.name, "islenen", n, "err", err)
					if firstErr == nil {
						firstErr = fmt.Errorf("saklama adımı %q: %w", s.name, err)
					}
				}
				if n > 0 {
					fields = append(fields, s.name, n)
				}
			}
			if len(fields) > 0 {
				slog.Info("saklama politikası uygulandı", fields...)
			}
			return firstErr
		},
	}
}

// retentionStep tek bir tablonun temizlik adımı.
type retentionStep struct {
	name string
	run  func(ctx context.Context, lim int32) (int64, error)
}

// drainBatches bir adımı, iş bitene ya da üst sınıra ulaşılana kadar partiler
// hâlinde koşturur ve toplam etkilenen satır sayısını döner.
//
// SON PARTİ TAM DOLU DEĞİLSE İŞ BİTMİŞTİR: sorgu `LIMIT retentionBatch` ile
// yazıldığı için `n < retentionBatch` "eşleşen satır kalmadı" demektir. Bu
// sayede boş bir turda tek sorgu çalışır, döngü dönmez.
func drainBatches(ctx context.Context, step func(context.Context, int32) (int64, error)) (int64, error) {
	var total int64
	for i := 0; i < retentionMaxBatches; i++ {
		n, err := step(ctx, retentionBatch)
		total += n
		if err != nil {
			return total, err
		}
		if n < retentionBatch {
			return total, nil
		}
	}
	return total, nil
}
