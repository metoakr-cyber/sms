-- +goose Up
-- +goose StatementBegin

-- ─────────────────────── ÜRÜN TİPLERİ ───────────────────────
-- Yeni bir ürün tipi eklemek bu enum'a değer eklemektir; orders, ledger_entries
-- ve price_quotes tablolarında DEĞİŞİKLİK GEREKMEZ (docs/design.md §4, ADR-010).
CREATE TYPE product_kind AS ENUM (
    'SMS_ACTIVATION',  -- v1: tek seferlik SMS doğrulama
    'SMS_RENTAL'       -- v1.1: kiralık numara (şema hazır, özellik kapalı)
);

-- Doğrulama tipi AYRI bir fiyat/stok eksenidir: HeroSMS'te offers uç noktasının
-- path segmentidir ve aynı servis+ülke için farklı fiyat/stok taşır.
CREATE TYPE verification_type AS ENUM ('sms', 'call');

CREATE TYPE provider_protocol AS ENUM (
    'FAKE',        -- test ve yerel geliştirme
    'HEROSMS_V1',  -- modern REST + legacy uyumluluk
    'FIVE_SIM'     -- v1.1
);

-- ─────────────────────── BOYUTLAR ───────────────────────

CREATE TABLE services (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code       TEXT NOT NULL UNIQUE,      -- 'wa', 'tg', 'ig'
    name       TEXT NOT NULL,             -- 'Whatsapp'
    name_tr    TEXT NOT NULL DEFAULT '',
    icon_url   TEXT NOT NULL DEFAULT '',
    is_visible BOOLEAN NOT NULL DEFAULT true,
    sort_order INT NOT NULL DEFAULT 1000,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX services_visible_idx ON services (is_visible, sort_order) WHERE is_visible;

CREATE TABLE countries (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    iso2        TEXT NOT NULL UNIQUE,      -- 'TR'
    name        TEXT NOT NULL,             -- 'Turkey'
    name_tr     TEXT NOT NULL DEFAULT '',  -- 'Türkiye'
    phone_code  TEXT NOT NULL DEFAULT '',
    is_visible  BOOLEAN NOT NULL DEFAULT true,
    supports_rent BOOLEAN NOT NULL DEFAULT false,  -- sağlayıcının 'rent' bayrağı
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX countries_visible_idx ON countries (is_visible, name_tr) WHERE is_visible;

-- Operatör boyutu HeroSMS için FİYATLANAMAZ: hiçbir uç nokta operatör kırılımlı
-- fiyat/stok vermiyor (docs/design.md ADR-029). Tablo yine de tutulur çünkü
-- satın alma isteğinde operatör tercihi gönderilebilir.
CREATE TABLE operators (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    country_id BIGINT NOT NULL REFERENCES countries(id) ON DELETE CASCADE,
    code       TEXT NOT NULL,             -- 'turkcell', 'any'
    name       TEXT NOT NULL DEFAULT '',
    UNIQUE (country_id, code)
);

-- ─────────────────────── ÜRÜN (SKU) ───────────────────────

CREATE TABLE products (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind              product_kind      NOT NULL,
    service_id        BIGINT REFERENCES services(id)  ON DELETE CASCADE,
    country_id        BIGINT REFERENCES countries(id) ON DELETE CASCADE,
    operator_id       BIGINT REFERENCES operators(id) ON DELETE SET NULL,
    verification_type verification_type NOT NULL DEFAULT 'sms',
    duration_minutes  INT,                       -- NULL = tek seferlik

    -- Genel ek boyut. Bugün NULL; e-posta ürünü gibi farklı eksenli bir tip
    -- eklendiğinde 'domain' burada yaşar. JSONB'ye koymak §4'ün kendi kuralını
    -- ihlal ederdi: fiyat/stok taşıyan bir eksen sorgulanabilir olmalıdır.
    dimension_a_id    BIGINT,

    attributes        JSONB NOT NULL DEFAULT '{}',
    is_active         BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- NULLS NOT DISTINCT ZORUNLU: Postgres varsayılanı NULL'ları farklı sayar,
    -- bu da NULL sütunlu ürünlerde SINIRSIZ ÇİFT SATIR demektir.
    CONSTRAINT products_unique_sku UNIQUE NULLS NOT DISTINCT
        (kind, service_id, country_id, operator_id, verification_type,
         duration_minutes, dimension_a_id)
);
CREATE INDEX products_lookup_idx ON products (kind, service_id, country_id) WHERE is_active;

-- ─────────────────────── SAĞLAYICI ───────────────────────

CREATE TABLE providers (
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name                  TEXT NOT NULL UNIQUE,
    -- Adaptör seçimi BURADAN yapılır, isimden DEĞİL. Eski prototip
    -- name.includes('hero') kullanıyordu; adı düzenlemek sistemi bozuyordu.
    protocol              provider_protocol NOT NULL,
    base_url              TEXT NOT NULL DEFAULT '',
    -- API anahtarı AES-GCM ile ŞİFRELİ saklanır; ham hali hiçbir yerde durmaz.
    api_key_enc           BYTEA,
    is_active             BOOLEAN NOT NULL DEFAULT false,
    priority              INT NOT NULL DEFAULT 100,   -- eşit fiyatta tercih sırası

    -- Sağlayıcıya özel maliyet düzeltmesi (komisyon, minimum tutar).
    -- Kâr marjı DEĞİLDİR — marj pricing_rules tablosunda yaşar.
    -- Eski prototipte 'balance' sütunu üç farklı anlamda kullanılıyordu.
    cost_multiplier       NUMERIC(8,4) NOT NULL DEFAULT 1.0
                          CONSTRAINT providers_multiplier_positive CHECK (cost_multiplier > 0),

    -- SAĞLAYICIDAKİ bakiyemiz (mikro-USD). Marj ile karıştırılmaz.
    account_balance_micro BIGINT NOT NULL DEFAULT 0,
    account_synced_at     TIMESTAMPTZ,

    capabilities          JSONB NOT NULL DEFAULT '[]',  -- desteklenen product_kind listesi
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX providers_active_idx ON providers (is_active, priority) WHERE is_active;

-- ─────────────────────── BOYUT EŞLEŞTİRME ───────────────────────
-- TEK eşleştirme tablosu. Eski şemada ProviderCountryMapping ve
-- ProviderPlatformMapping ayrıydı; yeni bir boyut eklemek yeni tablo
-- gerektiriyordu. Burada yeni boyut = yeni SATIR (docs/design.md §4).
CREATE TABLE provider_dimension_maps (
    provider_id BIGINT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    dimension   TEXT   NOT NULL,   -- 'service' | 'country' | 'operator' | ...
    local_id    BIGINT NOT NULL,
    remote_code TEXT   NOT NULL,
    synced_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider_id, dimension, local_id),
    UNIQUE (provider_id, dimension, remote_code)
);

-- ─────────────────────── FİYAT/STOK ÖNBELLEĞİ ───────────────────────
-- Kaynak doğruluk sağlayıcının API'sidir. Bu tablo LİSTELEME ve FİLTRELEME
-- içindir; satın alma anında HER ZAMAN canlı fiyat sorulur.
CREATE TABLE provider_offers (
    provider_id  BIGINT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    product_id   BIGINT NOT NULL REFERENCES products(id)  ON DELETE CASCADE,

    -- Maliyet MİKRO-birimde (6 hane). Sağlayıcı fiyatları 4 ondalıklı gelebiliyor
    -- ve sent'e (2 hane) yuvarlamak birim başına 0,0021 USD'ye kadar kırpar
    -- (docs/design.md ADR-026).
    cost_micro   BIGINT NOT NULL,
    cost_currency currency_code NOT NULL DEFAULT 'USD',

    -- GERÇEK stok. HeroSMS'te counts.physical'a karşılık gelir.
    -- counts.total ASLA kullanılmaz (test: catalog/sync_integration_test.go#
    -- TestSyncOffersPopulatesCatalog): canlı ölçümde WhatsApp×TR için
    -- total=56964 iken physical=0 geldi (docs/provider-herosms.md §3.3).
    stock        INT NOT NULL DEFAULT 0,
    is_available BOOLEAN NOT NULL DEFAULT false,
    synced_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider_id, product_id)
);
CREATE INDEX provider_offers_product_idx ON provider_offers (product_id, cost_micro)
    WHERE is_available;

-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER services_set_updated_at  BEFORE UPDATE ON services  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER countries_set_updated_at BEFORE UPDATE ON countries FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER providers_set_updated_at BEFORE UPDATE ON providers FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS provider_offers, provider_dimension_maps, providers, products, operators, countries, services;
DROP TYPE IF EXISTS provider_protocol, verification_type, product_kind;
-- +goose StatementEnd
