package order

// Sipariş yaşam döngüsü: iptal, iade, kod teslimi, süre dolumu.
//
// Ortak kural: durum değişikliği ÖNCE domain/order.Transition ile doğrulanır,
// SONRA kilitli bir transaction içinde yazılır. Veritabanındaki tetikleyici
// üçüncü savunma hattıdır.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	orderdom "github.com/ikmetrik/sms-platform/api/internal/domain/order"
	"github.com/ikmetrik/sms-platform/api/internal/port"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
)

/* ═══════════════════════════ Görüntüleme ═══════════════════════════ */

// Detail sipariş + mesajları.
type Detail struct {
	Order    db.Order
	Messages []db.OrderMessage
}

// Get kullanıcının siparişini döner.
//
// SAHİPLİK SORGUNUN PARÇASIDIR: bulunamayan ve başkasına ait sipariş AYNI
// hatayı verir. Farklı hata vermek, sipariş kimliklerinin varlığını sızdırır.
func (s *Service) Get(ctx context.Context, userID int64, publicID uuid.UUID) (Detail, error) {
	q := s.tx.Queries()
	ord, err := q.GetOrderForUser(ctx, db.GetOrderForUserParams{PublicID: publicID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, apperr.ErrOrderNotFound
		}
		return Detail{}, apperr.Internal(err)
	}
	msgs, err := q.ListOrderMessages(ctx, ord.ID)
	if err != nil {
		return Detail{}, apperr.Internal(err)
	}
	return Detail{Order: ord, Messages: msgs}, nil
}

// ListeSatiri sipariş kaydı + CANLI katalogdan gelen logo.
//
// Logo `db.Order` içinde DEĞİLDİR ve olmamalıdır: sipariş bir para kaydıdır ve
// 10 yıl saklanır; logo ise sunum verisidir, katalogla birlikte değişir.
// İkisini tek yapıda birleştirmek, satış kaydına sunum alanı sızdırırdı.
type ListeSatiri struct {
	Order   db.Order
	IconURL string
}

// ListeSuzgeci sipariş geçmişi süzgeçleri.
//
// `Q` ve `Status` NIL ise o süzgeç UYGULANMAZ — boş dize "hiçbir şeyle eşleşme"
// demek değildir, "süzme" demektir. Bu ayrımı taşımak için işaretçi kullanılır;
// transport katmanı boş sorgu parametresini nil'e çevirir.
type ListeSuzgeci struct {
	Q      *string
	Status *db.OrderStatus
	// SadeceAktif "kodu bekleyen ya da süren" siparişler: PENDING **veya**
	// ACTIVE. Tek durumlu `Status` ile ifade EDİLEMEZ; ayrı alan olmasının
	// gerekçesi queries/orders.sql başındaki nottadır.
	SadeceAktif bool
	Limit       int32
	Offset      int32
}

