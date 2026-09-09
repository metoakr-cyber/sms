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

-- Saklama temizliği (DeleteExpiredQuotes) queries/retention.sql içindedir.

-- ─────────────────────── Fiyat kuralı yönetimi (FR-703) ───────────────────────

-- name: ListPricingRulesForAdmin :many
-- Yönetim listesi: etkin kurallar + kapsamın İNSAN OKUR karşılığı.
--
-- `ListPricingRules` yalnız ham satırı döner; panelde "service_id 42" değil
-- "whatsapp × TR" yazmalı. Ad çözümlemesini Go tarafında N+1 sorguyla yapmak
-- yerine tek JOIN'de çözülür.
SELECT
    r.id, r.scope, r.margin_percent, r.fixed_fee_minor, r.min_price_minor,
    r.note, r.valid_from, r.valid_to, r.created_at,
    s.code AS service_code,
    c.iso2 AS country_iso2,
    p.duration_minutes AS product_duration_minutes
FROM pricing_rules r
LEFT JOIN services  s ON s.id = r.service_id
LEFT JOIN countries c ON c.id = r.country_id
LEFT JOIN products  p ON p.id = r.product_id
WHERE r.is_active
ORDER BY r.scope DESC, r.id;

-- name: GetPricingRule :one
SELECT * FROM pricing_rules WHERE id = $1;

-- name: LockActiveRuleForScope :one
-- Bir kapsamdaki ETKİN kuralı KİLİTLER.
--
-- Kural "güncelleme" yoktur: eski kural pasifleştirilip yenisi eklenir
-- (kısmi tekil indeksler aynı kapsamda iki etkin kurala izin vermez).
-- Kilit olmadan iki yönetici aynı anda kural yazdığında ikisi de eski satırı
-- görür, ikisi de INSERT eder ve biri 23505 ile düşer — hangisinin geçtiği
-- rastgele olur. FOR UPDATE bunu sıraya sokar.
--
-- IS NOT DISTINCT FROM kullanılır: NULL = NULL karşılaştırması `=` ile
-- daima NULL döner ve GLOBAL kapsam (üç alanı da NULL) hiç eşleşmezdi.
SELECT * FROM pricing_rules
WHERE is_active
  AND scope = @scope
  AND service_id IS NOT DISTINCT FROM sqlc.narg('service_id')::bigint
  AND country_id IS NOT DISTINCT FROM sqlc.narg('country_id')::bigint
  AND product_id IS NOT DISTINCT FROM sqlc.narg('product_id')::bigint
FOR UPDATE;

-- name: CountActiveRulesByScope :one
-- Bir kapsamdaki etkin kural sayısı.
--
-- Son GLOBAL kuralın pasifleştirilmesini engellemek için kullanılır: GLOBAL
-- kural kalmazsa ListApplicableRules boş döner, SelectRule ErrNoRule verir ve
-- SİSTEM SATIŞ YAPAMAZ hâle gelir.
SELECT count(*) FROM pricing_rules WHERE is_active AND scope = @scope;

-- name: GetPricingRuleLabels :one
-- Bir kuralın kapsamını İNSAN OKUR kodlarla verir.
--
-- Denetim kaydının entity_id'si için gerekir: oraya sayısal kimlik yazmak
-- (Değişmez #10) kaydı hem okunmaz hem de kimlik yeniden kullanıldığında
-- yanıltıcı yapar. "SERVICE_COUNTRY:whatsapp:TR" bir yıl sonra da aynı şeyi
-- anlatır.
SELECT
    r.scope,
    s.code AS service_code,
    c.iso2 AS country_iso2,
    p.duration_minutes AS product_duration_minutes
FROM pricing_rules r
LEFT JOIN services  s ON s.id = r.service_id
LEFT JOIN countries c ON c.id = r.country_id
LEFT JOIN products  p ON p.id = r.product_id
WHERE r.id = $1;
