// Bu dosya BİLİNÇLİ olarak `package mailer` içindedir (console_test.go
// `mailer_test` kullanır): sahte bir SMTP sunucusuna bağlanabilmek için
// adaptörün TLS yapılandırmasını değiştirmek gerekiyor ve bu ayarı üretim
// API'sinde dışa açmak, yanlışlıkla sertifika doğrulaması kapatmanın yolunu
// açardı.
package mailer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

const (
	testUser = "postmaster@ornek.com"
	testPass = "COK-GIZLI-SMTP-PAROLASI-42"
)

/* ═══════════════════════ Sahte SMTP sunucusu ═══════════════════════ */

// fakeSMTP tek oturumluk, betiklenebilir bir SMTP sunucusudur.
type fakeSMTP struct {
	t *testing.T
	// announceSTARTTLS false ise EHLO yanıtında eklenti duyurulmaz.
	announceSTARTTLS bool
	// authReply AUTH komutuna verilecek yanıt satırı (boş ise 235 kabul).
	authReply string
	tlsCert   tls.Certificate
	rootPEM   []byte
	addr      string
	// received teslim alınan mesaj gövdesi.
	received chan string
}

func newFakeSMTP(t *testing.T, announceSTARTTLS bool, authReply string) *fakeSMTP {
	t.Helper()
	cert, rootPEM := selfSignedCert(t)
	s := &fakeSMTP{
		t: t, announceSTARTTLS: announceSTARTTLS, authReply: authReply,
		tlsCert: cert, rootPEM: rootPEM, received: make(chan string, 1),
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.addr = ln.Addr().String()
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *fakeSMTP) host() string {
	h, _, _ := net.SplitHostPort(s.addr)
	return h
}

func (s *fakeSMTP) port() int {
	_, p, _ := net.SplitHostPort(s.addr)
	n, _ := strconv.Atoi(p)
	return n
}

// clientTLS sahte sunucunun kökünü tanıyan bir TLS yapılandırması döner.
func (s *fakeSMTP) clientTLS() *tls.Config {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(s.rootPEM) {
		s.t.Fatal("test sertifikası havuza eklenemedi")
	}
	return &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}
}

func (s *fakeSMTP) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }

	write("220 fake ESMTP")
	inData := false
	var data bytes.Buffer

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				select {
				case s.received <- data.String():
				default:
				}
				write("250 OK kuyruğa alındı")
				continue
			}
			data.WriteString(line + "\n")
			continue
		}

		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			if s.announceSTARTTLS {
				write("250-fake")
				write("250-STARTTLS")
				write("250 AUTH PLAIN LOGIN")
			} else {
				write("250-fake")
				write("250 AUTH PLAIN LOGIN")
			}
		case cmd == "STARTTLS":
			write("220 hazır")
			tc := tls.Server(conn, &tls.Config{
				Certificates: []tls.Certificate{s.tlsCert},
				MinVersion:   tls.VersionTLS12,
			})
			if err := tc.Handshake(); err != nil {
				return
			}
			conn = tc
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			br = bufio.NewReader(conn)
			write = func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
		case strings.HasPrefix(cmd, "AUTH"):
			if s.authReply != "" {
				write(s.authReply)
			} else {
				write("235 kabul")
			}
		case strings.HasPrefix(cmd, "MAIL FROM"):
			write("250 OK")
		case strings.HasPrefix(cmd, "RCPT TO"):
			write("250 OK")
		case cmd == "DATA":
			inData = true
			data.Reset()
			write("354 gövdeyi gönderin, sonunda .")
		case cmd == "QUIT":
			write("221 hoşça kalın")
			return
		default:
			write("250 OK")
		}
	}
}

// selfSignedCert testlik bir sertifika ve onun PEM kökünü üretir.
func selfSignedCert(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert, certPEM
}

func newTestSMTP(t *testing.T, s *fakeSMTP) *SMTP {
	t.Helper()
	a, err := NewSMTP(SMTPConfig{
		Host: s.host(), Port: s.port(),
		Username: testUser, Password: testPass,
		From: "SMS Platform <noreply@ornek.com>",
	})
	if err != nil {
		t.Fatal(err)
	}
	a.tlsConfig = s.clientTLS()
	a.dialTimeout = 5 * time.Second
	return a
}

/* ═══════════════════════ Testler ═══════════════════════ */

