-- name: InsertAuditLog :exec
INSERT INTO audit_logs
    (actor_user_id, action, entity_type, entity_id, before, after, ip, user_agent, request_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type')::text)
  AND (sqlc.narg('entity_id')::text   IS NULL OR entity_id   = sqlc.narg('entity_id')::text)
  AND (sqlc.narg('actor')::bigint     IS NULL OR actor_user_id = sqlc.narg('actor')::bigint)
ORDER BY created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- ─────────────────────── Denetim kaydı okuma (FR-705) ───────────────────────

-- name: ListAuditLogsForAdmin :many
-- Aktör / varlık / eylem / tarih süzgeçli, sayfalı denetim kaydı.
--
-- AKTÖR public_id İLE SÜZÜLÜR, sayısal id ile değil (Değişmez #10): dışarıya
-- verilen kimlik neyse süzgeç de onu almalı, yoksa panel önce kullanıcıyı
-- sayısal kimliğe çevirmek zorunda kalır ve o kimlik yanıtta da görünür.
--
-- E-POSTA SEÇİLMEZ. Aktörü tanımak için public_id + kullanıcı adı yeter;
-- e-posta denetim listesinde toplu olarak dışarı akan kişisel veridir.
SELECT
    a.id, a.action, a.entity_type, a.entity_id, a.before, a.after,
    a.ip, a.user_agent, a.request_id, a.created_at,
    u.public_id AS actor_public_id,
    COALESCE(u.username, '')::text AS actor_username
FROM audit_logs a
LEFT JOIN users u ON u.id = a.actor_user_id
WHERE (sqlc.narg('entity_type')::text IS NULL OR a.entity_type = sqlc.narg('entity_type')::text)
  AND (sqlc.narg('entity_id')::text   IS NULL OR a.entity_id   = sqlc.narg('entity_id')::text)
  AND (sqlc.narg('action')::text      IS NULL OR a.action      = sqlc.narg('action')::text)
  AND (sqlc.narg('actor')::uuid       IS NULL OR u.public_id   = sqlc.narg('actor')::uuid)
  AND (sqlc.narg('from')::timestamptz IS NULL OR a.created_at >= sqlc.narg('from')::timestamptz)
  AND (sqlc.narg('until')::timestamptz IS NULL OR a.created_at < sqlc.narg('until')::timestamptz)
-- id DESC ikinci ölçüt: aynı milisaniyede yazılan iki kayıt sayfalar arasında
-- yer değiştirirse aynı satır iki sayfada birden görünür ya da hiç görünmez.
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountAuditLogsForAdmin :one
SELECT count(*) FROM audit_logs a
LEFT JOIN users u ON u.id = a.actor_user_id
WHERE (sqlc.narg('entity_type')::text IS NULL OR a.entity_type = sqlc.narg('entity_type')::text)
  AND (sqlc.narg('entity_id')::text   IS NULL OR a.entity_id   = sqlc.narg('entity_id')::text)
  AND (sqlc.narg('action')::text      IS NULL OR a.action      = sqlc.narg('action')::text)
  AND (sqlc.narg('actor')::uuid       IS NULL OR u.public_id   = sqlc.narg('actor')::uuid)
  AND (sqlc.narg('from')::timestamptz IS NULL OR a.created_at >= sqlc.narg('from')::timestamptz)
  AND (sqlc.narg('until')::timestamptz IS NULL OR a.created_at < sqlc.narg('until')::timestamptz);
