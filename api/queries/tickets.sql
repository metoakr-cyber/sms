-- Destek talepleri (FR-600).
--
-- İKİ AYRI SORGU AİLESİ vardır ve karıştırılmaz:
--   *ForUser  → sahiplik SORGUNUN PARÇASIDIR (değişmez #7)
--   *ForAdmin → sahiplik kısıtı yoktur; koruma izin ara katmanındadır

-- name: CreateTicket :one
INSERT INTO tickets (user_id, subject, priority)
VALUES (@user_id, @subject, @priority)
RETURNING *;

-- name: CreateTicketMessage :one
-- 🔴 is_staff AÇIKÇA VERİLİR. Sütunun varsayılanı yoktur (00012_tickets.sql):
-- parametreyi unutan bir çağrı derlenmez ya da veritabanında düşer, sessizce
-- "kullanıcı yazdı" diye kaydedilmez (KK-600).
INSERT INTO ticket_messages (ticket_id, user_id, is_staff, body)
VALUES (@ticket_id, @user_id, @is_staff, @body)
RETURNING *;

-- name: GetTicketForUser :one
-- SAHİPLİK SORGUNUN PARÇASIDIR (CLAUDE.md değişmez #7). Başkasının talebi ile
-- var olmayan talep AYNI sonucu (sıfır satır) verir; servis ikisini de
-- ErrNotFound'a çevirir.
SELECT * FROM tickets WHERE public_id = @public_id AND user_id = @user_id;

-- name: LockTicketForUser :one
-- ÇAĞIRANIN TRANSACTION'I İÇİNDE çağrılır. Kilit, aynı talebe iki mesajın
-- eşzamanlı yazılıp durumun ikisinden yalnız birine göre hesaplanmasını
-- engeller. Sahiplik yine SORGUDADIR — kilit almak yetki vermez.
-- test: internal/service/ticket/ticket_integration_test.go#TestUserCannotWriteToOthersTicket
SELECT * FROM tickets WHERE public_id = @public_id AND user_id = @user_id FOR UPDATE;

-- name: LockTicket :one
-- Yönetim yolu: sahiplik kısıtı YOKTUR, izin kontrolü middleware'dedir
-- (tickets:reply).
SELECT * FROM tickets WHERE public_id = @public_id FOR UPDATE;

-- name: GetTicketForAdmin :one
-- Sayısal id dışarı verilmez: kullanıcı users.public_id ile gösterilir.
SELECT t.*,
       u.public_id AS user_public_id,
       u.email     AS user_email,
       u.username  AS user_username
FROM tickets t
JOIN users u ON u.id = t.user_id
WHERE t.public_id = @public_id;

-- name: ListUserTickets :many
-- tickets_user_idx (user_id, last_reply_at DESC) tam olarak bu sıralamayı kullanır.
--
-- Durum süzgeci `ListTicketsForAdmin` ile AYNI desen: `sqlc.narg` NULL ise
-- süzme yok. 🔴 `user_id` koşulu süzgeçten bağımsızdır (değişmez #7).
SELECT t.*,
       (SELECT count(*) FROM ticket_messages m WHERE m.ticket_id = t.id) AS message_count
FROM tickets t
WHERE t.user_id = @user_id
  AND (sqlc.narg('status')::ticket_status IS NULL
       OR t.status = sqlc.narg('status')::ticket_status)
  AND (sqlc.narg('q')::text IS NULL
       OR t.subject::text ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY t.last_reply_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountUserTickets :one
-- 🔴 SÜZGEÇ `ListUserTickets` İLE AYNI OLMAK ZORUNDA; ayrışırsa sayfalama
-- yalan söyler (bkz. orders.sql, CountUserOrders notu).
SELECT count(*) FROM tickets
WHERE user_id = @user_id
  AND (sqlc.narg('status')::ticket_status IS NULL
       OR status = sqlc.narg('status')::ticket_status)
  AND (sqlc.narg('q')::text IS NULL
       OR subject::text ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: ListTicketsForAdmin :many
-- Dinamik süzgeç sqlc.narg deseniyle; Go'da string birleştirilmez.
--
-- İKİ AYRI SÜZGEÇ vardır ve ikisi de gereklidir:
--   status      → tam durum eşleşmesi ("Kapatılanları göster")
--   only_pending→ YÖNETİCİNİN KUYRUĞU: OPEN **ve** USER_REPLIED birlikte
--
-- "Açık talepler" TEK BİR DURUM DEĞİLDİR. Yalnız OPEN süzmek, kullanıcının
-- yanıt yazdığı (USER_REPLIED) talepleri varsayılan ekrandan gizler — yani
-- yanıt bekleyen müşteri görünmez olur.
SELECT t.*,
       u.public_id AS user_public_id,
       u.email     AS user_email,
       u.username  AS user_username,
       (SELECT count(*) FROM ticket_messages m WHERE m.ticket_id = t.id) AS message_count
FROM tickets t
JOIN users u ON u.id = t.user_id
WHERE (sqlc.narg('status')::ticket_status IS NULL
       OR t.status = sqlc.narg('status')::ticket_status)
  AND (NOT sqlc.arg('only_pending')::boolean
       OR t.status IN ('OPEN', 'USER_REPLIED'))
  AND (sqlc.narg('q')::text IS NULL
       OR t.subject::text  ILIKE '%' || sqlc.narg('q')::text || '%'
       OR u.email::text    ILIKE '%' || sqlc.narg('q')::text || '%'
       OR u.username::text ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY t.last_reply_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountTicketsForAdmin :one
-- Süzgeç koşulu ListTicketsForAdmin ile BİREBİR AYNI olmalıdır; ayrışırsa
-- sayfalama "23 kayıt" der ama 12 satır gösterir.
-- `q` kullanıcı sütunlarında da arandığı için sayım `users`a katılmak zorunda.
SELECT count(*) FROM tickets t
JOIN users u ON u.id = t.user_id
WHERE (sqlc.narg('status')::ticket_status IS NULL
       OR t.status = sqlc.narg('status')::ticket_status)
  AND (NOT sqlc.arg('only_pending')::boolean
       OR t.status IN ('OPEN', 'USER_REPLIED'))
  AND (sqlc.narg('q')::text IS NULL
       OR t.subject::text  ILIKE '%' || sqlc.narg('q')::text || '%'
       OR u.email::text    ILIKE '%' || sqlc.narg('q')::text || '%'
       OR u.username::text ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: ListTicketMessagesForUser :many
-- KULLANICI GÖRÜNÜMÜ: personelin kullanıcı adı SEÇİLMEZ.
--
-- Personelin kimliği kullanıcıya gösterilmez; yazar etiketi is_staff'tan
-- türetilir ("Destek ekibi"). Sütunu getirip DTO'da atmak, bir gün birinin
-- onu yanıta koymasını bir satır uzaklığa indirirdi.
-- test: internal/transport/http/handler/ticket_integration_test.go#TestStaffUsernameNeverLeaksToUser
SELECT m.public_id, m.is_staff, m.body, m.created_at
FROM ticket_messages m
WHERE m.ticket_id = @ticket_id
ORDER BY m.created_at, m.id;

-- name: ListTicketMessagesForAdmin :many
-- YÖNETİM GÖRÜNÜMÜ: yazarın kullanıcı adı da gelir — kim yanıtladı sorusu
-- destek ekibinin iç sorusudur.
SELECT m.public_id, m.is_staff, m.body, m.created_at,
       COALESCE(u.username, '') AS author_username
FROM ticket_messages m
LEFT JOIN users u ON u.id = m.user_id
WHERE m.ticket_id = @ticket_id
ORDER BY m.created_at, m.id;

-- name: TouchTicketAfterMessage :one
-- Mesaj eklendikten SONRA durumu ve son yanıt zamanını yazar.
--
-- Yeni durum SERVİSTE domain/ticket ile hesaplanır (değişmez #13): bu sorgu
-- karar vermez, kararı yazar.
UPDATE tickets SET status = @status, last_reply_at = @last_reply_at
WHERE id = @id
RETURNING *;

-- name: SetTicketStatus :one
-- 🔴 closed_at, status ile AYNI ifadede yazılır: ticket_closed_has_time
-- CHECK'i (00012_tickets.sql) iki adımlı yazımda 23514 ile düşer.
-- Talep yeniden açılırken closed_at NULL'a döner.
UPDATE tickets SET status = @status, closed_at = sqlc.narg('closed_at')
WHERE public_id = @public_id
RETURNING *;
