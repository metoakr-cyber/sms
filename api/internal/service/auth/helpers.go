package auth

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
)

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// mapUniqueViolation veritabanının UNIQUE indeks ihlalini alan adına göre
// anlamlı bir uygulama hatasına çevirir.
//
// Uygulama içi "var mı" kontrolü tek savunma DEĞİLDİR: iki eşzamanlı kayıt
// isteği kontrolü aynı anda geçip ikisi de INSERT deneyebilir. Son sözü
// veritabanı söyler; burası o sözü kullanıcıya çevirir.
func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch {
	case strings.Contains(pgErr.ConstraintName, "email"):
		return apperr.ErrEmailTaken
	case strings.Contains(pgErr.ConstraintName, "username"):
		return apperr.ErrUsernameTaken
	default:
		return err
	}
}

// wrapDBErr uygulama hatalarını olduğu gibi geçirir, ham veritabanı hatalarını
// sarmalar. Ham hata kullanıcıya asla gösterilmez.
// test: scripts/smoke-auth.sh — hata yanıtlarında yalnız katalog metni döner
func wrapDBErr(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := apperr.As(err); ok {
		return err
	}
	if mapped := mapUniqueViolation(err); mapped != err {
		return mapped
	}
	return apperr.Internal(err)
}
