// Package storage yüklenen dosyaları (dekont) WEB KÖKÜ DIŞINDA saklar.
//
// FR-500 / KK-500 gereği üç kural bu paketin varlık sebebidir:
//
//  1. Dosya tipi SİHİRLİ BAYT ile belirlenir. İstemcinin Content-Type başlığı
//     ve dosya adı birer İPUCUDUR, kanıt değildir: ".php" uzantılı bir dosya
//     "image/jpeg" başlığıyla gönderilebilir.
//  2. Okuma SINIRLIDIR (io.LimitReader). Sınırsız okumak, tek bir istekle
//     belleği tüketme vektörüdür.
//  3. Dosya adı SUNUCUDA üretilir. Kullanıcının verdiği ad yola hiç karışmaz;
//     böylece "../../etc/passwd" gibi bir ad bir yol haline gelemez.
//
// Dosyalar buradan yalnız yetkili bir HTTP ucu üzerinden okunur; bu dizin
// hiçbir statik dosya sunucusuna bağlanmaz.
//
// test: storage_test.go#TestReceiptRejectsFakeImage
// test: storage_test.go#TestSavedNameIsServerGenerated
package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxReceiptBytes tek bir dekontun üst sınırı (FR-500: ≤ 5 MB).
const MaxReceiptBytes int64 = 5 << 20

// Hata sinyalleri. Servis katmanı bunları Türkçe, tipli uygulama hatalarına
// çevirir; ham hâlleri kullanıcıya gitmez.
var (
	// ErrTooLarge dosya sınırı aştı.
	ErrTooLarge = errors.New("storage: dosya 5 MB sınırını aşıyor")
	// ErrUnsupportedType sihirli bayt doğrulaması başarısız.
	ErrUnsupportedType = errors.New("storage: desteklenmeyen dosya tipi")
	// ErrEmpty gövde boş.
	ErrEmpty = errors.New("storage: dosya boş")
	// ErrNotFound kayıtlı dosya diskte yok.
	ErrNotFound = errors.New("storage: dosya bulunamadı")
	// ErrBadPath depodaki yol biçimi geçersiz (dizin dışına çıkma denemesi).
	ErrBadPath = errors.New("storage: geçersiz dosya yolu")
)

// Receipt saklanan dosyanın üst verisi.
type Receipt struct {
	// Path depo köküne GÖRELİ yoldur ("ab/cd/<32hex>.jpg").
	// Mutlak yol yerine göreli yol tutulur: depo dizini taşındığında
	// veritabanındaki kayıtların tamamı geçersizleşmesin.
	Path string
	MIME string
	Size int64
}

// signature bir dosya tipinin sihirli bayt imzası.
type signature struct {
	magic []byte
	mime  string
	ext   string
}

// İZİN VERİLEN TİPLER — JPEG, PNG, PDF (FR-500).
//
// SVG bilerek YOKTUR: içinde script taşıyabilen bir XML belgesidir ve
// tarayıcıda açıldığında çalışır.
var signatures = []signature{
	{magic: []byte{0xFF, 0xD8, 0xFF}, mime: "image/jpeg", ext: ".jpg"},
	{magic: []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, mime: "image/png", ext: ".png"},
	{magic: []byte("%PDF-"), mime: "application/pdf", ext: ".pdf"},
}

// detect baştaki baytlardan tipi belirler.
//
// İSTEMCİNİN SÖYLEDİĞİNE BAKILMAZ: ne dosya adına, ne Content-Type'a.
// KK-500 tam olarak bunu ölçer.
//
// test: storage_test.go#TestReceiptRejectsFakeImage
func detect(head []byte) (signature, bool) {
	for _, s := range signatures {
		if len(head) >= len(s.magic) && bytes.Equal(head[:len(s.magic)], s.magic) {
			return s, true
		}
	}
	return signature{}, false
}

// Local yerel dosya sistemine yazan depo.
type Local struct {
	root string
}

// NewLocal depoyu açar ve YAZILABİLİRLİĞİNİ DOĞRULAR.
//
// Doğrulama AÇILIŞTA yapılır, ilk yüklemede değil: çalışma anında keşfedilen
// bir izin hatası kullanıcıya "beklenmeyen hata" olarak döner ve sebebi
// günlerce fark edilmez. Hata dönerse çağıran süreci başlatmamalıdır.
func NewLocal(root string) (*Local, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("storage: kök dizin boş (UPLOAD_DIR)")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("storage: kök dizin çözülemedi: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("storage: kök dizin oluşturulamadı (%s): %w", abs, err)
	}
	probe, err := os.CreateTemp(abs, ".yazma-denemesi-*")
	if err != nil {
		return nil, fmt.Errorf("storage: kök dizin yazılabilir değil (%s): %w", abs, err)
	}
	name := probe.Name()
	_ = probe.Close()
	if err := os.Remove(name); err != nil {
		return nil, fmt.Errorf("storage: deneme dosyası silinemedi (%s): %w", abs, err)
	}
	return &Local{root: abs}, nil
}

// Root depo kökünün mutlak yolu (yalnız tanılama ve test için).
func (l *Local) Root() string { return l.root }

