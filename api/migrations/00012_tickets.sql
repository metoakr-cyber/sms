-- +goose Up
-- +goose StatementBegin

-- ═══════════════════════ DESTEK TALEPLERİ (FR-600) ═══════════════════════
--
-- Şema docs/design.md §"DESTEK / DENETİM" satırlarını izler:
--   tickets         (id, public_id, user_id, subject, priority, status, last_reply_at)
--   ticket_messages (id, ticket_id, user_id, body, is_staff)
--
-- `attachments JSONB` sütunu BİLEREK YOK: dosya eki bu turun kapsamı dışında.
-- Boş bir sütun eklemek, ekin var olduğunu ima eder ve ilk yükleyen kişi
-- dosyasının kaybolduğunu görür. Ek geldiğinde kendi migration'ı ile gelir.

CREATE TYPE ticket_status AS ENUM (
    'OPEN',          -- kullanıcı açtı / yanıt bekliyor  → yöneticinin işi
    'ANSWERED',      -- personel yanıtladı              → kullanıcının işi
    'USER_REPLIED',  -- kullanıcı yeniden yazdı         → yöneticinin işi
    'CLOSED'         -- kapatıldı
);

-- Öncelik KULLANICIDAN alınır (FR-600) ama sıralamayı belirlemez: acil olduğunu
-- söyleyen herkes 'HIGH' seçer. Yöneticiye bir ipucudur, bir söz değildir.
CREATE TYPE ticket_priority AS ENUM ('LOW', 'NORMAL', 'HIGH');

CREATE TABLE tickets (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id     UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id       BIGINT NOT NULL REFERENCES users(id),

    -- Uzunluk sınırı VERİTABANINDA da vardır. DTO doğrulaması ilk savunmadır;
    -- bir gün başka bir yol (CLI, iş, içe aktarma) satır yazarsa sınırsız metin
    -- hem depoyu hem listeleme ekranını bozar.
    -- Aralık docs/trd.md §10 doğrulama tablosundan gelir (5–120).
    subject       TEXT NOT NULL
                  CONSTRAINT ticket_subject_len
                  CHECK (char_length(subject) BETWEEN 5 AND 120),

    priority      ticket_priority NOT NULL DEFAULT 'NORMAL',
    status        ticket_status   NOT NULL DEFAULT 'OPEN',

    -- last_reply_at SIRALAMA ANAHTARIDIR: yöneticinin listesi "en son
    -- konuşulan üstte" olmalı, "en önce açılan üstte" değil. created_at ile
    -- sıralamak, üç gün önce açılıp bugün yanıt gelen talebi dibe gömer.
    last_reply_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at     TIMESTAMPTZ,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Zaman damgası durumla TUTARLI olmalı — orders tablosundaki aynı kalıp.
    CONSTRAINT ticket_closed_has_time CHECK (
        (status <> 'CLOSED') OR (closed_at IS NOT NULL))
);

CREATE UNIQUE INDEX tickets_public_id_idx ON tickets (public_id);

-- Kullanıcının talep listesi — en son konuşulan üstte.
CREATE INDEX tickets_user_idx ON tickets (user_id, last_reply_at DESC);

-- Yönetim listesi: durum süzgeci + aynı sıralama.
CREATE INDEX tickets_status_idx ON tickets (status, last_reply_at DESC);

