-- +goose Up
-- +goose StatementBegin

-- ── SAĞLAYICIDA KAPATMA İÇİN SAHİPLENME KİRASI ──
--
-- NEDEN VAR: `CloseAtProvider` bugüne kadar `GetOrderForUpdate` (SELECT …
-- FOR UPDATE) ile "kilitli" görünüyordu ama o çağrı bir transaction DIŞINDA,
-- havuz üzerinden tek deyim olarak koşuyordu. Tek deyimlik örtük transaction
-- deyim biter bitmez commit olur ve kilit BIRAKILIR: karşılıklı dışlama
-- yoktu. Yorum "kilit var" diyordu, kod sağlamıyordu.
--
-- Gerçek bir transaction da çözüm DEĞİL: kapatma bir HTTP çağrısı içerir ve
-- dış çağrı asla transaction içinde yapılmaz (CLAUDE.md değişmez #5). Bu
-- yüzden satır, çağrıdan ÖNCE koşullu bir UPDATE ile SAHİPLENİLİR:
-- ikinci işçi sıfır satır alır ve dokunmaz.
--
-- KİRA (lease), kalıcı bir bayrak değil: HTTP çağrısı sırasında ölen bir
-- işçi satırı sonsuza kadar bloke edemesin diye süre dolunca satır
-- kendiliğinden yeniden uygun hâle gelir.
--
-- test: internal/service/order/refund_retry_integration_test.go#TestConcurrentCloseSendsSingleProviderCall
ALTER TABLE orders ADD COLUMN close_claimed_at TIMESTAMPTZ;

COMMENT ON COLUMN orders.close_claimed_at IS
    'Sağlayıcıda kapatma denemesinin sahiplenme damgası (kira). NULL = sahipsiz.';

-- `activation-reaper` ve `provider-refund-retry` adaylarını süzerken bu
-- sütuna da bakar; kısmi indeksin yükleminde YER ALMAZ ama sıralama/filtre
-- satır üstünde ucuzdur (aday kümesi zaten `provider_closed_at IS NULL`).
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE orders DROP COLUMN close_claimed_at;
-- +goose StatementEnd
