-- +goose Up
-- +goose StatementBegin

-- BEFORE DELETE tetikleyicisi TRUNCATE'i YAKALAMAZ — ayrı bir olay tipidir.
-- Bu boşluk kapatılmazsa tek bir TRUNCATE tüm mali kaydı siler.
--
-- Kaçış kapısı: yalnız testler için, oturum düzeyinde açık izin gerekir:
--   SET LOCAL app.allow_ledger_truncate = 'on';
-- Üretim yapılandırmasında bu değişken ayarlanmaz; yalnız test temizliğinde
-- ve tek transaction içinde (SET LOCAL) kullanılır.
-- test: scripts/smoke-auth.sh — ortam kapısı + koruma doğrulaması
CREATE OR REPLACE FUNCTION ledger_truncate_guard() RETURNS trigger AS $$
BEGIN
    IF current_setting('app.allow_ledger_truncate', true) = 'on' THEN
        RETURN NULL;
    END IF;
    RAISE EXCEPTION
        'ledger_entries TRUNCATE edilemez. Mali kayit silinmez; duzeltme icin ters yonlu ADJUSTMENT ekleyin.'
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER ledger_no_truncate
    BEFORE TRUNCATE ON ledger_entries
    FOR EACH STATEMENT EXECUTE FUNCTION ledger_truncate_guard();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS ledger_no_truncate ON ledger_entries;
DROP FUNCTION IF EXISTS ledger_truncate_guard();
-- +goose StatementEnd
