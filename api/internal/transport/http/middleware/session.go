package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// SessionCookieName oturum çerezi.
const SessionCookieName = "sid"

// Gin context anahtarları.
const (
	CtxUserID      = "user_id"
	CtxSessionID   = "session_id"
	CtxPermissions = "permissions"
	CtxUserStatus  = "user_status"
)

// AuthDeps oturum ara katmanının bağımlılıkları.
type AuthDeps struct {
	Sessions   port.SessionStore
	Queries    *db.Queries
	SessionTTL time.Duration
	// FailFn hatayı istemciye yazar (transport paketinden enjekte edilir;
	// döngüsel import olmasın diye fonksiyon olarak alınır).
	FailFn func(c *gin.Context, err error)
}

// RequireAuth oturum çerezini doğrular ve kullanıcıyı context'e koyar.
//
// Rol ve izinler HER İSTEKTE taze okunur. JWT'de olduğu gibi token'a gömülselerdi
// bir kullanıcının yetkisi alındığında token ömrü boyunca geçerli kalırdı
// (docs/design.md ADR-003).
func RequireAuth(d AuthDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		sid, err := c.Cookie(SessionCookieName)
		if err != nil || sid == "" {
			d.FailFn(c, apperr.ErrUnauthenticated)
			return
		}

		sess, err := d.Sessions.Get(c.Request.Context(), sid)
		if err != nil {
			ClearSessionCookie(c)
			d.FailFn(c, apperr.ErrUnauthenticated)
			return
		}

		user, err := d.Queries.GetUserByID(c.Request.Context(), sess.UserID)
		if err != nil {
			// Kullanıcı silinmiş ama oturumu duruyorsa oturumu da düşür.
			_ = d.Sessions.Revoke(c.Request.Context(), sid)
			ClearSessionCookie(c)
			d.FailFn(c, apperr.ErrUnauthenticated)
			return
		}

		// Askıya alınan kullanıcının açık sekmesindeki bir sonraki istek
		// anında reddedilir (docs/trd.md KK-104).
		if user.Status == db.UserStatusSUSPENDED {
			_ = d.Sessions.RevokeAllForUser(c.Request.Context(), user.ID)
			ClearSessionCookie(c)
			d.FailFn(c, apperr.ErrAccountSuspended)
			return
		}

		perms, err := d.Queries.GetUserPermissions(c.Request.Context(), user.ID)
		if err != nil {
			d.FailFn(c, apperr.Internal(err))
			return
		}

		c.Set(CtxUserID, user.ID)
		c.Set(CtxSessionID, sid)
		c.Set(CtxPermissions, perms)
		c.Set(CtxUserStatus, string(user.Status))

		// Kayan pencere: oturum her istekte tazelenir.
		// Başarısız olursa istek REDDEDİLMEZ — oturum yine geçerlidir.
		if err := d.Sessions.Touch(c.Request.Context(), sid, d.SessionTTL); err != nil {
			slog.Warn("oturum süresi uzatılamadı", "err", err)
		}

		c.Next()
	}
}

// RequireVerifiedEmail e-postası doğrulanmamış kullanıcıyı engeller.
// Satın alma ve bakiye yükleme bu kontrolü gerektirir (docs/trd.md FR-101).
func RequireVerifiedEmail(fail func(*gin.Context, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if status, _ := c.Get(CtxUserStatus); status == string(db.UserStatusPENDINGVERIFICATION) {
			fail(c, apperr.ErrEmailNotVerified)
			return
		}
		c.Next()
	}
}

// UserIDFrom context'ten kullanıcı kimliğini okur.
func UserIDFrom(c *gin.Context) (int64, bool) {
	v, ok := c.Get(CtxUserID)
	if !ok {
		return 0, false
	}
	id, ok := v.(int64)
	return id, ok
}

// SetSessionCookie oturum çerezini yazar.
//
// SameSite=Lax + tek alan adı (ters vekil) topolojisi sayesinde CORS ve çapraz
// alan çerez sorunu yoktur (docs/design.md §10).
func SetSessionCookie(c *gin.Context, sid string, ttl time.Duration, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sid,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true, // JavaScript okuyamaz — XSS ile çalınamaz
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie çerezi siler.
func ClearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: SessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

var _ = context.Background