// List kullanıcının sipariş geçmişi.
//
// 🔴 `userID` SÜZGEÇTEN AYRI BİR PARAMETREDİR ve sorguya her zaman girer
// (değişmez #7). Süzgeç yapısına konulsaydı, çağıranın onu doldurmayı
// unutması bütün kullanıcıların siparişlerini döndürürdü; ayrı parametre
// bunu derleme zamanında imkânsız kılar.
//
// Çalışma zamanı tarafı da bağlıdır: başka kullanıcının siparişine BİREBİR
// uyan bir `q` bile onu döndürmez.
// test: order_integration_test.go#TestListUserOrdersFiltreleri
func (s *Service) List(ctx context.Context, userID int64, f ListeSuzgeci) ([]ListeSatiri, int64, error) {
	q := s.tx.Queries()
	rows, err := q.ListUserOrders(ctx, db.ListUserOrdersParams{
		UserID: userID, Q: f.Q, Status: f.Status, OnlyActive: f.SadeceAktif,
		Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	// Sayım listeyle AYNI süzgeci alır; ayrışırsa sayfalama yalan söyler
	// (bkz. queries/orders.sql, CountUserOrders notu).
	total, err := q.CountUserOrders(ctx, db.CountUserOrdersParams{
		UserID: userID, Q: f.Q, Status: f.Status, OnlyActive: f.SadeceAktif,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	out := make([]ListeSatiri, 0, len(rows))
	for _, r := range rows {
		out = append(out, ListeSatiri{Order: r.Order, IconURL: r.IconUrl})
	}
	return out, total, nil
}

/* ═══════════════════════════ İptal ═══════════════════════════ */

// Cancel kullanıcının iptal isteği.
//
// SIRA — bu sıra ürün ilkesidir, teknik tercih değil:
//  1. Sipariş CANCELLED yapılır ve kullanıcıya iade EDİLİR (koşulsuz, anında).
//  2. Sağlayıcıya iptal bildirimi AYRI ve ASENKRON yapılır.
//
// İkisini birleştirip "sağlayıcı onaylarsa iade edelim" deseydik, sağlayıcı
// 120 saniye "erken" dediğinde kullanıcı parasını alamazdı. Sağlayıcının
// iadeyi reddetmesi BİZİM giderimizdir; kullanıcının sorunu değil.
func (s *Service) Cancel(ctx context.Context, userID int64, publicID uuid.UUID) (db.Order, error) {
	var out db.Order

	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		ord, err := q.GetOrderForUser(ctx, db.GetOrderForUserParams{PublicID: publicID, UserID: userID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.ErrOrderNotFound
			}
			return apperr.Internal(err)
		}
		// Kilit: iki eşzamanlı iptal isteği çift iade yazmamalı.
		locked, err := q.GetOrderForUpdate(ctx, ord.ID)
		if err != nil {
			return apperr.Internal(err)
		}

		// Mesaj sayısı KİLİDİN ALTINDA okunur: kilit alınmadan önce okunsaydı,
		// tam o aralıkta teslim edilen bir kod hem kullanıcıya gösterilir hem
		// de iade edilirdi.
		msgCount, err := q.CountOrderMessages(ctx, locked.ID)
		if err != nil {
			return apperr.Internal(err)
		}

		now := s.clock.Now()
		check := orderdom.CanUserCancel(orderdom.CancelInput{
			Status: orderdom.Status(locked.Status),
			// Ürün türü karara GİRER: `refundable_until` nil ise anlamı
			// aktivasyonda "üst sınır yok", kiralıkta "sınır bilinmiyor →
			// iptal yok". Türü söylemezsek domain fail-open tarafa düşer.
			IsRental:        locked.ProductKind == db.ProductKindSMSRENTAL,
			CancellableAt:   locked.CancellableAt,
			RefundableUntil: locked.RefundableUntil,
			HasMessage:      msgCount > 0,
			Now:             now,
		})
		if !check.Allowed {
			if check.RetryAfter > 0 {
				return apperr.ErrCancelTooEarly
			}
			return apperr.ErrOrderNotCancellable
		}

		res, err := s.transitionAndRefund(ctx, q, locked, orderdom.StatusCancelled, now,
			"kullanıcı iptali", "Sipariş iptali — iade")
		if err != nil {
			return err
		}
		out = res
		return nil
	})
	if err != nil {
		return db.Order{}, err
	}

	refund := money.New(out.PricePaidMinor, money.TRY)
	s.publishCancelled(ctx, out, refund)

	// Sağlayıcıya bildirim: kullanıcı zaten parasını aldı, bu adım arka planda
	// ve başarısız olsa da kullanıcıyı etkilemez.
	s.scheduleProviderClose(ctx, out)
	return out, nil
}

// Expire süresi dolmuş siparişi kapatır (order-expirer işi).
//
// Kullanıcı HİÇBİR ŞEY YAPMADAN iadesini alır (KK-405).
func (s *Service) Expire(ctx context.Context, orderID int64) error {
	var out db.Order
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		locked, err := q.GetOrderForUpdate(ctx, orderID)
		if err != nil {
			return apperr.Internal(err)
		}
		// Yarış: poller bu arada kodu getirmiş olabilir. Beklemede değilse
		// dokunmayız — tamamlanmış siparişe iade yazmak gerçek para kaybıdır.
		if orderdom.Status(locked.Status) != orderdom.StatusPending {
			return nil
		}
		// 🔴 KİRALIK BU YOLA HİÇ GİRMEZ — İKİNCİ SAVUNMA HATTI.
		//
		// `ListExpiredPendingOrders` kiralıkları zaten dışlıyor; burası bir
		// kez daha bakar çünkü bu fonksiyonun gövdesinin tamamı TAM İADEdir ve
		// kiralıkta dönem sonu iade sebebi değildir (süre teslim edildi).
		// Sorgu ile gövde arasında tek bir gözden kaçma, 30 günlük bir
		// kiralığın tamamen iade edilmesi demektir.
		// test: rental_integration_test.go#TestRentalNeverAutoRefundsAtEndOfPeriod
		if locked.ProductKind == db.ProductKindSMSRENTAL {
			return nil
		}
		now := s.clock.Now()
		if !orderdom.IsExpired(locked.ExpiresAt, now) {
			return nil
		}
		res, err := s.transitionAndRefund(ctx, q, locked, orderdom.StatusCancelled, now,
			"süre doldu", "Kod gelmedi — otomatik iade")
		if err != nil {
			return err
		}
		out = res
		return nil
	})
	if err != nil || out.ID == 0 {
		return err
	}
	s.publishCancelled(ctx, out, money.New(out.PricePaidMinor, money.TRY))
	s.scheduleProviderClose(ctx, out)
	return nil
}

// transitionAndRefund siparişi CANCELLED→REFUNDED yapar ve parayı iade eder.
//
// TEK TRANSACTION içinde iki geçiş: CANCELLED ara durumdur, REFUNDED terminal.
// Arada bırakılırsa (süreç ölürse) kullanıcı iptal edilmiş ama parası iade
// edilmemiş bir siparişle kalır.
func (s *Service) transitionAndRefund(
	ctx context.Context, q *db.Queries, ord db.Order,
	via orderdom.Status, now time.Time, reason, note string,
) (db.Order, error) {
	from := orderdom.Status(ord.Status)
	if err := orderdom.Transition(from, via); err != nil {
		return db.Order{}, apperr.ErrInvalidStateTransition.Wrap(err)
	}
	if _, err := q.SetOrderStatus(ctx, db.SetOrderStatusParams{
		ID: ord.ID, Status: db.OrderStatus(via), Now: &now, Reason: reason,
	}); err != nil {
		return db.Order{}, apperr.Internal(err)
	}

	// İade tutarı HER ZAMAN price_paid_minor'dır — anlık kurla yeniden
	// hesaplanmaz. Kur oynadığında eksik/fazla iade, mutabakatı bozar.
	//
	// Anahtar deterministik: aynı sipariş iki kez iade edilemez.
	if _, err := s.wallet.Apply(ctx, q, walletsvc.Input{
		UserID:         ord.UserID,
		Amount:         money.New(ord.PricePaidMinor, money.TRY),
		Type:           db.LedgerTypeREFUND,
		IdempotencyKey: "order:" + ord.PublicID.String() + ":refund",
		ReferenceType:  "order",
		ReferenceID:    ord.PublicID.String(),
		Note:           note,
	}); err != nil {
		return db.Order{}, err
	}

	if err := orderdom.Transition(via, orderdom.StatusRefunded); err != nil {
		return db.Order{}, apperr.ErrInvalidStateTransition.Wrap(err)
	}
	out, err := q.SetOrderStatus(ctx, db.SetOrderStatusParams{
		ID: ord.ID, Status: db.OrderStatusREFUNDED, Now: &now, Reason: "",
	})
	if err != nil {
		return db.Order{}, apperr.Internal(err)
	}

	// Sağlayıcıdan iade TALEP EDİLECEK. Kullanıcının iadesiyle ilgisi yoktur;
	// ayrı eksende izlenir ve reddedilirse bizim giderimizdir.
	// test: order_integration_test.go#TestCompletedOrderIsFinishedNotCancelled
	if err := q.SetProviderRefundStatus(ctx, db.SetProviderRefundStatusParams{
		ID: ord.ID, ProviderRefundStatus: db.RefundStatusPENDING,
		ProviderRefundAmountMinor: 0, RefundNextAttemptAt: nil,
	}); err != nil {
		return db.Order{}, apperr.Internal(err)
	}
	return out, nil
}

