package wallet

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
)

// mapLedgerErr veritabanı kısıt ihlallerini anlamlı hatalara çevirir.
//
// Uygulama kontrolleri tek savunma DEĞİLDİR: yarış durumunda iki istek
// idempotency kontrolünü aynı anda geçip ikisi de INSERT deneyebilir.
// UNIQUE indeks son sözü söyler ve burada tekrar olarak yorumlanır.
func mapLedgerErr(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return apperr.Internal(err)
	}
	switch {
	case pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "idempotency"):
		// Eşzamanlı tekrar: diğer istek kazandı. Çağıran açısından işlem
		// BAŞARILIDIR — çift uygulamayı zaten önlemek istiyorduk.
		return errDuplicateKey
	case pgErr.Code == "23514" && strings.Contains(pgErr.ConstraintName, "balance_non_negative"):
		return apperr.ErrInsufficientBalance
	case pgErr.Code == "23514" && strings.Contains(pgErr.ConstraintName, "amount_nonzero"):
		return apperr.Internal(err)
	default:
		return apperr.Internal(err)
	}
}

// errDuplicateKey iç sinyaldir; dışarı sızmaz.
// test: wallet_integration_test.go#TestKK201_IdempotencyUnderConcurrency
var errDuplicateKey = errors.New("wallet: eşzamanlı tekrar")

// IsDuplicate eşzamanlı tekrar hatasını tanır.
func IsDuplicate(err error) bool { return errors.Is(err, errDuplicateKey) }
