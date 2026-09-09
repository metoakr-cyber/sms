package dto_test

// Yorum DTO doğrulamasının saf birim testleri.
//
// Bu katman "ilk savunma"dır: veritabanı CHECK'i ve domain kuralları arkada
// durur ama kullanıcının gördüğü Türkçe hata metni buradan çıkar.

import (
	"strings"
	"testing"

	reviewdom "github.com/ikmetrik/sms-platform/api/internal/domain/review"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
)

// TestTurkishBodyIsMeasuredInRunes uzunluğun KARAKTER ile ölçüldüğünü doğrular.
//
// `len(string)` bayt sayar: "ş" iki bayttır. Bayt sayılsaydı tamamı Türkçe
// 1000 karakterlik bir yorum ~1800 bayt eder ve sunucu kullanıcının gördüğü
// "1000/1000" sayacıyla çelişen bir hata verirdi. Veritabanındaki
// char_length() de karakter sayar; iki taraf aynı şeyi ölçmeli.
func TestTurkishBodyIsMeasuredInRunes(t *testing.T) {
	// Tam sınırda, tamamı çok baytlı: KABUL EDİLMELİ.
	tam := &dto.CreateReviewRequest{Rating: 5, Body: strings.Repeat("ş", reviewdom.MaxBodyLen)}
	if errs := tam.Validate(); len(errs) != 0 {
		t.Fatalf("%d Türkçe karakterlik yorum reddedildi (bayt sayılıyor): %+v",
			reviewdom.MaxBodyLen, errs)
	}

	// Bir karakter fazlası: REDDEDİLMELİ.
	fazla := &dto.CreateReviewRequest{Rating: 5, Body: strings.Repeat("ş", reviewdom.MaxBodyLen+1)}
	if errs := fazla.Validate(); len(errs) == 0 {
		t.Fatal("sınırı aşan yorum kabul edildi")
	}

	// Alt sınır da karakterle ölçülür: 9 Türkçe karakter (18 bayt) REDDEDİLMELİ.
	kisa := &dto.CreateReviewRequest{Rating: 5, Body: strings.Repeat("ş", reviewdom.MinBodyLen-1)}
	if errs := kisa.Validate(); len(errs) == 0 {
		t.Fatal("çok kısa yorum kabul edildi (bayt sayılıyor olabilir)")
	}
}

// TestCreateReviewRequestValidation puan ve metin kurallarını sınar.
func TestCreateReviewRequestValidation(t *testing.T) {
	cases := []struct {
		ad      string
		req     dto.CreateReviewRequest
		gecerli bool
	}{
		{"geçerli", dto.CreateReviewRequest{Rating: 4, Body: "Numara hızlı geldi."}, true},
		{"puan 0", dto.CreateReviewRequest{Rating: 0, Body: "Numara hızlı geldi."}, false},
		{"puan 6", dto.CreateReviewRequest{Rating: 6, Body: "Numara hızlı geldi."}, false},
		{"boş metin", dto.CreateReviewRequest{Rating: 4, Body: "   "}, false},
		// Baştaki/sondaki boşluk KIRPILIR: "        iyi        " on karakter
		// gibi görünür ama dokuz harflik bir yorum değildir.
		{"boşlukla şişirilmiş", dto.CreateReviewRequest{Rating: 4, Body: "   iyi    "}, false},
	}
	for _, c := range cases {
		t.Run(c.ad, func(t *testing.T) {
			errs := c.req.Validate()
			if c.gecerli && len(errs) != 0 {
				t.Fatalf("geçerli istek reddedildi: %+v", errs)
			}
			if !c.gecerli && len(errs) == 0 {
				t.Fatal("geçersiz istek kabul edildi")
			}
		})
	}
}

// TestRejectReviewRequestRequiresReason gerekçesiz reddin DTO katmanında da
// durdurulduğunu doğrular.
func TestRejectReviewRequestRequiresReason(t *testing.T) {
	if errs := (&dto.RejectReviewRequest{Reason: "  "}).Validate(); len(errs) == 0 {
		t.Fatal("boş gerekçe kabul edildi")
	}
	long := strings.Repeat("a", reviewdom.MaxRejectionReasonLen+1)
	if errs := (&dto.RejectReviewRequest{Reason: long}).Validate(); len(errs) == 0 {
		t.Fatal("aşırı uzun gerekçe kabul edildi")
	}
	if errs := (&dto.RejectReviewRequest{Reason: "Reklam içeriyor."}).Validate(); len(errs) != 0 {
		t.Fatalf("geçerli gerekçe reddedildi: %+v", errs)
	}
}
