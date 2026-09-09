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

	catalog map[key]*entry        // fiyat/stok
	orders  map[string]*fakeOrder // remoteID -> sipariş
	faults  Faults                // hata enjeksiyonu

	// SMSDelay kodun gelmesi için geçen süre. 0 ise kod ANINDA gelir.
	SMSDelay time.Duration
	// OrderTTL numaranın geçerlilik süresi.
	OrderTTL time.Duration
	// BalanceMicro sağlayıcıdaki bakiyemiz (mikro-USD).
	BalanceMicro int64
}

type key struct {
	service string
	country string
	verify  port.VerificationType
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
		orders:       make(map[string]*fakeOrder),
		SMSDelay:     0,
		OrderTTL:     20 * time.Minute,
		BalanceMicro: 100_000_000, // 100 USD
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
	o.state = port.StateCompleted
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
