-- +goose Up
-- +goose StatementBegin

-- ═══════════════════════════ SİPARİŞLER ═══════════════════════════
--
-- Ürünün kalbi: paranın dış dünyayla buluştuğu yer.
-- Durum makinesi docs/design.md §7.1'de tanımlıdır ve BURADA da zorlanır —
-- kodda bir hata olursa veritabanı ikinci savunma hattıdır.
--
-- test: internal/service/order/order_integration_test.go#TestTerminalOrderCannotChangeStatus

CREATE TYPE order_status AS ENUM (
    'PENDING',    -- numara alındı, kod bekleniyor
    'COMPLETED',  -- kod geldi (terminal)
    'CANCELLED',  -- kullanıcı iptali veya süre doldu
    'FAILED',     -- sipariş oluştuktan SONRA kalıcı sağlayıcı hatası
    'REFUNDED'    -- iade işlendi (terminal)
);

-- SAĞLAYICI İADE DURUMU — sipariş durumundan AYRI bir eksendir (FR-406b).
--
-- Kullanıcıya iade KOŞULSUZ ve ANINDA yapılır. Sağlayıcıdan parayı geri almak
-- ayrı, yavaş ve bazen başarısız bir iştir. İkisini tek alanda tutmak,
-- kullanıcıyı sağlayıcının yavaşlığına mahkûm ederdi: sağlayıcı 20 dakika
-- oyalarsa kullanıcı 20 dakika parasız kalırdı.
CREATE TYPE refund_status AS ENUM (
    'NOT_APPLICABLE',   -- iade söz konusu değil (kod geldi, Finish edildi)
    'PENDING',          -- talep edilecek
    'RETRY_SCHEDULED',  -- sağlayıcı "henüz olmaz" dedi (EARLY_CANCEL_DENIED)
    'REFUNDED',         -- sağlayıcı iade etti
    'DENIED'            -- sağlayıcı reddetti → bizim zararımız, gider kalemi
);

