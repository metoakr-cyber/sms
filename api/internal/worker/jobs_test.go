package worker

// İş KAYDININ saf birim testi: veritabanı yok, ağ yok.

import (
	"testing"
	"time"
)

// TestAllRegistersDataRetentionJob
//
// `All()` listesine eklenmemiş bir iş HİÇ ÇALIŞMAZ ve bunu kimse fark etmez:
// sunucu açılır, log sessiz kalır, veri durur. `data-retention` için bunun
// bedeli teknik değil hukukidir — gizlilik metni "90 gün sonra sileriz" der,
// sistemde silen kimse olmaz.
//
// `Every > 0` ve `Run != nil` de burada sınanır: Runner.Start bu iki koşuldan
// birini sağlamayan işi sessizce ATLAR (worker.go), yani iş listede görünse
// bile koşmayabilir.
func TestAllRegistersDataRetentionJob(t *testing.T) {
	var found *Job
	for i, j := range All(Deps{}) {
		if j.Name == "data-retention" {
			found = &All(Deps{})[i]
			break
		}
	}
	if found == nil {
		t.Fatal("`data-retention` işi All() listesinde yok — saklama politikası hiç uygulanmaz")
	}
	if found.Run == nil {
		t.Error("`data-retention` gövdesiz — Runner.Start onu atlar")
	}
	if found.Every <= 0 {
		t.Errorf("`data-retention` aralığı %v — Runner.Start sıfır/negatif aralıklı işi atlar", found.Every)
	}
	// Saatlik seçildi: günlük bir aralık, günde birden çok yeniden başlatılan
	// bir sunucuda işin hiç koşmamasına yol açardı.
	if found.Every > time.Hour {
		t.Errorf("`data-retention` aralığı %v — bir saatten seyrek koşmamalı", found.Every)
	}
	if found.RunAtStart {
		t.Error("`data-retention` açılışta koşuyor — dağıtım anında beş tabloyu birden tarar")
	}
}