/* ═══════════════════════ Kod teslimi ═══════════════════════ */

// DeliverMessages sağlayıcıdan gelen mesajları kaydeder ve siparişi tamamlar.
//
// webhook-ingest ve order-poller AYNI fonksiyonu çağırır: iki farklı yol, tek
// mantık. İki ayrı uygulama yazmak, birinde düzeltilen hatanın diğerinde
// kalmasıyla sonuçlanır.
//
// İŞ AT-LEAST-ONCE ÇALIŞIR: aynı mesaj sekiz kez gelebilir. Her adım
// idempotenttir.
// recordMessagesOnly mesajı YALNIZ kaydeder: durum değişmez, yayın yapılmaz.
//
// İade edilmiş bir siparişe kod geldiğinde kullanılır. Mesaj destek ve
// mutabakat için saklanır ama kullanıcıya AÇILMAZ (DTO süzer). Silmek yerine
// saklamak bilinçlidir: "kod gelmedi diye iade ettik ama aslında gelmişti"
// tartışmasının tek kanıtı bu satırdır.
//
// test: order_integration_test.go#TestRefundedOrderDoesNotLeakCode
func (s *Service) recordMessagesOnly(ctx context.Context, orderID int64, msgs []port.RemoteMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	return s.tx.InTx(ctx, func(q *db.Queries) error {
		now := s.clock.Now()
		for _, m := range msgs {
			otpID := m.RemoteID
			if otpID == "" {
				otpID = fmt.Sprintf("sha:%x", hashMessage(orderID, m))
			}
			received := m.ReceivedAt
			if received.IsZero() {
				received = now
			}
			_, err := q.InsertOrderMessage(ctx, db.InsertOrderMessageParams{
				OrderID: orderID, ProviderOtpID: otpID,
				Code: m.Code, Body: m.Body, Sender: m.Sender, ReceivedAt: received,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				continue // zaten kayıtlı
			}
			if err != nil {
				return apperr.Internal(err)
			}
		}
		return nil
	})
}

func (s *Service) DeliverMessages(ctx context.Context, orderID int64, msgs []port.RemoteMessage) error {
	if len(msgs) == 0 {
		return nil
	}

	var (
		out     db.Order
		fresh   []MessageView
		changed bool
	)

	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		locked, err := q.GetOrderForUpdate(ctx, orderID)
		if err != nil {
			return apperr.Internal(err)
		}
		out = locked
		now := s.clock.Now()

		for _, m := range msgs {
			otpID := m.RemoteID
			if otpID == "" {
				// Sağlayıcı kimlik vermezse içerikten türetiriz; yoksa aynı
				// mesaj her turda yeniden eklenir.
				otpID = fmt.Sprintf("sha:%x", hashMessage(orderID, m))
			}
			received := m.ReceivedAt
			if received.IsZero() {
				received = now
			}
			row, err := q.InsertOrderMessage(ctx, db.InsertOrderMessageParams{
				OrderID: orderID, ProviderOtpID: otpID,
				Code: m.Code, Body: m.Body, Sender: m.Sender, ReceivedAt: received,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				continue // zaten kayıtlı — dedup çalıştı
			}
			if err != nil {
				return apperr.Internal(err)
			}
			fresh = append(fresh, MessageView{
				Code: row.Code, Body: row.Body, Sender: row.Sender,
				ReceivedAt: row.ReceivedAt.Format(time.RFC3339),
			})
		}

		// Durum yalnız BEKLEMEDEYKEN değişir. Zaten tamamlanmış (ya da dönemi
		// süren) bir siparişe ikinci mesaj gelirse mesaj kaydedilir ama durum
		// aynı kalır (FR-415).
		if orderdom.Status(locked.Status) == orderdom.StatusPending && len(fresh) > 0 {
			// 🔴 ÜRÜN TÜRÜ BURADA SORULUR — bu satırın yokluğu asıl hataydı.
			//
			// AKTİVASYONDA ürün KODdur: ilk kod geldiğinde sipariş bitmiştir,
			// COMPLETED terminaldir ve numara sağlayıcıda Finish ile kapatılır.
			//
			// KİRALIKTA ürün SÜREdir: ilk mesaj dönemin başıdır, sonu değil.
			// Aynı dalı kiralığa uygulamak siparişi terminal yapar, hemen
			// ardından Finish() gönderir ve kullanıcının 30 gün için ödediği
			// numarayı ilk SMS'te öldürür — kalan sürenin tamamı yanar.
			// test: rental_integration_test.go#TestRentalFirstMessageDoesNotCloseAtProvider
			next := orderdom.StatusCompleted
			if locked.ProductKind == db.ProductKindSMSRENTAL {
				next = orderdom.StatusActive
			}

			if err := orderdom.Transition(orderdom.StatusPending, next); err != nil {
				return apperr.ErrInvalidStateTransition.Wrap(err)
			}
			upd, err := q.SetOrderStatus(ctx, db.SetOrderStatusParams{
				ID: orderID, Status: db.OrderStatus(next), Now: &now, Reason: "",
			})
			if err != nil {
				return apperr.Internal(err)
			}
			out = upd

			// Kapatma denemesi yalnız terminal olunca TETİKLENİR. ACTIVE bir
			// kiralıkta bunu atlamak boş bir goroutine ve gereksiz bir
			// sahiplenme turu tasarrufudur; asıl koruma değildir — ACTIVE bir
			// siparişe kapatma çağrısı gelse bile `CloseAtProvider` sahiplenmeyi
			// bırakıp sağlayıcıya hiç gitmez.
			// test: rental_integration_test.go#TestActiveRentalIsNotClosedAtProvider
			changed = next == orderdom.StatusCompleted

			if changed {
				// Kod teslim edildi → sağlayıcıdan iade TALEP EDİLMEZ.
				if err := q.SetProviderRefundStatus(ctx, db.SetProviderRefundStatusParams{
					ID: orderID, ProviderRefundStatus: db.RefundStatusNOTAPPLICABLE,
					ProviderRefundAmountMinor: 0, RefundNextAttemptAt: nil,
				}); err != nil {
					return apperr.Internal(err)
				}
			}
			// Kiralıkta iade ekseni HİÇ ELLENMEZ: satın almadan beri
			// 'NOT_APPLICABLE' ve öyle kalır. 'PENDING' yazmak, dönemi süren
			// canlı bir numaraya `provider-refund-retry` işinin iki dakikada
			// bir Cancel() göndermesi demek olurdu — kullanıcının parası iade
			// edilmemişken numara ölürdü.
			// test: rental_integration_test.go#TestRentalNeverEntersProviderRefundRetryQueue
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 🔴 İADE EDİLMİŞ SİPARİŞİN KODU YAYINLANMAZ.
	//
	// Süzgeç `handler.visibleMessages` ile AYNI kuraldır ve burada da olmak
	// zorundadır: kodun kullanıcıya ulaşmasının İKİ kanalı var (GET yanıtı ve
	// canlı akış) ve iki kanal iki ayrı yerde süzülüyorsa biri er geç
	// unutulur — unutulan buydu. REST süzerken SSE yayınlamaya devam ediyordu,
	// yani ekranı açık olan kullanıcı iadesini almışken kodu da görüyordu.
	//
	// Kiralıkta pencere 20 dakika değil 30 GÜN: iptal sağlayıcıda reddedilirse
	// numara dönem boyunca canlı kalır ve her yeni SMS bu yoldan geçer.
	//
	// test: rental_integration_test.go#TestRefundedOrderCodeIsNotPublishedOverSSE
	gizli := orderdom.Status(out.Status) == orderdom.StatusCancelled ||
		orderdom.Status(out.Status) == orderdom.StatusRefunded
	if len(fresh) > 0 && !gizli {
		// SSE İLK KODDA KAPANMAZ (FR-415): ikinci mesaj da yayınlanır.
		s.publish(ctx, out, "code", fresh)
	}
	if len(fresh) > 0 && gizli {
		slog.Warn("iade edilmiş siparişe kod geldi — SSE'ye YAYINLANMADI",
			"order", out.PublicID, "status", out.Status,
			"metric", "otp_after_refund_total")
	}
	if changed {
		s.scheduleProviderClose(ctx, out)
	}
	return nil
}

// hashMessage kimliksiz mesajlar için deterministik dedup anahtarı üretir.
//
// DEDUP POLİTİKASI TEK YERDEDİR ve burasıdır. Sağlayıcı adaptörü mesaj kimliği
// vermediğinde UYDURMA bir anahtar üretmez (`{id}:last`, `{id}:0`, `{id}:new:0`
// gibi); `RemoteID`i boş bırakır ve karar buraya düşer. Üç ayrı yerde üç ayrı
// kural, aynı SMS'in iki farklı anahtarla iki satır olarak yazılmasına yol
// açıyordu: yoklama `{id}:0`, webhook teyidi `{id}:last`.
//
// ÜÇ AYRINTI, üçü de gerçek bir hatayı kapatıyor:
//
//   - `ReceivedAt` UTC'ye çevrilir. Aynı an `/otp/last` yolunda "+00:00",
//     `ListActive` yolunda "Z" olarak gelir; ham biçimleme iki farklı anahtar
//     üretir ve dedup iki yol arasında çalışmaz.
//   - Sıfır `ReceivedAt` (parseTime RFC 3339 dışını sıfır döner) sabit boş
//     dizeye düşer. Yerine `now` konsaydı her yoklama turu yeni bir anahtar
//     üretir ve 30 günlük bir kiralıkta binlerce yinelenmiş satır yazardı.
//   - Gönderen ve kod da girdiye dahildir: sıfır zaman durumunda ayrımı
//     yapacak tek şey içeriktir.
//
// Aynı gövdeli iki GERÇEK mesajın tek satıra düşmesi kabul edilen risktir;
// alternatifi (zamanı anahtara koymak) sınırsız satır üretmekti.
// test: rental_integration_test.go#TestSameMessageFromBothPathsIsStoredOnce
func hashMessage(orderID int64, m port.RemoteMessage) []byte {
	ts := ""
	if !m.ReceivedAt.IsZero() {
		ts = m.ReceivedAt.UTC().Format(time.RFC3339Nano)
	}
	h := sha256sum(fmt.Sprintf("%d|%s|%s|%s|%s", orderID, ts, m.Sender, m.Code, m.Body))
	return h[:12]
}

/* ═══════════════════════ Kira dönemi sonu ═══════════════════════ */

// EndRental kira dönemi biten siparişi kapatır (`rental-closer` işi).
//
// 🔴 DEFTERE KAYIT YAZILMAZ. Kiralıkta satılan şey koddur değil SÜREdir:
// numara dönem boyunca kullanıcıya ayrıldı, sağlayıcıya bedeli ödendi ve
// kullanıcı istediği an kullanabildi. Kod gelmemesi bizim kusurumuz değildir
// ve iade sebebi değildir. Aktivasyondaki "kod gelmezse iade" sözü (KK-405)
// ürünün tek işlevinin O TEK KOD olmasından doğuyordu; kiralıkta o eşitlik yok.
//
// Bu yüzden `Expire` KULLANILMAZ ve bu ayrı bir metottur: `Expire`in gövdesinin
// tamamı `transitionAndRefund`dır, yani "süre doldu → iade et". Aynı gövdeye
// "süre doldu → iade ETME" dalını koymak, birbirinin zıddı iki para sonucunu
// tek fonksiyona sıkıştırmaktır — kaçınmaya çalıştığımız hata sınıfının kendisi.
//
// test: rental_integration_test.go#TestRentalNeverAutoRefundsAtEndOfPeriod
// test: rental_integration_test.go#TestRentalEndOfPeriodFinishesAtProvider
func (s *Service) EndRental(ctx context.Context, orderID int64) error {
	var out db.Order
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		locked, err := q.GetOrderForUpdate(ctx, orderID)
		if err != nil {
			return apperr.Internal(err)
		}
		// Sorgu zaten kiralık süzüyor; burası çağrının başka bir yerden
		// gelmesine karşı ikinci hattır. Bir aktivasyonu bu yoldan geçirmek,
		// hak edilmiş iadeyi sessizce silmek olurdu.
		if locked.ProductKind != db.ProductKindSMSRENTAL {
			return nil
		}
		from := orderdom.Status(locked.Status)
		if from != orderdom.StatusPending && from != orderdom.StatusActive {
			return nil // başka bir işçi ya da kullanıcı iptali önce davrandı
		}
		now := s.clock.Now()
		if !orderdom.IsExpired(locked.ExpiresAt, now) {
			return nil
		}
		if err := orderdom.Transition(from, orderdom.StatusCompleted); err != nil {
			return apperr.ErrInvalidStateTransition.Wrap(err)
		}
		upd, err := q.SetOrderStatus(ctx, db.SetOrderStatusParams{
			ID: orderID, Status: db.OrderStatusCOMPLETED, Now: &now,
			Reason: "kira dönemi tamamlandı",
		})
		if err != nil {
			return apperr.Internal(err)
		}
		out = upd
		return nil
	})
	if err != nil || out.ID == 0 {
		return err
	}
	s.publish(ctx, out, "status", nil)
	// Terminal sipariş sağlayıcıda KAPATILIR (FR-412 / değişmez #24).
	s.scheduleProviderClose(ctx, out)
	return nil
}

/* ═══════════════════════ Sağlayıcıda kapatma ═══════════════════════ */

// scheduleProviderClose siparişi sağlayıcıda kapatmayı DENER.
//
// Başarısız olursa sipariş `provider_closed_at IS NULL` kalır ve
// `activation-reaper` tekrar dener (FR-412). Burada senkron denememizin sebebi
// hızdır: çoğu durumda tek deneme yeter ve reaper'ın 60 saniyesini beklemeyiz.
func (s *Service) scheduleProviderClose(ctx context.Context, ord db.Order) {
	go func() {
		// Arka planda: çağıranın isteği bitmiş olabilir, bu yüzden yeni bağlam.
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), providerTimeout)
		defer cancel()
		if err := s.CloseAtProvider(bg, ord.ID); err != nil {
			slog.Warn("sipariş sağlayıcıda kapatılamadı — reaper tekrar deneyecek",
				"order", ord.PublicID, "err", err)
		}
	}()
}

