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
SELECT * FROM countries WHERE is_visible
ORDER BY (iso2 = 'TR') DESC, coalesce(nullif(name_tr, ''), name);

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
JOIN provider_offers o ON o.product_id = p.id
JOIN providers pr ON pr.id = o.provider_id AND pr.is_active
WHERE p.is_active AND p.kind = 'SMS_ACTIVATION'
  AND p.verification_type = 'sms'
  AND s.is_visible AND c.is_visible
  -- STOKSUZ SATIRLAR NORMALDE GİZLENİR (FR-306): kullanıcıyı parasının
  -- çekileceği ama numara gelmeyeceği bir akışa sokmamak için.
  --
  -- TEK İSTİSNA: kullanıcının KENDİ ÜLKESİ. Türkiye'yi listeden tamamen
  -- çıkarmak, Türkiye'den bakan kullanıcıya "bu site Türk numarası satmıyor"
  -- dedirtiyor — oysa doğrusu "şu an stok yok". Satır GÖRÜNÜR ama
  -- `inStock: false` gelir ve arayüz onu SEÇİLEMEZ gösterir.
  AND ((o.is_available AND o.stock > 0) OR c.iso2 = 'TR')
  AND (sqlc.narg('service_code')::text IS NULL OR s.code = sqlc.narg('service_code')::text)
  AND (sqlc.narg('country_iso')::text  IS NULL OR c.iso2 = sqlc.narg('country_iso')::text)
-- 🔴 `s.sort_order` GROUP BY'a AÇIKÇA eklenmek ZORUNDA. Gruplama anahtarındaki
-- `p.id` yalnız `products` sütunlarını işlevsel bağımlılıkla kurtarır;
-- `services` sütunları için PK (`s.id`) gerekirdi. Eksikse Postgres
-- "column s.sort_order must appear in the GROUP BY clause" hatasını verir.
--
-- NOT: bu cümle bizim verdiğimiz bir güvence değil, Postgres'in davranışının
-- tarifidir. "reddeder" gibi bir fiil kullanılırsa scripts/check-guarantees.py
-- bunu güvence sayar ve aynı blokta bir `test:` atfı arar — bir kez düşürdü.
GROUP BY p.id, s.sort_order, s.code, s.name, s.name_tr, s.icon_url,
         c.iso2, c.name_tr, c.phone_code
-- ÖNCE POPÜLERLİK, SONRA TÜRKİYE.
--
-- `s.sort_order` popülerlik sırasıdır (web/scripts/servis-siralama.sql).
-- Alfabetik sıra kullanıcının aradığı servisi değil, adı "A" ile başlayanı
-- öne çıkarırdı.
--
-- Ülke düzeyinde TÜRKİYE HER ZAMAN ÖNCE: kullanıcıların çoğu Türkiye'den ve
-- en çok aradıkları ülke bu. Alfabetik sırada "Türkiye" 190 ülkenin sonlarında
-- kalıyor ve kullanıcı her seferinde listeyi sonuna kadar kaydırıyor.
--
-- Sıralama SUNUCUDA yapılır: istemcide yapılsaydı her istemci kendi kuralını
-- uygular, mobil ve masaüstü farklı sıralanırdı.
ORDER BY s.sort_order, s.code, (c.iso2 = 'TR') DESC, c.name_tr;

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

-- name: ListServicesWithStock :many
-- Servis IZGARASI için özet: yalnız en az bir ülkede STOKLU olan servisler,
-- her biri için stoklu ülke sayısı.
--
-- NEDEN AYRI BİR SORGU: gerçek katalogda 9768 stoklu servis×ülke kombinasyonu
-- var ve hepsini tek yanıtta göndermek 1,09 MB ediyordu. Mobilde 4G'de bu
-- kabul edilemez (docs/frontend-contract.md §8). Izgara yalnız servisleri
-- gösterir; ülkeler servis seçilince ayrıca çekilir (~6 KB).
SELECT
    s.code          AS service_code,
    s.name          AS service_name,
    s.name_tr       AS service_name_tr,
    s.icon_url,
    count(DISTINCT c.id)::bigint AS country_count,
    min(o.cost_micro)::bigint    AS min_cost_micro
