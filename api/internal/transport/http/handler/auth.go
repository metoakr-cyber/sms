// Package handler HTTP işleyicilerini içerir.
//
// İşleyiciler İNCEDİR: gövdeyi bağla → doğrula → servisi çağır → yanıtı yaz.
// İş kuralı, veritabanı erişimi ve dış API çağrısı burada OLMAZ.
package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	authsvc "github.com/ikmetrik/sms-platform/api/internal/service/auth"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Responder yanıt yazma fonksiyonları (transport paketinden enjekte edilir).
type Responder struct {
	OK        func(*gin.Context, any)
	NoContent func(*gin.Context)
	Fail      func(*gin.Context, error)
	FailField func(*gin.Context, []dto.FieldError)
}

// Auth kimlik uç noktalarını yönetir.
type Auth struct {
	svc        *authsvc.Service
	queries    *db.Queries
	r          Responder
	sessionTTL time.Duration
	secure     bool // Secure çerez bayrağı (üretimde true)
}

func NewAuth(svc *authsvc.Service, q *db.Queries, r Responder, ttl time.Duration, secure bool) *Auth {
	return &Auth{svc: svc, queries: q, r: r, sessionTTL: ttl, secure: secure}
}

// Register POST /auth/register
func (h *Auth) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}

	err := h.svc.Register(c.Request.Context(), authsvc.RegisterInput{
		Email: req.Email, Username: req.Username, Password: req.Password,
		CaptchaToken: req.CaptchaToken, IP: c.ClientIP(),
	})
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, dto.MessageResponse{
		Message: "Kaydınız alındı. E-posta adresinize gönderilen bağlantıya tıklayarak hesabınızı etkinleştirin.",
	})
}

// VerifyEmail POST /auth/verify-email
func (h *Auth) VerifyEmail(c *gin.Context) {
	var req dto.TokenRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	if err := h.svc.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, dto.MessageResponse{Message: "E-posta adresiniz doğrulandı. Artık giriş yapabilirsiniz."})
}

// Login POST /auth/login
func (h *Auth) Login(c *gin.Context) {
	var req dto.LoginRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}

	sess, err := h.svc.Login(c.Request.Context(), authsvc.LoginInput{
		Email: req.Email, Password: req.Password, CaptchaToken: req.CaptchaToken,
		IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		h.r.Fail(c, err)
		return
	}

	middleware.SetSessionCookie(c, sess.ID, h.sessionTTL, h.secure)
	h.r.OK(c, dto.MessageResponse{Message: "Giriş başarılı."})
}

// Logout POST /auth/logout
func (h *Auth) Logout(c *gin.Context) {
	if sid, ok := c.Get(middleware.CtxSessionID); ok {
		if s, ok := sid.(string); ok {
			_ = h.svc.Logout(c.Request.Context(), s)
		}
	}
	middleware.ClearSessionCookie(c)
	h.r.OK(c, dto.MessageResponse{Message: "Çıkış yapıldı."})
}

// ForgotPassword POST /auth/password/forgot
//
// Kullanıcı olsun olmasın AYNI yanıtı döner: aksi halde bu uç nokta bir hesap
// sayım aracına dönüşür (docs/trd.md KK-103).
func (h *Auth) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	if err := h.svc.ForgotPassword(c.Request.Context(), req.Email, c.ClientIP()); err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, dto.MessageResponse{
		Message: "Eğer bu e-posta adresiyle bir hesap varsa, sıfırlama bağlantısı gönderildi.",
	})
}

// ResetPassword POST /auth/password/reset
func (h *Auth) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), req.Token, req.Password); err != nil {
		h.r.Fail(c, err)
		return
	}
	middleware.ClearSessionCookie(c)
	h.r.OK(c, dto.MessageResponse{
		Message: "Şifreniz güncellendi. Güvenliğiniz için tüm oturumlarınız kapatıldı.",
	})
}

// Me GET /me
func (h *Auth) Me(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	u, err := h.queries.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		h.r.Fail(c, apperr.Internal(err))
		return
	}
	perms, _ := c.Get(middleware.CtxPermissions)
	permList, _ := perms.([]string)
	if permList == nil {
		permList = []string{}
	}

	h.r.OK(c, dto.UserResponse{
		ID:            u.PublicID.String(),
		Email:         u.Email,
		Username:      u.Username,
		Status:        string(u.Status),
		EmailVerified: u.EmailVerifiedAt != nil,
		Balance:       moneyDTO(money.New(u.BalanceMinor, money.TRY)),
		Permissions:   permList,
		CreatedAt:     u.CreatedAt.Format(time.RFC3339),
	})
}

// ListSessions GET /me/sessions
func (h *Auth) ListSessions(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	current, _ := c.Get(middleware.CtxSessionID)
	currentID, _ := current.(string)

	rows, err := h.svc.ListSessions(c.Request.Context(), userID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	out := make([]dto.SessionResponse, 0, len(rows))
	for _, s := range rows {
		// ID olarak HANDLE verilir. Ham oturum kimliği bir taşıyıcı token'dır
		// ve JSON'a konursa çerezin httpOnly korumasını etkisiz kılar.
		out = append(out, dto.SessionResponse{
			ID:         authsvc.SessionHandle(s.ID),
			UserAgent:  s.UserAgent,
			IP:         ipString(s.Ip),
			CreatedAt:  s.CreatedAt.Format(time.RFC3339),
			LastSeenAt: s.LastSeenAt.Format(time.RFC3339),
			Current:    s.ID == currentID, // karşılaştırma SUNUCU tarafında
		})
	}
	h.r.OK(c, gin.H{"items": out})
}

// RevokeSession DELETE /me/sessions/:id
func (h *Auth) RevokeSession(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	// Sahiplik kontrolü servis katmanında yapılır: başkasının oturumu ile
	// var olmayan oturum AYNI hatayı döner (varlık sızdırılmaz).
	if err := h.svc.RevokeSession(c.Request.Context(), userID, c.Param("id")); err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.NoContent(c)
}

// moneyDTO para değerini değişmez gösterime çevirir.
func moneyDTO(m money.Money) dto.Money {
	return dto.Money{
		Minor:     m.Minor(),
		Currency:  m.Currency().Code,
		Formatted: formatTRY(m),
	}
}
