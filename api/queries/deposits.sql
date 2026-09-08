-- Bakiye yükleme talepleri (FR-500 … FR-503).
--
-- Ödeme YÖNTEMİ sorguları burada değil, queries/orders.sql içindedir
-- (deposit_methods CRUD'u zaten yazılmıştı).

-- name: CreateDeposit :one
-- 🔴 tx_hash HAVALEDE NULL BIRAKILIR, boş string yazılmaz: deposits_tx_hash_uniq
-- kısmi indeksi (WHERE tx_hash IS NOT NULL) NULL'ları hariç tutar. '' yazılırsa
-- kullanıcının İKİNCİ havale talebi 23505 alır ve bir daha havale bildiremez.
-- test: deposit_integration_test.go#TestBankTransferDepositLeavesTxHashNull
INSERT INTO deposits (user_id, method_id, method_name, amount_minor,
                      tx_hash, network, user_note, idempotency_key)
VALUES (@user_id, sqlc.narg('method_id'), @method_name, @amount_minor,
        sqlc.narg('tx_hash'), @network, @user_note, sqlc.narg('idempotency_key'))
RETURNING *;

-- name: GetDepositByIdempotencyKey :one
-- Tekrarlanan talep isteğinde MEVCUT talebi döner.
--
-- Kullanıcı kapsamı sorgunun parçasıdır: iki kullanıcının aynı anahtarı
-- üretmesi çakışma değil, ayrı işlemdir.
-- test: deposit_integration_test.go#TestDepositCreateIsIdempotent
SELECT * FROM deposits
WHERE user_id = @user_id AND idempotency_key = @idempotency_key;

-- name: GetDepositForUser :one
-- SAHİPLİK SORGUNUN PARÇASIDIR (CLAUDE.md değişmez #7). Başkasının talebi ile
-- var olmayan talep AYNI sonucu (sıfır satır) verir.
SELECT * FROM deposits WHERE public_id = @public_id AND user_id = @user_id;

-- name: GetDeposit :one
-- Yönetim yolu: sahiplik kısıtı YOKTUR, izin kontrolü middleware'dedir
-- (deposits:read / deposits:approve).
SELECT * FROM deposits WHERE public_id = @public_id;

-- name: ListUserDeposits :many
-- deposits_user_idx (user_id, created_at DESC) tam olarak bu sıralamayı kullanır.
SELECT * FROM deposits
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountUserDeposits :one
SELECT count(*) FROM deposits WHERE user_id = @user_id;

-- name: ListDepositsForAdmin :many
-- Sayısal id dışarı verilmez: kullanıcı users.public_id ile gösterilir.
SELECT d.*,
       u.public_id AS user_public_id,
       u.email     AS user_email,
       u.username  AS user_username
FROM deposits d
JOIN users u ON u.id = d.user_id
WHERE (sqlc.narg('status')::deposit_status IS NULL
       OR d.status = sqlc.narg('status')::deposit_status)
ORDER BY d.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountDepositsForAdmin :one
SELECT count(*) FROM deposits d
WHERE (sqlc.narg('status')::deposit_status IS NULL
       OR d.status = sqlc.narg('status')::deposit_status);

-- name: LockDepositForUpdate :one
-- ÇAĞIRANIN TRANSACTION'I İÇİNDE çağrılmalıdır. Kilit commit'e kadar tutulur;
-- eşzamanlı onay istekleri burada sıraya girer. Kilitsiz okuyup sonra yazmak,
-- iki yöneticinin (veya bir yöneticinin iki sekmesinin) aynı talebi iki kez
-- onaylamasına ve bakiyenin iki kez artmasına yol açar.
-- test: deposit_integration_test.go#TestApproveIsIdempotentUnderConcurrency
SELECT * FROM deposits WHERE public_id = @public_id FOR UPDATE;

-- name: ApproveDeposit :one
-- İKİNCİ SAVUNMA HATTI: `AND status = 'PENDING'` koşulu yarışta ikinci çağrıya
-- SIFIR satır döndürür (pgx.ErrNoRows) — bu "zaten sonuçlandırılmış" demektir.
--
-- 🔴 reviewed_at, status ile AYNI ifadede yazılır: deposit_reviewed_has_time
-- CHECK'i (00008_orders.sql) iki adımlı yazımda 23514 ile düşer.
UPDATE deposits SET
    status              = 'COMPLETED',
    credited_minor      = @credited_minor,
    admin_note          = @admin_note,
    reviewed_by_user_id = @reviewed_by_user_id,
    reviewed_at         = @reviewed_at
WHERE public_id = @public_id AND status = 'PENDING'
RETURNING *;

-- name: RejectDeposit :one
-- Bakiyeye HİÇ dokunulmaz (FR-503); burada yalnız durum ve gerekçe yazılır.
UPDATE deposits SET
    status              = 'REJECTED',
    rejection_reason    = @rejection_reason,
    admin_note          = @admin_note,
    reviewed_by_user_id = @reviewed_by_user_id,
    reviewed_at         = @reviewed_at
WHERE public_id = @public_id AND status = 'PENDING'
RETURNING *;

-- name: SetDepositReceipt :one
-- Dekont YALNIZ sahibi tarafından ve YALNIZ inceleme öncesinde eklenebilir:
-- sonuçlandırılmış bir talebin kanıtını değiştirmek denetimi anlamsız kılar.
UPDATE deposits SET receipt_path = @receipt_path
WHERE public_id = @public_id AND user_id = @user_id AND status = 'PENDING'
RETURNING *;