FROM products p
JOIN services  s ON s.id = p.service_id
JOIN countries c ON c.id = p.country_id
JOIN provider_offers o ON o.product_id = p.id AND o.is_available AND o.stock > 0
JOIN providers pr ON pr.id = o.provider_id AND pr.is_active
WHERE p.is_active AND p.kind = 'SMS_ACTIVATION'
  AND p.verification_type = 'sms'
  AND s.is_visible AND c.is_visible
-- 🔴 `s.sort_order` GROUP BY'a AÇIKÇA eklenmelidir: `services` PK'sı (`s.id`)
-- gruplama anahtarında olmadığı için işlevsel bağımlılık kurtarmaz.
GROUP BY s.code, s.name, s.name_tr, s.icon_url, s.sort_order
-- POPÜLERLİK ÖNCE. Sıra `services.sort_order` alanından gelir ve
-- web/scripts/servis-siralama.sql ile yönetilir; eşit değerler ada göre
-- alfabetik dizilir. Alfabetik sıralamak, kullanıcının aradığı servisi değil
-- adı "A" ile başlayanı öne çıkarıyordu (ızgaranın ilk ekranı Whatnot/Adobe).
--
-- İSTEMCİDE YENİDEN SIRALANMAZ: sıralama tek yerde, burada yapılır.
ORDER BY s.sort_order, s.name;

-- name: ResolveCountryRef :one
-- Sağlayıcının İngilizce ülke adını ISO2 + Türkçe ad + telefon koduna çevirir.
SELECT iso2, name_tr, phone_code FROM country_reference WHERE name_key = lower(@name_key);

-- name: ResolveDimensionLocal :one
-- Sağlayıcının boyut kodunu YEREL kimliğe çevirir.
--
-- Teklif senkronu eskiden ülkeyi `countries.iso2` üzerinden arıyordu. Bu, ancak
-- sağlayıcının kodu tesadüfen ISO2 ise çalışır; HeroSMS "62" gönderir ve arama
-- boş döner. Sağlayıcı kodu ile yerel kimlik arasındaki köprü BURASIDIR.
SELECT local_id FROM provider_dimension_maps
WHERE provider_id = @provider_id AND dimension = @dimension AND remote_code = @remote_code;

-- name: GetProviderRemoteCodes :one
-- Bir ürünün, BELİRLİ BİR SAĞLAYICIDAKİ karşılıklarını verir.
--
-- Sağlayıcıya istek atarken BİZİM kodlarımız (services.code, countries.iso2)
-- KULLANILAMAZ. HeroSMS ülkeyi "62" bilir, biz "TR" biliriz. Çeviri burada
-- yapılır; yapılmazsa sağlayıcı "böyle bir ülke yok" der ve teklif düşer.
-- test: internal/service/pricing/remote_codes_integration_test.go#TestProviderReceivesItsOwnCodes
SELECT
    ms.remote_code AS service_remote_code,
    mc.remote_code AS country_remote_code
FROM products p
JOIN provider_dimension_maps ms
     ON ms.provider_id = @provider_id AND ms.dimension = 'service' AND ms.local_id = p.service_id
JOIN provider_dimension_maps mc
     ON mc.provider_id = @provider_id AND mc.dimension = 'country' AND mc.local_id = p.country_id
WHERE p.id = @product_id;

-- name: UpsertRentalProduct :one
-- Kiralık ürün. Aktivasyondan TEK FARKI süre boyutunun dolu olması —
-- ama o fark benzersizlik anahtarının parçası olduğu için aynı servis×ülke
-- için sekiz ayrı ürün (sekiz süre) yan yana durabilir.
INSERT INTO products (kind, service_id, country_id, verification_type, duration_minutes)
VALUES ('SMS_RENTAL', @service_id, @country_id, 'sms', @duration_minutes)
ON CONFLICT (kind, service_id, country_id, operator_id, verification_type,
             duration_minutes, dimension_a_id)
