-- +goose Up
-- +goose StatementBegin

-- ═══════════════════════ MÜŞTERİ YORUMLARI ═══════════════════════
--
-- Akış: müşteri kendi panelinden yorum yazar → yönetici onaylar/reddeder →
-- YALNIZ onaylı yorumlar sitede görünür.
--
-- 🔴 TOHUM VERİSİ YOKTUR ve OLMAYACAKTIR. Bu migration tek bir örnek yorum
-- bile eklemez. Uydurma yorum, para yatırılan bir sitede yanıltıcı reklamdır;
-- yakalandığında geri kazanılamaz. Onaylı yorum yoksa sitedeki bölüm hiç
-- render edilmez (web/src/components/katalog/yorumlar.tsx).

CREATE TYPE review_status AS ENUM (
    'PENDING',   -- yazıldı, moderasyon bekliyor  → yöneticinin işi
    'APPROVED',  -- yayında
    'REJECTED'   -- yayımlanmadı ya da yayından kaldırıldı
);

CREATE TABLE reviews (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id   UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id     BIGINT NOT NULL REFERENCES users(id),

    -- Puan 1–5. SMALLINT yeterli; CHECK sınırı VERİTABANINDA da vardır çünkü
    -- DTO doğrulaması yalnız HTTP yolunu korur ve bir gün başka bir yol
    -- (CLI, içe aktarma, veri düzeltme betiği) satır yazabilir.
    rating      SMALLINT NOT NULL
                CONSTRAINT review_rating_range CHECK (rating BETWEEN 1 AND 5),

    -- 10–1000 KARAKTER (bayt değil): char_length() karakter sayar ve DTO
    -- doğrulaması da rune sayar. İki taraf aynı şeyi ölçmeli, yoksa tamamı
    -- Türkçe bir yorum sunucuda geçip veritabanında düşer.
    body        TEXT NOT NULL
                CONSTRAINT review_body_len
                CHECK (char_length(body) BETWEEN 10 AND 1000),

    status      review_status NOT NULL DEFAULT 'PENDING',

    -- Red gerekçesi kullanıcıya gösterilir: "reddedildi" deyip sebebini
    -- söylememek, kullanıcıyı aynı yorumu tekrar yazmaya iter.
    rejection_reason    TEXT NOT NULL DEFAULT '',

    -- Kararı VEREN yönetici. Hesabı silinirse karar kaydı KALIR
    -- (ON DELETE SET NULL): moderasyon geçmişinin yarısını kaybetmek,
    -- kalan yarısını yanıltıcı yapar.
    reviewed_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at         TIMESTAMPTZ,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Zaman damgası durumla TUTARLI olmalı (orders/tickets ile aynı kalıp):
    -- karar verilmişse ne zaman verildiği de bilinmeli.
    CONSTRAINT review_decided_has_time CHECK (
        (status = 'PENDING') = (reviewed_at IS NULL)),

    -- Gerekçe YALNIZ redde vardır. "Onaylandı ama gerekçesi 'küfür içeriyor'"
    -- gibi kendi kendisiyle çelişen satırlar oluşmasın.
    CONSTRAINT review_reason_only_when_rejected CHECK (
        (status = 'REJECTED' AND char_length(rejection_reason) > 0) OR
        (status <> 'REJECTED' AND rejection_reason = ''))
);

CREATE UNIQUE INDEX reviews_public_id_idx ON reviews (public_id);

-- ── KARAR: KULLANICI BAŞINA AYNI ANDA EN FAZLA BİR BEKLEYEN YORUM ──
--
-- Gerekçe: her bekleyen yorum bir yöneticiye iş üretir. Sınırsız olsaydı tek
-- bir hesap moderasyon kuyruğunu doldurup gerçek yorumları görünmez yapardı;
-- hız limiti bunu yavaşlatır ama ENGELLEMEZ (dakikada bir yorum da yeterlidir).
--
-- Sınır KISMİ BENZERSİZ İNDEKSTİR, uygulama katmanında "önce SELECT sonra
-- INSERT" kontrolü DEĞİL: iki eşzamanlı istek o kontrolü ikisi de geçer.
-- Burada ikinci INSERT 23505 ile düşer.
--
-- Kullanıcı yine de ZAMAN İÇİNDE birden fazla yorum yazabilir (karar verilen
-- her yorumdan sonra yenisini yazabilir) — bu bilinçlidir: hizmet değişir,
-- görüş de değişir. Sitede kullanıcının YALNIZ EN SON onaylı yorumu gösterilir
-- (queries/reviews.sql, ListApprovedReviews).
--
-- test: internal/service/review/review_integration_test.go#TestConcurrentSubmitsLeaveOnePending
CREATE UNIQUE INDEX reviews_one_pending_per_user_idx
    ON reviews (user_id) WHERE status = 'PENDING';

-- Kullanıcının kendi yorum listesi — en yeni üstte.
CREATE INDEX reviews_user_idx ON reviews (user_id, created_at DESC);

-- Yönetim kuyruğu: durum süzgeci + en eski bekleyen önce sıralanabilsin.
CREATE INDEX reviews_status_idx ON reviews (status, created_at DESC);

