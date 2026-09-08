-- +goose Up
-- Bakiye yükleme talebinde İDEMPOTENS.
--
-- 🔴 SORUN: kullanıcı 500 ₺ havale eder, formu doldurur, mobil ağda gönderir.
-- İstek sunucuya ULAŞIR ve talep yazılır ama yanıt kaybolur (tünel, zaman
-- aşımı, uygulamayı arka plana alma). Kullanıcı hata görür ve tekrar basar.
-- Sonuç: aynı havale için İKİ bekleyen talep. Yönetici ikisini de onaylarsa
-- 500 ₺'lik havaleye 1000 ₺ yazılır.
--
-- Defterin idempotency anahtarı bunu ENGELLEMEZ: anahtar `deposit:{public_id}`
-- üzerinden türüyor ve iki farklı talebin iki farklı public_id'si var.
-- Koruma TALEP oluşturma anında olmalı.
--
-- KISMİ TEKİL İNDEKS: anahtar NULL olabilir (eski kayıtlar ve anahtar
-- göndermeyen istemciler), ama verildiğinde kullanıcı başına tekildir.
-- Anahtar kullanıcı kapsamına alınır — iki kullanıcının aynı UUID'yi
-- üretmesi çakışma değil, ayrı işlemdir.
ALTER TABLE deposits ADD COLUMN idempotency_key TEXT;

CREATE UNIQUE INDEX deposits_idempotency_uniq
    ON deposits (user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS deposits_idempotency_uniq;
ALTER TABLE deposits DROP COLUMN IF EXISTS idempotency_key;
