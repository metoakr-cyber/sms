-- +goose Up
-- +goose StatementBegin

-- ═══════════════════ KİRALIK YAŞAM DÖNGÜSÜ ═══════════════════
--
-- Kiralıkta satılan şey KOD DEĞİL, SÜREDİR. Bu tek cümle iki para kuralını
-- doğuruyor ve ikisinin de şemada karşılığı olması gerekiyor:
--
--   1. İlk SMS dönemi bitirmez  → sipariş 'ACTIVE'e geçer, 'COMPLETED'a değil.
--   2. Dönem sonunda iade yoktur → süre teslim edildi; sağlayıcının ücretsiz
--      iptal penceresi (≈20 dk) 1. günde kapandığı için o parayı geri almanın
--      yolu da yoktur.

-- ── ÜRÜN TÜRÜ ANLIK GÖRÜNTÜSÜ ──
--
-- Ürün türü sipariş satırına KOPYALANIR; `product_id` üzerinden JOIN ile
-- okunmaz. Gerekçe 00008'in kendi yorumudur: katalog satırları senkronda
-- silinebilir ve `product_id` zaten `ON DELETE SET NULL`. "İade mi Finish mi"
-- kararını silinebilir bir satıra bağlamak, ürün kataloğundaki bir temizliğin
-- 30 günlük bir kiralığı 20 dakikalık aktivasyon gibi kapatmasıyla biter.
ALTER TABLE orders
    ADD COLUMN product_kind product_kind NOT NULL DEFAULT 'SMS_ACTIVATION';

-- 🔴 GERİ DOLDURMA — `DEFAULT` TEK BAŞINA YANLIŞ CEVABI VERİR.
--
-- Kolonu varsayılanla eklemek, VAR OLAN her kiralığı 'SMS_ACTIVATION' ilan
-- eder. Sonucu rapor değil PARA: `ListExpiredPendingOrders` o satırı görür,
-- `Expire`in ikinci savunma hattı da `product_kind`e baktığı için geçirir ve
-- 30 gün kullanılmış bir kiralığa TAM İADE yazılır.
--
-- Bugün etkisiz görünüyor (`rental_details` boş) ama etkisiz KALMIYOR: 00015
-- geri alınıp yeniden uygulandığında (Down kolonu düşürür, Up varsayılanla
-- geri koyar) canlı kiralıklar sessizce aktivasyona döner. Kiralığın kalıcı
-- kanıtı `rental_details.order_id`dir; kimlik oradan geri okunur.
UPDATE orders o SET product_kind = 'SMS_RENTAL'
WHERE EXISTS (SELECT 1 FROM rental_details rd WHERE rd.order_id = o.id);

-- ── İADE PENCERESİNİN ÜST SINIRI ──
--
-- Aktivasyonda üst sınır ÖRTÜKTÜ: `expires_at` ~20 dakikaydı ve tesadüfen
-- sağlayıcının ücretsiz iptal penceresiyle çakışıyordu. Kiralıkta `expires_at`
-- 30 gün olabilir; örtük sınır kayboldu.
--
-- NULL = ek bir üst sınır yok (aktivasyon). Aktivasyon davranışı bu migration
-- ile HİÇ DEĞİŞMEZ; kolon yalnız kiralıkta doldurulur.
ALTER TABLE orders ADD COLUMN refundable_until TIMESTAMPTZ;

-- 🔴 KİRALIKTA NULL BIRAKILAMAZ — ve geri doldurma `expires_at` DEĞİLDİR.
--
-- `CanUserCancel` üst sınırı yalnız kolon doluyken uygular; NULL "sınır yok"
-- demektir. Bir kiralık satırında NULL kalırsa 29. günde yapılan iptal TAM
-- İADE yazar (kullanıcı numarayı bir ay kullanır, parasını geri alır).
--
-- Geri doldurmada `expires_at` yazmak o sızıntının ta kendisidir: 30 günlük
-- bir kiralıkta "iade penceresi 30 gün" demek olurdu. Doğru değer satın alma
-- anındaki penceredir — `created_at + 15 dakika` (service/order/service.go
-- rentalRefundWindow) — ve numaranın kendi ömrünü aşamaz. Geçmiş satırlarda
-- bu değer zaten geçmiştedir, yani pencere kapalıdır: eksik veri "sınır yok"
-- değil "izin yok" tarafına düşer.
UPDATE orders SET refundable_until = least(created_at + interval '15 minutes', expires_at)
WHERE product_kind = 'SMS_RENTAL' AND refundable_until IS NULL;

