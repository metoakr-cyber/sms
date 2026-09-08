// Package auth kimlik kullanım senaryolarını yürütür.
//
// Bu katman HTTP'yi tanımaz (gin.Context almaz) ve somut adaptörleri bilmez;
// yalnız port arayüzlerini kullanır.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	domauth "github.com/ikmetrik/sms-platform/api/internal/domain/auth"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// Token geçerlilik süreleri — docs/trd.md FR-101, FR-103.
const (
	emailVerificationTTL = 24 * time.Hour
	passwordResetTTL     = 1 * time.Hour

	// Giriş denemesi limiti — docs/trd.md KK-102.
	loginAttemptLimit  = 5
	loginAttemptWindow = 15 * time.Minute
)

// DefaultUserRole yeni kayıtlara atanan rol.
const DefaultUserRole = "user"

type Service struct {
	tx       *postgres.TxRunner
	sessions port.SessionStore
	mailer   port.Mailer
	captcha  port.Captcha
	limiter  port.RateLimiter
	clock    port.Clock

	sessionTTL time.Duration
	baseURL    string
}

type Deps struct {
	TxRunner   *postgres.TxRunner
	Sessions   port.SessionStore
	Mailer     port.Mailer
	Captcha    port.Captcha
	Limiter    port.RateLimiter
	Clock      port.Clock
	SessionTTL time.Duration
	BaseURL    string
}

func New(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = port.RealClock{}
	}
	return &Service{
		tx: d.TxRunner, sessions: d.Sessions, mailer: d.Mailer,
		captcha: d.Captcha, limiter: d.Limiter, clock: d.Clock,
		sessionTTL: d.SessionTTL, baseURL: d.BaseURL,
	}
}

// ─────────────────────────── Kayıt ───────────────────────────

type RegisterInput struct {
	Email        string
	Username     string
	Password     string
	CaptchaToken string
	IP           string
}

// Register yeni bir kullanıcı oluşturur ve doğrulama e-postası gönderir.
func (s *Service) Register(ctx context.Context, in RegisterInput) error {
	in.Email = normalizeEmail(in.Email)
	in.Username = strings.TrimSpace(in.Username)

	if s.captcha.Enabled() {
		if err := s.captcha.Verify(ctx, in.CaptchaToken, in.IP); err != nil {
			return err
		}
	}

	// Şifre gücü: e-posta ve kullanıcı adı şifrenin içinde geçemez.
	if p := domauth.CheckPasswordStrength(in.Password, in.Email, in.Username); p != domauth.PasswordOK {
		return apperr.ErrWeakPassword.WithMessage(p.Message())
	}

	hash, err := domauth.HashPassword(in.Password)
	if err != nil {
		return apperr.Internal(err)
	}

	tok, err := domauth.NewToken()
	if err != nil {
		return apperr.Internal(err)
	}

	var created db.User
	err = s.tx.InTx(ctx, func(q *db.Queries) error {
		// Benzersizlik kontrolü burada YAPILIR ama tek savunma değildir:
		// yarış durumunda veritabanı UNIQUE indeksi son sözü söyler (aşağıda).
		if exists, err := q.EmailExists(ctx, in.Email); err != nil {
			return err
		} else if exists {
			return apperr.ErrEmailTaken
		}
		if exists, err := q.UsernameExists(ctx, in.Username); err != nil {
			return err
		} else if exists {
			return apperr.ErrUsernameTaken
		}

		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email:        in.Email,
			Username:     in.Username,
			PasswordHash: hash,
			Status:       db.UserStatusPENDINGVERIFICATION,
		})
		if err != nil {
			return mapUniqueViolation(err)
		}
		created = u

		role, err := q.GetRoleByName(ctx, DefaultUserRole)
		if err != nil {
			return fmt.Errorf("varsayılan rol %q bulunamadı: %w", DefaultUserRole, err)
		}
		if err := q.AssignRole(ctx, db.AssignRoleParams{UserID: u.ID, RoleID: role.ID}); err != nil {
			return err
		}

		_, err = q.CreateAuthToken(ctx, db.CreateAuthTokenParams{
			UserID:    u.ID,
			TokenHash: tok.Hash,
			Purpose:   db.TokenPurposeEMAILVERIFICATION,
			ExpiresAt: s.clock.Now().Add(emailVerificationTTL),
		})
		return err
	})
	if err != nil {
		return wrapDBErr(err)
	}

	// E-posta gönderimi transaction DIŞINDA: yavaş bir SMTP çağrısı veritabanı
	// kilidini tutmamalı. Gönderim başarısız olsa bile kayıt tamamlanmıştır;
	// kullanıcı "doğrulama e-postasını tekrar gönder" diyebilir.
	s.sendVerificationEmail(ctx, created.Email, tok.Plain)
	return nil
}

