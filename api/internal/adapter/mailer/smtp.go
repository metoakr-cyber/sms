package mailer

// SMTP adaptörü.
//
// NEDEN net/smtp: tek ihtiyacımız "kimliği doğrulanmış bir sunucuya şifreli
// bağlan ve bir mesaj bırak". Bunun için üçüncü parti bir kütüphane eklemek,
// e-posta yolunu kendi bağımlılık ağacıyla birlikte üretime taşımak demekti.
//
// NEDEN smtp.SendMail DEĞİL: standart kütüphanenin SendMail'i STARTTLS'i
// FIRSATÇI uygular — sunucu eklentiyi duyurmazsa mesajı DÜZ METİN gönderir.
// Doğrulama ve şifre sıfırlama bağlantısı taşıyan bir mesaj için bu kabul
// edilemez; el ile kurulan akışta TLS bir ön koşuldur.

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// smtpTimeout tüm oturum (bağlan → TLS → kimlik → veri) için üst sınır.
//
// Kayıt isteği e-posta gönderimini BEKLEMEZ ama gönderim yine de bir
// goroutine'i süresiz tutmamalıdır: yanıt vermeyen bir posta sunucusu, yavaşça
// biriken bağlantılarla sürecin kaynağını tüketir.
const smtpTimeout = 20 * time.Second

// SMTPConfig bağlantı bilgileri.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// SMTP kimlik doğrulamalı, STARTTLS zorunlu bir posta göndericisidir.
type SMTP struct {
	cfg SMTPConfig
	// fromAddr zarf göndericisi (MAIL FROM) — "Ad <adres>" biçiminden ayıklanır.
	fromAddr string
	// tlsConfig yalnız testte değiştirilir; üretimde nil bırakılır ve
	// varsayılan (sistem kök sertifikaları + sunucu adı doğrulaması) kullanılır.
	tlsConfig *tls.Config
	// dialTimeout testte kısaltılabilsin diye alan olarak tutulur.
	dialTimeout time.Duration
}

var _ port.Mailer = (*SMTP)(nil)

// NewSMTP adaptörü kurar. Eksik yapılandırmada HATA döner: e-postasız çalışan
// bir süreç sessiz arızadır (bkz. cmd/server/main.go mailer seçimi).
func NewSMTP(cfg SMTPConfig) (*SMTP, error) {
	var missing []string
	if strings.TrimSpace(cfg.Host) == "" {
		missing = append(missing, "SMTP_HOST")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		missing = append(missing, "SMTP_PORT")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		missing = append(missing, "SMTP_USERNAME")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		missing = append(missing, "SMTP_PASSWORD")
	}
	if strings.TrimSpace(cfg.From) == "" {
		missing = append(missing, "MAIL_FROM")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("smtp: eksik yapılandırma: %s", strings.Join(missing, ", "))
	}
	addr, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("smtp: MAIL_FROM geçerli bir e-posta adresi değil: %w", err)
	}
	return &SMTP{cfg: cfg, fromAddr: addr.Address, dialTimeout: smtpTimeout}, nil
}

// Send mesajı gönderir.
//
// Dönen hata METNİNDE parola bulunmaz: çağıran (service/auth) hatayı
// slog.Error ile kaydeder, yani buradan sızan her şey log'a düşer.
// test: smtp_test.go#TestSMTPErrorNeverLeaksPassword
func (s *SMTP) Send(ctx context.Context, m port.Mail) error {
	msg, err := s.buildMessage(m)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, s.dialTimeout)
	defer cancel()

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return apperr.ErrProviderUnavailable.Wrap(s.scrub(fmt.Errorf("smtp: bağlanılamadı: %w", err)))
	}
	// Oturumun tamamı context bütçesine bağlanır: sunucu yarı yolda susarsa
	// okuma sonsuza kadar beklemez.
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return apperr.ErrProviderUnavailable.Wrap(s.scrub(fmt.Errorf("smtp: oturum açılamadı: %w", err)))
	}
	defer func() { _ = c.Close() }()

	if err := s.secureAndAuthenticate(c); err != nil {
		return err
	}

	if err := c.Mail(s.fromAddr); err != nil {
		return apperr.ErrProviderUnavailable.Wrap(s.scrub(fmt.Errorf("smtp: MAIL FROM reddedildi: %w", err)))
	}
	if err := c.Rcpt(m.To); err != nil {
		// Alıcı adresi hata metnine KONMAZ (KVKK): hata log'a gider.
		return apperr.ErrProviderUnavailable.Wrap(s.scrub(fmt.Errorf("smtp: alıcı reddedildi: %w", err)))
	}
	w, err := c.Data()
	if err != nil {
		return apperr.ErrProviderUnavailable.Wrap(s.scrub(fmt.Errorf("smtp: DATA açılamadı: %w", err)))
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return apperr.ErrProviderUnavailable.Wrap(s.scrub(fmt.Errorf("smtp: gövde yazılamadı: %w", err)))
	}
	if err := w.Close(); err != nil {
		return apperr.ErrProviderUnavailable.Wrap(s.scrub(fmt.Errorf("smtp: mesaj kabul edilmedi: %w", err)))
	}
	// QUIT hatası mesajın teslim edilmediği anlamına gelmez; sunucu mesajı
	// kabul ettikten sonra bağlantıyı düşürebilir. Bu yüzden yutuluyor.
	_ = c.Quit()
	return nil
}