-- 'ACTIVE' yalnız kiralıkta anlamlıdır: bir aktivasyon siparişi o duruma
-- düşerse iade işleri onu görmez ve kullanıcının parası sessizce askıda kalır.
-- Kısıt bunu veritabanı düzeyinde tutar.
-- test: internal/service/order/rental_integration_test.go#TestActivationCannotEnterActiveStatus
ALTER TABLE orders ADD CONSTRAINT order_active_is_rental
    CHECK (status <> 'ACTIVE' OR product_kind = 'SMS_RENTAL');

-- 🔴 PARA GUARD'ININ GİRDİSİ DE SAVUNULUR.
--
-- Go tarafındaki kontrol (`CanUserCancel`, NULL → iptal yok) KARARI savunur;
-- bu kısıt VERİYİ savunur. İkisi ayrı hata sınıflarını yakalar: kod yolu
-- atlanabilir (admin kaydı, veri taşıma, ileride eklenecek `prolong`), kısıt
-- atlanamaz. `order_active_is_rental` durumu veritabanında savunuyordu ama
-- iade kararının GİRDİSİ savunmasız kalmıştı.
-- test: internal/service/order/rental_integration_test.go#TestRentalRowCannotLoseRefundWindow
ALTER TABLE orders ADD CONSTRAINT order_rental_has_refund_window
    CHECK (product_kind <> 'SMS_RENTAL' OR refundable_until IS NOT NULL);

-- ── YOKLAMA DÖNÜŞÜMÜ ──
--
-- 🔴 SABİT SIRALAMA + DEĞİŞMEYEN KÜME = AÇLIK.
--
-- `rental-poller` bir turda `LIMIT 100` satır alır. Aktivasyonda `created_at`
-- sıralaması güvenlidir çünkü satırlar ~20 dakikada kümeden çıkar ve sıra
-- kendiliğinden ilerler. Kiralık 24–4320 SAAT kümede kalır: 101. kiralık,
-- ilk 100'ün hiçbiri bitmeden HİÇ yoklanmaz — webhook kaybolursa (imzasız,
-- teslim garantisi yok) kullanıcı dönem boyunca tek mesaj görmez.
--
-- Kolon o turu dönüşümlü hâle getirir: en uzun süredir yoklanmamış satır
-- (ve hiç yoklanmamışlar) önce gelir.
-- test: internal/service/order/rental_integration_test.go#TestRentalPollNeverStarvesNewestRental
ALTER TABLE orders ADD COLUMN last_polled_at TIMESTAMPTZ;

-- İşçi sorgusu: `rental-closer` (dönemi biten) ve `rental-poller` (canlı olan)
-- aynı kısmi indeksi kullanır — ikisi de kiralık + (PENDING|ACTIVE) kümesini
-- `expires_at`e göre tarar.
CREATE INDEX orders_rental_lifecycle_idx ON orders (expires_at)
    WHERE product_kind = 'SMS_RENTAL' AND status IN ('PENDING', 'ACTIVE');

-- Yoklama turunun kendi sıralaması: aynı kısmi küme, farklı eksen.
-- `NULLS FIRST` yeni satırı turun başına alır — hiç yoklanmamış bir kiralık
-- ilk turda görülür.
-- test: internal/service/order/rental_integration_test.go#TestRentalPollNeverStarvesNewestRental
CREATE INDEX orders_rental_poll_idx ON orders (last_polled_at NULLS FIRST)
    WHERE product_kind = 'SMS_RENTAL' AND status IN ('PENDING', 'ACTIVE');
-- +goose StatementEnd