CREATE TRIGGER tickets_set_updated_at
    BEFORE UPDATE ON tickets FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE ticket_messages (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id  UUID NOT NULL DEFAULT gen_random_uuid(),
    ticket_id  BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,

    -- Yazarın hesabı silinirse mesaj KALIR (ON DELETE SET NULL): yazışmanın
    -- yarısı kaybolursa geriye kalan yarısı yanıltıcıdır.
    user_id    BIGINT REFERENCES users(id) ON DELETE SET NULL,

    -- 🔴 VARSAYILAN DEĞERİ YOKTUR ve NOT NULL'dur — bilerek.
    --
    -- KK-600: eski sistemde bu bayrak var olmayan bir alandan okunuyordu ve
    -- HER ZAMAN false çıkıyordu; personel yanıtları kullanıcı mesajı gibi
    -- görünüyordu. `DEFAULT false` yazsaydık aynı hata sessizce tekrarlanırdı:
    -- değeri koymayı unutan INSERT, "kullanıcı yazdı" diye kaydedilirdi.
    -- Varsayılan olmayınca eksik değer bir veritabanı hatasıdır.
    -- test: internal/service/ticket/ticket_integration_test.go#TestStaffFlagIsRecorded
    is_staff   BOOLEAN NOT NULL,

    -- Aralık docs/trd.md §10'dan (1–5000) DAHA DARDIR (1–4000): gösterim ve
    -- depolama sınırı olarak 4000 seçildi; ikisini de karşılar.
    body       TEXT NOT NULL
               CONSTRAINT ticket_message_len
               CHECK (char_length(body) BETWEEN 1 AND 4000),

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ticket_messages_public_id_idx ON ticket_messages (public_id);

-- Yazışma HER ZAMAN kronolojik okunur; detay ekranının tek sorgusu budur.
CREATE INDEX ticket_messages_ticket_idx ON ticket_messages (ticket_id, created_at, id);

-- +goose StatementEnd

-- +goose StatementBegin
-- ── DURUM MAKİNESİ KORUMASI ──
--
-- Geçişler `internal/domain/ticket` içinde tanımlıdır; burada İKİNCİ KEZ
-- zorlanır. Sipariş tablosundaki gerekçenin aynısı: kodda bir yol durum
-- alanına doğrudan yazarsa (arka plan işi, elle SQL, gelecekteki bir handler)
-- veritabanı son sözü söyler.
--
-- KAPALI TALEP KULLANICI TARAFINDAN YENİDEN AÇILAMAZ; yalnız CLOSED → OPEN
-- oku vardır ve onu YALNIZ yönetici kullanır (service/ticket, AdminSetStatus).
-- Gerekçe domain/ticket/ticket.go içindedir.
--
-- test: internal/service/ticket/ticket_integration_test.go#TestInvalidTicketTransitionIsRejectedByDB
CREATE OR REPLACE FUNCTION tickets_guard_transition() RETURNS trigger AS $$
BEGIN
    IF OLD.status = NEW.status THEN
        RETURN NEW;
    END IF;

    IF NOT (
        (OLD.status = 'OPEN'         AND NEW.status IN ('ANSWERED', 'CLOSED')) OR
        (OLD.status = 'ANSWERED'     AND NEW.status IN ('USER_REPLIED', 'CLOSED')) OR
        (OLD.status = 'USER_REPLIED' AND NEW.status IN ('ANSWERED', 'CLOSED')) OR
        (OLD.status = 'CLOSED'       AND NEW.status = 'OPEN')
    ) THEN
        RAISE EXCEPTION 'gecersiz talep gecisi: % -> %', OLD.status, NEW.status;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER tickets_guard_transition_trg
    BEFORE UPDATE ON tickets FOR EACH ROW EXECUTE FUNCTION tickets_guard_transition();

-- ── İZİNLER ──
--
-- `tickets:read` ve `tickets:reply` 00002_seed_rbac.sql'de ZATEN tanımlı ve
-- admin rolüne bağlıdır; burada yeniden eklenmez. Bu blok yalnız açıklamadır:
-- yeni bir izin kodu (örn. `tickets:write`) ÜRETİLMEDİ, çünkü docs/trd.md
-- §izinler tablosu `tickets:reply` diyor ve iki eş anlamlı izin, yetki
-- matrisini okunamaz hâle getirir.
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS ticket_messages, tickets;
DROP FUNCTION IF EXISTS tickets_guard_transition();
DROP TYPE IF EXISTS ticket_priority, ticket_status;
-- +goose StatementEnd
