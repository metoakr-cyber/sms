package herosms

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// TestUnknownProviderStatusStaysWaiting
//
// SÖZLEŞME: sağlayıcının sayısal durum kodu bizim durum makinemize DOĞRUDAN
// bağlanmaz. Yalnız 6, 8 ve 10 belgelidir; 1, 2, 3, 4 ve 7'nin anlamı spec'te
// YOKTUR (`x-enum-descriptions` alanı yok).
//
// Bilinmeyen bir kodu tahmin etmek yanlış para hareketi tetikler: sipariş
// yanlışlıkla terminal duruma düşerse iade ya hiç yapılmaz ya da hak edilmemiş
// bir iade yapılır.
func TestUnknownProviderStatusStaysWaiting(t *testing.T) {
	// Spec'te açıklaması OLMAYAN kodlar.
	for _, code := range []int{0, 1, 2, 3, 4, 5, 7, 9, 11, 99, -1} {
		if got := mapProviderStatus(code, false); got != port.StateWaiting {
			t.Errorf("mapProviderStatus(%d, mesajsız) = %q, beklenen %q — "+
				"bilinmeyen kod BEKLEMEDE bırakmalı", code, got, port.StateWaiting)
		}
	}

	// Belgeli kodlar doğru eşlenmeli.
	documented := map[int]port.RemoteOrderState{
		6:  port.StateCompleted,
		8:  port.StateCancelled,
		10: port.StateRefunded,
	}
	for code, want := range documented {
		if got := mapProviderStatus(code, false); got != want {
			t.Errorf("mapProviderStatus(%d) = %q, beklenen %q", code, got, want)
		}
	}
}

// TestMessagePresenceOverridesUnknownStatus
//
// Mesajın VARLIĞI gözlemlenebilir bir gerçektir; durum kodu ise yorumdur.
// Bilinmeyen kodla birlikte mesaj geldiyse sipariş tamamlanmıştır.
func TestMessagePresenceOverridesUnknownStatus(t *testing.T) {
	if got := mapProviderStatus(3, true); got != port.StateCompleted {
		t.Errorf("bilinmeyen kod + mesaj = %q, beklenen %q", got, port.StateCompleted)
	}
	// Ama BELGELİ bir iptal kodunu mesaj varlığı ezmemeli.
	if got := mapProviderStatus(8, true); got != port.StateCancelled {
		t.Errorf("iptal kodu + mesaj = %q, beklenen %q", got, port.StateCancelled)
	}
}

// TestExtractCodeFindsVerificationCode
//
// Sağlayıcının `code` alanı `required` DEĞİL ve nullable. Kodu kendi
// desenimizle çıkarırız (FR-410/10) — gelmesine güvenmeyiz.
func TestExtractCodeFindsVerificationCode(t *testing.T) {
	cases := []struct{ body, want string }{
		{"Your code is 12345", "12345"},
		{"WhatsApp kodunuz: 483920", "483920"},
		{"123-456 is your Google verification code", "123456"},
		{"Kod: 8842", "8842"},
		{"Your Telegram code is 55127. Do not give this code to anyone.", "55127"},
		// Telefon numarası KOD DEĞİLDİR: 8 haneden uzun diziler atlanır.
		{"Call us at 905321234567 for help", ""},
		{"", ""},
		{"Herhangi bir rakam yok", ""},
	}
	for _, c := range cases {
		if got := extractCode(c.body); got != c.want {
			t.Errorf("extractCode(%q) = %q, beklenen %q", c.body, got, c.want)
		}
	}
}

// TestOtpNormalizesAlternateFieldNames
//
// Aynı veri üç kaynaktan üç farklı isimle gelir (CLAUDE.md değişmez #26).
// Yalnız birini okumak, diğer iki kaynaktan gelen mesajı BOŞ gösterir.
func TestOtpNormalizesAlternateFieldNames(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		code string
		body string
	}{
		{"webhook biçimi", `{"code":"1234","text":"Kod 1234","receivedAt":"2026-09-08T10:00:00Z"}`, "1234", "Kod 1234"},
		{"legacy biçimi", `{"smsCode":"5678","smsText":"Kod 5678","date":"2026-09-08T10:00:00Z"}`, "5678", "Kod 5678"},
		{"kod yok, metinden çıkar", `{"text":"Your code is 9012"}`, "9012", "Your code is 9012"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var o otpItem
			if err := json.Unmarshal([]byte(c.raw), &o); err != nil {
				t.Fatal(err)
			}
			m := o.normalize("yedek-kimlik")
			if m.Code != c.code {
				t.Errorf("kod = %q, beklenen %q", m.Code, c.code)
			}
			if m.Body != c.body {
				t.Errorf("gövde = %q, beklenen %q", m.Body, c.body)
			}
			if m.RemoteID != "yedek-kimlik" {
				t.Errorf("kimlik yoksa yedek kullanılmalı, alınan %q", m.RemoteID)
			}
		})
	}
}