// ─────────────────────────── E-posta doğrulama ───────────────────────────

func (s *Service) VerifyEmail(ctx context.Context, plainToken string) error {
	if plainToken == "" {
		return apperr.ErrTokenInvalid
	}
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		// Tüketim ATOMİKTİR: UPDATE ... RETURNING. Ayrı SELECT + UPDATE olsaydı
		// iki eşzamanlı istek aynı token'ı kullanabilirdi.
		row, err := q.ConsumeAuthToken(ctx, db.ConsumeAuthTokenParams{
			TokenHash: domauth.HashToken(plainToken),
			Purpose:   db.TokenPurposeEMAILVERIFICATION,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrTokenInvalid
		}
		if err != nil {
			return err
		}
		return q.MarkEmailVerified(ctx, row.UserID)
	})
	return wrapDBErr(err)
}

// ─────────────────────────── Giriş ───────────────────────────

type LoginInput struct {
	Email        string
	Password     string
	CaptchaToken string
	IP           string
	UserAgent    string
}

// Login kimlik doğrular ve oturum açar.
func (s *Service) Login(ctx context.Context, in LoginInput) (port.Session, error) {
	in.Email = normalizeEmail(in.Email)

	// Hesap bazlı kilit: aynı e-postaya yapılan denemeler sayılır.
	attemptKey := "login:" + in.Email
	allowed, retryAfter, err := s.limiter.Allow(ctx, attemptKey, loginAttemptLimit, loginAttemptWindow)
	if err != nil {
		return port.Session{}, apperr.Internal(err)
	}
	if !allowed {
		return port.Session{}, apperr.ErrTooManyAttempts.WithMessage(
			fmt.Sprintf("Çok fazla başarısız deneme. Lütfen %d dakika sonra tekrar deneyin.",
				int(retryAfter.Minutes())+1))
	}

	if s.captcha.Enabled() {
		if err := s.captcha.Verify(ctx, in.CaptchaToken, in.IP); err != nil {
			return port.Session{}, err
		}
	}

	q := s.tx.Queries()
	user, err := q.GetUserByEmail(ctx, in.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		// KULLANICI SAYIMI KORUMASI: var olmayan e-posta ile yanlış şifre
		// AYNI hatayı döner. Ayrıca sahte bir doğrulama yaparak zamanlama
		// farkını da kapatırız — aksi halde "e-posta var mı" zamanla ölçülebilir.
		_, _ = domauth.VerifyPassword(in.Password, dummyHash)
		return port.Session{}, apperr.ErrInvalidCredentials
	}
	if err != nil {
		return port.Session{}, apperr.Internal(err)
	}

	ok, err := domauth.VerifyPassword(in.Password, user.PasswordHash)
	if err != nil || !ok {
		return port.Session{}, apperr.ErrInvalidCredentials
	}

	if user.Status == db.UserStatusSUSPENDED {
		return port.Session{}, apperr.ErrAccountSuspended
	}

	// Başarılı giriş sayaçları sıfırlar.
	_ = s.limiter.Reset(ctx, attemptKey)

	// Argon2 maliyeti artırıldıysa kullanıcı sessizce yükseltilir.
	if domauth.NeedsRehash(user.PasswordHash) {
		if newHash, err := domauth.HashPassword(in.Password); err == nil {
			if err := q.UpdatePasswordHash(ctx, db.UpdatePasswordHashParams{
				ID: user.ID, PasswordHash: newHash,
			}); err != nil {
				slog.Warn("şifre özeti yükseltilemedi", "user_id", user.ID, "err", err)
			}
		}
	}

	return s.createSession(ctx, user.ID, in.IP, in.UserAgent)
}