// SaveReceipt gövdeyi doğrular ve rastgele adla diske yazar.
//
// Çağıran dosya adı VERMEZ — imza yok. Kullanıcının verdiği ad yolun hiçbir
// parçasına girmez; dizin geçişi (path traversal) bu yüzden yapısal olarak
// imkânsızdır.
//
// test: storage_test.go#TestSavedNameIsServerGenerated
func (l *Local) SaveReceipt(ctx context.Context, r io.Reader) (Receipt, error) {
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}

	// SINIRLI OKUMA. +1 bayt bilerek: tam sınırda duran bir okuma ile
	// sınırı AŞAN bir gövde ayırt edilemezdi.
	limited := io.LimitReader(r, MaxReceiptBytes+1)

	head := make([]byte, 512)
	n, err := io.ReadFull(limited, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return Receipt{}, err
	}
	head = head[:n]
	if len(head) == 0 {
		return Receipt{}, ErrEmpty
	}
	sig, ok := detect(head)
	if !ok {
		return Receipt{}, ErrUnsupportedType
	}

	rel, err := randomRelPath(sig.ext)
	if err != nil {
		return Receipt{}, err
	}
	full := filepath.Join(l.root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return Receipt{}, fmt.Errorf("storage: dizin oluşturulamadı: %w", err)
	}

	// Önce geçici dosyaya yazılır, sonra yerine taşınır: sınırı aşan ya da
	// yarıda kesilen bir yükleme, yarım bir "geçerli" dekont bırakmaz.
	tmp, err := os.CreateTemp(filepath.Dir(full), ".yukleniyor-*")
	if err != nil {
		return Receipt{}, fmt.Errorf("storage: geçici dosya açılamadı: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	written, err := io.Copy(tmp, io.MultiReader(bytes.NewReader(head), limited))
	if err != nil {
		cleanup()
		return Receipt{}, fmt.Errorf("storage: yazılamadı: %w", err)
	}
	if written > MaxReceiptBytes {
		cleanup()
		return Receipt{}, ErrTooLarge
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return Receipt{}, fmt.Errorf("storage: kapatılamadı: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return Receipt{}, fmt.Errorf("storage: izinler ayarlanamadı: %w", err)
	}
	if err := os.Rename(tmpName, full); err != nil {
		_ = os.Remove(tmpName)
		return Receipt{}, fmt.Errorf("storage: taşınamadı: %w", err)
	}

	return Receipt{Path: rel, MIME: sig.mime, Size: written}, nil
}

// OpenReceipt kayıtlı bir dosyayı okumak için açar.
//
// Yol yeniden DOĞRULANIR. Veritabanından geliyor olması yeterli değildir:
// bir gün oraya başka bir yol yazan bir kod yolu eklenirse, tek savunma
// burada kalır.
//
// test: storage_test.go#TestOpenReceiptRejectsTraversal
func (l *Local) OpenReceipt(ctx context.Context, rel string) (io.ReadCloser, Receipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, Receipt{}, err
	}
	full, sig, err := l.resolve(rel)
	if err != nil {
		return nil, Receipt{}, err
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, Receipt{}, ErrNotFound
		}
		return nil, Receipt{}, fmt.Errorf("storage: açılamadı: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, Receipt{}, fmt.Errorf("storage: bilgi alınamadı: %w", err)
	}
	return f, Receipt{Path: rel, MIME: sig.mime, Size: st.Size()}, nil
}

// resolve göreli yolu kök içinde bir mutlak yola çevirir ve dizin dışına
// çıkma denemelerini reddeder.
//
// test: storage_test.go#TestOpenReceiptRejectsTraversal
func (l *Local) resolve(rel string) (string, signature, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", signature{}, ErrBadPath
	}
	// Mutlak yol, sürücü öneki ya da ters eğik çizgi kabul edilmez: depoya
	// yazdığımız yollar her zaman "ab/cd/<hex><uzantı>" biçimindedir.
	if strings.ContainsAny(rel, `\:`) || strings.HasPrefix(rel, "/") {
		return "", signature{}, ErrBadPath
	}
	clean := filepath.Clean(rel)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", signature{}, ErrBadPath
	}
	full := filepath.Join(l.root, clean)
	// Son kontrol: birleşim gerçekten kökün ALTINDA mı.
	if full != l.root && !strings.HasPrefix(full, l.root+string(os.PathSeparator)) {
		return "", signature{}, ErrBadPath
	}
	ext := strings.ToLower(filepath.Ext(clean))
	for _, s := range signatures {
		if s.ext == ext {
			return full, s, nil
		}
	}
	return "", signature{}, ErrBadPath
}

// randomRelPath sunucu tarafında üretilen, tahmin edilemez göreli yol.
//
// İlk iki bayt dizin katmanı olur: tek bir dizinde on binlerce dosya biriktiğinde
// dizin taraması yavaşlar.
func randomRelPath(ext string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("storage: rastgele ad üretilemedi: %w", err)
	}
	name := hex.EncodeToString(buf)
	return filepath.ToSlash(filepath.Join(name[0:2], name[2:4], name+ext)), nil
}