// TestFlexStringAcceptsBothTypes
//
// `phone` şemada integer ama örneklerde maskeli string; webhook'taki `id`
// alanının tipi hiç tanımlı değil. Katı tip ilk farklı yanıtta çöker.
func TestFlexStringAcceptsBothTypes(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`"79********1"`, "79********1"},
		{`79991234567`, "79991234567"},
		{`null`, ""},
		{`"abc-123"`, "abc-123"},
		// Büyük tam sayı: float üzerinden geçirilseydi basamak kaybederdi.
		{`9007199254740993`, "9007199254740993"},
	}
	for _, c := range cases {
		var f flexString
		if err := json.Unmarshal([]byte(c.raw), &f); err != nil {
			t.Fatalf("Unmarshal(%s): %v", c.raw, err)
		}
		if f.String() != c.want {
			t.Errorf("flexString(%s) = %q, beklenen %q", c.raw, f, c.want)
		}
	}
}

// TestCancelRejectionsAreDistinguished
//
// SÖZLEŞME: iptal reddi ÜÇ AYRI davranışa ayrılır (docs/provider-herosms.md §6.1).
// Tek bir "iptal başarısız" hatası altında toplamak, ya sonsuz yeniden deneme
// ya da kaybedilmiş iade üretir.
func TestCancelRejectionsAreDistinguished(t *testing.T) {
	t.Run("erken iptal → geçici, sağlayıcının verdiği süre kadar", func(t *testing.T) {
		body := []byte(`{"title":"EARLY_CANCEL_DENIED","info":{"minActivationTime":180}}`)
		err := mapLifecycleError(http.StatusConflict, body, "42")
		ra, ok := port.AsRetryAfter(err)
		if !ok {
			t.Fatalf("RetryAfterError bekleniyordu, alınan: %v", err)
		}
		if ra.After != 180*time.Second {
			t.Errorf("bekleme = %v, beklenen 180s — sağlayıcının verdiği süre kullanılmalı", ra.After)
		}
	})

	t.Run("süre yoksa tipik değere düşer ama sabit backoff değildir", func(t *testing.T) {
		err := mapLifecycleError(422, []byte(`{"title":"EARLY_CANCEL_DENIED"}`), "42")
		ra, ok := port.AsRetryAfter(err)
		if !ok || ra.After != 120*time.Second {
			t.Fatalf("120s yedeği bekleniyordu, alınan: %v", err)
		}
	})

	t.Run("pencere doldu → KALICI red", func(t *testing.T) {
		for _, title := range []string{"FREE_CANCELLATION_EXPIRED", "OTP_RECEIVED"} {
			err := mapLifecycleError(422, []byte(`{"title":"`+title+`"}`), "42")
			if _, retry := port.AsRetryAfter(err); retry {
				t.Errorf("%s yeniden denenebilir sayıldı — kalıcı red olmalı", title)
			}
		}
	})

	t.Run("yeni kod geldi → mesajlar taşınır", func(t *testing.T) {
		body := []byte(`{"title":"NEW_OTP_RECEIVED","info":{"data":[
			{"id":"9","code":"7788","text":"Kod 7788","receivedAt":"2026-09-08T10:00:00Z"}]}}`)
		err := mapLifecycleError(http.StatusConflict, body, "42")
		oe, ok := port.AsOTPArrived(err)
		if !ok {
			t.Fatalf("OTPArrivedError bekleniyordu, alınan: %v", err)
		}
		if len(oe.Messages) != 1 || oe.Messages[0].Code != "7788" {
			t.Fatalf("mesajlar taşınmadı: %+v", oe.Messages)
		}
	})

	t.Run("409 ve 422 aynı şekilde ele alınır", func(t *testing.T) {
		// Modern uçta iş kuralı reddinin hangi kodla geldiği spec'te tanımsız.
		for _, code := range []int{http.StatusConflict, http.StatusUnprocessableEntity} {
			err := mapLifecycleError(code, []byte(`{"title":"OTP_RECEIVED"}`), "42")
			if err == nil {
				t.Fatalf("HTTP %d için hata bekleniyordu", code)
			}
		}
	})
}

// TestPurchaseErrorsMapToDomainErrors
func TestPurchaseErrorsMapToDomainErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		is     error
	}{
		{"stok yok (başlık)", 422, `{"title":"NO_NUMBERS"}`, port.ErrOutOfStock},
		{"stok yok (404)", 404, `{}`, port.ErrOutOfStock},
		{"bakiye yok (başlık)", 422, `{"title":"NO_BALANCE"}`, port.ErrProviderNoBalance},
		{"bakiye yok (402)", 402, `{}`, port.ErrProviderNoBalance},
		{"fiyat değişti", 422, `{"title":"WRONG_MAX_PRICE","info":{"min":1.5}}`, port.ErrPriceChanged},
		{"eşleştirme", 422, `{"title":"WRONG_SERVICE"}`, port.ErrMappingMissing},
		{"kimlik", 401, `{"title":"Unauthenticated."}`, port.ErrProviderAuth},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := mapPurchaseError(c.status, []byte(c.body))
			if !errorsIs(err, c.is) {
				t.Errorf("mapPurchaseError(%d, %s) = %v, beklenen %v", c.status, c.body, err, c.is)
			}
		})
	}
}

func errorsIs(err, target error) bool {
	for e := err; e != nil; {
		if e == target {
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}
