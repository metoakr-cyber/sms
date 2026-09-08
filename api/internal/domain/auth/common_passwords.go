package auth

import "strings"

// commonPasswords en sık kullanılan şifreler. Türkçe konuşan kullanıcılara özgü
// olanlar (futbol takımları, "sifre", "parola", yaygın isimler) bilinçli olarak
// dahil edilmiştir — genel İngilizce listeler bunları kaçırır.
//
// Not: Bu liste kasıtlı olarak küçüktür (~300). Amaç, uzunluk kuralını (10+)
// geçen ama yine de tahmin edilebilir olan şifreleri elemektir. Tam bir
// 10.000'lik liste v1.1'de gömülü bir dosyadan yüklenecektir (docs/trd.md FR-100).
var commonPasswords = func() map[string]struct{} {
	raw := `
123456789 1234567890 12345678901 password1 password123 qwerty12345 qwertyuiop
iloveyou1 princess1 rockyou123 abc123456 1q2w3e4r5t 1qaz2wsx3edc zaq12wsx
football1 baseball1 superman1 batman123 michael123 jennifer1 sunshine1
trustno1234 letmein123 welcome123 monkey1234 dragon1234 master1234 shadow1234
admin12345 administrator root123456 passw0rd1 p@ssw0rd1 p@ssword123
sifre12345 sifre123456 parola12345 parola123456 sifrem12345 benimsifrem
galatasaray fenerbahce besiktas1 trabzonspor cimbom1905 fener1907 bjk1903
turkiye123 istanbul123 ankara12345 izmir123456 anadolu12345
mehmet12345 mustafa12345 ahmet123456 fatma123456 ayse12345678 emre12345678
selam12345 merhaba12345 hosgeldin123 kanka12345678
qwerty123456 asdfghjkl123 zxcvbnm12345 741852963000 963852741000
11111111111 00000000000 12121212121 10101010101 99999999999
computer123 internet123 whatever123 princess123 samsung12345
liverpool123 chelsea12345 arsenal12345 barcelona123 realmadrid12
starwars1234 pokemon12345 minecraft123 fortnite1234
azerty123456 motdepasse12 contrasena12
`
	m := make(map[string]struct{})
	for _, w := range strings.Fields(raw) {
		m[w] = struct{}{}
	}
	return m
}()

// commonBases tek başına yeterince uzun olmasa da TABAN olarak kabul edilemeyecek
// kelimeler. "password123!" gibi şifreler uzunluk kuralını geçer ama tahmin
// edilebilirdir: sondaki basit ekleri soyup bu listeye bakarız.
var commonBases = func() map[string]struct{} {
	raw := `
password parola sifre sifrem passwort motdepasse contrasena senha
qwerty qwertz azerty asdfgh zxcvbn
admin administrator root user guest test demo default
welcome hello merhaba selam hosgeldin
iloveyou princess sunshine monkey dragon master shadow letmein trustno
football baseball basketball soccer superman batman spiderman pokemon minecraft
galatasaray fenerbahce besiktas trabzonspor cimbom fener bjk gsgs
turkiye istanbul ankara izmir bursa antalya anadolu
mehmet mustafa ahmet fatma ayse emre burak canan deniz murat
computer internet whatever samsung android iphone google facebook
liverpool chelsea arsenal barcelona realmadrid juventus
starwars fortnite valorant
`
	m := make(map[string]struct{})
	for _, w := range strings.Fields(raw) {
		m[w] = struct{}{}
	}
	return m
}()

func isCommonPassword(lower string) bool {
	if _, ok := commonPasswords[lower]; ok {
		return true
	}
	// Sondaki ve baştaki basit ekleri soyup TABAN kelimeye bak:
	//   "password123!" → "password"   ·   "!!galatasaray1905" → "galatasaray"
	// Bu şifreler uzunluk kuralını geçer ama sözlük saldırısına açıktır.
	const filler = "0123456789!.*_-+=@#$%^&()"
	trimmed := strings.Trim(lower, filler)
	if len(trimmed) >= 4 {
		if _, ok := commonBases[trimmed]; ok {
			return true
		}
		if _, ok := commonPasswords[trimmed]; ok {
			return true
		}
	}
	return false
}
