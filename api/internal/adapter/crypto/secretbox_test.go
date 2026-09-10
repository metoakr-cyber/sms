package crypto_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	box, err := crypto.New(key(t))
	if err != nil {
		t.Fatal(err)
	}
	// 🔴 SAHTE BİR DİZE. Burada bir zamanlar GERÇEK sağlayıcı API anahtarı
	// yazılıydı ve depo GitHub'a itilmeden fark edildi (CLAUDE.md: "API
	// anahtarını koda yazma"). Test yalnız 32 karakterlik bir dizeye ihtiyaç
	// duyuyor; gerçek bir sırrın burada işi yok.
	const secret = "sahte-api-anahtari-yalnizca-test"

	sealed, err := box.SealString(secret)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte(secret)) {
		t.Fatal("şifreli metin düz sırrı içeriyor")
	}
	got, err := box.OpenString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if got != secret {
		t.Fatalf("çözülen = %q, beklenen %q", got, secret)
	}
}

// Aynı metin iki kez şifrelendiğinde FARKLI çıktı vermeli: sabit nonce
// GCM'de anahtarı kırılabilir hale getirir.
func TestNonceIsRandomPerCall(t *testing.T) {
	box, _ := crypto.New(key(t))
	a, _ := box.SealString("ayni-sir")
	b, _ := box.SealString("ayni-sir")
	if bytes.Equal(a, b) {
		t.Fatal("aynı metin aynı şifreli metni verdi — nonce tekrar ediyor")
	}
	for _, s := range [][]byte{a, b} {
		got, err := box.OpenString(s)
		if err != nil || got != "ayni-sir" {
			t.Fatalf("çözülemedi: %v %q", err, got)
		}
	}
}

func TestWrongKeyFails(t *testing.T) {
	box1, _ := crypto.New(key(t))
	box2, _ := crypto.New(key(t))
	sealed, _ := box1.SealString("gizli")
	if _, err := box2.Open(sealed); !errors.Is(err, crypto.ErrCiphertext) {
		t.Fatalf("yanlış anahtarla çözüldü: %v", err)
	}
}

// GCM kimlik doğrulamalıdır: kurcalanmış metin sessizce çöp üretmez, HATA verir.
func TestTamperingIsDetected(t *testing.T) {
	box, _ := crypto.New(key(t))
	sealed, _ := box.SealString("kurcalanacak-sir")

	for i := range sealed {
		bad := make([]byte, len(sealed))
		copy(bad, sealed)
		bad[i] ^= 0x01
		if _, err := box.Open(bad); err == nil {
			t.Fatalf("%d. bayt değiştirildiği halde çözüldü", i)
		}
	}
}

func TestRejectsBadInput(t *testing.T) {
	if _, err := crypto.New(make([]byte, 16)); !errors.Is(err, crypto.ErrKeySize) {
		t.Fatalf("16 baytlık anahtar kabul edildi: %v", err)
	}
	box, _ := crypto.New(key(t))
	for _, bad := range [][]byte{nil, {}, []byte("kisa")} {
		if _, err := box.Open(bad); !errors.Is(err, crypto.ErrCiphertext) {
			t.Errorf("bozuk girdi kabul edildi: %v", bad)
		}
	}
}

func TestMask(t *testing.T) {
	cases := map[string]string{
		"sahte-api-anahtari-yalnizca-test": "saht…test",
		"kisa":                             "****",
		"":                                 "****",
	}
	for in, want := range cases {
		if got := crypto.Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, beklenen %q", in, got, want)
		}
	}
}
