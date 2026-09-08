-- +goose Up
-- +goose StatementBegin

-- CITEXT: e-posta ve kullanıcı adında büyük/küçük harf duyarsız BENZERSİZLİK için.
-- LOWER() indeksleriyle taklit etmek yerine tip düzeyinde çözüyoruz.
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TYPE user_status AS ENUM ('PENDING_VERIFICATION', 'ACTIVE', 'SUSPENDED');

CREATE TABLE users (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id            UUID        NOT NULL DEFAULT gen_random_uuid(),
    email                CITEXT      NOT NULL,
    email_verified_at    TIMESTAMPTZ,
    username             CITEXT      NOT NULL,
    password_hash        TEXT        NOT NULL,

    -- Bakiye TÜRETİLMİŞ bir önbellektir; kaynak doğruluk ledger_entries tablosudur (M2).
    -- Negatif bakiye veritabanı düzeyinde de imkânsızdır (docs/trd.md FR-203).
    balance_minor        BIGINT      NOT NULL DEFAULT 0
                                     CONSTRAINT users_balance_non_negative CHECK (balance_minor >= 0),

    status               user_status NOT NULL DEFAULT 'PENDING_VERIFICATION',

    -- Referans sistemi v1.1'de açılacak; şema baştan hazır (docs/intent.md §7).
    referral_code        TEXT,
    referred_by_user_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,

    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at           TIMESTAMPTZ
);

CREATE UNIQUE INDEX users_public_id_key     ON users (public_id);
CREATE UNIQUE INDEX users_email_key         ON users (email)         WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_username_key      ON users (username)      WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_referral_code_key ON users (referral_code) WHERE referral_code IS NOT NULL;
CREATE INDEX users_status_idx ON users (status) WHERE deleted_at IS NULL;

-- ─────────────────────────── Yetkilendirme ───────────────────────────
-- İzinler VERİDİR, kod değil: yeni bir yetki eklemek satır eklemektir,
-- dağıtım gerektirmez (docs/trd.md FR-105).

CREATE TABLE roles (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_system   BOOLEAN NOT NULL DEFAULT false,  -- sistem rolleri silinemez
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE permissions (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,            -- 'deposits:approve'
    description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE role_permissions (
    role_id       BIGINT NOT NULL REFERENCES roles(id)       ON DELETE CASCADE,
    permission_id BIGINT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_roles (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id    BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX user_roles_role_id_idx ON user_roles (role_id);

-- ─────────────────────────── Oturumlar ───────────────────────────
-- Kaynak doğruluk Redis'tir (hızlı arama). Bu tablo kullanıcının "aktif
-- oturumlarım" listesini görebilmesi ve uzaktan sonlandırabilmesi içindir
-- (docs/trd.md FR-104).

CREATE TABLE sessions (
    id           TEXT        PRIMARY KEY,        -- opak, 32 bayt rastgele
    user_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip           INET,
    user_agent   TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX sessions_user_id_idx    ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- ─────────────────────────── Tek kullanımlık token'lar ───────────────────────────
-- E-posta doğrulama ve şifre sıfırlama. Token'ın KENDİSİ saklanmaz, yalnız SHA-256
-- özeti — veritabanı sızarsa token'lar kullanılamaz olsun.

CREATE TYPE token_purpose AS ENUM ('EMAIL_VERIFICATION', 'PASSWORD_RESET');

CREATE TABLE auth_tokens (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA         NOT NULL,
    purpose    token_purpose NOT NULL,
    expires_at TIMESTAMPTZ   NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX auth_tokens_hash_key   ON auth_tokens (token_hash);
CREATE INDEX auth_tokens_user_purpose_idx  ON auth_tokens (user_id, purpose) WHERE used_at IS NULL;

-- ─────────────────────────── Denetim kaydı ───────────────────────────
-- Ticari zorunluluk: kim, ne zaman, neyi, nasıl değiştirdi (docs/trd.md FR-705).

CREATE TABLE audit_logs (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action         TEXT        NOT NULL,          -- 'deposit.approve'
    entity_type    TEXT        NOT NULL,          -- 'deposit'
    entity_id      TEXT        NOT NULL,
    before         JSONB,
    after          JSONB,
    ip             INET,
    user_agent     TEXT        NOT NULL DEFAULT '',
    request_id     TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_entity_idx ON audit_logs (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_logs_actor_idx  ON audit_logs (actor_user_id, created_at DESC);

-- updated_at otomatik güncellemesi
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS users_set_updated_at ON users;
DROP FUNCTION IF EXISTS set_updated_at();
DROP TABLE IF EXISTS audit_logs, auth_tokens, sessions, user_roles, role_permissions, permissions, roles, users;
DROP TYPE IF EXISTS token_purpose, user_status;
-- +goose StatementEnd
