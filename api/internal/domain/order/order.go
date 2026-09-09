// Package order sipariş durum makinesidir.
//
// Durum geçişleri KODDA DAĞILMAZ: `status` alanına yalnız bu paketin
// doğruladığı bir geçişten sonra yazılır (docs/design.md §7). Handler veya
// arka plan işi içinde doğrudan bir durum ataması, geçersiz geçişleri
// sessizce mümkün kılar.
//
// Aynı kural veritabanında da zorlanır (migration 00008, orders_guard_transition):
// iki savunma hattı, çünkü bir yarış durumunda tamamlanmış bir siparişe iade
// yazmak gerçek para kaybettirir.
//
// test: ../../service/order/order_integration_test.go#TestTerminalOrderCannotChangeStatus
package order

import (
	"fmt"
	"time"
)

// Status sipariş durumu. Altı değer — yedincisi YOKTUR.
type Status string

const (
	// StatusPending numara alındı, kod bekleniyor.
	StatusPending Status = "PENDING"
	// StatusActive KİRALIK numara teslim edildi, kira dönemi SÜRÜYOR.
	//
	// Yalnız kiralıkta görülür. Aktivasyonda ilk kod ürünün tamamıdır ve
	// sipariş doğrudan COMPLETED olur; kiralıkta ürün SÜREDİR ve ilk mesaj
	// dönemi bitirmez — numara kalan gün boyunca yeni mesaj almaya devam eder.
	//
	// Terminal DEĞİL, ama iade yolu da YOKTUR: tek çıkışı COMPLETED'dır.
	// İade sorgularının (ListExpiredPendingOrders, ListRefundRetryOrders,
	// CanUserCancel) hiçbiri bu durumu görmez — kullanılmış bir kiralığa
	// yanlışlıkla iade yazmanın yolu böylece kapanır.
	StatusActive Status = "ACTIVE"
	// StatusCompleted aktivasyonda kod geldi, kiralıkta kira dönemi bitti.
	// TERMİNAL.
	StatusCompleted Status = "COMPLETED"
	// StatusCancelled kullanıcı iptali veya süre doldu. Terminal DEĞİL —
	// iade kaydı yazıldığında REFUNDED'a geçer.
	StatusCancelled Status = "CANCELLED"
	// StatusFailed sipariş oluştuktan SONRA kalıcı sağlayıcı hatası.
	// Terminal DEĞİL — otomatik iade ile REFUNDED'a geçer.
	//
	// DİKKAT: satın alma ANINDA sağlayıcı hata verirse sipariş HİÇ OLUŞMAZ
	// (FR-400/KK-400). Bu durum, sipariş var olduktan sonra sağlayıcının onu
	// kalıcı olarak kaybetmesi/reddetmesi içindir.
	StatusFailed Status = "FAILED"
	// StatusRefunded iade işlendi. TERMİNAL.
	StatusRefunded Status = "REFUNDED"
)

// AllStatuses tanımlı tüm durumlar (doğrulama ve test için).
func AllStatuses() []Status {
	return []Status{
		StatusPending, StatusActive, StatusCompleted,
		StatusCancelled, StatusFailed, StatusRefunded,
	}
}

// Valid bilinen bir durum mu.
func (s Status) Valid() bool {
	for _, x := range AllStatuses() {
		if s == x {
			return true
		}
	}
	return false
}

// IsTerminal durumdan çıkış var mı.
//
// YALNIZ COMPLETED ve REFUNDED terminaldir. CANCELLED, FAILED ve ACTIVE birer
// GEÇİŞTİR: ilk ikisinin REFUNDED'a, ACTIVE'in COMPLETED'a çıkan oku vardır.
// CANCELLED'ı terminal saymak, iade kaydının hiç yazılmaması ve kullanıcının
// parasını geri alamaması demektir. ACTIVE'i terminal saymak ise dönemi süren
// bir kiralığı sağlayıcıda kapatma kuyruğuna sokar — numara ölür.
func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusRefunded
}

// transitions izin verilen geçişler (docs/design.md §7.1).
//
// 🔴 ACTIVE → CANCELLED BİLEREK YOKTUR. Kiralığa ilk SMS geldikten sonra iade
// yolu kapalıdır (sağlayıcı da OTP_RECEIVED ile reddeder). Oku tabloya
// koymamak, ileride birinin iade yolunu ACTIVE bir siparişe uygulamasını
// burada, veritabanına inmeden durdurur.
// test: order_test.go#TestTransitionTable
var transitions = map[Status][]Status{
	StatusPending:   {StatusActive, StatusCompleted, StatusCancelled, StatusFailed},
	StatusActive:    {StatusCompleted},
	StatusCancelled: {StatusRefunded},
	StatusFailed:    {StatusRefunded},
	// Terminal durumlardan çıkış yok — bilerek boş.
	StatusCompleted: {},
	StatusRefunded:  {},
}

