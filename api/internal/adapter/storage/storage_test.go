package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x41}, 64)...)
	pngBytes  = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0x42}, 64)...)
	pdfBytes  = append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{0x43}, 64)...)
)

func newStore(t *testing.T) *Local {
	t.Helper()
	s, err := NewLocal(filepath.Join(t.TempDir(), "dekontlar"))
	if err != nil {
		t.Fatalf("depo açılamadı: %v", err)
	}
	return s
}

// TestReceiptRejectsFakeImage — KK-500'ün tam metni:
// ".php uzantılı, image/jpeg MIME başlığı gönderilen bir dosya reddedilir."
//
// Bu testin ölçtüğü şey, depoya HİÇBİR dosya adı ve HİÇBİR MIME başlığı
// verilmemesidir: karar yalnız içeriğin ilk baytlarına dayanır. Bir PHP
// betiği JPEG imzası taşımadığı için reddedilir.
func TestReceiptRejectsFakeImage(t *testing.T) {
	s := newStore(t)

	// Gerçek bir saldırı gövdesi: uzantısı .php, istemci "image/jpeg" diyor.
	php := []byte("<?php system($_GET['c']); ?>\n" + strings.Repeat("A", 64))

	if _, err := s.SaveReceipt(context.Background(), bytes.NewReader(php)); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("🔴 sahte MIME'lı PHP dosyası KABUL EDİLDİ (hata: %v)", err)
	}

	// Diske hiçbir şey sızmamış olmalı.
	var files []string
	_ = filepath.Walk(s.Root(), func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 0 {
		t.Fatalf("🔴 reddedilen yükleme diske yazıldı: %v", files)
	}

	// Diğer tehlikeli gövdeler de reddedilmeli.
	for name, body := range map[string][]byte{
		"html":     []byte("<html><script>alert(1)</script></html>" + strings.Repeat("x", 64)),
		"svg":      []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`),
		"elf":      append([]byte{0x7F, 'E', 'L', 'F'}, bytes.Repeat([]byte{0}, 64)...),
		"bos":      {},
		"jpegkuyr": append(bytes.Repeat([]byte("x"), 8), 0xFF, 0xD8, 0xFF), // imza BAŞTA değil
	} {
		if _, err := s.SaveReceipt(context.Background(), bytes.NewReader(body)); err == nil {
			t.Errorf("🔴 %q gövdesi kabul edildi", name)
		}
	}

	// İzin verilen üç tip GERÇEKTEN kabul edilmeli — aksi hâlde test
	// "her şeyi reddet" diyerek de geçerdi.
	for name, body := range map[string][]byte{"jpeg": jpegBytes, "png": pngBytes, "pdf": pdfBytes} {
		if _, err := s.SaveReceipt(context.Background(), bytes.NewReader(body)); err != nil {
			t.Errorf("🔴 geçerli %s reddedildi: %v", name, err)
		}
	}
}

// TestSavedNameIsServerGenerated dosya adının SUNUCUDA üretildiğini ve
// depo kökünün dışına çıkılamadığını doğrular.
//
// SaveReceipt bir dosya adı parametresi ALMAZ; bu test o imzayı bir daha
// kimse "kolaylık olsun" diye geri eklemesin diye vardır.
func TestSavedNameIsServerGenerated(t *testing.T) {
	s := newStore(t)

	rec, err := s.SaveReceipt(context.Background(), bytes.NewReader(jpegBytes))
	if err != nil {
		t.Fatalf("kaydedilemedi: %v", err)
	}

	// "ab/cd/<32 hex>.jpg"
	shape := regexp.MustCompile(`^[0-9a-f]{2}/[0-9a-f]{2}/[0-9a-f]{32}\.jpg$`)
	if !shape.MatchString(rec.Path) {
		t.Fatalf("🔴 dosya yolu sunucu üretimi değil: %q", rec.Path)
	}
	if rec.MIME != "image/jpeg" {
		t.Fatalf("MIME = %q", rec.MIME)
	}
	if rec.Size != int64(len(jpegBytes)) {
		t.Fatalf("boyut = %d, %d bekleniyordu", rec.Size, len(jpegBytes))
	}

	// İki yükleme ASLA aynı yola düşmez.
	rec2, err := s.SaveReceipt(context.Background(), bytes.NewReader(jpegBytes))
	if err != nil {
		t.Fatalf("ikinci kayıt: %v", err)
	}
	if rec2.Path == rec.Path {
		t.Fatal("🔴 iki yükleme aynı yola yazıldı — biri diğerini ezer")
	}

	// Dosya gerçekten kökün altında ve dünyaya kapalı.
	full := filepath.Join(s.Root(), filepath.FromSlash(rec.Path))
	st, err := os.Stat(full)
	if err != nil {
		t.Fatalf("dosya yok: %v", err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("🔴 dosya izinleri fazla açık: %v", st.Mode().Perm())
	}
}

// TestReceiptRejectsOversize 5 MB sınırının GERÇEKTEN uygulandığını ve
// okumanın sınırsız olmadığını doğrular.
func TestReceiptRejectsOversize(t *testing.T) {
	s := newStore(t)

	big := append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{0x41}, int(MaxReceiptBytes))...)
	if _, err := s.SaveReceipt(context.Background(), bytes.NewReader(big)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("🔴 5 MB'ı aşan dosya kabul edildi (hata: %v)", err)
	}

	// Yarım kalan geçici dosya bırakılmamalı.
	var leftovers []string
	_ = filepath.Walk(s.Root(), func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			leftovers = append(leftovers, p)
		}
		return nil
	})
	if len(leftovers) != 0 {
		t.Fatalf("🔴 reddedilen yükleme diskte kaldı: %v", leftovers)
	}

	// Tam sınırda olan dosya KABUL edilmeli (sınır kapsayıcıdır).
	exact := append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{0x41}, int(MaxReceiptBytes)-3)...)
	if _, err := s.SaveReceipt(context.Background(), bytes.NewReader(exact)); err != nil {
		t.Fatalf("🔴 tam sınırdaki dosya reddedildi: %v", err)
	}
}

// TestOpenReceiptRejectsTraversal veritabanından gelen yola da güvenilmediğini
// doğrular: kök dışına işaret eden hiçbir yol açılamaz.
func TestOpenReceiptRejectsTraversal(t *testing.T) {
	s := newStore(t)

	// Kökün dışında, okunabilir bir "sır" dosyası.
	secret := filepath.Join(filepath.Dir(s.Root()), "gizli.jpg")
	if err := os.WriteFile(secret, jpegBytes, 0o600); err != nil {
		t.Fatalf("hazırlık: %v", err)
	}

	bad := []string{
		"../gizli.jpg",
		"../../gizli.jpg",
		"ab/../../gizli.jpg",
		"/etc/passwd",
		secret,
		"",
		"   ",
		".",
		"ab/cd/betik.php",
		"ab/cd/dosya", // uzantısız
	}
	for _, p := range bad {
		rc, _, err := s.OpenReceipt(context.Background(), p)
		if err == nil {
			_ = rc.Close()
			t.Errorf("🔴 kök dışına/geçersiz yola erişim açıldı: %q", p)
		}
	}

	// Meşru yol ÇALIŞMALI — aksi hâlde test "her şeyi reddet" ile de geçerdi.
	rec, err := s.SaveReceipt(context.Background(), bytes.NewReader(pdfBytes))
	if err != nil {
		t.Fatalf("kaydedilemedi: %v", err)
	}
	rc, meta, err := s.OpenReceipt(context.Background(), rec.Path)
	if err != nil {
		t.Fatalf("🔴 meşru dekont açılamadı: %v", err)
	}
	defer func() { _ = rc.Close() }()
	if meta.MIME != "application/pdf" {
		t.Fatalf("MIME = %q", meta.MIME)
	}
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, pdfBytes) {
		t.Fatal("🔴 okunan içerik yazılanla aynı değil")
	}
}

// TestNewLocalRejectsUnwritableRoot açılışta yazılabilirliğin gerçekten
// denendiğini doğrular: ilk yükleme anında keşfedilen bir izin hatası
// kullanıcıya "beklenmeyen hata" olarak döner.
func TestNewLocalRejectsUnwritableRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root olarak koşarken izin denemesi anlamsız")
	}
	base := t.TempDir()
	locked := filepath.Join(base, "kilitli")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatalf("hazırlık: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	if _, err := NewLocal(filepath.Join(locked, "dekont")); err == nil {
		t.Fatal("🔴 yazılamayan dizin açılışta kabul edildi")
	}
	if _, err := NewLocal(""); err == nil {
		t.Fatal("🔴 boş kök dizin kabul edildi")
	}
}
