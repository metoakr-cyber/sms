-- ─────────────────────── Boyutlar ───────────────────────

-- name: UpsertService :one
INSERT INTO services (code, name, name_tr, sort_order)
VALUES ($1, $2, $3, $4)
ON CONFLICT (code) DO UPDATE
    SET name = EXCLUDED.name,
        -- Türkçe ad ve görünürlük ADMIN tarafından yönetilir; senkron bunları EZMEZ.
        name_tr = CASE WHEN services.name_tr = '' THEN EXCLUDED.name_tr ELSE services.name_tr END
RETURNING *;

-- name: UpsertCountry :one
INSERT INTO countries (iso2, name, name_tr, phone_code, supports_rent)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (iso2) DO UPDATE
    SET name = EXCLUDED.name,
        phone_code = EXCLUDED.phone_code,
        supports_rent = EXCLUDED.supports_rent,
        name_tr = CASE WHEN countries.name_tr = '' THEN EXCLUDED.name_tr ELSE countries.name_tr END
RETURNING *;

-- name: ListVisibleServices :many
SELECT * FROM services WHERE is_visible ORDER BY sort_order, name;

-- name: ListVisibleCountries :many
SELECT * FROM countries WHERE is_visible ORDER BY name_tr, name;

-- name: GetServiceByCode :one
SELECT * FROM services WHERE code = $1;

-- name: GetCountryByISO :one
SELECT * FROM countries WHERE iso2 = $1;

-- ─────────────────────── Ürün ───────────────────────

-- name: UpsertProduct :one
INSERT INTO products (kind, service_id, country_id, verification_type)
VALUES ($1, $2, $3, $4)
ON CONFLICT (kind, service_id, country_id, operator_id, verification_type,
             duration_minutes, dimension_a_id)
DO UPDATE SET is_active = true
RETURNING *;

-- name: GetProductForActivation :one
SELECT p.* FROM products p
WHERE p.kind = 'SMS_ACTIVATION'
  AND p.service_id = $1 AND p.country_id = $2
  AND p.verification_type = $3
  AND p.operator_id IS NULL AND p.duration_minutes IS NULL
  AND p.is_active;

-- ─────────────────────── Sağlayıcı ───────────────────────

-- name: ListActiveProviders :many
SELECT * FROM providers WHERE is_active ORDER BY priority, id;

-- name: GetProvider :one
SELECT * FROM providers WHERE id = $1;

-- name: GetProviderByName :one
SELECT * FROM providers WHERE name = $1;

-- name: CreateProvider :one
INSERT INTO providers (name, protocol, base_url, api_key_enc, is_active, priority, cost_multiplier, capabilities)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING *;

-- name: UpdateProviderBalance :exec
UPDATE providers SET account_balance_micro = $2, account_synced_at = now() WHERE id = $1;

-- name: SetProviderActive :exec
UPDATE providers SET is_active = $2 WHERE id = $1;

-- ─────────────────────── Boyut eşleştirme ───────────────────────

-- name: UpsertDimensionMap :exec
INSERT INTO provider_dimension_maps (provider_id, dimension, local_id, remote_code)
VALUES ($1,$2,$3,$4)
ON CONFLICT (provider_id, dimension, local_id)
DO UPDATE SET remote_code = EXCLUDED.remote_code, synced_at = now();

-- name: GetRemoteCode :one
SELECT remote_code FROM provider_dimension_maps
WHERE provider_id = $1 AND dimension = $2 AND local_id = $3;

-- name: ListDimensionMaps :many
SELECT * FROM provider_dimension_maps
WHERE provider_id = $1 AND dimension = $2
ORDER BY local_id;

-- name: CountDimensionMaps :one
SELECT count(*) FROM provider_dimension_maps WHERE provider_id = $1 AND dimension = $2;

-- ─────────────────────── Fiyat/stok önbelleği ───────────────────────

-- name: UpsertOffer :exec
-- synced_at UYGULAMADAN gelir, now() DEĞİL.
--
-- Bayat teklif tespiti bu alanı bir eşikle karşılaştırır; yazım veritabanı
-- saatini, karşılaştırma uygulama saatini kullanırsa aradaki kayma tüm
-- teklifleri sessizce "yok" işaretler. İki taraf AYNI saat kaynağını kullanmalı.
INSERT INTO provider_offers (provider_id, product_id, cost_micro, cost_currency, stock, is_available, synced_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (provider_id, product_id) DO UPDATE
    SET cost_micro = EXCLUDED.cost_micro,
        stock = EXCLUDED.stock,
        is_available = EXCLUDED.is_available,
        synced_at = EXCLUDED.synced_at;

-- name: MarkStaleOffersUnavailable :execrows
-- Senkron turunda görülmeyen teklifler "yok" sayılır. Aksi halde sağlayıcının
-- listeden çıkardığı bir kombinasyon sonsuza kadar stokta görünürdü.
UPDATE provider_offers SET is_available = false, stock = 0
WHERE provider_id = $1 AND synced_at < $2 AND is_available;

-- name: ListAvailableProductsForCatalog :many
-- Kullanıcıya gösterilecek katalog: en az bir sağlayıcıda stoklu ürünler.
SELECT
    p.id            AS product_id,
    s.code          AS service_code,
    s.name          AS service_name,
    s.name_tr       AS service_name_tr,
    s.icon_url,
    c.iso2          AS country_iso2,
    c.name_tr       AS country_name_tr,
    c.phone_code,
    min(o.cost_micro)::bigint AS min_cost_micro,
    sum(o.stock)::bigint      AS total_stock
FROM products p
JOIN services  s ON s.id = p.service_id
JOIN countries c ON c.id = p.country_id
JOIN provider_offers o ON o.product_id = p.id AND o.is_available AND o.stock > 0
JOIN providers pr ON pr.id = o.provider_id AND pr.is_active
WHERE p.is_active AND p.kind = 'SMS_ACTIVATION'
  AND p.verification_type = 'sms'
  AND s.is_visible AND c.is_visible
  AND (sqlc.narg('service_code')::text IS NULL OR s.code = sqlc.narg('service_code')::text)
  AND (sqlc.narg('country_iso')::text  IS NULL OR c.iso2 = sqlc.narg('country_iso')::text)
GROUP BY p.id, s.code, s.name, s.name_tr, s.icon_url, c.iso2, c.name_tr, c.phone_code
ORDER BY s.code, c.name_tr;

-- name: ListOffersForProduct :many
-- Bir ürün için sağlayıcı teklifleri; en ucuz önce.
SELECT o.*, pr.name AS provider_name, pr.protocol, pr.priority, pr.cost_multiplier
FROM provider_offers o
JOIN providers pr ON pr.id = o.provider_id
WHERE o.product_id = $1 AND o.is_available AND o.stock > 0 AND pr.is_active
ORDER BY o.cost_micro, pr.priority;

-- name: SetServiceIcon :one
-- Servis logosunu ayarlar. Yol `web/public/` köküne göredir: /servis-logolari/wa.svg
UPDATE services SET icon_url = @icon_url, updated_at = now()
WHERE code = @code
RETURNING code, name, icon_url;
