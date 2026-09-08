-- name: GetUserPermissions :many
SELECT DISTINCT p.code
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p       ON p.id = rp.permission_id
WHERE ur.user_id = $1
ORDER BY p.code;

-- name: GetUserRoles :many
SELECT r.* FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = $1
ORDER BY r.name;

-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = $1;

-- name: AssignRole :exec
INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)
ON CONFLICT (user_id, role_id) DO NOTHING;

-- name: RevokeRole :exec
DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2;

-- name: ReplaceUserRoles :exec
WITH cleared AS (DELETE FROM user_roles WHERE user_id = sqlc.arg('user_id'))
INSERT INTO user_roles (user_id, role_id)
SELECT sqlc.arg('user_id'), unnest(sqlc.arg('role_ids')::bigint[])
ON CONFLICT DO NOTHING;

-- name: UpsertPermission :one
INSERT INTO permissions (code, description) VALUES ($1, $2)
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description
RETURNING *;

-- name: UpsertRole :one
INSERT INTO roles (name, description, is_system) VALUES ($1, $2, $3)
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
RETURNING *;

-- name: GrantPermissionToRole :exec
INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListUsersForAdmin :many
-- Yönetim kullanıcı listesi.
--
-- 🔴 password_hash SEÇİLMEZ. Yönetim panelinde bile: bir sızıntıda parola
-- özetleri offline kırma denemelerine açık olur ve hiçbir yönetim işlevi
-- onlara ihtiyaç duymaz.
-- test: internal/transport/http/handler/admin_integration_test.go#TestListUsersNeverReturnsPasswordHash
SELECT
    u.public_id, u.email, u.username, u.status,
    u.email_verified_at, u.balance_minor, u.created_at,
    coalesce(string_agg(DISTINCT r.name, ','), '')::text AS roles,
    (SELECT count(*) FROM orders o WHERE o.user_id = u.id)::bigint AS order_count
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE (sqlc.narg(q)::text IS NULL
       OR u.email ILIKE '%' || sqlc.narg(q) || '%'
       OR u.username ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(status)::user_status IS NULL OR u.status = sqlc.narg(status))
GROUP BY u.id
ORDER BY u.created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountUsersForAdmin :one
SELECT count(*) FROM users u
WHERE (sqlc.narg(q)::text IS NULL
       OR u.email ILIKE '%' || sqlc.narg(q) || '%'
       OR u.username ILIKE '%' || sqlc.narg(q) || '%')
  AND (sqlc.narg(status)::user_status IS NULL OR u.status = sqlc.narg(status));


