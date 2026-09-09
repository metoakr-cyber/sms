package review

import (
	"net/http"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
)

// Müşteri yorumu hata kataloğu.
//
// Kullanıcı ASLA ham veritabanı hatası görmez (değişmez #12): her biri Türkçe
// bir metin ve açık bir HTTP kodu taşır. "duplicate key value violates unique
// constraint reviews_one_pending_per_user_idx" bir kullanıcı mesajı değildir.
//
// test: internal/transport/http/handler/review_integration_test.go#TestUserCannotSeeOthersReview
var (
	// ErrNotFound yorum yok VEYA başkasına ait.
	//
	// İKİSİ AYNI HATAYA DÜŞER, bilerek: "bu yorum başkasının" demek geçerli
	// yorum kimliklerinin varlığını sızdırır ve numaralandırma saldırısına
	// kapı açar (ErrQuoteNotFound / ticket ErrNotFound ile aynı gerekçe).
	ErrNotFound = apperr.NewStatus(apperr.KindDomain, "REVIEW_NOT_FOUND",
		"Yorum bulunamadı.", http.StatusNotFound)

	// ErrPendingExists kullanıcının zaten karar bekleyen bir yorumu var.
	//
	// Metin ne yapılacağını söyler: kullanıcı "neden gönderemiyorum?" diye
	// destek talebi açmasın.
	ErrPendingExists = apperr.NewStatus(apperr.KindDomain, "REVIEW_PENDING_EXISTS",
		"Onay bekleyen bir yorumunuz zaten var. O sonuçlanınca yenisini yazabilirsiniz.",
		http.StatusConflict)

	// ErrAlreadyDecided yorum karara bağlanmış; ikinci karar yazılamaz.
	//
	// İki yönetici aynı yoruma aynı anda baktığında ikincisi bunu görür —
	// "onayladım ama reddedilmiş görünüyor" durumu oluşmaz.
	ErrAlreadyDecided = apperr.NewStatus(apperr.KindDomain, "REVIEW_ALREADY_DECIDED",
		"Bu yorum için karar zaten verilmiş.", http.StatusConflict)

	// ErrReasonRequired gerekçesiz red denemesi.
	ErrReasonRequired = apperr.NewStatus(apperr.KindDomain, "REVIEW_REASON_REQUIRED",
		"Red gerekçesi zorunludur.", http.StatusUnprocessableEntity)
)