CREATE TABLE orders (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id         UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id           BIGINT NOT NULL REFERENCES users(id),

    -- ── SAĞLAYICI TARAFI ──
    provider_id       BIGINT NOT NULL REFERENCES providers(id),

    -- remote_order_id metin, provider_activation_id sayısal.
    --
    -- İKİSİ DE VAR çünkü sağlayıcı iki farklı yerde iki farklı tip kullanıyor:
    -- satın alma yanıtındaki `id` bir sayı, webhook'taki `activationId` de sayı
    -- ama örneklerde string olarak da görülüyor. Metin sürüm kanonik kimliktir;
    -- sayısal sürüm webhook korelasyonu için indekslenir.
    remote_order_id       TEXT   NOT NULL,
    provider_activation_id BIGINT,
    phone_number      TEXT   NOT NULL,
    verification_type verification_type NOT NULL DEFAULT 'sms',

    -- ── KATALOG ANLIK GÖRÜNTÜSÜ ──
    --
    -- product_id'ye BAĞIMLI KALINMAZ: ürün, servis ve ülke satırları katalog
    -- senkronunda silinebilir (ON DELETE CASCADE zincirleri var). Sipariş
    -- geçmişi kataloğun bugünkü hâline bağlı olamaz — bir yıl sonra
    -- "hangi servis için almıştım?" sorusunun cevabı burada durmalı.
    product_id        BIGINT REFERENCES products(id) ON DELETE SET NULL,
    service_code      TEXT NOT NULL,
    service_name      TEXT NOT NULL,
    country_iso2      TEXT NOT NULL,
    country_name      TEXT NOT NULL,
    phone_code        TEXT NOT NULL DEFAULT '',

    -- ── PARA ──
    --
    -- price_paid_minor DEĞİŞMEZ. İade her zaman bu tutar kadardır; anlık kurla
    -- yeniden hesaplanmaz (docs/design.md §7.1).
    --
    -- quote_id ZORUNLU BİR İZDİR: yetim provizyon sorgusu (tüketilmiş ama
    -- siparişi olmayan teklifler) buna dayanır (FR-408).
    quote_id          BIGINT REFERENCES price_quotes(id),
    price_paid_minor  BIGINT NOT NULL
                      CONSTRAINT order_price_positive CHECK (price_paid_minor > 0),

    -- MİKRO-birim (6 hane), kuruş DEĞİL. Sağlayıcı fiyatları 4 ondalıklı ve
    -- MaxPrice.minimum = 0.0067 sente sığmaz (ADR-026).
    cost_micro        BIGINT NOT NULL,
    fx_rate           NUMERIC(18,8) NOT NULL,

    -- ── DURUM ──
    status            order_status NOT NULL DEFAULT 'PENDING',

    -- ── ZAMAN ──
    --
    -- expires_at SAĞLAYICI YANITINDAKİ `expiredAt`ten gelir (eksi güvenlik
    -- payı), koda gömülmez (FR-405).
    expires_at        TIMESTAMPTZ NOT NULL,

    -- İptal butonu ilk ~120 saniye pasiftir (FR-416). Süre SUNUCUDAN gelir;
    -- istemcinin saatine güvenilmez. Değer sağlayıcının minActivationTime
    -- bilgisinden türer — sağlayıcı o süreden önce iptali reddeder.
    -- test: internal/service/order/order_integration_test.go#TestCancelTooEarlyIsRejected
    cancellable_at    TIMESTAMPTZ NOT NULL,

    completed_at      TIMESTAMPTZ,
    cancelled_at      TIMESTAMPTZ,
    refunded_at       TIMESTAMPTZ,

    -- ── SAĞLAYICIDA KAPATMA (FR-412) ──
    --
    -- NULL = sağlayıcıda kapatılmadı. `activation-reaper` bu alanın NULL olduğu
    -- TERMİNAL siparişleri tarar. Kapatma BAŞARILI OLMADAN buraya yazılmaz;
    -- yazılırsa reaper o siparişi bir daha hiç denemez ve aktivasyon sağlayıcıda
    -- açık kalır.
    provider_closed_at TIMESTAMPTZ,
    close_attempts     INT NOT NULL DEFAULT 0,
    close_last_error   TEXT NOT NULL DEFAULT '',

    -- ── SAĞLAYICI İADESİ (FR-406b) ──
    provider_refund_status refund_status NOT NULL DEFAULT 'NOT_APPLICABLE',
    provider_refund_amount_minor BIGINT NOT NULL DEFAULT 0,
    refund_attempts        INT NOT NULL DEFAULT 0,
    refund_next_attempt_at TIMESTAMPTZ,

    cancel_reason     TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- ÇİFT SİPARİŞ DB DÜZEYİNDE İMKÂNSIZ (FR-402).
    -- Kodda bir yarış durumu kalsa bile aynı aktivasyon iki siparişe bağlanamaz.
    -- test: internal/service/order/order_integration_test.go#TestSameQuoteCannotBeBoughtTwice
    CONSTRAINT orders_remote_uniq UNIQUE (provider_id, remote_order_id),

    -- Zaman damgaları durumla TUTARLI olmalı. Kodda bir hata "COMPLETED ama
    -- completed_at boş" bırakırsa rapor ve iade mantığı sessizce yanlış çalışır.
    CONSTRAINT order_completed_has_time CHECK (
        (status <> 'COMPLETED') OR (completed_at IS NOT NULL)),
    CONSTRAINT order_cancelled_has_time CHECK (
        (status NOT IN ('CANCELLED', 'REFUNDED')) OR (cancelled_at IS NOT NULL)),
    CONSTRAINT order_refunded_has_time CHECK (
        (status <> 'REFUNDED') OR (refunded_at IS NOT NULL))
);

CREATE UNIQUE INDEX orders_public_id_idx ON orders (public_id);

-- Webhook YALNIZ activationId taşır; korelasyon bu indeksle yapılır.
-- Kısmi UNIQUE: sağlayıcı sayısal kimlik vermezse (NULL) çakışma olmaz.
CREATE UNIQUE INDEX orders_activation_uniq ON orders (provider_id, provider_activation_id)
    WHERE provider_activation_id IS NOT NULL;