DO UPDATE SET is_active = true
RETURNING *;

-- name: GetProductForRental :one
SELECT p.* FROM products p
WHERE p.kind = 'SMS_RENTAL'
  AND p.service_id = @service_id AND p.country_id = @country_id
  AND p.duration_minutes = @duration_minutes
  AND p.operator_id IS NULL
  AND p.is_active;

-- name: ListRentalDurationsForCatalog :many
-- Bir servis × ülke için kiralanabilir süreler ve en düşük maliyet.
--
-- Fiyat BURADA DÖNMEZ: kullanıcıya gösterilen fiyat teklif (quote) ile
-- verilir. Buradaki maliyet yalnız SIRALAMA içindir ve dışa açılmaz.
SELECT
    p.duration_minutes,
    min(o.cost_micro)::bigint AS min_cost_micro,
    sum(o.stock)::bigint      AS total_stock
FROM products p
JOIN provider_offers o ON o.product_id = p.id AND o.is_available AND o.stock > 0
JOIN providers pr ON pr.id = o.provider_id AND pr.is_active
JOIN services s ON s.id = p.service_id
JOIN countries c ON c.id = p.country_id
WHERE p.kind = 'SMS_RENTAL' AND p.is_active
  AND s.code = @service_code AND c.iso2 = @country_iso
GROUP BY p.duration_minutes
ORDER BY p.duration_minutes;

-- name: ListRentalServicesWithStock :many
-- Kiralık ızgarası: en az bir ülke×sürede stoklu servisler.
SELECT
    s.code AS service_code, s.name AS service_name, s.name_tr AS service_name_tr,
    s.icon_url,
    count(DISTINCT c.id)::bigint AS country_count,
    count(DISTINCT p.duration_minutes)::bigint AS duration_count
FROM products p
JOIN services  s ON s.id = p.service_id
JOIN countries c ON c.id = p.country_id
JOIN provider_offers o ON o.product_id = p.id AND o.is_available AND o.stock > 0
JOIN providers pr ON pr.id = o.provider_id AND pr.is_active
WHERE p.kind = 'SMS_RENTAL' AND p.is_active
  AND s.is_visible AND c.is_visible
-- `s.sort_order` GROUP BY'da açıkça yer almalı (bkz. ListServicesWithStock).
GROUP BY s.code, s.name, s.name_tr, s.icon_url, s.sort_order
-- Aktivasyon ızgarasıyla AYNI popülerlik sırası: kullanıcı iki ekranda farklı
-- sıra görürse listenin rastgele olduğunu düşünür.
ORDER BY s.sort_order, s.name;

-- name: ListRentalCountriesForService :many
SELECT DISTINCT
    c.iso2 AS country_iso2, c.name AS country_name, c.name_tr AS country_name_tr,
    c.phone_code,
    -- DISTINCT ile ORDER BY ifadeleri seçim listesinde OLMAK ZORUNDA.
    -- Bu iki sütun yalnız sıralama içindir; DTO'ya taşınmaz.
    (c.iso2 = 'TR') AS is_home,
    coalesce(nullif(c.name_tr, ''), c.name) AS sort_name
FROM products p
JOIN services  s ON s.id = p.service_id
JOIN countries c ON c.id = p.country_id
JOIN provider_offers o ON o.product_id = p.id AND o.is_available AND o.stock > 0
JOIN providers pr ON pr.id = o.provider_id AND pr.is_active
WHERE p.kind = 'SMS_RENTAL' AND p.is_active
  AND s.code = @service_code AND s.is_visible AND c.is_visible
ORDER BY is_home DESC, sort_name;

-- name: GetProductByID :one
SELECT * FROM products WHERE id = @id;

