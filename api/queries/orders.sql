-- name: CreateOrder :one
-- Sipariş kaydı — T2 içinde, sağlayıcı çağrısı BAŞARILI olduktan sonra.
--
-- Katalog alanları ANLIK GÖRÜNTÜ olarak yazılır: servis/ülke satırları katalog
-- senkronunda silinebilir, sipariş geçmişi buna dayanamaz.
-- `product_kind` de bir ANLIK GÖRÜNTÜDÜR: "iade mi Finish mi" kararı katalog
-- satırına JOIN ile verilemez, çünkü o satır silinebilir (product_id ON DELETE
-- SET NULL). `refundable_until` NULL ise ek bir iptal tavanı yoktur (aktivasyon).
INSERT INTO orders (
    user_id, provider_id, remote_order_id, provider_activation_id,
    phone_number, verification_type,
    product_id, product_kind, service_code, service_name, country_iso2, country_name, phone_code,
    quote_id, price_paid_minor, cost_micro, fx_rate,
    expires_at, cancellable_at, refundable_until
) VALUES (
    @user_id, @provider_id, @remote_order_id, sqlc.narg(provider_activation_id),
    @phone_number, @verification_type,
    sqlc.narg(product_id), @product_kind, @service_code, @service_name, @country_iso2, @country_name, @phone_code,
    sqlc.narg(quote_id), @price_paid_minor, @cost_micro, @fx_rate,
    @expires_at, @cancellable_at, sqlc.narg(refundable_until)
)
RETURNING *;

-- name: CreateRentalDetail :one
-- Kira dönemi kaydı — sipariş satırıyla AYNI transaction'da (T2) yazılır.
--
-- Tablo 00008'de açıldı ama hiçbir kod ona yazmıyordu: `duration_hours`
-- sağlayıcı çağrısından sonra çöpe gidiyordu ve `rental_ends_at` diye bir
-- gerçek yoktu. Kira dönemi üzerine kurulacak her rapor "hiç kiralık yok"
-- derdi — para akarken.
--
-- YETKİLİ KAYNAK `orders.expires_at`TİR; bu satır dönemin KAYDIDIR. İkisini
-- bağımsız iki doğruluk kaynağı yapmak, uzatmadan sonra birinin
-- güncellenmemesiyle biter.
INSERT INTO rental_details (order_id, duration_hours, rental_ends_at)
VALUES (@order_id, @duration_hours, @rental_ends_at)
ON CONFLICT (order_id) DO UPDATE
    SET duration_hours = EXCLUDED.duration_hours,
        rental_ends_at = EXCLUDED.rental_ends_at
RETURNING *;

-- name: GetRentalDetail :one
SELECT * FROM rental_details WHERE order_id = @order_id;

