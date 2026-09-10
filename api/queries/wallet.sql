-- Cüzdan sorguları.
--
-- BU DOSYADAKİ SIRALAMA KRİTİKTİR ve wallet servisinde birebir uygulanır:
--   1) idempotency kontrolü  →  2) satır KİLİDİ  →  3) yeterlilik  →  4) yazım
-- Kilitsiz bir okuma-değiştir-yazma döngüsü eşzamanlı isteklerde bakiyeyi bozar
-- (docs/memory.md §3.6 — eski prototipin hatası).

-- name: FindLedgerEntryByKey :one
-- İdempotency: bu anahtarla daha önce işlem yapıldıysa sonucu döner.
SELECT * FROM ledger_entries WHERE idempotency_key = $1;

-- name: LockUserForUpdate :one
-- Kullanıcı satırını KİLİTLER. Bu satır olmadan çift harcama mümkündür:
-- iki eşzamanlı istek aynı bakiyeyi okuyup ikisi de yeterli sanabilir.
-- Kilit, transaction bitene kadar tutulur.
SELECT id, balance_minor, status FROM users
WHERE id = $1 AND deleted_at IS NULL
FOR UPDATE;

-- name: InsertLedgerEntry :one
INSERT INTO ledger_entries (
    user_id, amount_minor, currency, entry_type,
    reference_type, reference_id, balance_after_minor,
    idempotency_key, created_by_user_id, note
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING *;

-- name: SetUserBalance :exec
UPDATE users SET balance_minor = $2 WHERE id = $1;

-- name: GetUserBalance :one
SELECT balance_minor FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: ListLedgerEntries :many
-- Kullanıcının hareket dökümü. SAHİPLİK sorgunun parçasıdır: user_id ayrı bir
-- if kontrolü değil, WHERE koşuludur (docs/design.md §10).
--
-- `q` SERBEST METİN: not ve referans kimliği. Tür adı ARANMAZ — "iade" yazan
-- kullanıcı REFUND satırlarını beklerdi ama tür adı veritabanında DURMAZ,
-- sunucuda enum'dan üretilir (`ledgerTypeLabel`). O iş tür süzgecinindir.
SELECT * FROM ledger_entries
WHERE user_id = sqlc.arg('user_id')
  AND (sqlc.narg('entry_type')::ledger_type IS NULL OR entry_type = sqlc.narg('entry_type')::ledger_type)
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR created_at >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz   IS NULL OR created_at <= sqlc.narg('to_ts')::timestamptz)
  AND (sqlc.narg('q')::text IS NULL
       OR coalesce(note, '')::text ILIKE '%' || sqlc.narg('q')::text || '%'
       OR coalesce(reference_id, '')::text ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountLedgerEntries :one
--
-- 🔴 SÜZGEÇLER `ListLedgerEntries` İLE BİREBİR AYNI.
--
-- ÖNCEDEN DEĞİLDİ: liste `from_ts`/`to_ts` süzerken sayım SÜZMÜYORDU. Tarih
-- aralığı arayüzde henüz açılmamıştı, yani hata gizli kalmıştı; açıldığı gün
-- sayfalama "1–25 / 412" derken liste 3 satır gösterecekti. `q` eklenirken
-- ikisi tek elden yazıldı.
-- test: wallet_integration_test.go#TestListLedgerEntriesSuzgecleri
SELECT count(*) FROM ledger_entries
WHERE user_id = sqlc.arg('user_id')
  AND (sqlc.narg('entry_type')::ledger_type IS NULL OR entry_type = sqlc.narg('entry_type')::ledger_type)
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR created_at >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz   IS NULL OR created_at <= sqlc.narg('to_ts')::timestamptz)
  AND (sqlc.narg('q')::text IS NULL
       OR coalesce(note, '')::text ILIKE '%' || sqlc.narg('q')::text || '%'
       OR coalesce(reference_id, '')::text ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: FindLedgerEntriesByReference :many
SELECT * FROM ledger_entries
WHERE reference_type = $1 AND reference_id = $2
ORDER BY id;

-- name: ListReconciliationDrift :many
-- Mutabakat: defter toplamı ile önbelleklenmiş bakiyenin uyuşmadığı kullanıcılar.
-- Boş dönmesi beklenir; dönmezse ALARM üretilir (docs/trd.md FR-205).
SELECT user_id, email, cached_balance::bigint AS cached_balance,
       ledger_balance::bigint AS ledger_balance, drift::bigint AS drift
FROM ledger_reconciliation
WHERE drift <> 0
ORDER BY abs(drift) DESC
LIMIT $1;

-- name: SumLedgerByType :many
-- Kâr raporu ve muhasebe özeti girdisi.
SELECT entry_type, count(*)::bigint AS entry_count, sum(amount_minor)::bigint AS total_minor
FROM ledger_entries
WHERE created_at >= $1 AND created_at < $2
GROUP BY entry_type
ORDER BY entry_type;
