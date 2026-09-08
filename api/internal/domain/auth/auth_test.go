package auth_test

import (
	"strings"
	"testing"

	"github.com/ikmetrik/sms-platform/api/internal/domain/auth"
)

func TestHashAndVerify(t *testing.T) {
	const pw = "dogru-at-pil-zimba-42"

	h, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("PHC biçimi beklenmiyordu: %s", h)
	}
	if strings.Contains(h, pw) {
		t.Fatal("özet düz şifreyi içeriyor")
	}

	ok, err := auth.VerifyPassword(pw, h)
	if err != nil || !ok {
		t.Fatalf("doğru şifre doğrulanamadı: %v %v", ok, err)
	}
	ok, err = auth.VerifyPassword(pw+"x", h)
	if err != nil || ok {
		t.Fatalf("yanlış şifre kabul edildi: %v %v", ok, err)
	}
}

func TestHashIsSaltedUniquely(t *testing.T) {
	const pw = "ayni-sifre-ama-farkli-ozet"
	a, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("aynı şifre aynı özeti verdi — tuz kullanılmıyor")
	}
	// ikisi de doğrulanmalı
	for _, h := range []string{a, b} {
		if ok, err := auth.VerifyPassword(pw, h); err != nil || !ok {
			t.Fatalf("özet doğrulanamadı: %v %v", ok, err)
		}
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	bad := []string{
		"", "duz-metin", "$argon2id$", "$bcrypt$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=99$m=65536,t=3,p=4$c2FsdA$aGFzaA", // yanlış sürüm
		"$argon2id$v=19$m=0,t=0,p=0$c2FsdA$aGFzaA",     // sıfır parametre
		"$argon2id$v=19$m=65536,t=3,p=4$!!!$aGFzaA",    // bozuk base64
		"$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$",       // boş özet
	}
	for _, h := range bad {
		if ok, err := auth.VerifyPassword("herhangi", h); err == nil && ok {
			t.Errorf("bozuk özet kabul edildi: %q", h)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	h, _ := auth.HashPassword("guncel-parametrelerle-uretildi")
	if auth.NeedsRehash(h) {
		t.Fatal("güncel özet için rehash istendi")
	}
	// eski/düşük maliyetli parametre
	old := "$argon2id$v=19$m=4096,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaA"
	if !auth.NeedsRehash(old) {
		t.Fatal("düşük maliyetli özet için rehash istenmedi")
	}
	if !auth.NeedsRehash("bozuk") {
		t.Fatal("bozuk özet için rehash istenmedi")
	}
}

func TestPasswordStrength(t *testing.T) {
	tests := []struct {
		name     string
		pw       string
		identity []string
		want     auth.PasswordProblem
	}{
		{"kısa", "kisa123", nil, auth.PasswordTooShort},
		{"tam sınırın altı", "123456789", nil, auth.PasswordTooShort},
		{"yaygın", "password123", nil, auth.PasswordTooCommon},
		{"yaygın türkçe", "galatasaray", nil, auth.PasswordTooCommon},
		{"yaygın + ek", "password123!", nil, auth.PasswordTooCommon},
		{"tek karakter", "aaaaaaaaaaaa", nil, auth.PasswordTooSimple},
		{"ardışık artan", "abcdefghij", nil, auth.PasswordTooSimple},
		{"klavye yürüyüşü", "qwertyuiop", nil, auth.PasswordTooCommon},
		{"sadece rakam", "58371940275", nil, auth.PasswordTooSimple},
		{"e-posta içeriyor", "mehmet-guclu-sifre", []string{"mehmet@ornek.com"}, auth.PasswordContainsIdentity},
		{"kullanıcı adı içeriyor", "xx-kaptan42-xx", []string{"", "kaptan42"}, auth.PasswordContainsIdentity},
		{"kısa kimlik yok sayılır", "ali-guclu-bir-sifre", []string{"ali@x.com"}, auth.PasswordOK},
		{"geçerli", "dogru-at-pil-zimba", nil, auth.PasswordOK},
		{"geçerli karışık", "T9!vurgun_deniz", nil, auth.PasswordOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auth.CheckPasswordStrength(tt.pw, tt.identity...)
			if got != tt.want {
				t.Fatalf("CheckPasswordStrength(%q) = %v (%q), beklenen %v",
					tt.pw, got, got.Message(), tt.want)
			}
			if tt.want != auth.PasswordOK && got.Message() == "" {
				t.Error("reddedilen şifre için kullanıcı mesajı boş")
			}
		})
	}
}

func TestTokenLifecycle(t *testing.T) {
	tok, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok.Hash) != 32 {
		t.Fatalf("özet uzunluğu = %d, beklenen 32", len(tok.Hash))
	}
	if tok.Plain == "" || strings.Contains(string(tok.Hash), tok.Plain) {
		t.Fatal("ham token özet içinde görünüyor")
	}
	if !auth.TokenMatches(tok.Plain, tok.Hash) {
		t.Fatal("token kendi özetiyle eşleşmedi")
	}
	if auth.TokenMatches(tok.Plain+"x", tok.Hash) {
		t.Fatal("yanlış token eşleşti")
	}
}

func TestTokensAreUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		tok, err := auth.NewToken()
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[tok.Plain]; dup {
			t.Fatalf("%d. turda token tekrarlandı", i)
		}
		seen[tok.Plain] = struct{}{}
	}
}

func TestSessionIDsAreUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id, err := auth.NewSessionID()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) < 40 {
			t.Fatalf("oturum kimliği çok kısa: %d karakter", len(id))
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("%d. turda oturum kimliği tekrarlandı", i)
		}
		seen[id] = struct{}{}
	}
}

func BenchmarkHashPassword(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := auth.HashPassword("kiyaslama-sifresi-42"); err != nil {
			b.Fatal(err)
		}
	}
}

// Uzunluk kuralını geçen ama sözlük saldırısına açık şifreler reddedilmelidir.
func TestCommonBaseWithDecoration(t *testing.T) {
	rejected := []string{
		"password123!", "Password1234", "parola12345!", "sifre123456",
		"galatasaray1905", "!!fenerbahce!!", "admin@12345", "welcome-2026",
		"qwerty123456", "iloveyou1234", "istanbul34343", "mehmet-1234567",
	}
	for _, pw := range rejected {
		if got := auth.CheckPasswordStrength(pw); got == auth.PasswordOK {
			t.Errorf("kabul edildi ama reddedilmeliydi: %q", pw)
		}
	}
	// Yanlış pozitif olmamalı: taban kelime İÇEREN ama ondan İBARET olmayanlar geçmeli
	accepted := []string{
		"password-yerine-baska-bir-sey", "kirmizi-galatasaray-marti",
		"istanbul-bogazinda-marti-var", "benim-cok-guclu-sifrem",
	}
	for _, pw := range accepted {
		if got := auth.CheckPasswordStrength(pw); got != auth.PasswordOK {
			t.Errorf("reddedildi ama kabul edilmeliydi: %q → %v", pw, got)
		}
	}
}