-- name: GetOrderForUser :one
-- SAHİPLİK SORGUNUN PARÇASIDIR (CLAUDE.md değişmez #7).
-- Ayrı bir `if order.UserID != userID` kontrolü yazılmaz: unutulabilir.
SELECT * FROM orders WHERE public_id = @public_id AND user_id = @user_id;

-- name: GetOrderForUpdate :one
-- Durum değiştirmeden ÖNCE kilitle. Kilitsiz okuma + yazma, iki işçinin aynı
-- siparişi aynı anda sonlandırmasına ve çift iadeye yol açar.
SELECT * FROM orders WHERE id = @id FOR UPDATE;

-- name: GetOrderByRemote :one
-- Webhook korelasyonu: sağlayıcı YALNIZ aktivasyon kimliğini taşır.
SELECT * FROM orders
WHERE provider_id = @provider_id AND remote_order_id = @remote_order_id;

-- name: ListUserOrders :many
--
-- 🔴 `icon_url` CANLI KATALOGDAN gelir, sipariş kaydından DEĞİL.
--
-- `orders.service_name` bilerek anlık görüntüdür: satış kaydı, katalog sonradan
-- değişse bile ne satıldığını korumalıdır (para kaydı, 10 yıl saklanıyor).
-- Logo ise sunum verisidir; markanın logosu güncellendiğinde eski siparişlerde
-- de yeni logonun görünmesi İSTENİR. Bu yüzden sipariş satırına kopyalanmaz,
-- her okumada katalogdan çekilir.
--
-- LEFT JOIN gerekçesi: `service_code` satın alma anındaki metin anlık
-- görüntüsüdür; katalog senkronu sağlayıcının kodunu değiştirirse eşleşme
-- kaybolur. INNER JOIN, o kullanıcının geçmiş siparişini listeden sessizce
-- düşürürdü. (Servis satırının SİLİNMESİ mümkün değil — orders → price_quotes
-- → products → services yabancı anahtar zinciri buna izin vermiyor.)
-- test: order_integration_test.go#TestListUserOrdersCarriesServiceIcon
--
-- `sqlc.embed(o)` kullanılır, sütunlar tek tek sayılmaz: `SELECT o.*` ya da elle
-- sütun listesi, sqlc'ye `db.Order` yerine YENİ bir satır tipi ürettirir ve aynı
-- tipi bekleyen tüm çağrı yerleri kırılır. `embed` `db.Order`'ı olduğu gibi
-- korur, `icon_url`'i yanına ekler — ve şemaya sütun eklendiğinde bu sorgu
-- kendiliğinden güncel kalır.
--
-- SÜZGEÇLER — `sqlc.narg()` + "NULL ise süzme" deseni (CLAUDE.md sqlc kuralları).
--
-- ÜÇ SÜZGEÇ VARDIR VE `only_active` AYRI DURMAK ZORUNDADIR:
--   status       → TEK durum eşleşmesi ("İptal edilenleri göster")
--   only_active  → "AKTİF" = PENDING **veya** ACTIVE, yani tek durum DEĞİL
--   q            → serbest metin
-- `ListTicketsForAdmin`'deki `only_pending` ile aynı gerekçe: bir kullanıcı
-- kavramı ("aktif siparişim") birden çok duruma karşılık geliyorsa, onu tek
-- durumlu süzgece sıkıştırmak listeyi sessizce eksiltir. ACTIVE, kiralık
-- siparişin süren dönemidir ve özet ekranında gizlenemez.
-- Go'da string birleştirerek SQL kurulmaz; `ListUsers` ile AYNI desen.
--
-- 🔴 `user_id` KOŞULU HER İKİ SÜZGEÇTEN ÖNCE GELİR ve kaldırılamaz: sahiplik
-- sorgunun parçasıdır (değişmez #7). Hiçbir `q`/`status` bileşimi başka bir
-- kullanıcının siparişini döndüremez.
--
-- İNDEKS EKLENMEDİ: `ILIKE '%...%'` bir btree indeksi kullanamaz zaten, ama
-- tarama `user_id` ile ZATEN daraltılmış küçük bir kümede olur (bir kullanıcının
-- kendi siparişleri) — planlayıcı `orders_user_idx` (user_id, created_at DESC) ile satırları
-- getirir, süzgeç o küme üzerinde çalışır. Gerçek bir sorun ölçülürse çare
-- `pg_trgm` GIN indeksidir, gövdede string birleştirmek değil.
SELECT sqlc.embed(o), coalesce(s.icon_url, '')::text AS icon_url
FROM orders o
LEFT JOIN services s ON s.code = o.service_code
WHERE o.user_id = @user_id
  AND (sqlc.narg('status')::order_status IS NULL OR o.status = sqlc.narg('status')::order_status)
  AND (NOT sqlc.arg('only_active')::boolean OR o.status IN ('PENDING', 'ACTIVE'))
  AND (sqlc.narg('q')::text IS NULL
       OR o.service_name::text ILIKE '%' || sqlc.narg('q')::text || '%'
       OR o.phone_number::text ILIKE '%' || sqlc.narg('q')::text || '%'
       OR o.country_name::text ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY o.created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountUserOrders :one
--
-- 🔴 SÜZGEÇLER `ListUserOrders` İLE BİREBİR AYNI OLMAK ZORUNDA. Ayrışırsa
-- sayfalama yalan söyler: "1–25 / 300" yazarken liste 4 satır gösterir ve
-- "Sonraki" boş sayfa açar. İki sorgu tek bir kavramın iki yarısıdır.
-- test: order_integration_test.go#TestListUserOrdersFiltreleri
SELECT count(*) FROM orders o
WHERE o.user_id = @user_id
  AND (sqlc.narg('status')::order_status IS NULL OR o.status = sqlc.narg('status')::order_status)
  AND (NOT sqlc.arg('only_active')::boolean OR o.status IN ('PENDING', 'ACTIVE'))
  AND (sqlc.narg('q')::text IS NULL
       OR o.service_name::text ILIKE '%' || sqlc.narg('q')::text || '%'
       OR o.phone_number::text ILIKE '%' || sqlc.narg('q')::text || '%'
       OR o.country_name::text ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: SetOrderStatus :one
-- Durum yazımı — YALNIZ domain/order.Transition doğruladıktan sonra çağrılır.
-- Veritabanındaki tetikleyici ikinci savunma hattıdır.
UPDATE orders SET
    status       = @status,
    completed_at = CASE WHEN @status::order_status = 'COMPLETED' THEN coalesce(completed_at, @now) ELSE completed_at END,
    cancelled_at = CASE WHEN @status::order_status IN ('CANCELLED','REFUNDED') THEN coalesce(cancelled_at, @now) ELSE cancelled_at END,
    refunded_at  = CASE WHEN @status::order_status = 'REFUNDED' THEN coalesce(refunded_at, @now) ELSE refunded_at END,
    cancel_reason = CASE WHEN @reason::text <> '' THEN @reason ELSE cancel_reason END
WHERE id = @id
RETURNING *;

-- name: SetProviderRefundStatus :exec
UPDATE orders SET
    provider_refund_status       = @provider_refund_status,
    provider_refund_amount_minor = @provider_refund_amount_minor,
    refund_attempts              = refund_attempts + 1,
    refund_next_attempt_at       = sqlc.narg(refund_next_attempt_at)
WHERE id = @id;

-- name: MarkProviderClosed :exec
-- Kapatma BAŞARILI OLDUKTAN SONRA çağrılır.
-- Başarısızken yazılırsa reaper o siparişi bir daha hiç denemez ve aktivasyon
-- sağlayıcıda açık kalır (FR-412).
UPDATE orders SET provider_closed_at = @now, close_last_error = '' WHERE id = @id;

-- name: RecordCloseFailure :exec
UPDATE orders SET close_attempts = close_attempts + 1, close_last_error = @close_last_error
WHERE id = @id;

-- ─────────────────────────── İşçi sorguları ───────────────────────────

-- name: ListPendingOrdersForPoll :many
-- `order-poller` için: kod bekleyen, süresi dolmamış AKTİVASYON siparişleri.
--
-- 🔴 KİRALIKLAR DIŞARIDA — sıklık farkı yüzünden, tercih olduğu için değil.
-- Aktivasyon ~20 dakika yaşar; kiralık 24–4320 SAAT. Sıralama `created_at`
-- olduğu için 100 canlı kiralık bu LIMIT'in başını günlerce işgal eder ve
-- aktivasyon siparişleri hiç yoklanmaz: webhook kaybolduğunda kod hiç gelmez,
-- order-expirer tam iade yazar — hem sağlayıcı maliyeti hem satış kaybı.
-- Kiralıkların kendi turu vardır (ListActiveRentalsForPoll, 5 dakika).
-- test: ../internal/service/order/rental_integration_test.go#TestRentalsDoNotStarveActivationPoll
SELECT * FROM orders
WHERE status = 'PENDING' AND product_kind = 'SMS_ACTIVATION' AND expires_at > @now
ORDER BY created_at
LIMIT @lim;

-- name: ListExpiredPendingOrders :many
-- `order-expirer` için: süresi dolmuş ama hâlâ beklemede olan AKTİVASYONLAR.
--
-- 🔴 KİRALIKLAR DIŞARIDA — bu sorgunun sonucu TAM İADE demektir.
--
-- Kiralıkta satılan şey süredir: numara 30 gün ayrıldı, sağlayıcıya ödendi,
-- kullanıcı istediği an kullanabildi. Kod gelmemesi iade sebebi değildir ve
-- sağlayıcının ücretsiz iptal penceresi (≈20 dk) 1. günde kapandığı için o
-- parayı geri almanın yolu da yoktur: buradan yazılacak iadenin %100'ü bizim
-- cebimizden çıkar ve "kiralık al, 30. günde paranı al" tekrarlanabilir bir
-- sızıntı olur. Dönem sonu `ListEndedRentals` → EndRental ile işlenir; orada
-- deftere kayıt yazılmaz.
-- test: ../internal/service/order/rental_integration_test.go#TestRentalNeverAutoRefundsAtEndOfPeriod
SELECT * FROM orders
WHERE status = 'PENDING' AND product_kind = 'SMS_ACTIVATION' AND expires_at <= @now
ORDER BY expires_at
LIMIT @lim;

-- name: ListActiveRentalsForPoll :many
-- `rental-poller` için: dönemi süren kiralıklar.
--
-- İKİ DURUM BİRDEN: 'PENDING' (henüz mesaj gelmedi) ve 'ACTIVE' (en az bir
-- mesaj geldi, dönem sürüyor). Kiralıkta mesaj gelmesi yoklamayı bitirmez —
-- ürünün tamamı "30 gün boyunca gelen HER mesajı gör"dür.
--
-- 🔴 SIRALAMA `created_at` DEĞİL — ve bu bir hata düzeltmesidir.
--
-- `created_at ASC` + `LIMIT` deseni aktivasyonda güvenlidir çünkü satırlar ~20
-- dakikada kümeden çıkar; kiralıkta küme dönem boyunca (24–4320 saat) SABİT
-- kalır. 101. kiralık, ilk 100'ün hiçbiri bitmeden hiçbir turda görünmezdi:
-- webhook kaybolduğunda o kullanıcı 30 gün boyunca tek mesaj görmez.
--
-- `last_polled_at` turu dönüşümlü yapar. `NULLS FIRST` yeni satırın ilk turda
-- görülmesini garanti eder; `id` eşitlik hâlinde belirlenimli sıra verir
-- (aynı damgayı taşıyan bir toplu güncellemeden sonra tur ilerlemeye devam
-- etsin).
-- test: ../internal/service/order/rental_integration_test.go#TestRentalPollNeverStarvesNewestRental
SELECT * FROM orders
WHERE product_kind = 'SMS_RENTAL'
  AND status IN ('PENDING', 'ACTIVE')
  AND expires_at > @now
ORDER BY last_polled_at ASC NULLS FIRST, id
LIMIT @lim;

-- name: MarkOrdersPolled :exec
-- Yoklama turunun damgası. Sıradaki tur en uzun süredir yoklanmamış satırla
-- başlasın diye, sağlayıcı çağrısından ÖNCE yazılır: sağlayıcı erişilemezse
-- bile tur ilerler ve tek bir arızalı grup kümenin geri kalanını aç bırakmaz.
UPDATE orders SET last_polled_at = @now WHERE id = ANY(@ids::bigint[]);

-- name: ListEndedRentals :many
-- `rental-closer` için: kira dönemi bitmiş ama hâlâ kapatılmamış kiralıklar.
--
-- 'PENDING' de dahildir: hiç mesaj almamış bir kiralık 'ACTIVE'e hiç geçmez ve
-- dönem sonunda yine kapatılmalıdır (FR-412: her sipariş sağlayıcıda kapanır).
SELECT * FROM orders
WHERE product_kind = 'SMS_RENTAL'
  AND status IN ('PENDING', 'ACTIVE')
  AND expires_at <= @now
ORDER BY expires_at
LIMIT @lim;

-- name: ListUnclosedTerminalOrders :many
-- `activation-reaper` için: terminal ama sağlayıcıda kapatılmamış siparişler.
-- KK-412: bu sorgunun sonucu uzun vadede BOŞ olmalıdır.
--
-- ROL AYRIMI: iade ekseni AÇIK olan satırlar ('PENDING','RETRY_SCHEDULED')
-- `provider-refund-retry` işinin sorumluluğundadır ve buraya DÜŞMEZ. Bu filtre
-- olmadan iki iş aynı satıra Cancel() gönderir ve sağlayıcının verdiği
-- Retry-After süresi (FR-414) 60 saniyede bir çiğnenir.
--
-- Kapatma ekseni (FR-412) YİNE reaper'ındır: iade ekseni DENIED/REFUNDED/
-- NOT_APPLICABLE'a düştüğü an satır buraya geri döner — kapatma denemesinden
-- vazgeçilmez.
-- test: refund_retry_integration_test.go#TestReaperIgnoresScheduledRefundOrders
SELECT * FROM orders
WHERE provider_closed_at IS NULL
  AND status IN ('COMPLETED', 'CANCELLED', 'FAILED', 'REFUNDED')
  AND provider_refund_status NOT IN ('PENDING', 'RETRY_SCHEDULED')
ORDER BY updated_at
LIMIT @lim;

-- name: ListRefundRetryOrders :many
-- `provider-refund-retry` için: sağlayıcıdan iade beklenen siparişler.
SELECT * FROM orders
WHERE provider_refund_status IN ('PENDING', 'RETRY_SCHEDULED')
  AND (refund_next_attempt_at IS NULL OR refund_next_attempt_at <= @now)
ORDER BY coalesce(refund_next_attempt_at, created_at)
LIMIT @lim;

-- name: ClaimOrderClose :one
-- SAHİPLENME: sağlayıcıda kapatma denemesini TEK bir işçiye verir.
--
-- Kapatma bir HTTP çağrısı içerir; dış çağrı transaction içinde yapılmaz
-- (değişmez #5), dolayısıyla satır kilidi çağrı boyunca tutulamaz —
-- `GetOrderForUpdate` bir transaction dışında çağrıldığında kilit deyim
-- biter bitmez bırakılır ve hiçbir şeyi korumaz. Bunun yerine satır koşullu
-- bir UPDATE ile sahiplenilir: yarışı kaybeden işçi SIFIR satır alır
-- (ConsumeQuote ile aynı desen) ve sağlayıcıya hiç gitmez.
--
-- Kira süresi dolduğunda satır kendiliğinden yeniden uygun hâle gelir: çağrı
-- sırasında ölen bir işçi satırı sonsuza kadar bloke edemez.
--
-- `provider_closed_at IS NULL` koşulu da buradadır: zaten kapatılmış siparişe
-- ikinci kez Cancel/Finish gönderilmez.
--
-- 🔴 DURUM SÜZGECİ SAHİPLENMENİN PARÇASIDIR, ayrı bir `if` değil.
--
-- Süzgeç Go tarafında dururken kapatılabilir olmayan bir satır (PENDING /
-- ACTIVE) de sahiplenilebiliyordu: işçi satırı alıyor, durumu görüp
-- `ReleaseOrderClose` ile bırakıyordu. O iki adımın ARASINDA sipariş terminal
-- olursa, terminal geçişin doğurduğu kapatma goroutine'i sahiplenmeyi
-- KAYBEDİYOR ve sessizce nil dönüyordu; hemen ardından gelen `release` de
-- kirayı siliyordu. Sonuç: terminal sipariş, `provider_closed_at IS NULL` ve
-- sağlayıcıya SIFIR çağrı — numara `activation-reaper`ın bir sonraki turuna
-- kadar (≤60 sn) açıkta.
--
-- Deterministik olarak üretilebiliyordu: `-count=10` → 10/10.
-- test: rental_integration_test.go#TestRentalEndRaceWithProviderCloseHasSingleEffect
UPDATE orders SET close_claimed_at = @now
WHERE id = @id
  AND provider_closed_at IS NULL
  AND status IN ('COMPLETED', 'CANCELLED', 'FAILED', 'REFUNDED')
  AND (close_claimed_at IS NULL OR close_claimed_at <= @claim_stale_before)
RETURNING *;

-- name: ReleaseOrderClose :exec
-- Sahiplenmeyi BIRAKIR: kapatılacak bir şey olmadığı anlaşıldığında çağrılır
-- (örn. sipariş hâlâ beklemede). Kirayı boşuna tutmak, siparişin terminal
-- olduğu anda yapılacak kapatmayı kira süresi kadar geciktirirdi.
--
-- BAŞARISIZLIKTA ÇAĞRILMAZ: kira, başarısız bir denemenin hemen ardından
-- ikinci bir denemeyi engelleyerek sağlayıcının verdiği Retry-After süresine
-- saygı gösterir.
UPDATE orders SET close_claimed_at = NULL WHERE id = @id;

-- name: SettleProviderRefund :exec
-- İade eksenini KAPATIR: sağlayıcıya bir daha istek gönderilmez.
--
-- `SetProviderRefundStatus`ten iki farkı var ve ikisi de kasıtlı:
--   1. `refund_attempts` ARTIRILMAZ — kapatmak bir deneme değildir.
--   2. Koşulludur: yalnız iade ekseni AÇIKKEN yazar, böylece iki işçi aynı
--      anda kapatmaya çalışsa da sonuç tektir ve REFUNDED/DENIED bir satırın
--      üstüne yazılmaz.
-- test: ../internal/service/order/refund_retry_integration_test.go#TestFinishedOrderLeavesRefundQueue
UPDATE orders SET
    provider_refund_status = @provider_refund_status,
    refund_next_attempt_at = NULL
WHERE id = @id
  AND provider_refund_status IN ('PENDING', 'RETRY_SCHEDULED');

-- name: ListOrphanHolds :many
-- `orphan-hold-reaper` için: PARASI ÇEKİLMİŞ ama siparişi olmayan teklifler.
--
-- T1 (teklif tüket + bakiye düş) ile T2 (sipariş yaz) arasında süreç ölürse
-- kullanıcının parası gitmiş ama elinde numara yoktur. Bu sorgu o durumu bulur.
--
-- `consumed_at` eşiği, henüz T2'ye ulaşmamış NORMAL akışları yakalamamak için:
-- satın alma birkaç saniye sürer, hemen "yetim" ilan etmek çalışan bir siparişi
-- iade eder.
--
-- 🔴 İADE EDİLMİŞ OLANLAR DIŞLANIR — yoksa bu sorgu KALICI OLARAK TIKANIR.
--
-- Sağlayıcı hatasıyla düşen HER satın alma (stok yok, 5xx, geçmiş expiredAt)
-- tam olarak bu profilde bir satır bırakır: teklif tüketilmiş, iade `refundHold`
-- tarafından ZATEN yazılmış, sipariş satırı hiç oluşmamış. Bu satırları dışlayan
-- bir kolon yoktu; her başarısız satın alma sorguya kalıcı bir satır ekliyordu.
-- 100 böyle satır biriktiğinde (tek sağlayıcı kesintisinde dakikalar sürer)
-- `LIMIT` tümüyle ölü satırlarla dolar ve GERÇEKTEN iade edilmemiş yeni yetimler
-- hiç görülmez. Yani güvenlik ağı sessizce yırtılır.
--
-- Ayrı bir kolon yerine DEFTERE bakılır: iade anahtarı zaten deterministik
-- (`order:{quote_public_id}:refund`, service/order/service.go:368) ve defter
-- değişmezdir — ikinci bir doğruluk kaynağı yaratmaktansa var olanı sorgularız.
-- test: order_integration_test.go#TestOrphanHoldQuerySkipsAlreadyRefunded
SELECT q.* FROM price_quotes q
LEFT JOIN orders o ON o.quote_id = q.id
WHERE q.consumed_at IS NOT NULL
  AND q.consumed_at < @older_than
  AND o.id IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM ledger_entries le
      WHERE le.idempotency_key = 'order:' || q.public_id::text || ':refund'
  )
ORDER BY q.consumed_at
LIMIT @lim;

-- ─────────────────────────── Mesajlar ───────────────────────────

-- name: InsertOrderMessage :one
-- DEDUP: aynı mesaj webhook, yoklama ve yeniden gönderimlerle en az sekiz kez
-- gelebilir. UNIQUE kısıt son savunma hattıdır; ON CONFLICT ile sessizce
-- yutulur ve `inserted` alanı gerçekten yeni mi söyler.
INSERT INTO order_messages (order_id, provider_otp_id, code, body, sender, received_at)
VALUES (@order_id, @provider_otp_id, @code, @body, @sender, @received_at)
ON CONFLICT (order_id, provider_otp_id) DO NOTHING
RETURNING *;

-- name: ListOrderMessages :many
SELECT * FROM order_messages WHERE order_id = @order_id ORDER BY received_at, id;

-- name: CountOrderMessages :one
SELECT count(*) FROM order_messages WHERE order_id = @order_id;

-- ─────────────────────────── Yönetim ───────────────────────────

-- name: ListAllOrders :many
-- Yönetim paneli — `orders:read_all` izni gerektirir.
SELECT o.*, u.username, u.email
FROM orders o
JOIN users u ON u.id = o.user_id
WHERE (sqlc.narg(status)::order_status IS NULL OR o.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR u.email ILIKE '%' || sqlc.narg(q) || '%'
       OR u.username ILIKE '%' || sqlc.narg(q) || '%'
       OR o.phone_number ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY o.created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountAllOrders :one
SELECT count(*)
FROM orders o JOIN users u ON u.id = o.user_id
WHERE (sqlc.narg(status)::order_status IS NULL OR o.status = sqlc.narg(status))
  AND (sqlc.narg(q)::text IS NULL
       OR u.email ILIKE '%' || sqlc.narg(q) || '%'
       OR u.username ILIKE '%' || sqlc.narg(q) || '%'
       OR o.phone_number ILIKE '%' || sqlc.narg(q) || '%');

-- name: OrderStatsSummary :one
-- Yönetim özeti. Tek sorguda: N ayrı COUNT sorgusu atmak, tablo büyüdükçe
-- panelin açılışını yavaşlatır.
--
-- KOVALAR TOPLAMI `total`A EŞİTTİR. Eskiden değildi: 'ACTIVE' ve 'FAILED'
-- hiçbir kovaya girmiyordu, yani panel "toplam 100, dağılım 78" gösterirdi.
-- Bir sayının nereye gittiği görünmüyorsa gösterge okunamaz.
SELECT
    count(*)                                                   AS total,
    count(*) FILTER (WHERE status = 'PENDING')                 AS pending,
    -- Dönemi süren kiralık: ödendi, teslim edildi, henüz bitmedi.
    count(*) FILTER (WHERE status = 'ACTIVE')                  AS active,
    count(*) FILTER (WHERE status = 'COMPLETED')               AS completed,
    count(*) FILTER (WHERE status IN ('CANCELLED','REFUNDED')) AS cancelled,
    count(*) FILTER (WHERE status = 'FAILED')                  AS failed,
    -- KK-412 ölçüsü: TERMİNAL olduğu hâlde sağlayıcıda kapatılmamış siparişler.
    -- 'ACTIVE' de 'PENDING' gibi hariçtir — dönemi süren bir kiralık numara
    -- sağlayıcıda AÇIK OLMALIDIR; onu "kapatılmamış" saymak, sıfıra gitmesi
    -- beklenen bir göstergeyi normal işleyişle doldururdu.
    count(*) FILTER (WHERE provider_closed_at IS NULL
                       AND status NOT IN ('PENDING', 'ACTIVE'))AS unclosed,
    -- 🔴 'ACTIVE' DE GELİRDİR.
    --
    -- Yalnız 'COMPLETED' toplamak kiralıkta geliri dönem boyunca (24–4320
    -- saat) SIFIR gösterir: para akarken panel boş kalır — eski prototipin
    -- tam olarak bu hatası vardı. 'ACTIVE' bir kiralık ödenmiştir ve geri
    -- alınamaz: durum makinesinde ACTIVE→CANCELLED oku YOKTUR, yani o tutar
    -- iade edilemez. 'PENDING' bilerek dışarıda — o para hâlâ iade edilebilir
    -- ve gerçekleşmiş sayılamaz.
    coalesce(sum(price_paid_minor)
             FILTER (WHERE status IN ('COMPLETED', 'ACTIVE')), 0)::bigint AS revenue_minor
FROM orders
WHERE created_at >= @since;

-- name: GetQuoteContext :one
-- Sipariş anlık görüntüsü için servis/ülke adları.
--
-- Sipariş satırına KOPYALANIR: katalog satırları senkronda silinebilir ve
-- sipariş geçmişi bir yıl sonra da okunabilir olmalıdır.
SELECT
    s.code AS service_code, s.name AS service_name, s.name_tr AS service_name_tr,
    c.iso2 AS country_iso2, c.name AS country_name, c.name_tr AS country_name_tr,
    c.phone_code
FROM price_quotes q
JOIN products  p ON p.id = q.product_id
JOIN services  s ON s.id = p.service_id
JOIN countries c ON c.id = p.country_id
WHERE q.id = @quote_id;

-- ─────────────────────── Ödeme yöntemleri ───────────────────────

-- name: ListDepositMethods :many
-- Yönetim: tüm yöntemler (pasifler dahil).
SELECT * FROM deposit_methods ORDER BY sort_order, name;

-- name: ListActiveDepositMethods :many
-- Kullanıcı: yalnız aktif yöntemler.
SELECT * FROM deposit_methods WHERE is_active ORDER BY sort_order, name;

-- name: GetDepositMethod :one
SELECT * FROM deposit_methods WHERE public_id = @public_id;

-- name: CreateDepositMethod :one
INSERT INTO deposit_methods (code, kind, name, instructions, config,
                             min_amount_minor, max_amount_minor, sort_order)
VALUES (@code, @kind, @name, @instructions, @config,
        @min_amount_minor, @max_amount_minor, @sort_order)
RETURNING *;

-- name: UpdateDepositMethod :one
UPDATE deposit_methods SET
    name             = @name,
    instructions     = @instructions,
    config           = @config,
    min_amount_minor = @min_amount_minor,
    max_amount_minor = @max_amount_minor,
    sort_order       = @sort_order
WHERE public_id = @public_id
RETURNING *;

-- name: SetDepositMethodActive :one
-- Aktif/pasif AYRI bir sorgu: tek bir düğmeye basmak, o sırada düzenlenmekte
-- olan diğer alanları yazmamalı.
UPDATE deposit_methods SET is_active = @is_active WHERE public_id = @public_id
RETURNING *;

-- name: DeleteDepositMethod :exec
-- Yöntem SİLİNİR ama geçmiş yükleme kayıtları KALIR: deposits.method_id
-- ON DELETE SET NULL ve method_name anlık görüntü olarak saklanıyor.
DELETE FROM deposit_methods WHERE public_id = @public_id;