// Başarı yolu: STARTTLS + AUTH + DATA.
func TestSMTPSendsOverSTARTTLS(t *testing.T) {
	srv := newFakeSMTP(t, true, "")
	a := newTestSMTP(t, srv)

	err := a.Send(context.Background(), port.Mail{
		To: "kullanici@ornek.com", Subject: "Şifre sıfırlama", Text: "Merhaba",
	})
	if err != nil {
		t.Fatalf("gönderim başarısız: %v", err)
	}

	select {
	case body := <-srv.received:
		if !strings.Contains(body, "To: kullanici@ornek.com") {
			t.Fatalf("To başlığı yok:\n%s", body)
		}
		// Türkçe konu RFC 2047 ile kodlanmalı, ham UTF-8 gitmemeli.
		if !strings.Contains(body, "Subject: =?utf-8?") {
			t.Fatalf("konu kodlanmamış:\n%s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sunucu mesajı almadı")
	}
}

// STARTTLS duyurmayan sunucuya mesaj VERİLMEZ.
//
// Bu, adaptörün var oluş sebebidir: net/smtp.SendMail aynı sunucuya mesajı
// düz metin gönderirdi.
func TestSMTPRefusesServerWithoutSTARTTLS(t *testing.T) {
	srv := newFakeSMTP(t, false, "")
	a := newTestSMTP(t, srv)

	err := a.Send(context.Background(), port.Mail{
		To: "kullanici@ornek.com", Subject: "Konu", Text: "Gövde",
	})
	if err == nil {
		t.Fatal("STARTTLS'siz sunucuya gönderim BAŞARILI döndü — düz metin sızıntısı")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("hata sebebi anlaşılmıyor: %v", err)
	}
	select {
	case body := <-srv.received:
		t.Fatalf("mesaj yine de teslim edilmiş:\n%s", body)
	case <-time.After(200 * time.Millisecond):
	}
}

// 🔴 EN KRİTİK TEST: sunucu parolayı geri yansıtsa bile ne hata metnine ne
// log'a düşer. service/auth bu hatayı slog.Error ile kaydediyor.
func TestSMTPErrorNeverLeaksPassword(t *testing.T) {
	// Kötü niyetli (ya da sadece konuşkan) bir sunucu: reddederken hem ham
	// parolayı hem base64 AUTH dizesini geri yansıtıyor.
	authLine := base64.StdEncoding.EncodeToString(
		[]byte("\x00" + testUser + "\x00" + testPass))
	srv := newFakeSMTP(t, true,
		fmt.Sprintf("535 5.7.8 hatalı kimlik: user=%s pass=%s raw=%s",
			testUser, testPass, authLine))
	a := newTestSMTP(t, srv)

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)

	err := a.Send(context.Background(), port.Mail{
		To: "kullanici@ornek.com", Subject: "Konu", Text: "Gövde",
	})
	if err == nil {
		t.Fatal("kimlik doğrulama başarısızken gönderim başarılı döndü")
	}

	// service/auth'un yaptığının aynısı: hatayı log'a yaz.
	slog.Error("doğrulama e-postası gönderilemedi", "err", err)

	for _, secret := range []string{testPass, authLine} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("PAROLA hata metnine sızdı:\n%v", err)
		}
		if strings.Contains(buf.String(), secret) {
			t.Fatalf("PAROLA log'a sızdı:\n%s", buf.String())
		}
	}
	if !strings.Contains(err.Error(), "[GİZLENDİ]") {
		t.Fatalf("temizlik izi yok — sır hiç eşleşmemiş olabilir:\n%v", err)
	}
}

// Başlık enjeksiyonu: alıcı adresi kullanıcıdan gelir.
func TestSMTPRejectsHeaderInjection(t *testing.T) {
	srv := newFakeSMTP(t, true, "")
	a := newTestSMTP(t, srv)

	cases := []port.Mail{
		{To: "kurban@ornek.com\r\nBcc: saldirgan@kotu.com", Subject: "Konu", Text: "x"},
		{To: "kurban@ornek.com", Subject: "Konu\r\nBcc: saldirgan@kotu.com", Text: "x"},
	}
	for i, m := range cases {
		err := a.Send(context.Background(), m)
		if err == nil {
			t.Fatalf("vaka %d: satır sonu içeren başlık kabul edildi", i)
		}
		if strings.Contains(err.Error(), "saldirgan@kotu.com") {
			// Hata metni de log'a gider; enjekte edilen adresi taşımasın.
			t.Logf("uyarı: hata metni enjekte edilen adresi taşıyor: %v", err)
		}
	}
	select {
	case body := <-srv.received:
		t.Fatalf("enjeksiyonlu mesaj teslim edilmiş:\n%s", body)
	case <-time.After(200 * time.Millisecond):
	}
}

// Eksik yapılandırma açılışta yakalanır — çalışma anında sessiz arıza olmaz.
func TestNewSMTPRequiresFullConfig(t *testing.T) {
	base := SMTPConfig{Host: "smtp.ornek.com", Port: 587, Username: "u", Password: "p", From: "a@b.com"}
	cases := map[string]func(*SMTPConfig){
		"host yok":      func(c *SMTPConfig) { c.Host = "" },
		"port geçersiz": func(c *SMTPConfig) { c.Port = 0 },
		"kullanıcı yok": func(c *SMTPConfig) { c.Username = "" },
		"parola yok":    func(c *SMTPConfig) { c.Password = "" },
		"from yok":      func(c *SMTPConfig) { c.From = "" },
		"from bozuk":    func(c *SMTPConfig) { c.From = "bu bir adres değil" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := base
			mutate(&c)
			if _, err := NewSMTP(c); err == nil {
				t.Fatal("eksik yapılandırma kabul edildi")
			}
		})
	}
	if _, err := NewSMTP(base); err != nil {
		t.Fatalf("geçerli yapılandırma reddedildi: %v", err)
	}
}