// dummyHash var olmayan kullanıcıda zamanlama farkını kapatmak için kullanılır.
// Gerçek bir argon2id özetidir; hiçbir şifre bununla eşleşmez.
var dummyHash = "$argon2id$v=19$m=65536,t=3,p=4$" +
	"AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// createSession oturum kaydını hem Redis'e hem veritabanına yazar.
func (s *Service) createSession(ctx context.Context, userID int64, ip, ua string) (port.Session, error) {
	// Oturum kimliği HER girişte yeniden üretilir (oturum sabitleme koruması).
	id, err := domauth.NewSessionID()
	if err != nil {
		return port.Session{}, apperr.Internal(err)
	}
	now := s.clock.Now()
	sess := port.Session{
		ID: id, UserID: userID, IP: ip, UserAgent: truncate(ua, 400),
		CreatedAt: now, ExpiresAt: now.Add(s.sessionTTL),
	}

	if err := s.sessions.Create(ctx, sess); err != nil {
		return port.Session{}, apperr.Internal(err)
	}

	// Veritabanı kopyası "aktif oturumlarım" listesi içindir (FR-104).
	// Yazılamazsa giriş BAŞARISIZ SAYILMAZ — kaynak doğruluk Redis'tir.
	var ipAddr *netip.Addr
	if a, err := netip.ParseAddr(ip); err == nil {
		ipAddr = &a
	}
	if _, err := s.tx.Queries().CreateSession(ctx, db.CreateSessionParams{
		ID: id, UserID: userID, Ip: ipAddr,
		UserAgent: sess.UserAgent, ExpiresAt: sess.ExpiresAt,
	}); err != nil {
		slog.Warn("oturum veritabanına yazılamadı", "user_id", userID, "err", err)
	}
	return sess, nil
}

// ─────────────────────────── Çıkış ───────────────────────────

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	if err := s.sessions.Revoke(ctx, sessionID); err != nil {
		return apperr.Internal(err)
	}
	if err := s.tx.Queries().RevokeSession(ctx, sessionID); err != nil {
		slog.Warn("oturum veritabanında iptal edilemedi", "err", err)
	}
	return nil
}

// ─────────────────────────── Şifre sıfırlama ───────────────────────────

// ForgotPassword sıfırlama bağlantısı gönderir.
//
// Kullanıcı olsun olmasın AYNI sonucu döner (nil): aksi halde bu uç nokta bir
// hesap sayım aracına dönüşür.
func (s *Service) ForgotPassword(ctx context.Context, email, ip string) error {
	email = normalizeEmail(email)

	// Kötüye kullanım koruması: aynı adrese sınırsız e-posta gönderilemez.
	if allowed, _, err := s.limiter.Allow(ctx, "forgot:"+email, 3, time.Hour); err == nil && !allowed {
		return nil // sessizce yut — istemci farkı göremez
	}

	q := s.tx.Queries()
	user, err := q.GetUserByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("şifre sıfırlama: kullanıcı okunamadı", "err", err)
		}
		return nil
	}

	tok, err := domauth.NewToken()
	if err != nil {
		return apperr.Internal(err)
	}

	err = s.tx.InTx(ctx, func(q *db.Queries) error {
		// Önceki sıfırlama token'ları geçersizleşir: aynı anda birden çok
		// geçerli bağlantı olmamalı.
		if err := q.InvalidateUserTokens(ctx, db.InvalidateUserTokensParams{
			UserID: user.ID, Purpose: db.TokenPurposePASSWORDRESET,
		}); err != nil {
			return err
		}
		_, err := q.CreateAuthToken(ctx, db.CreateAuthTokenParams{
			UserID:    user.ID,
			TokenHash: tok.Hash,
			Purpose:   db.TokenPurposePASSWORDRESET,
			ExpiresAt: s.clock.Now().Add(passwordResetTTL),
		})
		return err
	})
	if err != nil {
		slog.Error("şifre sıfırlama token'ı oluşturulamadı", "err", err)
		return nil
	}

	s.sendPasswordResetEmail(ctx, user.Email, tok.Plain)
	return nil
}

