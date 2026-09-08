-- name: CreateAuthToken :one
INSERT INTO auth_tokens (user_id, token_hash, purpose, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ConsumeAuthToken :one
-- Tek kullanımlıktır: UPDATE ... RETURNING ile atomik olarak tüketilir.
-- Ayrı SELECT + UPDATE yapılsaydı iki eşzamanlı istek aynı token'ı kullanabilirdi.
UPDATE auth_tokens
SET used_at = now()
WHERE token_hash = $1
  AND purpose = $2
  AND used_at IS NULL
  AND expires_at > now()
RETURNING *;

-- name: InvalidateUserTokens :exec
UPDATE auth_tokens SET used_at = now()
WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL;

-- name: DeleteExpiredAuthTokens :execrows
DELETE FROM auth_tokens WHERE expires_at < now() - interval '7 days';
