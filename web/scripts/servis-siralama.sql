-- ─────────────────────────────────────────────────────────────────────────────
-- SERVİS POPÜLERLİK SIRALAMASI  (services.sort_order)
--
-- NE İŞE YARAR
--   Servis ızgaralarında (`/`, `/fiyatlar`, `/servisler`, panel → numara al)
--   hangi servisin önce görüneceğini belirler. Sorgular her yerde
--   `ORDER BY s.sort_order, s.name` kullanır: küçük değer önce gelir, eşit
--   değerler kendi aralarında ADA GÖRE alfabetik dizilir.
--
-- NEDEN GEREKLİ
--   `sort_order` alanını katalog senkronu dolduruyor
--   (api/internal/service/catalog/sync.go → `SortOrder: int32(i * 10)`), yani
--   değer SAĞLAYICININ liste sırasıdır — popülerlikle hiçbir ilgisi yoktur.
--   Sonuç: kullanıcı ilk ekranda Whatnot / Adobe / Baidu görüyor, WhatsApp
--   listenin ortasında kalıyordu.
--
--   Bu yüzden ÖNCE tüm tablo tek bir yüksek değere (100000) düzleştirilir,
--   SONRA aşağıdaki 39 servise 10'ar adımla popülerlik sırası verilir.
--   Düzleştirme olmadan yalnız 39 satırı güncellemek YETMEZ: geri kalan ~770
--   servis 0–8090 arasındaki sağlayıcı sırasını korur ve popüler bloğun
--   arasına karışır.
--
-- NE ZAMAN ÇALIŞTIRILIR
--   1. İlk kurulumdan sonra (migration'lar + katalog senkronu bittikten sonra).
--   2. HER KATALOG SENKRONUNDAN SONRA. Senkron yeni servis eklerse o servis
--      `sort_order` varsayılanını (bkz. 00005_catalog.sql) veya sağlayıcı
--      sırasını alır ve popüler bloğun önüne geçebilir. Bu dosyayı yeniden
--      çalıştırmak durumu düzeltir.
--   3. Popüler listesi değiştiğinde (aşağıdaki VALUES bloğunu düzenleyip
--      yeniden çalıştırın).
--
--   İSTEDİĞİNİZ KADAR ÇALIŞTIRILABİLİR (idempotent): dosya her seferinde
--   tabloyu sıfırlayıp yeniden yazdığı için sonuç hep aynıdır.
--
-- NASIL ÇALIŞTIRILIR
--   Geliştirme:
--     docker exec -i smsplatform-dev-postgres-1 \
--       psql -U smsplatform -d smsplatform -v ON_ERROR_STOP=1 \
--       < web/scripts/servis-siralama.sql
--   Üretim: aynı komut, üretim bağlantı bilgileriyle.
--
-- NOTLAR
--   • Sıra numaraları 10'ar artar; araya servis eklemek için her yuvada 9 boş
--     değer vardır (örn. WhatsApp ile Instagram arasına 45 verilebilir).
--   • Listede olmayan servis 100000 alır ve ada göre alfabetik sıralanır.
--   • Burada bulunmayan bir kod sessizce yok sayılır (UPDATE ... FROM eşleşmez).
--     Kontrol sorgusu dosyanın sonundadır.
--   • `updated_at` bilerek güncellenmez: bu dosya sıralama düzeltir, servis
--     içeriğini değiştirmez.
-- ─────────────────────────────────────────────────────────────────────────────

BEGIN;

-- 1) DÜZLEŞTİR — herkes eşit; ikincil anahtar `name` alfabetik sıralasın.
UPDATE services SET sort_order = 100000 WHERE sort_order <> 100000;

-- 2) POPÜLER SERVİSLER — sıra referans listesinin sırasıdır.
--    (kod, sıra, açıklama = DB'deki `name`, yalnız okunabilirlik için)
UPDATE services s
SET    sort_order = p.sira
FROM (VALUES
    ('go',   10),   -- Google,youtube,Gmail
    ('fb',   20),   -- facebook
    ('tw',   30),   -- Twitter/X
    ('wa',   40),   -- Whatsapp
    ('ig',   50),   -- Instagram+Threads
    ('ds',   60),   -- Discord
    ('tg',   70),   -- Telegram
    ('oi',   80),   -- Tinder
    ('ts',   90),   -- PayPal
    ('nf',  100),   -- Netflix
    ('mm',  110),   -- Microsoft
    ('tn',  120),   -- LinkedIN
    ('yi',  130),   -- Yemeksepeti
    ('ul',  140),   -- Getir
    ('am',  150),   -- Amazon
    ('vk',  160),   -- vk.com
    ('bkd', 170),   -- Sahibinden
    ('pr',  180),   -- Trendyol
    ('gx',  190),   -- Hepsiburadacom
    ('dh',  200),   -- eBay
    ('vm',  210),   -- OkCupid
    ('brg', 220),   -- Letgo
    ('hb',  230),   -- Twitch
    ('rc',  240),   -- Skype
    ('pm',  250),   -- AOL  (DİKKAT: 'aol' kodu Paysera'ya aittir, AOL'a değil)
    ('ya',  260),   -- Yandex
    ('mb',  270),   -- Yahoo
    ('ma',  280),   -- Mail.ru
    ('dp',  290),   -- ProtonMail
    ('bw',  300),   -- Signal
    ('lf',  310),   -- TikTok/Douyin
    ('mt',  320),   -- Steam
    ('wx',  330),   -- Apple
    ('dr',  340),   -- OpenAI
    ('uv',  350),   -- BinBin
    ('fu',  360),   -- Snapchat
    ('bl',  370),   -- BIGO LIVE
    ('me',  380),   -- Line messenger
    ('rr',  390)    -- Wolt
) AS p(kod, sira)
WHERE s.code = p.kod
  AND s.sort_order <> p.sira;

COMMIT;

-- ── DOĞRULAMA ────────────────────────────────────────────────────────────────
-- Beklenen: 39 satır, sıra 10..390, "eksik" sütunu boş.
\echo 'Popüler blok (beklenen 39 satır):'
SELECT sort_order, code, name FROM services
WHERE sort_order < 100000 ORDER BY sort_order;

\echo 'Listede olup veritabaninda BULUNAMAYAN kodlar (bos olmali):'
SELECT p.kod
FROM (VALUES
    ('go'),('fb'),('tw'),('wa'),('ig'),('ds'),('tg'),('oi'),('ts'),('nf'),
    ('mm'),('tn'),('yi'),('ul'),('am'),('vk'),('bkd'),('pr'),('gx'),('dh'),
    ('vm'),('brg'),('hb'),('rc'),('pm'),('ya'),('mb'),('ma'),('dp'),('bw'),
    ('lf'),('mt'),('wx'),('dr'),('uv'),('fu'),('bl'),('me'),('rr')
) AS p(kod)
LEFT JOIN services s ON s.code = p.kod
WHERE s.code IS NULL;