// ResetPassword şifreyi değiştirir ve TÜM oturumları düşürür.
func (s *Service) ResetPassword(ctx context.Context, plainToken, newPassword string) error {
	if plainToken == "" {
		return apperr.ErrTokenInvalid
	}

	var userID int64
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		row, err := q.ConsumeAuthToken(ctx, db.ConsumeAuthTokenParams{
			TokenHash: domauth.HashToken(plainToken),
			Purpose:   db.TokenPurposePASSWORDRESET,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.ErrTokenInvalid
		}
		if err != nil {
			return err
		}
		userID = row.UserID

		user, err := q.GetUserByID(ctx, userID)
		if err != nil {
			return err
		}
		if p := domauth.CheckPasswordStrength(newPassword, user.Email, user.Username); p != domauth.PasswordOK {
			return apperr.ErrWeakPassword.WithMessage(p.Message())
		}
		hash, err := domauth.HashPassword(newPassword)
		if err != nil {
			return apperr.Internal(err)
		}
		if err := q.UpdatePasswordHash(ctx, db.UpdatePasswordHashParams{
			ID: userID, PasswordHash: hash,
		}); err != nil {
			return err
		}
		return q.RevokeAllUserSessions(ctx, userID)
	})
	if err != nil {
		return wrapDBErr(err)
	}

	// Şifre değişti: eski oturumların hepsi anında geçersiz (FR-103).
	if err := s.sessions.RevokeAllForUser(ctx, userID); err != nil {
		slog.Error("şifre sıfırlama sonrası oturumlar düşürülemedi", "user_id", userID, "err", err)
	}
	return nil
}

// ─────────────────────────── Oturum yönetimi ───────────────────────────

func (s *Service) ListSessions(ctx context.Context, userID int64) ([]db.Session, error) {
	rows, err := s.tx.Queries().ListUserSessions(ctx, userID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return rows, nil
}

// SessionHandle bir oturum kimliğinin DIŞARI VERİLEBİLİR karşılığıdır.
//
// Ham oturum kimliği bir TAŞIYICI TOKEN'dır: onu bilen kişi o oturumdur.
// Çerez httpOnly olduğu için JavaScript okuyamaz — ama listeleme uç noktası
// aynı değeri JSON'da dönerse bu koruma tümüyle etkisiz kalır: tek bir XSS
// veya kötü niyetli bağımlılık, kullanıcının TÜM cihazlarındaki oturumları
// çalabilir.
//
// Handle tek yönlüdür: listelemeye ve iptale yeter, kimlik doğrulamaya yetmez.
//
// test: handler/auth_wallet_integration_test.go#TestSessionListDoesNotLeakToken
func SessionHandle(sessionID string) string {
	sum := sha256.Sum256([]byte("session-handle:" + sessionID))
	return hex.EncodeToString(sum[:8])
}

// RevokeSession bir oturumu sonlandırır. SAHİPLİK kontrolü burada yapılır:
// kullanıcı yalnız kendi oturumunu düşürebilir.
//
// Parametre bir HANDLE'dır, ham oturum kimliği değil. Kullanıcının kendi
// oturumları taranıp handle eşleşmesi aranır; böylece sahiplik kontrolü
// aramanın PARÇASI olur ve atlanamaz.
func (s *Service) RevokeSession(ctx context.Context, userID int64, handle string) error {
	rows, err := s.tx.Queries().ListUserSessions(ctx, userID)
	if err != nil {
		return apperr.Internal(err)
	}
	for _, row := range rows {
		if SessionHandle(row.ID) == handle {
			return s.Logout(ctx, row.ID)
		}
	}
	// Var olmayan ve başkasına ait oturum AYNI hatayı döner.
	return apperr.ErrNotFound
}

// SuspendUser kullanıcıyı askıya alır ve TÜM oturumlarını anında düşürür (KK-104).
func (s *Service) SuspendUser(ctx context.Context, userID int64) error {
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		if err := q.SetUserStatus(ctx, db.SetUserStatusParams{
			ID: userID, Status: db.UserStatusSUSPENDED,
		}); err != nil {
			return err
		}
		return q.RevokeAllUserSessions(ctx, userID)
	})
	if err != nil {
		return wrapDBErr(err)
	}
	return s.sessions.RevokeAllForUser(ctx, userID)
}