-- Kullanıcının sipariş geçmişi — en yeni önce.
CREATE INDEX orders_user_idx ON orders (user_id, created_at DESC);

-- İşçilerin taradığı KISMİ indeksler. Tam tablo taraması yerine yalnız
-- ilgilenilen satırlar: sipariş sayısı büyüdükçe fark açılır.
CREATE INDEX orders_pending_expiry_idx ON orders (status, expires_at)
    WHERE status = 'PENDING';
CREATE INDEX orders_unclosed_idx ON orders (updated_at)
    WHERE provider_closed_at IS NULL AND status IN ('COMPLETED', 'CANCELLED', 'FAILED', 'REFUNDED');
CREATE INDEX orders_refund_retry_idx ON orders (refund_next_attempt_at)
    WHERE provider_refund_status IN ('PENDING', 'RETRY_SCHEDULED');

CREATE TRIGGER orders_set_updated_at
    BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- +goose StatementBegin
-- ── DURUM MAKİNESİ KORUMASI ──
--
-- Terminal durumdan çıkış YOKTUR. Bu kural koda yazıldı (domain/order) ama
-- veritabanında da zorlanır: arka plan işleri (order-expirer, activation-reaper)
-- yarış durumunda tamamlanmış bir siparişe iade yazabilirdi.
--
-- test: order_integration_test.go#TestTerminalOrderCannotChangeStatus
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

-- +goose StatementBegin
CREATE TRIGGER orders_guard_transition_trg
    BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION orders_guard_transition();

-- ═══════════════════════ SİPARİŞ MESAJLARI ═══════════════════════
--
-- Bir sipariş BİRDEN FAZLA mesaj alabilir (FR-415). SSE ilk koddan sonra
-- kapanmaz; ikinci doğrulama kodu da kullanıcıya ulaşmalıdır.
--
-- MESAJLAR KALICIDIR. Sağlayıcıdan geçmiş mesaj OKUNAMAZ: kapatılmış bir
-- aktivasyon için `409 ACTIVATION_NOT_ACTIVE` döner. Saklama politikamız
-- sağlayıcıya bağlanamaz.
CREATE TABLE order_messages (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id     BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,

    -- DEDUP ANAHTARI. Birincil kaynak sağlayıcının mesaj kimliğidir; gelmezse
    -- SHA-256(activationId + receivedAt + text) kullanılır. Aynı SMS webhook,
    -- yoklama ve yeniden gönderimlerle en az sekiz kez gelebilir; kullanıcıya
    -- sekiz kez göstermek kabul edilemez.
    provider_otp_id TEXT NOT NULL,

    -- Kod BOŞ OLABİLİR: 'call' tipi doğrulamada kod sesli okunur, ayrıca
    -- metinden kod ayrıştırılamayabilir. Boş kod bir hata DEĞİLDİR ve
    -- "kod gelmedi" anlamına GELMEZ; mesajın kendisi yine de gösterilir.
    code         TEXT NOT NULL DEFAULT '',
    body         TEXT NOT NULL DEFAULT '',
    sender       TEXT NOT NULL DEFAULT '',
    received_at  TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT order_messages_uniq UNIQUE (order_id, provider_otp_id)
);

CREATE INDEX order_messages_order_idx ON order_messages (order_id, received_at);

