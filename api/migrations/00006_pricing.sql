-- +goose Up
-- +goose StatementBegin

-- Kural kapsamı. En SPESİFİK olan kazanır:
--   PRODUCT > SERVICE_COUNTRY > SERVICE > COUNTRY > GLOBAL
-- Bu sıra kodda tek bir yerde tanımlanır (domain/pricing) ve testle sabitlenir.
CREATE TYPE pricing_scope AS ENUM (
    'GLOBAL', 'COUNTRY', 'SERVICE', 'SERVICE_COUNTRY', 'PRODUCT'
);

-- ─────────────────────── FİYAT KURALLARI ───────────────────────
-- Kâr marjı bir İŞ KARARIDIR; koda gömülmez, dağıtım gerektirmez.
-- Eski prototipte marj koda yazılmıştı ve HİÇ UYGULANMIYORDU:
-- myProfitMargin tanımlanıyor ama finalUserPrice = bestOffer.cost olarak
-- dönüyordu — platform maliyetine satıyordu (docs/memory.md §3.5).
CREATE TABLE pricing_rules (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    scope           pricing_scope NOT NULL,
    service_id      BIGINT REFERENCES services(id)  ON DELETE CASCADE,
    country_id      BIGINT REFERENCES countries(id) ON DELETE CASCADE,
    product_id      BIGINT REFERENCES products(id)  ON DELETE CASCADE,

    margin_percent  NUMERIC(7,2) NOT NULL
                    CONSTRAINT pricing_margin_range CHECK (margin_percent >= 0 AND margin_percent <= 1000),
    fixed_fee_minor BIGINT NOT NULL DEFAULT 0
                    CONSTRAINT pricing_fee_non_negative CHECK (fixed_fee_minor >= 0),
    min_price_minor BIGINT NOT NULL DEFAULT 0
                    CONSTRAINT pricing_min_non_negative CHECK (min_price_minor >= 0),

    is_active       BOOLEAN NOT NULL DEFAULT true,
    valid_from      TIMESTAMPTZ,
    valid_to        TIMESTAMPTZ,
    note            TEXT NOT NULL DEFAULT '',
    created_by_user_id BIGINT REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Kapsam ile doldurulan alanlar TUTARLI olmalı. Aksi halde "SERVICE
    -- kapsamlı ama service_id NULL" gibi bir kural sessizce hiçbir şeye
    -- uymaz ve fiyatlandırma sessizce GLOBAL'e düşer.
    CONSTRAINT pricing_scope_consistent CHECK (
        (scope = 'GLOBAL'          AND service_id IS NULL AND country_id IS NULL AND product_id IS NULL) OR
        (scope = 'COUNTRY'         AND service_id IS NULL AND country_id IS NOT NULL AND product_id IS NULL) OR
        (scope = 'SERVICE'         AND service_id IS NOT NULL AND country_id IS NULL AND product_id IS NULL) OR
        (scope = 'SERVICE_COUNTRY' AND service_id IS NOT NULL AND country_id IS NOT NULL AND product_id IS NULL) OR
        (scope = 'PRODUCT'         AND product_id IS NOT NULL)
    )
);

-- Aynı kapsamda ETKİN iki kural olamaz: hangisinin uygulanacağı belirsiz olurdu.
CREATE UNIQUE INDEX pricing_rules_global_uniq  ON pricing_rules (scope)
    WHERE is_active AND scope = 'GLOBAL';
CREATE UNIQUE INDEX pricing_rules_country_uniq ON pricing_rules (country_id)
    WHERE is_active AND scope = 'COUNTRY';
CREATE UNIQUE INDEX pricing_rules_service_uniq ON pricing_rules (service_id)
    WHERE is_active AND scope = 'SERVICE';
CREATE UNIQUE INDEX pricing_rules_sc_uniq      ON pricing_rules (service_id, country_id)
    WHERE is_active AND scope = 'SERVICE_COUNTRY';
CREATE UNIQUE INDEX pricing_rules_product_uniq ON pricing_rules (product_id)
    WHERE is_active AND scope = 'PRODUCT';

-- ─────────────────────── KUR ───────────────────────
CREATE TABLE fx_rates (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    base       currency_code NOT NULL,
    quote      currency_code NOT NULL,
    rate       NUMERIC(18,8) NOT NULL CONSTRAINT fx_rate_positive CHECK (rate > 0),
    source     TEXT NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX fx_rates_lookup_idx ON fx_rates (base, quote, fetched_at DESC);

-- ─────────────────────── FİYAT TEKLİFİ ───────────────────────
-- "Gösterilen fiyat = tahsil edilen fiyat" güvencesinin taşıyıcısı.
--
-- İstemciye YALNIZ public_id gönderilir (test: scripts/smoke-auth.sh —
-- teklif yanıtında sağlayıcı/maliyet aranır). provider_id ve maliyet ASLA
-- gönderilmez: eski prototipte istemci hangi sağlayıcıdan alacağını ve hangi
-- fiyata alacağını gövdede belirleyebiliyordu (docs/memory.md §3.8).
CREATE TABLE price_quotes (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id        UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id       BIGINT NOT NULL REFERENCES products(id),
    provider_id      BIGINT NOT NULL REFERENCES providers(id),

    cost_micro       BIGINT NOT NULL,              -- sağlayıcı maliyeti (mikro-USD)
    cost_currency    currency_code NOT NULL DEFAULT 'USD',
    fx_rate          NUMERIC(18,8) NOT NULL,
    margin_percent   NUMERIC(7,2) NOT NULL,
    pricing_rule_id  BIGINT REFERENCES pricing_rules(id),

    -- SÖZLEŞME: tahsil edilecek tutar (TRY kuruş).
    sell_price_minor BIGINT NOT NULL CONSTRAINT quote_price_positive CHECK (sell_price_minor > 0),
    stock_at_quote   INT NOT NULL DEFAULT 0,

    expires_at       TIMESTAMPTZ NOT NULL,
    consumed_at      TIMESTAMPTZ,                  -- TEK KULLANIMLIK
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX price_quotes_public_id_key ON price_quotes (public_id);
CREATE INDEX price_quotes_user_idx ON price_quotes (user_id, created_at DESC);
-- Yetim provizyon kurtarma işi bu indeksi kullanır (FR-408).
CREATE INDEX price_quotes_consumed_idx ON price_quotes (consumed_at) WHERE consumed_at IS NOT NULL;

-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER pricing_rules_set_updated_at
    BEFORE UPDATE ON pricing_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- Başlangıç kuralı: %40 global marj.
-- Bu satır olmadan fiyatlandırma hiçbir kural bulamaz ve satış yapılamaz;
-- "kural yoksa maliyetine sat" davranışı KABUL EDİLEMEZ.
-- +goose StatementBegin
INSERT INTO pricing_rules (scope, margin_percent, note)
VALUES ('GLOBAL', 40.00, 'Varsayılan global marj — admin panelinden ayarlanır');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS price_quotes, fx_rates, pricing_rules;
DROP TYPE IF EXISTS pricing_scope;
-- +goose StatementEnd