// ErrInvalidTransition geçersiz durum geçişi.
type ErrInvalidTransition struct {
	From, To Status
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("order: geçersiz durum geçişi: %s → %s", e.From, e.To)
}

// CanTransition geçiş izinli mi.
func CanTransition(from, to Status) bool {
	if from == to {
		return true // yeniden yazım (idempotent güncelleme) serbesttir
	}
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// Transition geçişi doğrular.
//
// test: order_test.go#TestTransitionTable
func Transition(from, to Status) error {
	if !from.Valid() {
		return fmt.Errorf("order: bilinmeyen kaynak durum %q", from)
	}
	if !to.Valid() {
		return fmt.Errorf("order: bilinmeyen hedef durum %q", to)
	}
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}
	return nil
}

// ─────────────────────────── Kapatma kararı ───────────────────────────

// CloseAction sipariş sağlayıcıda nasıl kapatılacak.
type CloseAction string

const (
	// CloseFinish kod TESLİM EDİLDİ — iade talep edilmez.
	CloseFinish CloseAction = "FINISH"
	// CloseCancel kod gelmedi — iade TALEP EDİLİR.
	CloseCancel CloseAction = "CANCEL"
)

// DecideClose siparişin sağlayıcıda nasıl kapatılacağını söyler (ADR-023).
//
// Cancel() ve Finish() AYNI ŞEY DEĞİLDİR:
//   - Kod geldiği hâlde Cancel() → sağlayıcı OTP_RECEIVED ile kalıcı reddeder,
//     üstelik yanlış sinyal vermiş oluruz.
//   - Kod gelmediği hâlde Finish() → hak ettiğimiz iadeden vazgeçmiş oluruz.
//
// Karar TEK ÖLÇÜTE dayanır: elimizde teslim edilmiş bir mesaj var mı.
// Sipariş durumuna değil, çünkü durum kodu bir yorumdur; mesajın varlığı
// gözlemlenebilir bir gerçektir.
//
// test: order_test.go#TestDecideClose
func DecideClose(hasDeliveredMessage bool) CloseAction {
	if hasDeliveredMessage {
		return CloseFinish
	}
	return CloseCancel
}

// DecideRentalClose kiralık siparişin sağlayıcıda nasıl kapatılacağını söyler.
//
// DecideClose'dan AYRI BİR FONKSİYONDUR ve o değiştirilmedi: aktivasyon
// sözleşmesi ("mesaj var mı") kiralıkta yanlış cevabı verir.
//
// AKTİVASYONDA ölçüt "kod geldi mi"ydi, çünkü ürün KODdu. KİRALIKTA ürün
// SÜREdir ve dönem sonunda süre teslim edilmiştir — mesaj gelmiş olsun ya da
// olmasın. Dönem sonunda Cancel() göndermek sağlayıcının ~20 dakikalık ücretsiz
// iptal penceresinin çok dışındadır: sonuç garantili FREE_CANCELLATION_EXPIRED,
// yani sekiz boş deneme ve gerçekte hiç iade edilebilir olmayan bir kaydın
// "gider" metriğini kirletmesi.
//
// TEK İSTİSNA erken vazgeçmedir: pencere HÂLÂ AÇIKKEN ve HİÇ mesaj yokken
// yapılan iptalde sağlayıcı da parayı geri verir; orada Cancel doğrudur ve
// bize maliyeti sıfırdır.
//
// test: order_test.go#TestDecideRentalClose
func DecideRentalClose(hasDeliveredMessage, withinFreeCancelWindow bool) CloseAction {
	if withinFreeCancelWindow && !hasDeliveredMessage {
		return CloseCancel
	}
	return CloseFinish
}

// ─────────────────────────── İptal edilebilirlik ───────────────────────────

// CancelCheck iptal isteğinin sonucu.
type CancelCheck struct {
	Allowed bool
	// RetryAfter iptal henüz mümkün değilse ne kadar beklenmeli.
	RetryAfter time.Duration
	Reason     string
}

// CancelInput iptal kontrolünün girdisi.
//
// YAPI OLARAK TAŞINIYOR, konumsal parametre olarak değil: alan sayısı dörde
// çıktı ve `CanUserCancel(st, a, b, now)` çağrısında iki zaman damgasının
// yerini karıştırmak, iade penceresini iptal gecikmesiyle takas eder — sessiz
// ve para maliyetli bir hata.
type CancelInput struct {
	Status Status
	// IsRental sipariş bir KİRALIK mı.
	//
	// Yalnız `RefundableUntil`in NİL anlamını belirlemek için var: aynı nil
	// aktivasyonda "üst sınır yok", kiralıkta "sınır BİLİNMİYOR" demektir ve
	// ikisinin para sonucu birbirinin zıddıdır.
	IsRental bool
	// CancellableAt iptalin mümkün olduğu EN ERKEN an (sağlayıcının
	// minActivationTime değeri).
	CancellableAt time.Time
	// RefundableUntil iptalin mümkün olduğu EN GEÇ an.
	//
	// AKTİVASYONDA NİL = ek bir üst sınır yok. Üst sınır örtük olarak
	// `expires_at`tir (~20 dk) ve order-expirer zaten iadeyi kendisi yazar;
	// bu alan aktivasyon davranışını değiştirmez.
	//
	// 🔴 KİRALIKTA NİL = İPTAL YOK. Gerekçe `CanUserCancel`in 5. koşulunda.
	RefundableUntil *time.Time
	// HasMessage siparişe teslim edilmiş en az bir mesaj var mı.
	HasMessage bool
	Now        time.Time
}