-- +goose StatementBegin
-- ── DURUM MAKİNESİ KORUMASI (00008'in güncellenmiş hâli) ──
--
-- İKİ YENİ OK:  PENDING → ACTIVE   (kiralığa ilk mesaj geldi)
--               ACTIVE  → COMPLETED (kira dönemi bitti)
--
-- 🔴 'ACTIVE' → 'CANCELLED' BİLEREK YOKTUR. İlk SMS geldikten sonra iade yolu
-- kapalıdır (sağlayıcı da `OTP_RECEIVED` ile reddeder). Oku tabloya koymamak,
-- ileride birinin yanlışlıkla iade yolunu çağırmasını domain katmanının
-- ötesinde, veritabanında da durdurur.
--
-- 🔴 TERMİNAL KORUMASI GEVŞETİLMEDİ: 'COMPLETED' ve 'REFUNDED' hâlâ çıkışsız.
-- O satır, order-expirer ile DeliverMessages arasındaki yarışın para kaybına
-- dönüşmesini durduran şeydir; kiralık için gevşetilseydi aktivasyon tarafında
-- da "tamamlanmış siparişe iade" kapısı açılırdı.
-- test: internal/service/order/order_integration_test.go#TestTerminalOrderCannotChangeStatus
-- test: internal/service/order/rental_integration_test.go#TestActiveRentalCannotBeCancelledByDB
CREATE OR REPLACE FUNCTION orders_guard_transition() RETURNS trigger AS $$
BEGIN
    IF OLD.status = NEW.status THEN
        RETURN NEW;
    END IF;

    IF OLD.status IN ('COMPLETED', 'REFUNDED') THEN
        RAISE EXCEPTION
            'gecersiz siparis gecisi: % -> % (terminal durumdan cikis yok)',
            OLD.status, NEW.status;
    END IF;

    IF NOT (
        (OLD.status = 'PENDING'   AND NEW.status IN ('ACTIVE', 'COMPLETED', 'CANCELLED', 'FAILED')) OR
        (OLD.status = 'ACTIVE'    AND NEW.status = 'COMPLETED') OR
        (OLD.status = 'CANCELLED' AND NEW.status = 'REFUNDED') OR
        (OLD.status = 'FAILED'    AND NEW.status = 'REFUNDED')
    ) THEN
        RAISE EXCEPTION 'gecersiz siparis gecisi: % -> %', OLD.status, NEW.status;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- ACTIVE satırlar önce terminale çekilir: 00014'ün Down'ı enum etiketini
-- düşüremediği için etiket kalır, ama satır kalmaz.
UPDATE orders SET status = 'COMPLETED', completed_at = coalesce(completed_at, now())
WHERE status = 'ACTIVE';

DROP INDEX IF EXISTS orders_rental_poll_idx;
DROP INDEX IF EXISTS orders_rental_lifecycle_idx;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS order_rental_has_refund_window;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS order_active_is_rental;
ALTER TABLE orders DROP COLUMN IF EXISTS last_polled_at;
ALTER TABLE orders DROP COLUMN IF EXISTS refundable_until;
ALTER TABLE orders DROP COLUMN IF EXISTS product_kind;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION orders_guard_transition() RETURNS trigger AS $$
BEGIN
    IF OLD.status = NEW.status THEN
        RETURN NEW;
    END IF;

    IF OLD.status IN ('COMPLETED', 'REFUNDED') THEN
        RAISE EXCEPTION
            'gecersiz siparis gecisi: % -> % (terminal durumdan cikis yok)',
            OLD.status, NEW.status;
    END IF;

    IF NOT (
        (OLD.status = 'PENDING'   AND NEW.status IN ('COMPLETED', 'CANCELLED', 'FAILED')) OR
        (OLD.status = 'CANCELLED' AND NEW.status = 'REFUNDED') OR
        (OLD.status = 'FAILED'    AND NEW.status = 'REFUNDED')
    ) THEN
        RAISE EXCEPTION 'gecersiz siparis gecisi: % -> %', OLD.status, NEW.status;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
