-- Müşteri yorumları.
--
-- ÜÇ AYRI SORGU AİLESİ vardır ve karıştırılmaz:
--   *ForUser   → sahiplik SORGUNUN PARÇASIDIR (değişmez #7)
--   *ForAdmin  → sahiplik kısıtı yoktur; koruma izin ara katmanındadır
--   Approved*  → OTURUMSUZ, sitede gösterilir; KİŞİSEL VERİ SEÇMEZ

-- name: CreateReview :one
-- Durum İSTEMCİDEN ALINMAZ: yeni yorum her zaman PENDING'dir (varsayılan) ve
-- oradan yalnız durum makinesi çıkarır (değişmez #13).
--
-- 🔴 "Bu kullanıcının bekleyen yorumu var mı?" diye ÖNCE SELECT YAPILMAZ.
-- Kısıt kısmi benzersiz indekstedir (reviews_one_pending_per_user_idx) ve bu
-- INSERT 23505 ile düşer. Uygulama katmanındaki kontrol iki eşzamanlı isteğin
-- ikisi tarafından da geçilirdi.
-- test: internal/service/review/review_integration_test.go#TestConcurrentSubmitsLeaveOnePending
INSERT INTO reviews (user_id, rating, body)
VALUES (@user_id, @rating, @body)
RETURNING *;

-- name: ListReviewsForUser :many
-- Kullanıcının KENDİ yorumları ve durumları. En yeni üstte.
--
-- Durum süzgeci: `sqlc.narg` NULL ise süzme yok.
-- 🔴 `user_id` koşulu süzgeçten bağımsızdır (değişmez #7).
SELECT * FROM reviews
WHERE user_id = @user_id
  AND (sqlc.narg('status')::review_status IS NULL
       OR status = sqlc.narg('status')::review_status)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountReviewsForUser :one
-- 🔴 SÜZGEÇ `ListReviewsForUser` İLE AYNI OLMAK ZORUNDA; ayrışırsa sayfalama
-- yalan söyler (bkz. orders.sql, CountUserOrders notu).
SELECT count(*) FROM reviews
WHERE user_id = @user_id
  AND (sqlc.narg('status')::review_status IS NULL
       OR status = sqlc.narg('status')::review_status);

-- name: GetReviewForUser :one
-- SAHİPLİK SORGUNUN PARÇASIDIR (değişmez #7). Başkasının yorumu ile var
-- olmayan yorum AYNI sonucu (sıfır satır) verir; servis ikisini de
-- ErrNotFound'a çevirir.
-- test: internal/transport/http/handler/review_integration_test.go#TestUserCannotSeeOthersReview
SELECT * FROM reviews WHERE public_id = @public_id AND user_id = @user_id;

-- name: LockReview :one
-- Yönetim yolu: ÇAĞIRANIN TRANSACTION'I İÇİNDE. Kilit, aynı yoruma iki
-- yöneticinin aynı anda karar vermesini (biri onay, biri red) engeller.
-- test: internal/service/review/review_integration_test.go#TestConcurrentDecisionsLeaveOneOutcome
SELECT * FROM reviews WHERE public_id = @public_id FOR UPDATE;

-- name: DecideReview :one
-- Kararı YAZAR, VERMEZ. Yeni durum serviste domain/review ile hesaplanır
-- (değişmez #13); bu sorgu yalnız sonucu kaydeder.
--
-- 🔴 status, rejection_reason ve reviewed_at AYNI İFADEDE yazılır:
-- review_decided_has_time ve review_reason_only_when_rejected CHECK'leri
-- iki adımlı bir yazımda 23514 ile düşer.
UPDATE reviews
SET status = @status,
    rejection_reason = @rejection_reason,
    reviewed_by_user_id = @reviewed_by_user_id,
    reviewed_at = @reviewed_at
WHERE public_id = @public_id
RETURNING *;

-- name: ListReviewsForAdmin :many
-- Dinamik süzgeç sqlc.narg deseniyle; Go'da string birleştirilmez.
--
-- Yönetim görünümü kullanıcıyı TANIMLAR (e-posta dâhil): moderasyon kararı
-- kimin yazdığını bilmeden verilemez. Bu sütunlar SİTEDEKİ sorguda YOKTUR.
SELECT r.*,
       u.public_id AS user_public_id,
       u.email     AS user_email,
       u.username  AS user_username
FROM reviews r
JOIN users u ON u.id = r.user_id
WHERE (sqlc.narg('status')::review_status IS NULL
       OR r.status = sqlc.narg('status')::review_status)
  AND (sqlc.narg('q')::text IS NULL
       OR r.body::text     ILIKE '%' || sqlc.narg('q')::text || '%'
       OR u.email::text    ILIKE '%' || sqlc.narg('q')::text || '%'
       OR u.username::text ILIKE '%' || sqlc.narg('q')::text || '%')
-- Bekleyenler EN ESKİ ÖNCE: moderasyon bir kuyruktur, en uzun bekleyen ilk
-- sırada olmalı. Karara bağlanmışlar en yeni önce: yönetici son ne yaptığına
-- bakar. Tek bir sıralama ikisini de doğru yapamaz.
ORDER BY (r.status = 'PENDING') DESC,
         CASE WHEN r.status = 'PENDING' THEN r.created_at END ASC,
         r.created_at DESC,
         r.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountReviewsForAdmin :one
-- Süzgeç koşulu ListReviewsForAdmin ile BİREBİR AYNI olmalıdır; ayrışırsa
-- sayfalama "23 kayıt" der ama 12 satır gösterir.
-- `q` kullanıcı sütunlarında da arandığı için sayım `users`a katılmak zorunda.
SELECT count(*) FROM reviews r
JOIN users u ON u.id = r.user_id
WHERE (sqlc.narg('status')::review_status IS NULL
       OR r.status = sqlc.narg('status')::review_status)
  AND (sqlc.narg('q')::text IS NULL
       OR r.body::text     ILIKE '%' || sqlc.narg('q')::text || '%'
       OR u.email::text    ILIKE '%' || sqlc.narg('q')::text || '%'
       OR u.username::text ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: CountPendingReviews :one
-- Yönetim menüsündeki rozet için: kaç yorum karar bekliyor.
SELECT count(*) FROM reviews WHERE status = 'PENDING';

-- name: ListApprovedReviews :many
-- 🔴 SİTEDE GÖSTERİLEN LİSTE — OTURUMSUZ ERİŞİLİR.
--
-- E-POSTA, sayısal id ve user_id BU SORGUDA SEÇİLMEZ. "Yanıtta gizleriz"
-- yeterli değildir: alan seçilirse bir gün birinin onu DTO'ya koyması bir
-- satır uzaklıktadır ve sızıntı sessiz olur. Seçilmeyen sütun sızamaz.
-- test: internal/transport/http/handler/review_integration_test.go#TestPublicReviewsNeverExposeEmail
--
-- DISTINCT ON (user_id): bir kullanıcının zaman içinde birden fazla onaylı
-- yorumu olabilir (fikri değişip yenisini yazar). Sitede YALNIZ EN SONU
-- gösterilir; aksi hâlde aynı kişi listede iki kez çıkar ve yorum sayısı
-- gerçekte olduğundan kalabalık görünür.
WITH sonuncu AS (
    SELECT DISTINCT ON (r.user_id)
           r.public_id, r.rating, r.body, r.reviewed_at, u.username
    FROM reviews r
    JOIN users u ON u.id = r.user_id
    WHERE r.status = 'APPROVED'
    ORDER BY r.user_id, r.reviewed_at DESC, r.id DESC
)
SELECT public_id, rating, body, reviewed_at, username
FROM sonuncu
ORDER BY reviewed_at DESC
LIMIT sqlc.arg('lim');

-- name: ApprovedReviewStats :one
-- Sitedeki özet: kaç onaylı yorum ve ortalama puan.
--
-- 🔴 ORTALAMA ONDA BİRLİK TAM SAYI OLARAK döner (4.7 → 47), kayan nokta
-- olarak değil. Para değil ama aynı gerekçe geçerli: `float64` üzerinden
-- taşınan bir ortalama JSON'da 4.699999999999999 olur ve arayüzde iki farklı
-- yerde iki farklı yuvarlanır. Bölme TEK YERDE, gösterimde yapılır.
-- Yorum yoksa total 0 gelir ve ARAYÜZ BÖLÜMÜ HİÇ RENDER ETMEZ.
WITH sonuncu AS (
    SELECT DISTINCT ON (r.user_id) r.rating
    FROM reviews r
    WHERE r.status = 'APPROVED'
    ORDER BY r.user_id, r.reviewed_at DESC, r.id DESC
)
SELECT count(*)::bigint AS total,
       COALESCE(round(avg(rating) * 10), 0)::bigint AS average_x10
FROM sonuncu;