// secureAndAuthenticate STARTTLS ve kimlik doğrulamayı ZORUNLU kılar.
//
// Sunucu STARTTLS duyurmuyorsa mesaj GÖNDERİLMEZ: düz metin bir SMTP oturumu
// hem parolayı hem de şifre sıfırlama bağlantısını ağa açar.
// test: smtp_test.go#TestSMTPRefusesServerWithoutSTARTTLS
func (s *SMTP) secureAndAuthenticate(c *smtp.Client) error {
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return apperr.Internal(fmt.Errorf(
			"smtp: sunucu STARTTLS duyurmuyor — şifresiz gönderim yapılmadı (host=%s)", s.cfg.Host))
	}
	tc := s.tlsConfig
	if tc == nil {
		tc = &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
	}
	if err := c.StartTLS(tc); err != nil {
		return apperr.ErrProviderUnavailable.Wrap(s.scrub(fmt.Errorf("smtp: STARTTLS başarısız: %w", err)))
	}
	// PlainAuth, şifrelenmemiş bir bağlantıda kimlik bilgisini vermemek için
	// kendi kontrolünü de yapar; STARTTLS zorunluluğuyla iki katmanlı olur.
	if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
		return apperr.Internal(s.scrub(fmt.Errorf("smtp: kimlik doğrulama başarısız: %w", err)))
	}
	return nil
}

// scrub hata metninden parola ve kullanıcı adını temizler.
//
// Kötü yapılandırılmış (veya kötü niyetli) bir posta sunucusu, reddetme
// metninde gönderdiğimiz kimlik bilgisini geri yansıtabilir. O metin
// doğrudan slog'a gider.
// test: smtp_test.go#TestSMTPErrorNeverLeaksPassword
func (s *SMTP) scrub(err error) error {
	if err == nil {
		return nil
	}
	// AUTH PLAIN gövdesi ayrıca aranır: sunucu reddederken gönderdiğimiz
	// base64 bloğu olduğu gibi geri yansıtabilir ve o blok parolayı içerir.
	// test: smtp_test.go#TestSMTPErrorNeverLeaksPassword
	authBlob := base64.StdEncoding.EncodeToString(
		[]byte("\x00" + s.cfg.Username + "\x00" + s.cfg.Password))
	msg := redact(err.Error(), s.cfg.Password, authBlob, s.cfg.Username)
	if msg == err.Error() {
		return err
	}
	// Sarmalama zinciri BİLİNÇLİ olarak kesiliyor: iç hatanın metni sırrı
	// içeriyordu ve %w ile taşınırsa temizlik işe yaramazdı.
	return errors.New(msg)
}

// redact verilen sırları metinden çıkarır. Ham hâlleri kadar base64
// kodlanmış hâlleri de aranır: SMTP AUTH PLAIN kimlik bilgisini base64
// gönderir ve sunucu bazen o dizeyi olduğu gibi geri yansıtır.
func redact(s string, secrets ...string) string {
	for _, sec := range secrets {
		if len(sec) < 4 {
			// Çok kısa bir dize metnin her yerinde eşleşir ve hatayı
			// okunamaz hale getirir; böyle bir sır zaten kullanılmamalı.
			continue
		}
		s = strings.ReplaceAll(s, sec, "[GİZLENDİ]")
		s = strings.ReplaceAll(s, base64.StdEncoding.EncodeToString([]byte(sec)), "[GİZLENDİ]")
	}
	return s
}

/* ═══════════════════════ Mesaj kurma ═══════════════════════ */

// buildMessage RFC 5322 mesajını üretir.
//
// Başlıklara satır sonu KARAKTERİ GEÇEMEZ: alıcı adresi kullanıcıdan gelir ve
// içine "\r\nBcc: ..." sıkıştıran bir kayıt, tek bir e-postayı toplu gönderime
// çevirirdi (SMTP başlık enjeksiyonu).
// test: smtp_test.go#TestSMTPRejectsHeaderInjection
func (s *SMTP) buildMessage(m port.Mail) ([]byte, error) {
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return nil, apperr.ErrValidation.Wrap(fmt.Errorf("smtp: alıcı adresi geçersiz: %w", err))
	}
	if strings.ContainsAny(m.Subject, "\r\n") || strings.ContainsAny(to.Address, "\r\n") {
		return nil, apperr.ErrValidation.Wrap(
			fmt.Errorf("smtp: başlıkta satır sonu — mesaj kurulmadı"))
	}

	var b strings.Builder
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", s.cfg.From)
	h("To", to.Address)
	h("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h("Date", time.Now().Format(time.RFC1123Z))
	h("MIME-Version", "1.0")

	if m.HTML == "" {
		h("Content-Type", `text/plain; charset="UTF-8"`)
		h("Content-Transfer-Encoding", "base64")
		b.WriteString("\r\n")
		b.WriteString(wrapBase64(m.Text))
		return []byte(b.String()), nil
	}

	boundary, err := randomBoundary()
	if err != nil {
		return nil, apperr.Internal(err)
	}
	h("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	for _, part := range []struct{ ctype, body string }{
		{`text/plain; charset="UTF-8"`, m.Text},
		{`text/html; charset="UTF-8"`, m.HTML},
	} {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + part.ctype + "\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(wrapBase64(part.body))
	}
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String()), nil
}

// wrapBase64 gövdeyi base64'e çevirip 76 karakterlik satırlara böler
// (RFC 2045 sınırı; uzun satırlar bazı sunucularda mesajı bozar).
func wrapBase64(s string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76])
		b.WriteString("\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc)
	b.WriteString("\r\n")
	return b.String()
}

func randomBoundary() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("smtp: sınır dizesi üretilemedi: %w", err)
	}
	return "sms-" + hex.EncodeToString(buf[:]), nil
}
