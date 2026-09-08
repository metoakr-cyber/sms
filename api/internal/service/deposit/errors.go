package deposit

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/storage"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
)

// Bakiye yükleme hata kataloğu.
//
// Kullanıcı ASLA ham veritabanı hatası görmez (değişmez #12): her biri Türkçe
// bir metin ve açık bir HTTP kodu taşır.
//
// test: deposit_integration_test.go#TestDuplicateTxHashRejected
var (
	// ErrNotFound talep yok VEYA başkasına ait.
	//
	// İKİSİ AYNI HATAYA DÜŞER, bilerek: "bu talep başkasının" demek geçerli
	// talep kimliklerinin varlığını sızdırır (ErrQuoteNotFound ile aynı gerekçe).
	ErrNotFound = apperr.NewStatus(apperr.KindDomain, "DEPOSIT_NOT_FOUND",
		"Yükleme talebi bulunamadı.", http.StatusNotFound)

	// ErrNotPending talep zaten onaylanmış ya da reddedilmiş.
	ErrNotPending = apperr.NewStatus(apperr.KindDomain, "DEPOSIT_NOT_PENDING",
		"Bu talep zaten sonuçlandırılmış.", http.StatusConflict)

	// ErrTxHashDuplicate aynı işlem hash'i ikinci kez bildirildi (KK-501).
	ErrTxHashDuplicate = apperr.NewStatus(apperr.KindDomain, "TX_HASH_DUPLICATE",
		"Bu işlem numarası daha önce bildirilmiş. Destek ile iletişime geçin.",
		http.StatusConflict)

	// ErrMethodInactive yöntem pasif VEYA hiç yok (aynı gerekçe: numaralandırma).
	ErrMethodInactive = apperr.NewStatus(apperr.KindDomain, "DEPOSIT_METHOD_INACTIVE",
		"Bu ödeme yöntemi şu an kullanılamıyor.", http.StatusConflict)

	// ErrAmountRange tutar yöntemin ya da tablonun sınırları dışında.
	ErrAmountRange = apperr.NewStatus(apperr.KindDomain, "DEPOSIT_AMOUNT_RANGE",
		"Yükleme tutarı bu yöntem için izin verilen aralığın dışında.",
		http.StatusUnprocessableEntity)

	// ErrReceiptTooLarge dekont 5 MB sınırını aştı.
	ErrReceiptTooLarge = apperr.NewStatus(apperr.KindDomain, "RECEIPT_TOO_LARGE",
		"Dekont dosyası en fazla 5 MB olabilir.", http.StatusRequestEntityTooLarge)

	// ErrReceiptUnsupported sihirli bayt doğrulaması başarısız (KK-500).
	ErrReceiptUnsupported = apperr.NewStatus(apperr.KindDomain, "RECEIPT_UNSUPPORTED",
		"Yalnız JPEG, PNG veya PDF dosyası yükleyebilirsiniz.",
		http.StatusUnprocessableEntity)

	// ErrReceiptMissing talebe dekont eklenmemiş.
	ErrReceiptMissing = apperr.NewStatus(apperr.KindDomain, "RECEIPT_NOT_FOUND",
		"Bu talebe ait dekont bulunamadı.", http.StatusNotFound)

	// ErrReceiptDisabled dosya deposu kurulmamış (UPLOAD_DIR tanımsız).
	ErrReceiptDisabled = apperr.NewStatus(apperr.KindInfra, "RECEIPT_DISABLED",
		"Dekont yükleme şu an kullanılamıyor.", http.StatusServiceUnavailable)
)

// mapDBErr veritabanı kısıt ihlallerini anlamlı hatalara çevirir.
//
// Uygulama kontrolleri tek savunma değildir: iki istek aynı tx_hash'i aynı
// anda gönderirse ikisi de "bu hash yeni" diye düşünebilir; kısmi UNIQUE
// indeks son sözü söyler ve burada 409'a çevrilir.
//
// test: deposit_integration_test.go#TestDuplicateTxHashRejected
func mapDBErr(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return apperr.Internal(err)
	}
	switch {
	case pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "deposits_tx_hash_uniq"):
		return ErrTxHashDuplicate
	case pgErr.Code == "23514" && strings.Contains(pgErr.ConstraintName, "deposit_amount_range"):
		// Yönetici yöntemin sınırını tablo CHECK'inin ötesine çekmiş olabilir;
		// tablo son savunmadır ve kullanıcı 500 değil Türkçe bir uyarı görmeli.
		return ErrAmountRange
	default:
		// deposit_reviewed_has_time gibi kısıtlar programlama hatasıdır:
		// kullanıcıya gösterilecek anlamlı bir karşılığı yoktur.
		return apperr.Internal(err)
	}
}

// mapStorageErr depo hatalarını kullanıcıya gösterilebilir hâle çevirir.
func mapStorageErr(err error) error {
	switch {
	case errors.Is(err, storage.ErrTooLarge):
		return ErrReceiptTooLarge
	case errors.Is(err, storage.ErrUnsupportedType), errors.Is(err, storage.ErrEmpty):
		return ErrReceiptUnsupported
	case errors.Is(err, storage.ErrNotFound), errors.Is(err, storage.ErrBadPath):
		return ErrReceiptMissing
	default:
		return apperr.Internal(err)
	}
}

// isUniqueViolation belirli bir tekil indeks ihlali mi.
func isUniqueViolation(err error, kısıt string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, kısıt)
}