-- ═══════════════════════ KİRALIK (v1.1) ═══════════════════════
--
-- Şema BUGÜN eklenir, özellik v1.1'de gelir. Sonradan tablo eklemek kolaydır;
-- sonradan sipariş modelini kiralık kavramına uydurmak değildir.
CREATE TABLE rental_details (
    order_id       BIGINT PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
    duration_hours INT NOT NULL,
    renewed_count  INT NOT NULL DEFAULT 0,
    rental_ends_at TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ═══════════════════════ BAKİYE YÜKLEME YÖNTEMLERİ ═══════════════════════
--
-- Yönetici panelden yöntem ekler, düzenler, aktif/pasif yapar. Kod değişikliği
-- GEREKMEZ: yeni bir banka hesabı veya yeni bir USDT ağı eklemek için dağıtım
-- beklemek, operasyonu geliştiriciye bağımlı kılar.
CREATE TYPE deposit_method_kind AS ENUM ('BANK_TRANSFER', 'CRYPTO');

CREATE TABLE deposit_methods (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id      UUID NOT NULL DEFAULT gen_random_uuid(),
    code           TEXT NOT NULL UNIQUE,          -- 'havale-ziraat', 'usdt-trc20'
    kind           deposit_method_kind NOT NULL,
    name           TEXT NOT NULL,                 -- kullanıcıya görünen ad
    instructions   TEXT NOT NULL DEFAULT '',      -- Türkçe talimat metni

    -- Hesap bilgileri: BANK_TRANSFER → {iban, hesapAdi, banka}
    --                  CRYPTO        → {ag, cuzdanAdresi}
    --
    -- JSONB SEÇİLDİ çünkü bu alanlar yönteme göre TAMAMEN değişir ve hiçbiri
    -- fiyat/stok gibi SORGULANAN bir eksen değildir — design.md §4'ün
    -- "sorgulanan veri JSONB'ye konmaz" kuralını ihlal etmez.
    config         JSONB NOT NULL DEFAULT '{}',

    -- Sınırlar trd.md'den: en az 10,00 ₺, en çok 50.000,00 ₺.
    min_amount_minor BIGINT NOT NULL DEFAULT 1000
                     CONSTRAINT dm_min_range CHECK (min_amount_minor >= 0),
    max_amount_minor BIGINT NOT NULL DEFAULT 5000000
                     CONSTRAINT dm_max_range CHECK (max_amount_minor >= 0),

    -- VARSAYILAN PASİF: yeni eklenen bir yöntem, IBAN'ı doldurulmadan
    -- kullanıcıya görünmemeli. Boş IBAN'lı bir "aktif" yöntem, paranın
    -- hiçbir yere gitmemesi demektir.
    is_active      BOOLEAN NOT NULL DEFAULT false,
    sort_order     INT NOT NULL DEFAULT 100,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT dm_max_gte_min CHECK (max_amount_minor = 0 OR max_amount_minor >= min_amount_minor)
);

CREATE UNIQUE INDEX deposit_methods_public_id_idx ON deposit_methods (public_id);
CREATE INDEX deposit_methods_active_idx ON deposit_methods (sort_order) WHERE is_active;

CREATE TRIGGER deposit_methods_set_updated_at
    BEFORE UPDATE ON deposit_methods FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ═══════════════════════ BAKİYE YÜKLEME TALEPLERİ ═══════════════════════
CREATE TYPE deposit_status AS ENUM ('PENDING', 'COMPLETED', 'REJECTED', 'REFUNDED');

CREATE TABLE deposits (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id         UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id           BIGINT NOT NULL REFERENCES users(id),
    method_id         BIGINT REFERENCES deposit_methods(id) ON DELETE SET NULL,

    -- Yöntem adı ANLIK GÖRÜNTÜ olarak saklanır: yönetici yöntemi silse veya
    -- yeniden adlandırsa da geçmiş kayıt "hangi yolla yatırdım?" sorusunu
    -- cevaplayabilmeli.
    method_name       TEXT NOT NULL,

    -- amount_minor: kullanıcının BİLDİRDİĞİ tutar.
    -- credited_minor: bakiyeye GERÇEKTEN yazılan tutar.
    -- İkisi ayrıdır çünkü kripto yatırımlarda ağ ücreti sonrası gelen tutar
    -- farklı olabilir; tek alanda tutmak "bildirdiğim kadar yazılmamış"
    -- tartışmasını çözümsüz bırakır.
    amount_minor      BIGINT NOT NULL
                      CONSTRAINT deposit_amount_range
                      CHECK (amount_minor >= 1000 AND amount_minor <= 5000000),
    credited_minor    BIGINT NOT NULL DEFAULT 0
                      CONSTRAINT deposit_credited_non_negative CHECK (credited_minor >= 0),

    status            deposit_status NOT NULL DEFAULT 'PENDING',

    -- Kripto yatırımlar için işlem hash'i. Aynı hash ikinci kez bildirilemez
    -- (KK-501) — kısmi UNIQUE, çünkü havale yatırımlarında NULL'dur.
    tx_hash           TEXT,
    network           TEXT NOT NULL DEFAULT '',
    receipt_path      TEXT NOT NULL DEFAULT '',

    -- İKİ AYRI NOT ALANI.
    --
    -- user_note kullanıcıya GÖSTERİLİR; admin_note yalnız yöneticiler içindir.
    -- Tek alanda tutmak, bir yöneticinin iç değerlendirmesinin
    -- ("şüpheli hesap") kullanıcıya gösterilmesiyle sonuçlanır.
    user_note         TEXT NOT NULL DEFAULT '',
    admin_note        TEXT NOT NULL DEFAULT '',
    rejection_reason  TEXT NOT NULL DEFAULT '',

    reviewed_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT deposit_reviewed_has_time CHECK (
        (status = 'PENDING') OR (reviewed_at IS NOT NULL))
);

CREATE UNIQUE INDEX deposits_public_id_idx ON deposits (public_id);
CREATE INDEX deposits_user_idx ON deposits (user_id, created_at DESC);
CREATE INDEX deposits_pending_idx ON deposits (status, created_at DESC);
-- Aynı işlem hash'i iki kez bildirilemez (KK-501).
CREATE UNIQUE INDEX deposits_tx_hash_uniq ON deposits (tx_hash) WHERE tx_hash IS NOT NULL;

CREATE TRIGGER deposits_set_updated_at
    BEFORE UPDATE ON deposits FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Başlangıç yöntemleri — HEPSİ PASİF.
--
-- Bilgileri (IBAN, cüzdan adresi) yönetici panelden doldurup aktifleştirir.
-- Örnek IBAN yazmak, birinin onu gerçek sanıp para göndermesiyle sonuçlanır.
INSERT INTO deposit_methods (code, kind, name, instructions, config, is_active, sort_order)
VALUES
  ('havale-eft', 'BANK_TRANSFER', 'Banka Havalesi / EFT',
   'Aşağıdaki hesaba havale yaptıktan sonra dekont numarasını girip bildirin. '
   'Ödemeniz kontrol edildikten sonra bakiyeniz tanımlanır.',
   '{"banka":"","hesapAdi":"","iban":""}'::jsonb, false, 10),
  ('usdt-trc20', 'CRYPTO', 'USDT (TRC-20)',
   'Aşağıdaki cüzdana YALNIZ TRC-20 ağı üzerinden USDT gönderin. '
   'Farklı ağdan gönderilen tutarlar kaybolur ve iade edilemez. '
   'Gönderim sonrası işlem hash''ini girin.',
   '{"ag":"TRC-20","cuzdanAdresi":""}'::jsonb, false, 20);

-- Yeni izinler — yönetim paneli için (trd.md §izinler).
INSERT INTO permissions (code, description) VALUES
  ('orders:read_all',   'Tüm siparişleri görüntüle'),
  ('deposits:read',     'Bakiye yükleme taleplerini görüntüle'),
  ('deposits:approve',  'Bakiye yükleme taleplerini onayla/reddet'),
  ('providers:read',    'Sağlayıcıları görüntüle'),
  ('providers:write',   'Sağlayıcı ekle/düzenle, API anahtarı güncelle')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'admin'
  AND p.code IN ('orders:read_all', 'deposits:read', 'deposits:approve',
                 'providers:read', 'providers:write')
ON CONFLICT DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions WHERE permission_id IN (
  SELECT id FROM permissions WHERE code IN
    ('orders:read_all','deposits:read','deposits:approve','providers:read','providers:write'));
DELETE FROM permissions WHERE code IN
  ('orders:read_all','deposits:read','deposits:approve','providers:read','providers:write');
DROP TABLE IF EXISTS deposits, deposit_methods, rental_details, order_messages, orders;
DROP FUNCTION IF EXISTS orders_guard_transition();
DROP TYPE IF EXISTS deposit_status, deposit_method_kind, refund_status, order_status;
-- +goose StatementEnd