// closeClaimLease bir kapatma denemesinin sahiplenme süresi.
//
// Sağlayıcı çağrısı en fazla providerTimeout (10 sn) sürer; kira çok daha
// uzundur çünkü iki iş görür:
//   - Çağrı sırasında ölen bir işçinin satırı sonsuza kadar bloke etmesini
//     engeller (kira dolunca satır kendiliğinden yeniden uygun olur).
//   - Başarısız bir denemenin hemen ardından ikincisini geciktirir. 2 dakika,
//     sağlayıcının EARLY_CANCEL_DENIED için verdiği tipik süre (120 sn) ve
//     `provider-refund-retry` turuyla aynıdır: 60 saniyede bir koşan reaper,
//     sağlayıcının "daha erken olmaz" cevabını çiğneyemez.
//
// test: refund_retry_integration_test.go#TestConcurrentCloseSendsSingleProviderCall
const closeClaimLease = 2 * time.Minute

// CloseAtProvider siparişi sağlayıcıda kapatır (FR-412).
//
// Kod geldiyse Finish(), gelmediyse Cancel(). İkisi AYNI ŞEY DEĞİLDİR:
// Cancel iade talep eder, Finish etmez.
func (s *Service) CloseAtProvider(ctx context.Context, orderID int64) error {
	q := s.tx.Queries()

	// 🔴 SAHİPLENME — burada eskiden SAHTE BİR KİLİT vardı.
	//
	// Önceki kod `GetOrderForUpdate` çağırıyordu: `SELECT … FOR UPDATE` bir
	// transaction DIŞINDA, havuz üzerinden tek deyim olarak koşar; deyim
	// bittiği anda örtük transaction commit olur ve kilit BIRAKILIR. Yorum
	// "kilit var" izlenimi veriyordu ama karşılıklı dışlama yoktu:
	// activation-reaper, provider-refund-retry ve kullanıcı iptalinin arka
	// plan goroutine'i aynı siparişe aynı anda Cancel/Finish gönderebilirdi.
	//
	// Gerçek bir transaction da çözüm DEĞİL: kapatma bir HTTP çağrısı içerir
	// ve dış çağrı transaction içinde yapılmaz (değişmez #5). Bunun yerine
	// satır koşullu bir UPDATE ile SAHİPLENİLİR; yarışı kaybeden işçi SIFIR
	// satır alır ve sağlayıcıya hiç gitmez.
	// test: refund_retry_integration_test.go#TestConcurrentCloseSendsSingleProviderCall
	now := s.clock.Now()
	staleBefore := now.Add(-closeClaimLease)
	ord, err := q.ClaimOrderClose(ctx, db.ClaimOrderCloseParams{
		ID: orderID, Now: &now, ClaimStaleBefore: &staleBefore,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// ÜÇ SEBEPTEN BİRİ: sipariş zaten kapatılmış · başka bir işçi şu
			// anda kapatıyor · sipariş HENÜZ TERMİNAL DEĞİL. Üçünde de
			// yapılacak bir şey yoktur ve hiçbirinde kira tutulmaz.
			// test: rental_integration_test.go#TestNonTerminalOrderCannotBeClaimedForClose
			return nil
		}
		return apperr.Internal(err)
	}
	if !orderdom.Status(ord.Status).IsTerminal() &&
		orderdom.Status(ord.Status) != orderdom.StatusCancelled &&
		orderdom.Status(ord.Status) != orderdom.StatusFailed {
		// 🔴 ULAŞILAMAZ OLMASI BEKLENEN İKİNCİ HAT.
		//
		// Durum süzgeci artık `ClaimOrderClose`un KENDİSİNDE: terminal olmayan
		// satır hiç sahiplenilmez. Bu blok, süzgeç sorgudan düşerse (ya da bir
		// sonraki migration'da unutulursa) davranışın hâlâ doğru kalması için
		// duruyor — ama asıl koruma yukarıdadır.
		//
		// Sahiplen-sonra-bırak deseni BİLEREK BIRAKILDI: iki adımın arasındaki
		// aralık, eşzamanlı bir terminal geçişin kapatma çağrısını yutuyordu
		// (o goroutine sahiplenmeyi kaybedip sessizce nil dönüyor, ardından
		// gelen `release` kirayı siliyordu; sonuç terminal + kapatılmamış +
		// sağlayıcıya sıfır çağrı).
		// test: rental_integration_test.go#TestRentalEndRaceWithProviderCloseHasSingleEffect
		if err := q.ReleaseOrderClose(ctx, orderID); err != nil {
			return apperr.Internal(err)
		}
		return nil
	}

	count, err := q.CountOrderMessages(ctx, orderID)
	if err != nil {
		return apperr.Internal(err)
	}

	// KARAR ÜRÜN TÜRÜNE GÖRE AYRIŞIR.
	//
	// Aktivasyonda ölçüt "mesaj var mı"dır. Kiralıkta o ölçüt tek başına
	// yanlıştır: dönem sonunda mesaj gelmemiş olsa bile süre teslim
	// edilmiştir ve sağlayıcının ücretsiz iptal penceresi (≈20 dk) çoktan
	// kapanmıştır — Cancel() göndermek garantili bir redde koşmaktır.
	var action orderdom.CloseAction
	if ord.ProductKind == db.ProductKindSMSRENTAL {
		action = orderdom.DecideRentalClose(count > 0, s.withinFreeCancelWindow(ord))
	} else {
		action = orderdom.DecideClose(count > 0)
	}

	adapter, creds, err := s.providerFor(ctx, ord.ProviderID)
	if err != nil {
		return err
	}

	// effective SAĞLAYICIDA GERÇEKTEN BAŞARILI OLAN çağrı.
	//
	// `action` niyettir; iptal reddedilip yerine `Finish` gönderildiğinde
	// niyet gerçekleşmemiştir. Aşağıdaki iade ekseni yazımı NİYETE değil
	// GERÇEKLEŞENE bakmalıdır — yoksa reddedilmiş bir iptal
	// `provider_refund_status = REFUNDED` olarak, yani "sağlayıcı paramızı
	// geri verdi" diye kaydedilir ve gider hiç görünmez.
	effective := action

	switch action {
	case orderdom.CloseFinish:
		err = adapter.Finish(ctx, creds, ord.RemoteOrderID)
	default:
		err = adapter.Cancel(ctx, creds, ord.RemoteOrderID)
	}

	if err != nil {
		if oe, ok := port.AsOTPArrived(err); ok {
			// SMS tam iptal anında geldi. İki ayrı durum var ve ayrımı
			// PARA belirler:
			//
			//  · Sipariş HÂLÂ ÖDENMİŞ (kod gelmemiş sayılıp iade YAZILMAMIŞ):
			//    kullanıcı parasını ödedi, kod da geldi — hakkı olan şeydir,
			//    kaydet ve Finish ile kapat.
			//
			//  · Sipariş İADE EDİLMİŞ (süre doldu ya da kullanıcı iptal etti;
			//    para GERİ VERİLDİ): kodu kullanıcıya açmak, ona hem parayı
			//    hem numarayı vermek demektir. 🔴 Bu bir SIZINTIDIR, ürün
			//    kararı değil: kullanıcı iadesini almışken doğrulama kodunu
			//    da görürse hizmeti bedavaya almış olur.
			//
			// Mesaj yine de KAYDEDİLİR (destek ve mutabakat için gerekli) ama
			// kullanıcıya AÇILMAZ; DeliverMessages'ın yayın/durum yolu
			// çalıştırılmaz. Sağlayıcıda Finish edilir — Cancel reddedildi ve
			// aktivasyonun açık kalması bizim zararımızdır.
			//
			// test: order_integration_test.go#TestRefundedOrderDoesNotLeakCode
			iadeEdilmis := orderdom.Status(ord.Status) == orderdom.StatusRefunded ||
				orderdom.Status(ord.Status) == orderdom.StatusCancelled

			if iadeEdilmis {
				slog.Warn("iade edilmiş siparişe kod geldi — kullanıcıya AÇILMADI",
					"order", ord.PublicID, "status", ord.Status,
					"metric", "otp_after_refund_total")
				if rerr := s.recordMessagesOnly(ctx, orderID, oe.Messages); rerr != nil {
					return rerr
				}
			} else if derr := s.DeliverMessages(ctx, orderID, oe.Messages); derr != nil {
				return derr
			}
			// İptal REDDEDİLDİ; sağlayıcıdan iade gelmeyecek. Gerçekleşen
			// çağrı `Finish`tir ve eksen ona göre yazılır.
			effective = orderdom.CloseFinish
			if ferr := adapter.Finish(ctx, creds, ord.RemoteOrderID); ferr != nil {
				// Denenen çağrı ARTIK `Finish`tir: iptal reddedildi ve
				// yerine Finish gönderildi. İade eksenine yazılmaz.
				return s.recordCloseFailure(ctx, orderID, orderdom.CloseFinish, ferr)
			}
		} else {
			return s.recordCloseFailure(ctx, orderID, action, err)
		}
	}

	// SADECE BAŞARIDAN SONRA işaretlenir. Başarısızken yazsaydık reaper bu
	// siparişi bir daha hiç denemez ve aktivasyon sağlayıcıda açık kalırdı.
	closedAt := s.clock.Now()
	if err := q.MarkProviderClosed(ctx, db.MarkProviderClosedParams{ID: orderID, Now: &closedAt}); err != nil {
		return apperr.Internal(err)
	}

	// Cancel edildiyse sağlayıcıdan iade bekleriz; Finish'te iade yoktur.
	// test: rental_integration_test.go#TestDeniedCancelIsNotRecordedAsProviderRefund
	if effective == orderdom.CloseCancel {
		if err := q.SetProviderRefundStatus(ctx, db.SetProviderRefundStatusParams{
			ID: orderID, ProviderRefundStatus: db.RefundStatusREFUNDED,
			ProviderRefundAmountMinor: ord.CostMicro, RefundNextAttemptAt: nil,
		}); err != nil {
			return apperr.Internal(err)
		}
		return nil
	}

	// Finish ile kapatıldı: sağlayıcıdan iade TALEP EDİLMEDİ, dolayısıyla iade
	// ekseni de kapanır. Bu yazım olmadan satır `provider_refund_status`
	// 'PENDING' kalır ve `provider-refund-retry` kuyruğunda sonsuza kadar
	// döner — sipariş sağlayıcıda kapalı olduğu için hiçbir deneme durumu
	// değiştiremez.
	// test: refund_retry_integration_test.go#TestFinishedOrderLeavesRefundQueue
	if err := q.SettleProviderRefund(ctx, db.SettleProviderRefundParams{
		ID: orderID, ProviderRefundStatus: db.RefundStatusNOTAPPLICABLE,
	}); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// withinFreeCancelWindow kiralıkta sağlayıcının ÜCRETSİZ İPTAL penceresi
// hâlâ açık mıydı.
//
// 🔴 ÇIPA "ŞU AN" DEĞİL, İPTALİN İSTENDİĞİ ANDIR.
//
// Karar `s.clock.Now()` ile verilirken her yeniden denemede yeniden
// hesaplanıyordu ve pencere (15 dk) `provider-refund-retry` aralığının (2 dk ×
// 8 deneme ≈ 14 dk) tam ortasında bittiği için AYNI SİPARİŞE 1. denemede
// `Cancel`, 7. denemede `Finish` gidiyordu. Sonucu ölçüldü:
//
//	ilk deneme:     cancel=1 finish=0  RETRY_SCHEDULED
//	yeniden deneme: cancel=1 finish=1  NOT_APPLICABLE
//
// Yani kullanıcıya para verildikten sonra sağlayıcıdan iade istemekten
// VAZGEÇİLİYOR (sağlayıcının kendi penceresi 20 dakikayken 15. dakikada) ve
// eksene "hiç iade talep etmedik" yazılıyordu: `provider_refund_denied_total`
// bu gideri hiç görmezdi. Sessiz gider, sıfır alarm.
//
// `cancelled_at` bir kez yazılır (`SetOrderStatus` içinde `coalesce`), yani
// çıpa sabittir ve karar sekiz denemenin sekizinde de aynıdır. Dönem sonu
// kapanışında (`EndRental` → COMPLETED) `cancelled_at` NİL'dir → pencere
// kapalı → `Finish`, ki doğru olan da odur.
//
// test: rental_integration_test.go#TestRentalRefundRetryKeepsCancelDecision
func (s *Service) withinFreeCancelWindow(ord db.Order) bool {
	if ord.RefundableUntil == nil {
		return false
	}
	anchor := ord.CancelledAt
	if anchor == nil {
		// Henüz iptal edilmemiş: karar şu ana göre verilir (satın almadan
		// hemen sonraki bir kapatma denemesi bu daldan geçer).
		now := s.clock.Now()
		anchor = &now
	}
	return anchor.Before(*ord.RefundableUntil)
}

// recordCloseFailure başarısız kapatmayı kaydeder.
//
// 🔴 `action` PARAMETRESİ ZORUNLUDUR: `Cancel` ile `Finish`in başarısızlığı
// AYNI ŞEY DEĞİLDİR (değişmez #24) ve fonksiyon hangisinin denendiğini
// bilmeden iade eksenine yazamaz.
func (s *Service) recordCloseFailure(
	ctx context.Context, orderID int64, action orderdom.CloseAction, cause error,
) error {
	q := s.tx.Queries()
	_ = q.RecordCloseFailure(ctx, db.RecordCloseFailureParams{
		ID: orderID, CloseLastError: cause.Error(),
	})

	// İADE EKSENİ YALNIZ `Cancel` YOLUNDA YAZILIR.
	//
	// `Finish` iade TALEP ETMEZ; başarısızlığı bir "iade reddi" değildir.
	// Eskiden ayrım yoktu ve başarısız bir `Finish` şunları yazıyordu:
	//   · provider_refund_status = DENIED → gider metriği (KK-406b) kirlenir,
	//   · MarkProviderClosed        → sipariş KAPANMADIĞI hâlde kapalı
	//     işaretlenir ve reaper bir daha hiç denemez, yani "her terminal
	//     sipariş sağlayıcıda kapatılır" güvencesi tam bu dalda geçersizleşir.
	//
	// Kiralıkta bu istisna değil KURALDI: `DecideRentalClose` dönem sonunda
	// HER kiralığa `Finish` gönderir ve sağlayıcı aktivasyonu `expiredAt`te
	// kendisi kapattıysa yanıt `ACTIVATION_NOT_ACTIVE` → `ErrCancelDenied`
	// olur. Yani normal işleyen her kiralık gider metriğine düşerdi.
	//
	// Başarısız `Finish` yeniden deneme yolunda KALIR: satır
	// `provider_closed_at IS NULL` olduğu için `ListUnclosedTerminalOrders`
	// onu görmeye devam eder (FR-412: kapatmadan asla vazgeçilmez).
	// test: rental_integration_test.go#TestFailedFinishDoesNotTouchRefundAxis
	if action != orderdom.CloseCancel {
		return cause
	}

	// Kalıcı red mi, geçici mi? Ayrımı yapmazsak ya sonsuz yeniden deneme
	// ya da kaybedilmiş iade olur.
	if ra, ok := port.AsRetryAfter(cause); ok {
		next := s.clock.Now().Add(ra.After)
		_ = q.SetProviderRefundStatus(ctx, db.SetProviderRefundStatusParams{
			ID: orderID, ProviderRefundStatus: db.RefundStatusRETRYSCHEDULED,
			ProviderRefundAmountMinor: 0, RefundNextAttemptAt: &next,
		})
		return cause
	}
	if errors.Is(cause, port.ErrCancelDenied) {
		// Kalıcı red — sağlayıcı iade etmeyecek. Bu bizim giderimizdir ve
		// ölçülebilir olmalıdır.
		_ = q.SetProviderRefundStatus(ctx, db.SetProviderRefundStatusParams{
			ID: orderID, ProviderRefundStatus: db.RefundStatusDENIED,
			ProviderRefundAmountMinor: 0, RefundNextAttemptAt: nil,
		})
		// Sağlayıcı tarafında iş bitti: aktivasyon kapalı sayılır.
		now := s.clock.Now()
		_ = q.MarkProviderClosed(ctx, db.MarkProviderClosedParams{ID: orderID, Now: &now})
	}
	return cause
}

func (s *Service) providerFor(ctx context.Context, providerID int64) (port.ProviderPort, port.Creds, error) {
	prov, err := s.tx.Queries().GetProvider(ctx, providerID)
	if err != nil {
		return nil, port.Creds{}, apperr.Internal(err)
	}
	adapter, err := s.registry.Resolve(string(prov.Protocol))
	if err != nil {
		return nil, port.Creds{}, apperr.Internal(err)
	}
	creds := port.Creds{BaseURL: prov.BaseUrl}
	if len(prov.ApiKeyEnc) > 0 {
		key, err := s.secrets.OpenString(prov.ApiKeyEnc)
		if err != nil {
			return nil, port.Creds{}, apperr.Internal(err)
		}
		creds.APIKey = key
	}
	return adapter, creds, nil
}

func (s *Service) publishCancelled(ctx context.Context, ord db.Order, refund money.Money) {
	if s.pub == nil {
		return
	}
	ev := Event{
		Type: "cancelled", OrderID: ord.PublicID.String(), Status: string(ord.Status),
		Refunded: &refund,
	}
	if err := s.pub.Publish(ctx, ord.PublicID.String(), ev); err != nil {
		slog.Warn("iptal olayı yayınlanamadı", "order", ord.PublicID, "err", err)
	}
}
