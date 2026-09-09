package order

// Sunucu tarafı yoklama (FR-404) — webhook'un GÜVENLİK AĞI.
//
// Birincil yol webhook'tur (ADR-020) ama teslim garantisi yoktur, imzası
// yoktur ve URL'i sağlayıcı panelinden elle kaydedilir. Bir bildirim
// kaybolduğunda kullanıcı kodunu HİÇ göremez, süresi dolar ve iade yazılır:
// hem müşteri hem sağlayıcı maliyeti kaybı. Bu iş o boşluğu kapatır.

import (
	"context"
	"errors"
	"log/slog"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// pollPageSize sağlayıcının sayfa üst sınırı (provider-herosms.md §7.2).
const pollPageSize = 25

// pollFallbackLimit toplu yoklamayı DESTEKLEMEYEN sağlayıcıda tur başına
// atılacak azami tekil istek.
//
// Böyle bir sağlayıcıda yük kaçınılmaz olarak sipariş sayısıyla büyür; üst
// sınır, o büyümeyi bir turla sınırlar. KK-404'ün asıl güvencesi toplu yoldur.
const pollFallbackLimit = 25

// PollReport bir yoklama turunun özeti.
type PollReport struct {
	Scanned       int // taranan bekleyen sipariş
	Matched       int // sağlayıcı yanıtında bulunanlar
	Delivered     int // kodu teslim edilenler
	ProviderCalls int // sağlayıcıya giden istek (KK-404 ölçüsü)
}

// PollPending bekleyen siparişleri sağlayıcıdan TOPLU yoklar (FR-404).
//
// Worker sağlayıcı adaptörüne erişemez (katman kuralı); sağlayıcı seçimi,
// sayfalama ve teslim bu metodun içindedir.
func (s *Service) PollPending(ctx context.Context, limit int32) (PollReport, error) {
	now := s.clock.Now()
	rows, err := s.tx.Queries().ListPendingOrdersForPoll(ctx,
		db.ListPendingOrdersForPollParams{Now: now, Lim: limit})
	if err != nil {
		return PollReport{}, apperr.Internal(err)
	}
	return s.pollRows(ctx, rows), nil
}

// PollRentals dönemi süren kiralıkları yoklar (`rental-poller` işi).
//
// AYRI BİR TURDUR, aynı listeye eklenmemiştir — sıklık farkı yüzünden.
// Aktivasyon ~20 dakika yaşar ve 30 saniyelik bir ağ o ömre orantılıdır;
// kiralık 24–4320 saat yaşar. 100 kiralığı 30 saniyelik tura sokmak günde
// 11.520 sağlayıcı isteği eder ve aktivasyonların sayfa bütçesini de şişirerek
// KK-404'ün tek uygulayıcısını bozar. 5 dakikada aynı yük 1.152 isteğe iner;
// en kısa kiralıkta (24 saat) en kötü gecikme ömrün %0,35'idir ve webhook
// zaten birincil yoldur (ADR-020) — yoklama güvenlik ağıdır.
//
// KİRALIKTA YOKLAMA İLK MESAJLA BİTMEZ: 'ACTIVE' siparişler de taranır, çünkü
// ürünün tamamı "dönem boyunca gelen HER mesajı gör"dür.
// test: rental_integration_test.go#TestRentalPollerDeliversLaterMessages
// 🔴 TUR DÖNÜŞÜMLÜDÜR. Kiralık küme dönem boyunca (24–4320 saat) DEĞİŞMEZ:
// sabit bir sıralamayla `LIMIT` kullanmak, 101. kiralığın 30 gün boyunca hiç
// yoklanmaması demektir (aktivasyonda aynı desen güvenlidir çünkü satırlar ~20
// dakikada kümeden çıkar). Sorgu `last_polled_at`e göre sıralar; bu metot da
// turun damgasını atar.
func (s *Service) PollRentals(ctx context.Context, limit int32) (PollReport, error) {
	now := s.clock.Now()
	q := s.tx.Queries()
	rows, err := q.ListActiveRentalsForPoll(ctx,
		db.ListActiveRentalsForPollParams{Now: now, Lim: limit})
	if err != nil {
		return PollReport{}, apperr.Internal(err)
	}

	// DAMGA SAĞLAYICI ÇAĞRISINDAN ÖNCE atılır: sağlayıcı erişilemez olsa bile
	// tur ilerlemeli, yoksa arızalı tek bir grup kümenin geri kalanını sonsuza
	// kadar aç bırakır — düzeltmeye çalıştığımız açlığın aynısı, sebebi farklı.
	if len(rows) > 0 {
		ids := make([]int64, len(rows))
		for i, o := range rows {
			ids[i] = o.ID
		}
		if err := q.MarkOrdersPolled(ctx, db.MarkOrdersPolledParams{Now: &now, Ids: ids}); err != nil {
			// Damga yazılamazsa tur yine de yapılır: mesaj teslimi damgadan
			// daha önemlidir. Bir sonraki tur aynı satırları görür (açlık
			// riski geri döner), bu yüzden sessiz geçilmez.
			slog.Error("kiralık yoklama damgası yazılamadı — tur dönüşümü duraklayabilir",
				"satir", len(ids), "err", err)
		}
	}
	return s.pollRows(ctx, rows), nil
}

// pollRows verilen sipariş kümesini sağlayıcı bazında yoklar.
//
// AKTİVASYON VE KİRALIK AYNI GÖVDEYİ PAYLAŞIR: iki ayrı uygulama yazmak,
// birinde düzeltilen hatanın diğerinde kalmasıyla sonuçlanır (DeliverMessages
// ile aynı gerekçe).
func (s *Service) pollRows(ctx context.Context, rows []db.Order) PollReport {
	var rep PollReport
	rep.Scanned = len(rows)
	if len(rows) == 0 {
		return rep
	}

	// Sağlayıcıya göre grupla: adaptör ve kimlik bilgisi sağlayıcı başına BİR
	// kez çözülür.
	byProvider := make(map[int64][]db.Order, 2)
	for _, o := range rows {
		byProvider[o.ProviderID] = append(byProvider[o.ProviderID], o)
	}

	for provID, orders := range byProvider {
		// TEK SAĞLAYICININ HATASI TURU DURDURMAZ: biri erişilemezse
		// diğerinin siparişleri kodsuz kalmamalı.
		if err := s.pollProvider(ctx, provID, orders, &rep); err != nil {
			slog.Error("sağlayıcı yoklanamadı", "provider", provID, "err", err)
		}
	}
	return rep
}

func (s *Service) pollProvider(ctx context.Context, providerID int64, orders []db.Order, rep *PollReport) error {
	adapter, creds, err := s.providerFor(ctx, providerID)
	if err != nil {
		return err
	}

	// Korelasyon haritası: sağlayıcı YALNIZ aktivasyon kimliğini taşır
	// (webhook yolundaki eşleştirmenin toplu karşılığı).
	pending := make(map[string]db.Order, len(orders))
	for _, o := range orders {
		pending[o.RemoteOrderID] = o
	}

	bp, ok := adapter.(port.BatchPoller)
	if !ok {
		// Toplu uç yoksa tekil yola düşeriz. Sahte bir "toplu" uygulama
		// yazmak, sessizce N istek atan bir yol üretirdi (port.BatchPoller
		// yorumundaki gerekçe).
		return s.pollOneByOne(ctx, adapter, creds, orders, rep)
	}

	// 🔴 KK-404'ÜN TEK UYGULAYICISI: sayfa sayısı BİZİM bekleyen sipariş
	// sayımızdan türer. `NextCursor` boş dönene kadar sayfalamak yanlış
	// olurdu — `GET /activations` HESABIN TÜM aktif aktivasyonlarını döner
	// (prod/staging aynı hesabı paylaşırsa birbirininkileri de) ve bin
	// aktivasyonda kırk istek atardık.
	// test: poller_integration_test.go#TestPollerUsesAtMostFourRequestsFor100Orders
	maxPages := (len(pending) + pollPageSize - 1) / pollPageSize
	if maxPages < 1 {
		maxPages = 1
	}

	cursor := ""
	for page := 0; page < maxPages; page++ {
		cctx, cancel := context.WithTimeout(ctx, providerTimeout)
		pg, err := bp.ListActive(cctx, creds, cursor, pollPageSize)
		cancel()
		rep.ProviderCalls++
		if err != nil {
			return err
		}

		for _, it := range pg.Items {
			ord, ok := pending[it.RemoteOrderID]
			if !ok {
				// Bize ait olmayan aktivasyon — NORMAL bir durum, hata değil.
				// Sağlayıcı hesabı geneldir; başkasının (ya da staging'in)
				// aktivasyonu bizim siparişimize YAZILMAZ.
				// test: poller_integration_test.go#TestPollerSkipsForeignActivations
				continue
			}
			delete(pending, it.RemoteOrderID)
			rep.Matched++

			if len(it.Messages) == 0 {
				// Sağlayıcının State değeri `orders.status` alanına
				// YANSITILMAZ (değişmez #13): durum yalnız DeliverMessages
				// veya Expire üzerinden değişir. Mesajsız "tamamlandı"
				// yanıtı ölçülebilir olsun diye log'lanır.
				// test: poller_integration_test.go#TestPollerNeverWritesProviderStateToOrderStatus
				if it.State == port.StateCompleted {
					slog.Warn("sağlayıcı tamamlandı dedi ama kod yok",
						"order", ord.PublicID, "metric", "provider_poll_completed_without_otp")
				}
				continue
			}

			// Kod teslimi TEK fonksiyondan geçer: webhook-ingest de aynı
			// yolu kullanır ve idempotens orada tanımlıdır.
			if err := s.DeliverMessages(ctx, ord.ID, it.Messages); err != nil {
				slog.Error("yoklamayla gelen kod kaydedilemedi",
					"order", ord.PublicID, "err", err)
				continue
			}
			rep.Delivered++
		}

		// ERKEN ÇIKIŞ: aradığımız her sipariş bulunduysa kalan sayfalar
		// istenmez.
		if len(pending) == 0 || pg.NextCursor == "" {
			break
		}
		cursor = pg.NextCursor
	}
	return nil
}

// pollOneByOne toplu yoklamayı desteklemeyen sağlayıcı için yedek yol.
func (s *Service) pollOneByOne(
	ctx context.Context, adapter port.ProviderPort, creds port.Creds,
	orders []db.Order, rep *PollReport,
) error {
	if len(orders) > pollFallbackLimit {
		slog.Warn("sağlayıcı toplu yoklamayı desteklemiyor — tur kırpıldı",
			"bekleyen", len(orders), "islenen", pollFallbackLimit)
		orders = orders[:pollFallbackLimit]
	}
	for _, ord := range orders {
		cctx, cancel := context.WithTimeout(ctx, providerTimeout)
		st, err := adapter.GetStatus(cctx, creds, ord.RemoteOrderID)
		cancel()
		rep.ProviderCalls++
		if err != nil {
			// Kapanmış veya tanınmayan sipariş yoklamanın işi değildir;
			// süresi dolduğunda order-expirer ilgilenir.
			if errors.Is(err, port.ErrOrderClosed) || errors.Is(err, port.ErrOrderNotFound) {
				continue
			}
			slog.Error("sipariş yoklanamadı", "order", ord.PublicID, "err", err)
			continue
		}
		rep.Matched++
		if len(st.Messages) == 0 {
			continue
		}
		if err := s.DeliverMessages(ctx, ord.ID, st.Messages); err != nil {
			slog.Error("yoklamayla gelen kod kaydedilemedi",
				"order", ord.PublicID, "err", err)
			continue
		}
		rep.Delivered++
	}
	return nil
}