-- Sitedeki liste: onaylılar, yayımlanma sırasına göre.
CREATE INDEX reviews_approved_idx ON reviews (reviewed_at DESC)
    WHERE status = 'APPROVED';

CREATE TRIGGER reviews_set_updated_at
    BEFORE UPDATE ON reviews FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose StatementBegin
-- ── DURUM MAKİNESİ KORUMASI ──
--
-- Geçişler `internal/domain/review` içinde tanımlıdır; burada İKİNCİ KEZ
-- zorlanır (orders/deposits/tickets ile aynı gerekçe): kodda bir yol durum
-- alanına doğrudan yazarsa veritabanı son sözü söyler.
--
-- İzinli geçişler:
--   PENDING  → APPROVED | REJECTED
--   APPROVED → REJECTED            (YAYINDAN KALDIRMA)
--
-- 🔴 APPROVED → REJECTED oku BİLEREK VARDIR. Onayı tümüyle terminal yapmak
-- daha "temiz" görünür ama yanlışlıkla onaylanmış ya da sonradan hakaret
-- içerdiği anlaşılan bir yorumu siteden indirmenin HİÇBİR YOLU kalmazdı.
-- Gerekçe yine zorunludur (review_reason_only_when_rejected).
--
-- 🔴 REJECTED'DAN ÇIKIŞ YOKTUR. Reddedilen bir yorumu geri getirmek, kullanıcının
-- yazmadığı bir metni yayımlama riskini açar; kullanıcı isterse yeni bir yorum
-- yazar (bekleyen yorum sınırı buna izin verir).
--
-- test: internal/service/review/review_integration_test.go#TestInvalidReviewTransitionIsRejectedByDB
CREATE OR REPLACE FUNCTION reviews_guard_transition() RETURNS trigger AS $$
BEGIN
    IF OLD.status = NEW.status THEN
        RETURN NEW;
    END IF;

    IF NOT (
        (OLD.status = 'PENDING'  AND NEW.status IN ('APPROVED', 'REJECTED')) OR
        (OLD.status = 'APPROVED' AND NEW.status = 'REJECTED')
    ) THEN
        RAISE EXCEPTION 'gecersiz yorum gecisi: % -> %', OLD.status, NEW.status;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER reviews_guard_transition_trg
    BEFORE UPDATE ON reviews FOR EACH ROW EXECUTE FUNCTION reviews_guard_transition();
-- +goose StatementEnd

-- +goose StatementBegin
-- ── METİN DEĞİŞTİRİLEMEZ ──
--
-- KARAR: onaylanmış bir yorumun metni SONRADAN DEĞİŞTİRİLEMEZ; puanı da öyle.
--
-- Gerekçe: "onaylandı" bir yöneticinin O METNE verdiği karardır. Metin sonradan
-- değişebilseydi, kullanıcı nazik bir yorum yazdırıp onaylattıktan sonra içeriği
-- reklama ya da hakarete çevirebilirdi ve hiçbir yerde iz kalmazdı. Kullanıcı
-- fikrini değiştirirse YENİ bir yorum yazar ve o yorum yeniden onaya girer.
--
-- Kısıt BEKLEYEN yorumlar için de geçerlidir: uygulamada güncelleme uç noktası
-- yoktur, dolayısıyla bu tetikleyici yalnız "gelecekte biri eklerse" kapısıdır.
--
-- test: internal/service/review/review_integration_test.go#TestReviewBodyIsImmutable
CREATE OR REPLACE FUNCTION reviews_body_is_immutable() RETURNS trigger AS $$
BEGIN
    IF NEW.body <> OLD.body OR NEW.rating <> OLD.rating THEN
        RAISE EXCEPTION 'yorum metni ve puani degistirilemez (yeni yorum yazilmali)';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER reviews_body_is_immutable_trg
    BEFORE UPDATE ON reviews FOR EACH ROW EXECUTE FUNCTION reviews_body_is_immutable();

-- ── İZİNLER ──
--
-- İzinler VERİDİR (00002_seed_rbac.sql ile aynı kalıp): yeni yetki eklemek
-- satır eklemektir. Okuma ile moderasyon AYRI izinlerdir — destek personelinin
-- kuyruğu görmesi, yayına karar verebilmesi anlamına gelmez.
INSERT INTO permissions (code, description) VALUES
    ('reviews:read',     'Tüm müşteri yorumlarını görüntüleme'),
    ('reviews:moderate', 'Müşteri yorumlarını onaylama/reddetme')
ON CONFLICT (code) DO NOTHING;

-- admin rolü tüm izinlere sahiptir (00002 ile aynı kural).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.name = 'admin' AND p.code IN ('reviews:read', 'reviews:moderate')
ON CONFLICT DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions WHERE permission_id IN
    (SELECT id FROM permissions WHERE code IN ('reviews:read', 'reviews:moderate'));
DELETE FROM permissions WHERE code IN ('reviews:read', 'reviews:moderate');

DROP TABLE IF EXISTS reviews;
DROP FUNCTION IF EXISTS reviews_guard_transition();
DROP FUNCTION IF EXISTS reviews_body_is_immutable();
DROP TYPE IF EXISTS review_status;
-- +goose StatementEnd