-- name: ListProvidersForAdmin :many
-- Yönetim sağlayıcı listesi.
--
-- 🔴 api_key_enc SEÇİLMEZ. Şifreli hâli bile dışarı verilmez: panelde
-- gösterilecek bir şey değil ve varlığı/uzunluğu bilgi sızdırır.
-- Anahtarın TANIMLI OLUP OLMADIĞI yeterli bilgidir.
SELECT
    p.id, p.name, p.protocol, p.base_url, p.is_active, p.priority,
    p.cost_multiplier, p.capabilities,
    -- COALESCE ZORUNLU: api_key_enc NULL ise length() de NULL döner ve
    -- `bool` alana tarama "cannot scan NULL into *bool" ile ÇÖKER — yani
    -- anahtarı henüz kurulmamış TEK bir sağlayıcı, tüm listeyi 500 yapardı.
    -- test: internal/transport/http/handler/admin_integration_test.go#TestListProvidersNeverReturnsAPIKey
    COALESCE(length(p.api_key_enc) > 0, false)::bool AS has_api_key,
    p.account_balance_micro, p.account_synced_at,
    p.created_at, p.updated_at
FROM providers p
ORDER BY p.priority, p.name;

-- name: UpdateProviderSettings :one
-- Sağlayıcı ayarları. API ANAHTARI BURADAN GÜNCELLENMEZ — ayrı bir yol var.
UPDATE providers SET
    base_url        = @base_url,
    is_active       = @is_active,
    priority        = @priority,
    cost_multiplier = @cost_multiplier,
    updated_at      = now()
WHERE id = @id
RETURNING id, name, protocol, base_url, is_active, priority, cost_multiplier;

-- name: UpdateProviderAPIKey :one
-- API anahtarı güncelleme — AYRI bir sorgu.
--
-- Diğer ayarlarla aynı UPDATE'e konsaydı, ayar değiştiren her istek anahtarı
-- da yazardı ve boş bir alan anahtarı SİLERDİ. Ayrı tutmak, "kaydet"e basmanın
-- anahtarı yanlışlıkla silmesini imkânsız kılar.
-- test: internal/transport/http/handler/admin_integration_test.go#TestSavingProviderSettingsDoesNotEraseAPIKey
--
-- 🔴 `:exec` DEĞİL `:one`: etkilenen satır sayısı GÖRÜLMELİDİR. `:exec` yalnız
-- hata döner, "hiçbir satır güncellenmedi" hata DEĞİLDİR — var olmayan bir
-- sağlayıcı kimliğine anahtar yazmak sessizce başarılı görünüyordu ve yönetici
-- maskeli önizlemeyi görüp anahtarı kaydettiğini sanıyordu. Sağlayıcı
-- çalışmadığında sebep aranacak en son yer orasıdır.
-- test: internal/transport/http/handler/admin_provider_integration_test.go#TestSetAPIKeyOnMissingProviderIs404
UPDATE providers SET api_key_enc = @api_key_enc, updated_at = now()
WHERE id = @id
RETURNING id;

-- name: ListOffersForProductAnyStock :many
-- YALNIZ YÖNETİM ÖNİZLEMESİ İÇİN: stok koşulu yoktur.
--
-- `ListOffersForProduct` stoksuz teklifleri eler — satış yolunda doğru olan
-- budur. Ama yönetici marjı değiştirirken stoğu tükenmiş bir ürünün fiyatını
-- da görebilmeli: maliyet biliniyor, satılamıyor olması fiyatı bilinmez
-- yapmaz. Bu sorgu SATIŞ YOLUNDA KULLANILMAZ.
-- test: internal/service/pricing/rules_integration_test.go#TestPreviewWorksWhenOutOfStock
SELECT o.*, pr.name AS provider_name, pr.protocol, pr.priority, pr.cost_multiplier
FROM provider_offers o
JOIN providers pr ON pr.id = o.provider_id
WHERE o.product_id = $1 AND pr.is_active
ORDER BY o.cost_micro, pr.priority;
