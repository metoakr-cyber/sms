-- name: CreateUser :one
INSERT INTO users (email, username, password_hash, status)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByPublicID :one
SELECT * FROM users WHERE public_id = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL;

-- name: EmailExists :one
SELECT EXISTS (SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL);

-- name: UsernameExists :one
SELECT EXISTS (SELECT 1 FROM users WHERE username = $1 AND deleted_at IS NULL);

-- name: MarkEmailVerified :exec
UPDATE users
SET email_verified_at = now(),
    status = CASE WHEN status = 'PENDING_VERIFICATION' THEN 'ACTIVE'::user_status ELSE status END
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdatePasswordHash :exec
UPDATE users SET password_hash = $2 WHERE id = $1 AND deleted_at IS NULL;

-- name: SetUserStatus :exec
UPDATE users SET status = $2 WHERE id = $1 AND deleted_at IS NULL;

-- name: ListUsers :many
SELECT * FROM users
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::user_status IS NULL OR status = sqlc.narg('status')::user_status)
  AND (sqlc.narg('search')::text IS NULL
       OR email::text ILIKE '%' || sqlc.narg('search')::text || '%'
       OR username::text ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');
