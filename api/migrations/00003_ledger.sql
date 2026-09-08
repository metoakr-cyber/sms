-- +goose Up
-- +goose StatementBegin

-- Para birimi. Kullanıcı cüzdanı YALNIZ TRY'dir (docs/design.md ADR-017).
-- USD yalnız sağlayıcı maliyetlerinde kullanılır ve orada mikro-birimde saklanır.
CREATE TYPE currency_code AS ENUM ('TRY', 'USD');

CREATE TYPE ledger_type AS ENUM (
    'DEPOSIT',     -- bakiye yükleme onaylandı
    'PURCHASE',    -- numara satın alındı (negatif)
    'REFUND',      -- iade (pozitif)
    'ADJUSTMENT',  -- admin manuel düzeltme
    'COMMISSION',  -- referans komisyonu (v1.1)
    'CHARGEBACK'   -- ödeme itirazı (negatif)
);

-- ─────────────────────── HAREKET DEFTERİ ───────────────────────
-- Bakiye bir SAYI değil, bu defterin SONUCUDUR.
-- users.balance_minor türetilmiş bir önbellektir ve her zaman
-- SUM(ledger_entries.amount_minor) ile eşit olmak zorundadır (FR-200).

CREATE TABLE ledger_entries (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id             BIGINT        NOT NULL REFERENCES users(id),
    amount_minor        BIGINT        NOT NULL,   -- + alacak, - borç
    currency            currency_code NOT NULL DEFAULT 'TRY',
    entry_type          ledger_type   NOT NULL,

    -- Hangi işlemden doğdu: 'deposit' | 'order' | 'manual' | 'referral'
    reference_type      TEXT,
    reference_id        TEXT,

    -- İşlem SONRASI bakiye. Mutabakat ve denetim için kaydedilir; böylece
    -- her satırda "o an ne olduğu" bağımsız olarak doğrulanabilir.
    balance_after_minor BIGINT        NOT NULL,

    -- ÇİFT İŞLEM KORUMASI. Aynı anahtarla ikinci çağrı yeni kayıt üretmez.
    -- Eski prototipte bu yoktu: iki paralel onay isteği bakiyeyi iki kez
    -- yüklüyordu (docs/memory.md §3.6).
    idempotency_key     TEXT          NOT NULL UNIQUE,

    created_by_user_id  BIGINT REFERENCES users(id),  -- admin işlemiyse
    note                TEXT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),

    -- Sıfır tutarlı hareket anlamsızdır ve mutabakatı gürültüler.
    CONSTRAINT ledger_amount_nonzero CHECK (amount_minor <> 0),
    -- Hareket sonrası bakiye negatif olamaz.
    CONSTRAINT ledger_balance_non_negative CHECK (balance_after_minor >= 0)
);

CREATE INDEX ledger_user_created_idx ON ledger_entries (user_id, created_at DESC, id DESC);
CREATE INDEX ledger_reference_idx    ON ledger_entries (reference_type, reference_id);
CREATE INDEX ledger_type_idx         ON ledger_entries (entry_type, created_at DESC);

-- +goose StatementEnd

-- ─────────────────────── DEĞİŞMEZLİK ───────────────────────
-- Defter kaydı ASLA güncellenmez veya silinmez (FR-204).
-- test: scripts/smoke-auth.sh — ledger koruma tetikleyicisi doğrulaması
-- Düzeltme, ters yönlü bir ADJUSTMENT kaydıyla yapılır.
--
-- Bu kural uygulamada da var ama veritabanında DA zorlanır: elle açılan bir
-- psql oturumu, yanlış yazılmış bir migration veya ileride eklenecek bir
-- yönetim aracı defteri bozamaz.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ledger_is_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION
        'ledger_entries degistirilemez veya silinemez (islem: %). Duzeltme icin ters yonlu bir ADJUSTMENT kaydi ekleyin.',
        TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER ledger_no_update
    BEFORE UPDATE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_is_append_only();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER ledger_no_delete
    BEFORE DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_is_append_only();
-- +goose StatementEnd

-- ─────────────────────── MUTABAKAT ───────────────────────
-- Günlük kontrol: her kullanıcı için defter toplamı ile önbelleklenmiş bakiye
-- eşit mi (FR-205). Sapma bir HATANIN BELİRTİSİDİR; otomatik düzeltilmez,
-- alarm üretilir — düzeltmek belirtiyi gizler.

-- +goose StatementBegin
CREATE VIEW ledger_reconciliation AS
SELECT
    u.id                                        AS user_id,
    u.email,
    u.balance_minor                             AS cached_balance,
    COALESCE(SUM(l.amount_minor), 0)::BIGINT    AS ledger_balance,
    u.balance_minor - COALESCE(SUM(l.amount_minor), 0)::BIGINT AS drift
FROM users u
LEFT JOIN ledger_entries l ON l.user_id = u.id
WHERE u.deleted_at IS NULL
GROUP BY u.id, u.email, u.balance_minor;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP VIEW IF EXISTS ledger_reconciliation;
DROP TRIGGER IF EXISTS ledger_no_delete ON ledger_entries;
DROP TRIGGER IF EXISTS ledger_no_update ON ledger_entries;
DROP FUNCTION IF EXISTS ledger_is_append_only();
DROP TABLE IF EXISTS ledger_entries;
DROP TYPE IF EXISTS ledger_type, currency_code;
-- +goose StatementEnd
