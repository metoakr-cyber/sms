-- name: CreateOrder :one
-- Sipariş kaydı — T2 içinde, sağlayıcı çağrısı BAŞARILI olduktan sonra.
--
-- Katalog alanları ANLIK GÖRÜNTÜ olarak yazılır: servis/ülke satırları katalog
-- senkronunda silinebilir, sipariş geçmişi buna dayanamaz.
INSERT INTO orders (
    user_id, provider_id, remote_order_id, provider_activation_id,
    phone_number, verification_type,
    product_id, service_code, service_name, country_iso2, country_name, phone_code,
    quote_id, price_paid_minor, cost_micro, fx_rate,
    expires_at, cancellable_at
) VALUES (
    @user_id, @provider_id, @remote_order_id, sqlc.narg(provider_activation_id),
    @phone_number, @verification_type,
    sqlc.narg(product_id), @service_code, @service_name, @country_iso2, @country_name, @phone_code,
    sqlc.narg(quote_id), @price_paid_minor, @cost_micro, @fx_rate,
    @expires_at, @cancellable_at
)
RETURNING *;

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
SELECT * FROM orders
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountUserOrders :one
SELECT count(*) FROM orders WHERE user_id = @user_id;

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
-- `order-poller` için: kod bekleyen, süresi dolmamış siparişler.
SELECT * FROM orders
WHERE status = 'PENDING' AND expires_at > @now
ORDER BY created_at
LIMIT @lim;

-- name: ListExpiredPendingOrders :many
-- `order-expirer` için: süresi dolmuş ama hâlâ beklemede olan siparişler.
SELECT * FROM orders
WHERE status = 'PENDING' AND expires_at <= @now
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
-- test: refund_retry_integration_test.go#TestConcurrentCloseSendsSingleProviderCall
UPDATE orders SET close_claimed_at = @now
WHERE id = @id
  AND provider_closed_at IS NULL
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
SELECT q.* FROM price_quotes q
LEFT JOIN orders o ON o.quote_id = q.id
WHERE q.consumed_at IS NOT NULL
  AND q.consumed_at < @older_than
  AND o.id IS NULL
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
SELECT
    count(*)                                                   AS total,
    count(*) FILTER (WHERE status = 'PENDING')                 AS pending,
    count(*) FILTER (WHERE status = 'COMPLETED')               AS completed,
    count(*) FILTER (WHERE status IN ('CANCELLED','REFUNDED')) AS cancelled,
    count(*) FILTER (WHERE provider_closed_at IS NULL
                       AND status <> 'PENDING')                AS unclosed,
    coalesce(sum(price_paid_minor) FILTER (WHERE status = 'COMPLETED'), 0)::bigint AS revenue_minor
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
