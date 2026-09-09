package dto

// Müşteri yorumu DTO'ları.
//
// Dışa açık her kimlik public_id (UUID) stringidir; sayısal veritabanı
// kimliği hiçbir yanıtta yer almaz (değişmez #10).

import (
	"strings"
	"unicode/utf8"

	reviewdom "github.com/ikmetrik/sms-platform/api/internal/domain/review"
)

/* ═══════════════════════ İstekler ═══════════════════════ */

// CreateReviewRequest yeni yorum.
//
// Durum İSTEMCİDEN ALINMAZ: yeni yorum her zaman PENDING'dir ve durum yalnız
// durum makinesi fonksiyonu üzerinden değişir (değişmez #13). Gövdede
// `status` alanı BULUNMAZ ki "onaylı gönderme" denemesi bile mümkün olmasın.
type CreateReviewRequest struct {
	Rating int    `json:"rating"`
	Body   string `json:"body"`
}

func (r *CreateReviewRequest) Validate() []FieldError {
	var errs []FieldError
	if !reviewdom.ValidRating(r.Rating) {
		errs = append(errs, FieldError{"rating", "Puan 1 ile 5 arasında olmalıdır."})
	}
	errs = appendReviewBodyErrors(errs, r.Body)
	return errs
}

// RejectReviewRequest red gerekçesi.
//
// Gerekçe ZORUNLUDUR ve bu yalnız istemci nezaketi değildir: domain de
// gerekçesiz reddi durdurur (domain/review, Decide). Kullanıcı "reddedildi"
// yazısını sebepsiz görürse aynı yorumu yeniden yazar ve kuyruk kendi
// kendini besler.
type RejectReviewRequest struct {
	Reason string `json:"reason"`
}

func (r *RejectReviewRequest) Validate() []FieldError {
	n := utf8.RuneCountInString(strings.TrimSpace(r.Reason))
	switch {
	case n == 0:
		return []FieldError{{"reason", "Red gerekçesi zorunludur."}}
	case n > reviewdom.MaxRejectionReasonLen:
		return []FieldError{{"reason", "Gerekçe en fazla 500 karakter olabilir."}}
	}
	return nil
}

/* ═══════════════════════ Doğrulama yardımcıları ═══════════════════════ */

// UZUNLUK RUNE İLE ÖLÇÜLÜR, bayt ile değil.
//
// `len(string)` Türkçe metinde bayt sayar: "ş" iki bayttır. 1000 baytlık bir
// sınır, tamamı Türkçe karakterlerden oluşan bir yorumu ~500 karakterde
// keserdi ve kullanıcı "1000 karakter" yazan sayaçla çelişen bir hata görürdü.
// Veritabanındaki char_length() de KARAKTER sayar; iki taraf aynı şeyi ölçmeli.
//
// test: review_test.go#TestTurkishBodyIsMeasuredInRunes
func appendReviewBodyErrors(errs []FieldError, raw string) []FieldError {
	n := utf8.RuneCountInString(strings.TrimSpace(raw))
	switch {
	case n < reviewdom.MinBodyLen:
		return append(errs, FieldError{"body", "Yorum en az 10 karakter olmalıdır."})
	case n > reviewdom.MaxBodyLen:
		return append(errs, FieldError{"body", "Yorum en fazla 1000 karakter olabilir."})
	}
	return errs
}

/* ═══════════════════════ Yanıtlar ═══════════════════════ */

// ReviewResponse kullanıcıya dönen KENDİ yorumu.
//
// Kullanıcı kendi red gerekçesini görür — başkasınınkini değil.
type ReviewResponse struct {
	ID          string `json:"id"`
	Rating      int    `json:"rating"`
	Body        string `json:"body"`
	Status      string `json:"status"`
	StatusLabel string `json:"statusLabel"`
	// RejectionReason yalnız REJECTED durumda dolar.
	RejectionReason string `json:"rejectionReason,omitempty"`
	CreatedAt       string `json:"createdAt"`
	ReviewedAt      string `json:"reviewedAt,omitempty"`
}

type ReviewListResponse struct {
	Items  []ReviewResponse `json:"items"`
	Total  int64            `json:"total"`
	Limit  int32            `json:"limit"`
	Offset int32            `json:"offset"`
}

// PublicReviewResponse SİTEDE gösterilen yorum.
//
// 🔴 E-POSTA ALANI YOKTUR ve eklenmeyecektir. Sorgu da e-postayı seçmez
// (queries/reviews.sql, ListApprovedReviews) — iki katman.
//
// `authorName` kullanıcı adıdır (harf/rakam/alt çizgi; e-posta olamaz,
// dto/auth.go validateUsername). Ad soyad ya da iletişim bilgisi DEĞİLDİR.
//
// test: review_integration_test.go#TestPublicReviewsNeverExposeEmail
type PublicReviewResponse struct {
	ID         string `json:"id"`
	Rating     int    `json:"rating"`
	Body       string `json:"body"`
	AuthorName string `json:"authorName"`
	// PublishedAt onay anı (RFC 3339). Safari boşluklu biçimi ayrıştıramaz.
	PublishedAt string `json:"publishedAt,omitempty"`
}

// PublicReviewListResponse sitedeki bölümün tüm verisi.
//
// Total 0 geldiğinde arayüz bölümü HİÇ RENDER ETMEZ; sunucu boş bir liste
// yerine uydurma bir yorum ÜRETMEZ.
type PublicReviewListResponse struct {
	Items []PublicReviewResponse `json:"items"`
	Total int64                  `json:"total"`
	// AverageX10 ortalamanın onda birlik tam sayı hâli (4.7 → 47).
	// Kayan nokta JSON'da taşınmaz; bölme yalnız gösterimde yapılır.
	AverageX10 int64 `json:"averageX10"`
}

// AdminReviewResponse yönetim listesindeki yorum.
//
// Kullanıcı bilgisi (e-posta, kullanıcı adı) YALNIZ bu yapıdadır ve log'a
// yazılmaz. Moderasyon kararı kimin yazdığını bilmeden verilemez.
type AdminReviewResponse struct {
	ID              string `json:"id"`
	UserID          string `json:"userId"` // users.public_id
	UserEmail       string `json:"userEmail"`
	UserUsername    string `json:"userUsername"`
	Rating          int    `json:"rating"`
	Body            string `json:"body"`
	Status          string `json:"status"`
	StatusLabel     string `json:"statusLabel"`
	RejectionReason string `json:"rejectionReason,omitempty"`
	CreatedAt       string `json:"createdAt"`
	ReviewedAt      string `json:"reviewedAt,omitempty"`
}

type AdminReviewListResponse struct {
	Items []AdminReviewResponse `json:"items"`
	Total int64                 `json:"total"`
	// PendingTotal süzgeçten BAĞIMSIZ bekleyen sayısıdır: yönetici
	// "onaylananlar" sekmesindeyken de kuyrukta kaç iş kaldığını görmeli.
	PendingTotal int64 `json:"pendingTotal"`
	Limit        int32 `json:"limit"`
	Offset       int32 `json:"offset"`
}
