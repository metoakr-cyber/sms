package herosms

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// gizliAnahtar testte kullanılan sahte API anahtarı.
//
// Tanınabilir ve tesadüfen oluşamayacak bir dizi seçildi: log çıktısında
// aranırken yanlış eşleşme olmasın.
const gizliAnahtar = "TEST-ANAHTAR-9f3c1b7e-SIZMAMALI"

// yakala slog çıktısını tampona yönlendirir ve geri alma fonksiyonu döner.
func yakala(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	eski := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(eski) })
	return &buf
}

// TestLegacyCallNeverLogsAPIKey
//
// 🔴 LEGACY YOLDA API ANAHTARI SORGU DİZESİNDE GİDER. Tam URL'i log'a yazmak
// anahtarı uygulama günlüğüne, oradan da log toplayıcıya ve yedeklere taşır —
// şifreli saklamanın (providers.api_key_enc) bütün anlamı kaybolur.
//
// Anahtar ne log'da ne de DÖNEN HATA METNİNDE geçmelidir: hata metinleri
// çağıran tarafından log'lanıyor (service katmanı), yani oraya sızan da
// sonunda log'a düşer.
func TestLegacyCallNeverLogsAPIKey(t *testing.T) {
	// Sunucu her yolda hata üretsin: hata yolları en çok URL sızdıran yerdir.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("BAD_KEY"))
	}))
	defer srv.Close()

	buf := yakala(t)
	p := New()
	c := port.Creds{APIKey: gizliAnahtar, BaseURL: srv.URL}

	// Legacy yolu kullanan üç ayrı çağrı.
	_, err1 := p.ListCountries(context.Background(), c)
	_, err2 := p.ListServices(context.Background(), c)
	_, err3 := p.ListRentOffers(context.Background(), c, "wa")

	log := buf.String()
	for ad, err := range map[string]error{"ListCountries": err1, "ListServices": err2, "ListRentOffers": err3} {
		if err != nil && strings.Contains(err.Error(), gizliAnahtar) {
			t.Errorf("🔴 %s HATA METNİNDE anahtar var: %v", ad, err)
		}
	}
	if strings.Contains(log, gizliAnahtar) {
		t.Fatalf("🔴 API ANAHTARI LOG'A DÜŞTÜ. Çıktı:\n%s", log)
	}
	// URL'in kendisi de geçmemeli: api_key sorgu parametresini taşır.
	if strings.Contains(log, "api_key") {
		t.Errorf("🔴 log'da 'api_key' geçiyor — tam URL yazılmış olabilir:\n%s", log)
	}
}

// TestLegacyRawCallNeverLogsAPIKey
//
// legacyGetRaw ayrı bir yoldur (kiralık uçlar hataları JSON olarak döndürdüğü
// için yazıldı) ve aynı sızıntı riskini taşır. Ayrı test, çünkü aynı
// korumanın iki yolda da bulunduğunu tek test ispatlayamaz.
func TestLegacyRawCallNeverLogsAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bağlantıyı yarıda kes: taşıma katmanı hatası URL'i en çok
		// sızdıran senaryodur (net/http hataları URL'i içerir).
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	buf := yakala(t)
	p := New()
	c := port.Creds{APIKey: gizliAnahtar, BaseURL: srv.URL}

	_, err := p.AllowedDurations(context.Background(), c)
	if err != nil && strings.Contains(err.Error(), gizliAnahtar) {
		t.Errorf("🔴 hata metninde anahtar var: %v", err)
	}
	if s := buf.String(); strings.Contains(s, gizliAnahtar) || strings.Contains(s, "api_key") {
		t.Fatalf("🔴 anahtar veya api_key log'a düştü:\n%s", s)
	}
}
