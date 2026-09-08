// Package testsupport entegrasyon testleri için ortak güvenlik kapılarını içerir.
package testsupport

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// MustTestDatabaseURL entegrasyon testlerinin bağlanacağı veritabanını döner.
//
// 🔴 VERİTABANI ADI "_test" İLE BİTMİYORSA SÜREÇ DURUR.
//
// Entegrasyon testleri koşulsuz `DELETE FROM providers/users/price_quotes`
// yapar. Kapı olmadan `make test-integration` .env'deki DATABASE_URL'i
// kullanır ve GELİŞTİRME veritabanını siler — üretim sunucusunda
// `set -a; source deploy/.env` sonrası çalıştırılırsa ÜRETİM kataloğunu,
// sağlayıcı kaydını ve kullanıcıları siler.
//
// Kapı Makefile'da DEĞİL BURADA: bir hedefi düzeltmek, `go test` komutunu
// elle yazan kişiyi korumaz. Testin kendisi kendini korumalı.
func MustTestDatabaseURL() (string, error) {
	raw := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if raw == "" {
		if os.Getenv("ALLOW_SKIP_INTEGRATION") == "1" {
			return "", nil // çağıran atlamayı seçmiş
		}
		return "", fmt.Errorf("DATABASE_URL tanımsız — entegrasyon testleri çalıştırılamıyor.\n" +
			"  Çözüm: `make check` ya da `set -a; source .env; set +a` (veritabanı adı _test ile bitmeli)")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("DATABASE_URL ayrıştırılamadı: %w", err)
	}
	ad := strings.TrimPrefix(u.Path, "/")
	if !strings.HasSuffix(ad, "_test") {
		return "", fmt.Errorf(
			"🔴 GÜVENLİK KAPISI: entegrasyon testleri %q veritabanına bağlanmaya çalıştı.\n"+
				"  Bu testler DELETE FROM providers/users/price_quotes yapar; adı \"_test\" ile\n"+
				"  bitmeyen bir veritabanında koşmaları VERİ KAYBIDIR.\n"+
				"  Çözüm: DATABASE_URL=...%s_test ile çalıştırın ya da `make check` kullanın.",
			ad, ad)
	}
	return raw, nil
}
