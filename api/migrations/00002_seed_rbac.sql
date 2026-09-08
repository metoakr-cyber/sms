-- +goose Up
-- +goose StatementBegin

-- İzinler VERİDİR: yeni bir yetki eklemek satır eklemektir, kod dağıtımı gerektirmez.
INSERT INTO permissions (code, description) VALUES
    ('users:read',      'Kullanıcıları görüntüleme'),
    ('users:write',     'Kullanıcı bilgilerini ve bakiyesini değiştirme'),
    ('deposits:read',   'Bakiye yükleme taleplerini görüntüleme'),
    ('deposits:approve','Bakiye yükleme taleplerini onaylama/reddetme'),
    ('providers:read',  'Sağlayıcıları görüntüleme'),
    ('providers:write', 'Sağlayıcı ve eşleştirme yönetimi'),
    ('pricing:read',    'Fiyat kurallarını görüntüleme'),
    ('pricing:write',   'Fiyat kurallarını değiştirme'),
    ('orders:read_all', 'Tüm siparişleri görüntüleme'),
    ('tickets:read',    'Tüm destek taleplerini görüntüleme'),
    ('tickets:reply',   'Destek taleplerine personel olarak yanıt verme'),
    ('audit:read',      'Denetim kaydını görüntüleme')
ON CONFLICT (code) DO NOTHING;

INSERT INTO roles (name, description, is_system) VALUES
    ('user',  'Standart kullanıcı', true),
    ('admin', 'Tam yetkili yönetici', true)
ON CONFLICT (name) DO NOTHING;

-- admin rolü tüm izinlere sahiptir.
-- 'user' rolü BİLİNÇLİ olarak izinsizdir: kendi kaynaklarına erişim izinle değil
-- SAHİPLİK ile belirlenir (WHERE user_id = $current) — docs/design.md §10.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE r.name = 'admin'
ON CONFLICT DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions
    WHERE role_id IN (SELECT id FROM roles WHERE name IN ('user','admin'));
DELETE FROM roles WHERE name IN ('user','admin');
DELETE FROM permissions WHERE code IN (
    'users:read','users:write','deposits:read','deposits:approve',
    'providers:read','providers:write','pricing:read','pricing:write',
    'orders:read_all','tickets:read','tickets:reply','audit:read');
-- +goose StatementEnd
