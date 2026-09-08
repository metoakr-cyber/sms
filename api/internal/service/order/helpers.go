package order

import (
	"crypto/sha256"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// displayName Türkçe ad varsa onu, yoksa özgün adı döner.
func displayName(tr, fallback string) string {
	if tr != "" {
		return tr
	}
	return fallback
}

// numericToString pgtype.Numeric'i ondalık metne çevirir.
//
// float ARACILIĞIYLA GEÇİRİLMEZ: kur 8 ondalıklıdır ve float64 dönüşümü
// son basamakları kaydırır. Sipariş satırındaki kur, iade ve kâr
// hesaplarının kanıtıdır.
func numericToString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0"
	}
	b, err := n.MarshalJSON()
	if err != nil {
		return "0"
	}
	return string(b)
}

// mustNumeric metni pgtype.Numeric'e çevirir.
//
// Hatada SIFIR DÖNMEZ, panik de etmez: geçersiz bir kur sessizce sıfır
// olursa sipariş satırı yalan söyler. Çağıran hatayı görmeli.
func mustNumeric(s string) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		// Buraya düşmek bir programlama hatasıdır: değer veritabanından
		// okunmuş bir NUMERIC'ten geliyor. Yine de geçersiz işaretleriz;
		// NOT NULL kısıtı yazımı reddeder ve hata görünür olur.
	// test: order_integration_test.go#TestPurchaseDeductsExactlyOnce (kur sipariş satırına yazılıyor)
		return pgtype.Numeric{}
	}
	return n
}

// parseInt64 metni int64'e çevirir.
func parseInt64(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("sayısal değil: %q", s)
	}
	return n, nil
}

// sha256sum kısa yardımcı — mesaj dedup anahtarı için.
func sha256sum(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}
