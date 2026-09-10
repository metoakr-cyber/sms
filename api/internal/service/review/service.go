// Package review müşteri yorumu kullanım senaryolarını yürütür.
//
// İki akış vardır ve ikisi de bu pakettedir:
//
//	kullanıcı → yorum gönder, kendi yorumlarını ve durumlarını oku
//	yönetici  → kuyruğu listele, onayla / reddet
//
// Bu paket dış HTTP çağrısı YAPMAZ ve para hareketi üretmez. Karar yazma TEK
// TRANSACTION'dır: satır kilidi → durum kararı (domain/review) → yazım.
// Kilitsiz yazmak, aynı yoruma iki yöneticinin aynı anda karar vermesi ve
// ikinci kararın birinciyi sessizce ezmesi demekti.
package review

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	reviewdom "github.com/ikmetrik/sms-platform/api/internal/domain/review"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// PublicListLimit sitede gösterilecek en fazla yorum sayısı.
//
// Sayfalama YOKTUR ve gerekmez: bu bir pazarlama bölümüdür, arşiv değil.
// Sınırsız bırakmak, katalog büyüdüğünde ana sayfayı yavaşlatırdı.
const PublicListLimit = 24

type txRunner interface {
	Queries() *db.Queries
	InTx(ctx context.Context, fn func(*db.Queries) error) error
}

// Deps servis bağımlılıkları.
type Deps struct {
	TxRunner txRunner
	Clock    port.Clock
}

// Service müşteri yorumu servisi.
type Service struct {
	tx    txRunner
	clock port.Clock
}

func New(d Deps) *Service { return &Service{tx: d.TxRunner, clock: d.Clock} }

/* ═══════════════════════ Kullanıcı akışı ═══════════════════════ */

// SubmitInput yeni yorum.
type SubmitInput struct {
	UserID int64
	Rating int
	Body   string
}

// Submit kullanıcının yorumunu PENDING olarak kaydeder.
//
// 🔴 "Bu kullanıcının bekleyen yorumu var mı?" diye ÖNCE SORULMAZ.
// Kısıt kısmi benzersiz indekstedir (reviews_one_pending_per_user_idx) ve
// ihlal burada 409'a çevrilir. Uygulama katmanındaki bir "önce SELECT sonra
// INSERT" kontrolünü iki eşzamanlı istek İKİSİ DE geçerdi ve kuyrukta aynı
// kullanıcının iki yorumu olurdu.
//
// test: review_integration_test.go#TestConcurrentSubmitsLeaveOnePending
func (s *Service) Submit(ctx context.Context, in SubmitInput) (db.Review, error) {
	body := strings.TrimSpace(in.Body)

	row, err := s.tx.Queries().CreateReview(ctx, db.CreateReviewParams{
		UserID: in.UserID,
		Rating: int16(in.Rating),
		Body:   body,
	})
	if err != nil {
		return db.Review{}, mapDBErr(err)
	}
	return row, nil
}

