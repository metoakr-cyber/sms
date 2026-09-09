// Package fake ağ gerektirmeyen bir sağlayıcı taklidi sağlar.
//
// Amacı yalnız "derlensin" değil, GERÇEKÇİ olmaktır: kod anında gelmez,
// stok tükenir, fiyat değişir, sağlayıcı bazen hata verir. Fazla iyimser bir
// taklit yalancı güven verir ve gerçek sağlayıcıya geçildiğinde akış çöker.
//
// Kullanım: yerel geliştirme, entegrasyon testleri, kaos senaryoları.
package fake

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// Provider bellek içi sahte sağlayıcı. Eşzamanlı kullanıma güvenlidir.
type Provider struct {
	mu sync.Mutex

	clock port.Clock
	seq   int64
	// nonce süreç ömrü boyunca sabit, rastgele bir ek. Uzak sipariş
	// kimliğinin KALICI bir veritabanında iki koşum arasında çakışmasını
	// engeller — sayaç her açılışta sıfırlanıyor.
	//
	// test: fake_test.go#TestRemoteOrderIDIsUniquePerProcess
	nonce string

	catalog map[key]*entry        // fiyat/stok (aktivasyon)
	rents   map[rentKey]*entry    // fiyat/stok (kiralık)
	orders  map[string]*fakeOrder // remoteID -> sipariş
	faults  Faults                // hata enjeksiyonu

	// SMSDelay kodun gelmesi için geçen süre. 0 ise kod ANINDA gelir.
	SMSDelay time.Duration
	// OrderTTL numaranın geçerlilik süresi.
	OrderTTL time.Duration
	// BalanceMicro sağlayıcıdaki bakiyemiz (mikro-USD).
	BalanceMicro int64

	// RentMessageInterval kiralık numaraya ne sıklıkla mesaj düşeceği.
	//
	// 🔴 KİRALIKTA ÇOKLU MESAJ ŞARTTIR. Taklit tek mesaj üretirken "ilk SMS'te
	// kiralık kapanıyor" hatası hiçbir testte görünmezdi: ikinci mesaj hiç
	// oluşmadığı için kaybı ölçecek bir gözlem yoktu.
	RentMessageInterval time.Duration
	// RentMessageCount bir kiralık numaranın dönem boyunca alacağı azami mesaj.
	RentMessageCount int
	// RentDurations kabul edilen kiralama süreleri (saat).
	RentDurations []int
}

type key struct {
	service string
	country string
	verify  port.VerificationType
}

// rentKey kiralık kataloğun anahtarı. Süre AYRI BİR EKSENDİR: aynı servis ×
// ülke için 24 saat ile 720 saatin fiyatı ve stoku farklıdır.
type rentKey struct {
	service string
	country string
	hours   int
}

type entry struct {
	costMicro int64
	stock     int
}

type fakeOrder struct {
	id        string
	phone     string
	service   string
	country   string
	costMicro int64
	state     port.RemoteOrderState
	createdAt time.Time
	expiresAt time.Time
	messages  []port.RemoteMessage
	// smsAt kodun geleceği an. Sonrasında GetStatus kodu döndürür.
	smsAt time.Time
	// closed Cancel veya Finish çağrıldı mı — sipariş kapandıysa mesaj okunamaz.
	closed bool

	// kind ürün türü. Kiralıkta davranış üç yerde ayrışır: mesaj SAYISI (bir
	// değil, N), ilk mesajın durumu (tamamlamaz) ve iptal penceresi (20 dk).
	kind port.ProductKind
	// rentalHours kiralama süresi (saat). Yalnız kiralıkta anlamlı.
	rentalHours int
}

// Faults hata enjeksiyonu. Testler belirli senaryoları zorlamak için kullanır.
type Faults struct {
	// PurchaseFails ayarlıysa Purchase her zaman bu hatayı döner.
	PurchaseFails error
	// StatusFails ayarlıysa GetStatus bu hatayı döner.
	StatusFails error
	// CancelFails ayarlıysa Cancel bu hatayı döner.
	CancelFails error
	// PriceDrift satın alma anında maliyeti bu oranda değiştirir (örn. 1.5 = %50 artış).
	// MaxCost aşılırsa ErrPriceChanged döner — fiyat garantisinin sınavı.
	PriceDrift float64
	// RentalDurationIgnored sağlayıcı `duration`ı YOK SAYAR.
	//
	// 🔴 BU KİP OLMADAN HATA GÖRÜNMEZDİ. Taklit sağlayıcıların ikisi de
	// `DurationHours`u HER ZAMAN onurlandırıyordu, yani "sağlayıcı ödenen
	// süreyi vermezse ne olur" sorusunun testte bir karşılığı yoktu — ve
	// cevabı "kullanıcı parasını kaybeder"di.
	//
	// Gerçek karşılığı: HeroSMS `duration`ı yok sayar ya da desteklemediği
	// kademeyi düşürür ve normal bir ~20 dakikalık aktivasyon döndürür.
	// Kip bunu birebir taklit eder: TTL `OrderTTL`e düşer ve `subtype`
	// aktivasyon olur.
	// test: rental_test.go#TestFakeCanIgnoreRentalDuration
	RentalDurationIgnored bool
	// Latency her çağrıya gecikme ekler (zaman aşımı testleri).
	Latency time.Duration
}