// CanUserCancel kullanıcı bu siparişi şu an iptal edebilir mi (FR-416).
//
// BEŞ KOŞUL:
//  1. Sipariş PENDING olmalı — terminal, iptal edilmiş ya da dönemi süren
//     (ACTIVE) bir sipariş iptal edilemez.
//  2. Teslim edilmiş mesaj OLMAMALI. Aktivasyonda bu kontrol gereksizdi (mesaj
//     gelen sipariş zaten COMPLETED olurdu); kiralıkta ilk mesaj siparişi
//     ACTIVE yapar ve 1. koşul çoğu yolu kapatır. Yine de AÇIKÇA yazılıyor:
//     mesajı olan bir siparişi iade etmek, kullanıcıya hem kodu hem parayı
//     vermektir ve sağlayıcı o iadeyi zaten OTP_RECEIVED ile geri çevirir.
//  3. `CancellableAt` geçmiş olmalı — sağlayıcı ilk ~120 saniye iptali
//     reddediyor. Bu süre SAĞLAYICIDAN gelir ve sunucuda tutulur; istemcinin
//     saatine güvenilmez.
//  4. `RefundableUntil` (varsa) HENÜZ GEÇMEMİŞ olmalı — sağlayıcının ücretsiz
//     iptal penceresi kapandıktan sonra yazacağımız her kuruş net giderdir.
//  5. KİRALIKTA `RefundableUntil` DOLU OLMALI. Aşağıya bakınız.
//
// 3. koşulu atlarsak kullanıcı butona basar, sağlayıcı reddeder ve kullanıcı
// "iptal çalışmıyor" der — oysa yalnız erkendir.
//
// 🔴 EKSİK VERİ "SINIR YOK" DEĞİL, "İZİN YOK" DEMEKTİR.
//
// 4. koşul üst sınırı yalnız kolon doluyken uyguluyordu; NİL sessizce guard'ı
// AÇIYORDU (fail-open). Aktivasyonda bu doğrudur — orada nil gerçekten "ek
// sınır yok" demektir ve order-expirer ~20 dakikada iadeyi kendisi yazar.
// Kiralıkta aynı nil ölçülmüş bir para kaybıdır: hiç mesaj almamış bir kiralık
// 30 gün PENDING kalır (`Expire` kiralıkta kapalı), `HasMessage` false olur,
// `CancellableAt` çoktan geçmiştir — ve 29. günde yapılan iptal TAM İADE
// yazar. Bir denetimde ölçüldü: bakiye 55000 → 100000.
//
// Bu yüzden kararın varsayılanı GÜVENLİ TARAFA düşürülüyor. Kolonu doldurmayı
// atlayan her yol (veri taşıma, admin kaydı, ileride eklenecek `prolong`)
// artık iadeyi açmak yerine kapatır. Aynı kural veritabanında da duruyor
// (`order_rental_has_refund_window`): biri KARARI, diğeri VERİYİ savunur.
//
// test: order_test.go#TestCanUserCancel
func CanUserCancel(in CancelInput) CancelCheck {
	if in.Status != StatusPending {
		return CancelCheck{Reason: "sipariş beklemede değil"}
	}
	if in.HasMessage {
		return CancelCheck{Reason: "siparişe mesaj teslim edilmiş"}
	}
	if in.IsRental && in.RefundableUntil == nil {
		return CancelCheck{Reason: "kiralıkta iade penceresi bilinmiyor"}
	}
	if in.Now.Before(in.CancellableAt) {
		return CancelCheck{
			RetryAfter: in.CancellableAt.Sub(in.Now),
			Reason:     "iptal için henüz erken",
		}
	}
	if in.RefundableUntil != nil && !in.Now.Before(*in.RefundableUntil) {
		return CancelCheck{Reason: "iptal ve iade penceresi kapandı"}
	}
	return CancelCheck{Allowed: true}
}

// IsExpired sipariş süresi doldu mu.
//
// Karar SUNUCUDAKİ expires_at ile verilir; istemcinin gönderdiği hiçbir süre
// bilgisi kullanılmaz (KK-405).
//
// test: order_test.go#TestIsExpired
func IsExpired(expiresAt, now time.Time) bool {
	return !now.Before(expiresAt)
}
