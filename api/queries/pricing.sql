-- ─────────────────────── Kur ───────────────────────

-- name: InsertFXRate :one
INSERT INTO fx_rates (base, quote, rate, source, fetched_at)
VALUES ($1,$2,$3,$4,$5)
RETURNING *;

-- name: GetLatestFXRate :one
SELECT * FROM fx_rates
WHERE base = $1 AND quote = $2
ORDER BY fetched_at DESC, id DESC
LIMIT 1;

-- ─────────────────────── Fiyat kuralları ───────────────────────

-- name: ListApplicableRules :many
-- Bir ürün için uygulanabilir TÜM kuralları döner.
-- En spesifik olanı seçmek domain/pricing.SelectRule'ün işidir; öncelik
-- mantığı SQL'e dağıtılmaz, tek yerde kalır (docs/trd.md FR-303).
SELECT * FROM pricing_rules
WHERE is_active
  AND (valid_from IS NULL OR valid_from <= now())
  AND (valid_to   IS NULL OR valid_to   >  now())
  AND (
        scope = 'GLOBAL'
     OR (scope = 'COUNTRY'         AND country_id = sqlc.arg('country_id'))
     OR (scope = 'SERVICE'         AND service_id = sqlc.arg('service_id'))
     OR (scope = 'SERVICE_COUNTRY' AND service_id = sqlc.arg('service_id')
                                   AND country_id = sqlc.arg('country_id'))
     OR (scope = 'PRODUCT'         AND product_id = sqlc.arg('product_id'))
  );

-- name: CreatePricingRule :one
INSERT INTO pricing_rules
    (scope, service_id, country_id, product_id, margin_percent,
     fixed_fee_minor, min_price_minor, note, created_by_user_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING *;

-- name: DeactivatePricingRule :exec
UPDATE pricing_rules SET is_active = false WHERE id = $1;

-- name: ListPricingRules :many
SELECT * FROM pricing_rules WHERE is_active ORDER BY scope DESC, id;

-- ─────────────────────── Teklif ───────────────────────

-- name: CreateQuote :one
INSERT INTO price_quotes
    (user_id, product_id, provider_id, cost_micro, cost_currency, fx_rate,
     margin_percent, pricing_rule_id, sell_price_minor, stock_at_quote, expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING *;

-- name: LockQuoteForConsumption :one
-- Teklifi KİLİTLER. Aynı teklifle iki eşzamanlı satın alma denemesinde
-- yalnız biri geçmelidir (docs/trd.md KK-402).
SELECT * FROM price_quotes
WHERE public_id = $1 AND user_id = $2
FOR UPDATE;

-- name: ConsumeQuote :one
-- Tek kullanımlık işaretleme.
-- WHERE consumed_at IS NULL sayesinde yarış durumunda ikinci çağrı
-- SIFIR satır döner — kilitle birlikte çift savunma.
UPDATE price_quotes SET consumed_at = $2
WHERE id = $1 AND consumed_at IS NULL
RETURNING *;

-- name: GetQuoteByPublicID :one
SELECT * FROM price_quotes WHERE public_id = $1;

-- name: DeleteExpiredQuotes :execrows
-- Tüketilmemiş ve süresi geçmiş teklifler temizlenir.
-- Tüketilmiş olanlar SAKLANIR: sipariş kaydının fiyat kanıtıdır.
DELETE FROM price_quotes
WHERE consumed_at IS NULL AND expires_at < $1;
