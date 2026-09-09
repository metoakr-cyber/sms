-- +goose Up
-- +goose StatementBegin

-- ═══════════════════════ SİPARİŞ DURUMU: 'ACTIVE' ═══════════════════════
--
-- Kiralık numaranın "teslim edildi ama dönemi bitmedi" hâli.
--
-- NEDEN AYRI BİR DEĞER: bugüne kadar 'PENDING' hem "kod bekleniyor" hem de
-- "iade edilebilir" demekti ve para işleri (order-expirer, CanUserCancel,
-- ListRefundRetryOrders) tam olarak bu değere bakıyordu. Kiralığı 'PENDING'de
-- bırakmak, o işlerin HER BİRİNE ayrı bir kiralık istisnası eklemek demekti;
-- birini atlamak 30 gün kullanılmış bir kiralığın tam iadesiyle sonuçlanırdı.
-- Yeni bir değerde atlamak GÜVENLİ tarafa düşer: iade sorgularının hiçbiri
-- 'ACTIVE' görmez, dolayısıyla yanlışlıkla iade YAZAMAZ.
-- test: internal/service/order/rental_integration_test.go#TestRentalNeverAutoRefundsAtEndOfPeriod
--
-- 🔴 BU MIGRATION TEK BAŞINA DURUYOR. PostgreSQL'de bir enum'a eklenen değer
-- EKLENDİĞİ TRANSACTION İÇİNDE KULLANILAMAZ; goose her dosyayı kendi
-- transaction'ında koşturur. Kısmi indeks ve CHECK kısıtı gibi 'ACTIVE'
-- değerini KULLANAN her şey bir sonraki dosyaya (00015) ayrıldı.
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'ACTIVE' AFTER 'PENDING';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 🔴 GERİ ALINAMAZ — ve bu bilinçli bir istisnadır.
--
-- PostgreSQL bir enum değerini DÜŞÜREMEZ (`ALTER TYPE ... DROP VALUE` yoktur).
-- Dürüst tek alternatif tipi yeniden yaratmaktı: orders.status'u TEXT'e çevir,
-- eski tipi düşür, yeniden yarat, geri çevir. O yol tabloyu tümüyle yeniden
-- yazar, tüm bağımlı görünüm/indeks/kısıtları düşürür ve bir GERİ ALMA
-- adımında yapılabilecek en riskli iştir — kaybedilen şey bir kolon değil,
-- para tablosunun kendisidir.
--
-- Kullanılmayan bir enum etiketi bırakmak zararsızdır: hiçbir satır onu
-- taşımaz (00015'in Down'ı ACTIVE satırları temizler), sorgular onu üretmez.
-- Bu adım bilerek BOŞTUR ve gerekçesi yukarıdadır.
SELECT 1;
-- +goose StatementEnd