// List kullanıcının KENDİ yorumlarını ve durumlarını döner.
// `status` NIL ise durum süzgeci UYGULANMAZ. Sayım listeyle AYNI süzgeci alır.
//
// 🔴 `userID` süzgeçten AYRI parametredir ve sorguya her zaman girer
// (değişmez #7); hiçbir `status` değeri başkasının yorumunu döndüremez.
func (s *Service) List(
	ctx context.Context, userID int64, status *reviewdom.Status, limit, offset int32,
) (
	[]db.Review, int64, error,
) {
	// Alan adı dönüşümü `AdminList` ile aynı: domain tipi veritabanı enum'una
	// SERVİSTE çevrilir, transport katmanı `db` tipini hiç tanımaz.
	var filter *db.ReviewStatus
	if status != nil {
		v := db.ReviewStatus(*status)
		filter = &v
	}
	q := s.tx.Queries()
	rows, err := q.ListReviewsForUser(ctx, db.ListReviewsForUserParams{
		UserID: userID, Status: filter, Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	total, err := q.CountReviewsForUser(ctx, db.CountReviewsForUserParams{
		UserID: userID, Status: filter,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return rows, total, nil
}

// Get kullanıcının KENDİ yorumunu döner.
//
// Sahiplik sorgunun parçasıdır (değişmez #7); burada ayrı bir if yoktur.
// Başkasının yorumu ile var olmayan yorum AYNI yanıtı (404) alır.
//
// test: internal/transport/http/handler/review_integration_test.go#TestUserCannotSeeOthersReview
func (s *Service) Get(ctx context.Context, userID int64, publicID uuid.UUID) (db.Review, error) {
	row, err := s.tx.Queries().GetReviewForUser(ctx, db.GetReviewForUserParams{
		PublicID: publicID, UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Review{}, ErrNotFound
		}
		return db.Review{}, apperr.Internal(err)
	}
	return row, nil
}

/* ═══════════════════════ Sitedeki liste (oturumsuz) ═══════════════════════ */

// PublicReview sitede gösterilen yorum.
//
// 🔴 E-POSTA VE SAYISAL KİMLİK BU YAPIDA YOKTUR ve sorgu da onları SEÇMEZ
// (queries/reviews.sql, ListApprovedReviews). "DTO'da atarız" yeterli
// değildir: alan yapıda dururken bir gün birinin onu yanıta koyması bir satır
// uzaklıktadır.
//
// test: internal/transport/http/handler/review_integration_test.go#TestPublicReviewsNeverExposeEmail
type PublicReview struct {
	PublicID   uuid.UUID
	Rating     int16
	Body       string
	AuthorName string
	// PublishedAt onay anıdır, yazılma anı değil: sitede "ne zaman
	// yayımlandı" sorusunun cevabı budur.
	PublishedAt *time.Time
}

// PublicSummary sitedeki özet: kaç yorum, ortalama kaç.
type PublicSummary struct {
	Total int64
	// AverageX10 ortalamanın ONDA BİRLİK TAM SAYI hâli (4.7 → 47).
	// Kayan nokta taşınmaz; bölme yalnız gösterimde yapılır.
	AverageX10 int64
}

// PublicList onaylı yorumları ve özeti döner.
//
// Hiç onaylı yorum yoksa BOŞ dilim ve Total=0 döner; arayüz bölümü hiç
// render etmez. Servis burada uydurma bir varsayılan ÜRETMEZ.
func (s *Service) PublicList(ctx context.Context) ([]PublicReview, PublicSummary, error) {
	q := s.tx.Queries()

	rows, err := q.ListApprovedReviews(ctx, PublicListLimit)
	if err != nil {
		return nil, PublicSummary{}, apperr.Internal(err)
	}
	out := make([]PublicReview, 0, len(rows))
	for _, r := range rows {
		out = append(out, PublicReview{
			PublicID:    r.PublicID,
			Rating:      r.Rating,
			Body:        r.Body,
			AuthorName:  r.Username,
			PublishedAt: r.ReviewedAt,
		})
	}

	st, err := q.ApprovedReviewStats(ctx)
	if err != nil {
		return nil, PublicSummary{}, apperr.Internal(err)
	}
	return out, PublicSummary{Total: st.Total, AverageX10: st.AverageX10}, nil
}

/* ═══════════════════════ Yönetim akışı ═══════════════════════ */

// AdminFilter yönetim listesinin süzgeci.
type AdminFilter struct {
	Status *reviewdom.Status
	// Q serbest metin: yorum gövdesi + kullanıcı e-postası/adı. NIL ise
	// aranmaz; boş dize "hiçbir şeyle eşleşme" değil, "arama yok" demektir.
	Q *string
}

// AdminList süzgeçle tüm yorumları döner.
//
// Sahiplik kısıtı YOKTUR; koruma izin ara katmanındadır (reviews:read).
func (s *Service) AdminList(ctx context.Context, f AdminFilter, limit, offset int32) (
	[]db.ListReviewsForAdminRow, int64, int64, error,
) {
	var filter *db.ReviewStatus
	if f.Status != nil {
		v := db.ReviewStatus(*f.Status)
		filter = &v
	}
	q := s.tx.Queries()
	rows, err := q.ListReviewsForAdmin(ctx, db.ListReviewsForAdminParams{
		Status: filter, Q: f.Q, Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, 0, 0, apperr.Internal(err)
	}
	// Sayım listeyle AYNI süzgeci alır (queries/reviews.sql notu).
	total, err := q.CountReviewsForAdmin(ctx, db.CountReviewsForAdminParams{
		Status: filter, Q: f.Q,
	})
	if err != nil {
		return nil, 0, 0, apperr.Internal(err)
	}
	// Bekleyen sayısı SÜZGEÇTEN BAĞIMSIZ okunur: yönetici "onaylananlar"
	// sekmesindeyken de kuyrukta kaç iş kaldığını görmeli.
	pending, err := q.CountPendingReviews(ctx)
	if err != nil {
		return nil, 0, 0, apperr.Internal(err)
	}
	return rows, total, pending, nil
}

// Decide yöneticinin onay/red kararını yazar.
//
// Karar domain/review'a aittir: geçiş doğrulaması, gerekçe zorunluluğu ve
// onayda gerekçenin temizlenmesi orada yapılır (değişmez #13). Bu metot
// kilidi alır, kararı sorar ve sonucu yazar.
//
// test: review_integration_test.go#TestApprovedReviewCanBeUnpublishedButNotReQueued
func (s *Service) Decide(
	ctx context.Context, staffUserID int64, publicID uuid.UUID,
	target reviewdom.Status, reason string,
) (db.Review, error) {
	var out db.Review
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		cur, err := q.LockReview(ctx, publicID)
		if err != nil {
			return err
		}

		decision, err := reviewdom.Decide(reviewdom.Status(cur.Status), target, reason)
		if err != nil {
			return err
		}

		now := s.clock.Now()
		staff := staffUserID
		out, err = q.DecideReview(ctx, db.DecideReviewParams{
			PublicID:         publicID,
			Status:           db.ReviewStatus(decision.Status),
			RejectionReason:  decision.Reason,
			ReviewedByUserID: &staff,
			ReviewedAt:       &now,
		})
		return err
	})
	if err != nil {
		return db.Review{}, mapTxErr(err)
	}
	return out, nil
}

/* ═══════════════════════ Hata haritalaması ═══════════════════════ */

// mapDBErr veritabanı kısıt ihlallerini kullanıcıya gösterilebilir hâle çevirir.
//
// Kullanıcı ham `duplicate key value violates unique constraint …` metnini
// GÖRMEZ (değişmez #12).
//
// test: review_integration_test.go#TestConcurrentSubmitsLeaveOnePending
func mapDBErr(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return apperr.Internal(err)
	}
	switch {
	case pgErr.Code == "23505" &&
		strings.Contains(pgErr.ConstraintName, "reviews_one_pending_per_user_idx"):
		return ErrPendingExists
	case pgErr.Code == "23514" && strings.Contains(pgErr.ConstraintName, "review_body_len"):
		// DTO doğrulaması ilk savunmadır; buraya düşmek onun atlandığı
		// anlamına gelir ve kullanıcı yine anlaşılır bir metin görmeli.
		return apperr.ErrValidation.WithMessage("Yorum 10-1000 karakter olmalıdır.")
	case pgErr.Code == "23514" && strings.Contains(pgErr.ConstraintName, "review_rating_range"):
		return apperr.ErrValidation.WithMessage("Puan 1 ile 5 arasında olmalıdır.")
	default:
		return apperr.Internal(err)
	}
}

// mapTxErr transaction içinden çıkan hataları kullanıcıya gösterilebilir
// hâle çevirir. Tipli hatalar olduğu gibi geçer.
func mapTxErr(err error) error {
	var inv reviewdom.ErrInvalidTransition
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case errors.Is(err, reviewdom.ErrReasonRequired):
		return ErrReasonRequired
	case errors.As(err, &inv):
		// Geçiş reddedildi: yorum zaten karara bağlanmış demektir.
		return ErrAlreadyDecided
	}
	if _, ok := apperr.As(err); ok {
		return err
	}
	return mapDBErr(err)
}