// New varsayılan kataloglu bir sahte sağlayıcı üretir.
func New(clock port.Clock) *Provider {
	if clock == nil {
		clock = port.RealClock{}
	}
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Rastgelelik alınamıyorsa süreç zamanına düşeriz: amaç gizlilik
		// değil, iki koşumun aynı kimliği üretmemesi.
		b = []byte(fmt.Sprintf("%08x", time.Now().UnixNano()))[:4]
	}
	p := &Provider{
		nonce:        hex.EncodeToString(b),
		clock:        clock,
		catalog:      make(map[key]*entry),
		rents:        make(map[rentKey]*entry),
		orders:       make(map[string]*fakeOrder),
		SMSDelay:     0,
		OrderTTL:     20 * time.Minute,
		BalanceMicro: 100_000_000, // 100 USD

		// Kiralık numaraya dönem boyunca birden çok mesaj gelir; taklit de
		// öyle davranmalı ki "ilk mesajda kapanıyor" hatası testte görünsün.
		RentMessageInterval: time.Hour,
		RentMessageCount:    5,
		// HeroSMS'in canlıdan okunmuş listesiyle AYNI: taklidin daha cömert
		// olması, kabul edilmeyen bir süreyle satın almanın testte görünmemesi
		// demektir.
		RentDurations: []int{24, 72, 168, 336, 720, 1440, 2160, 4320},
	}
	p.seedCatalog()
	return p
}

var _ port.ProviderPort = (*Provider)(nil)

func (p *Provider) Protocol() string { return "FAKE" }

func (p *Provider) Capabilities() []port.ProductKind {
	return []port.ProductKind{port.KindSMSActivation, port.KindSMSRental}
}

// SetFaults hata enjeksiyonunu ayarlar.
func (p *Provider) SetFaults(f Faults) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.faults = f
}

// SetStock bir kombinasyonun stokunu ayarlar (test için).
func (p *Provider) SetStock(service, country string, stock int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := key{service, country, port.VerifySMS}
	if e, ok := p.catalog[k]; ok {
		e.stock = stock
		return
	}
	p.catalog[k] = &entry{costMicro: 350_000, stock: stock}
}

// SetCost bir kombinasyonun maliyetini ayarlar (mikro-USD).
func (p *Provider) SetCost(service, country string, costMicro int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := key{service, country, port.VerifySMS}
	if e, ok := p.catalog[k]; ok {
		e.costMicro = costMicro
		return
	}
	p.catalog[k] = &entry{costMicro: costMicro, stock: 100}
}

// SetRentStock bir kiralık kombinasyonun stokunu ayarlar (test için).
func (p *Provider) SetRentStock(service, country string, hours, stock int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := rentKey{service, country, hours}
	if e, ok := p.rents[k]; ok {
		e.stock = stock
		return
	}
	p.rents[k] = &entry{costMicro: 3_000_000, stock: stock}
}

// SetRentCost bir kiralık kombinasyonun maliyetini ayarlar (mikro-USD).
func (p *Provider) SetRentCost(service, country string, hours int, costMicro int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := rentKey{service, country, hours}
	if e, ok := p.rents[k]; ok {
		e.costMicro = costMicro
		return
	}
	p.rents[k] = &entry{costMicro: costMicro, stock: 10}
}

// DeliverSMS bir siparişe elle mesaj düşürür (test için).
func (p *Provider) DeliverSMS(remoteID, code string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	o, ok := p.orders[remoteID]
	if !ok {
		return port.ErrOrderNotFound
	}
	if o.closed {
		return port.ErrOrderClosed
	}
	o.messages = append(o.messages, port.RemoteMessage{
		RemoteID:   fmt.Sprintf("%s-msg-%d", remoteID, len(o.messages)+1),
		Code:       code,
		Body:       fmt.Sprintf("Dogrulama kodunuz: %s", code),
		Sender:     "SERVIS",
		ReceivedAt: p.clock.Now(),
	})
	// KİRALIKTA MESAJ SİPARİŞİ TAMAMLAMAZ: numara dönem boyunca canlı kalır ve
	// yeni mesaj almaya devam eder. Aktivasyonda ürün o tek koddur; kiralıkta
	// ürün süredir.
	// test: rental_test.go#TestFakeRentalStaysOpenAfterFirstMessage
	if o.kind != port.KindSMSRental {
		o.state = port.StateCompleted
	}
	return nil
}

func (p *Provider) delay(ctx context.Context) error {
	p.mu.Lock()
	d := p.faults.Latency
	p.mu.Unlock()
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		// Zaman aşımı gerçek sağlayıcıda da böyle görünür.
		return fmt.Errorf("%w: %v", port.ErrUnavailable, ctx.Err())
	}
}
