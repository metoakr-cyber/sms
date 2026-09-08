package fake

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/port"
)

func (p *Provider) GetPriceAndStock(ctx context.Context, _ port.Creds, q port.PriceQuery) (*port.PriceResult, error) {
	if err := p.delay(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	vt := q.VerificationType
	if vt == "" {
		vt = port.VerifySMS
	}
	e, ok := p.catalog[key{q.ServiceCode, q.CountryCode, vt}]
	if !ok {
		// Eşleştirme var ama sağlayıcıda bu kombinasyon yok.
		return nil, port.ErrMappingMissing
	}
	return &port.PriceResult{
		Cost:  money.New(e.costMicro, money.USD),
		Stock: e.stock,
	}, nil
}

func (p *Provider) ListOffers(ctx context.Context, _ port.Creds, vt port.VerificationType) ([]port.OfferSnapshot, error) {
	if err := p.delay(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if vt == "" {
		vt = port.VerifySMS
	}
	out := make([]port.OfferSnapshot, 0, len(p.catalog))
	for k, e := range p.catalog {
		if k.verify != vt {
			continue
		}
		out = append(out, port.OfferSnapshot{
			ServiceCode:      k.service,
			CountryCode:      k.country,
			VerificationType: k.verify,
			Cost:             money.New(e.costMicro, money.USD),
			Stock:            e.stock,
		})
	}
	return out, nil
}

func (p *Provider) Purchase(ctx context.Context, _ port.Creds, cmd port.PurchaseCmd) (*port.PurchaseResult, error) {
	if err := p.delay(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.faults.PurchaseFails != nil {
		return nil, p.faults.PurchaseFails
	}

	vt := cmd.VerificationType
	if vt == "" {
		vt = port.VerifySMS
	}
	e, ok := p.catalog[key{cmd.ServiceCode, cmd.CountryCode, vt}]
	if !ok {
		return nil, port.ErrMappingMissing
	}
	if e.stock <= 0 {
		return nil, port.ErrOutOfStock
	}

	// Fiyat kayması: gerçek sağlayıcıda fiyat teklif ile satın alma arasında
	// değişebilir. MaxCost bu durumu SAĞLAYICI SINIRINDA yakalar (ADR-018).
	cost := e.costMicro
	if p.faults.PriceDrift > 0 {
		cost = int64(float64(cost) * p.faults.PriceDrift)
	}
	if !cmd.MaxCost.IsZero() && cost > cmd.MaxCost.Minor() {
		return nil, fmt.Errorf("%w: istenen azami %d, gerçek %d",
			port.ErrPriceChanged, cmd.MaxCost.Minor(), cost)
	}

	if p.BalanceMicro < cost {
		return nil, port.ErrProviderNoBalance
	}

	e.stock--
	p.BalanceMicro -= cost
	p.seq++

	now := p.clock.Now()
	id := fmt.Sprintf("fake-%d", p.seq)
	phone := fmt.Sprintf("%s%09d", fakeCountries[cmd.CountryCode].phoneCode, 100000000+p.seq)

	o := &fakeOrder{
		id: id, phone: phone,
		service: cmd.ServiceCode, country: cmd.CountryCode,
		costMicro: cost,
		state:     port.StateWaiting,
		createdAt: now,
		expiresAt: now.Add(p.OrderTTL),
		smsAt:     now.Add(p.SMSDelay),
	}
	p.orders[id] = o

	return &port.PurchaseResult{
		RemoteOrderID: id,
		PhoneNumber:   phone,
		CountryCode:   cmd.CountryCode,
		OperatorCode:  "any",
		Cost:          money.New(cost, money.USD),
		ExpiresAt:     o.expiresAt,
		Subtype:       port.KindSMSActivation,
	}, nil
}

func (p *Provider) GetStatus(ctx context.Context, _ port.Creds, remoteID string) (*port.RemoteStatus, error) {
	if err := p.delay(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.faults.StatusFails != nil {
		return nil, p.faults.StatusFails
	}
	o, ok := p.orders[remoteID]
	if !ok {
		return nil, port.ErrOrderNotFound
	}
	if o.closed && o.state != port.StateCompleted {
		// Kapanmış sipariş: mesajlar artık okunamaz (HeroSMS'te 409).
		return nil, port.ErrOrderClosed
	}

	p.deliverDue(o, p.clock.Now())

	msgs := make([]port.RemoteMessage, len(o.messages))
	copy(msgs, o.messages)
	return &port.RemoteStatus{State: o.state, Messages: msgs}, nil
}

// deliverDue zamanı gelen SMS'i düşürür ve süresi dolanı iptal eder.
//
// TEKİL VE TOPLU YOL AYNI SİMÜLASYONDAN BESLENİR: `GetStatus` ile
// `ListActive` iki ayrı uygulama olsaydı biri diğerinden sessizce ayrışır ve
// toplu yolun hatası testte görünmezdi (docs/memory.md §3.15 dersi).
//
// Kilit ÇAĞIRANDA tutulur: iki metot da p.mu altında çağırır.
// test: fake_test.go#TestFakeListActiveMatchesGetStatus
func (p *Provider) deliverDue(o *fakeOrder, now time.Time) {
	// SMS zamanı geldiyse otomatik teslim et.
	if o.state == port.StateWaiting && !now.Before(o.smsAt) && len(o.messages) == 0 {
		code := fmt.Sprintf("%06d", (p.seq*7919)%1000000)
		o.messages = append(o.messages, port.RemoteMessage{
			RemoteID:   o.id + "-msg-1",
			Code:       code,
			Body:       "Dogrulama kodunuz: " + code,
			Sender:     "SERVIS",
			ReceivedAt: now,
		})
		o.state = port.StateCompleted
	}
	// Süre dolduysa ve kod gelmediyse iptal.
	if o.state == port.StateWaiting && now.After(o.expiresAt) {
		o.state = port.StateCancelled
	}
}

// minCancelWait iptal için asgari bekleme. HeroSMS'te 120 saniye
// (info.minActivationTime). Gerçek kısıtı taklit ederiz ki arayüzdeki
// "iptal butonu 120 sn pasif" kuralı (FR-416) burada da sınansın.
const minCancelWait = 120 * time.Second

func (p *Provider) Cancel(ctx context.Context, _ port.Creds, remoteID string) error {
	if err := p.delay(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.faults.CancelFails != nil {
		return p.faults.CancelFails
	}
	o, ok := p.orders[remoteID]
	if !ok {
		return port.ErrOrderNotFound
	}
	if o.closed {
		return nil // idempotent
	}
	if len(o.messages) > 0 {
		// OTP geldiyse iptal edilemez — para iade edilmez.
		return fmt.Errorf("%w: OTP alınmış", port.ErrCancelDenied)
	}
	if elapsed := p.clock.Now().Sub(o.createdAt); elapsed < minCancelWait {
		return port.NewRetryAfter("asgari bekleme süresi dolmadı", minCancelWait-elapsed, port.ErrCancelDenied)
	}

	o.state = port.StateRefunded
	o.closed = true
	p.BalanceMicro += o.costMicro // sağlayıcı bize iade etti
	if e, ok := p.catalog[key{o.service, o.country, port.VerifySMS}]; ok {
		e.stock++
	}
	return nil
}

func (p *Provider) Finish(ctx context.Context, _ port.Creds, remoteID string) error {
	if err := p.delay(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	o, ok := p.orders[remoteID]
	if !ok {
		return port.ErrOrderNotFound
	}
	if o.closed {
		return nil // idempotent
	}
	// Finish İADE ETMEZ — Cancel'dan farkı tam olarak budur.
	o.closed = true
	if o.state == port.StateWaiting {
		o.state = port.StateCompleted
	}
	return nil
}

func (p *Provider) GetBalance(ctx context.Context, _ port.Creds) (money.Money, error) {
	if err := p.delay(ctx); err != nil {
		return money.Zero(money.USD), err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return money.New(p.BalanceMicro, money.USD), nil
}

/* ═══════════════════ Toplu yoklama ═══════════════════ */

var _ port.BatchPoller = (*Provider)(nil)

// fakeMaxPageSize gerçek sağlayıcının sayfa üst sınırıyla AYNI (25).
//
// Taklidin daha cömert olması sayfalama hatalarını testte gizlerdi: 100
// bekleyen sipariş tek sayfada dönseydi KK-404'ün "tur başına ≤ 4 istek"
// sınavı hiçbir şey ölçmezdi.
const fakeMaxPageSize = 25

// ListActive açık aktivasyonları sayfalı döner (port.BatchPoller).
//
// HeroSMS ile AYNI SÖZLEŞME: `size` 25'e kırpılır, `cursor` 1 tabanlı sayfa
// numarasının metin hâlidir, son sayfada `NextCursor` boş döner
// (herosms/operations.go ListActive).
//
// test: fake_test.go#TestFakeListActivePagesAtTwentyFive
func (p *Provider) ListActive(ctx context.Context, _ port.Creds, cursor string, size int) (port.ActivePage, error) {
	if err := p.delay(ctx); err != nil {
		return port.ActivePage{}, err
	}
	if size <= 0 || size > fakeMaxPageSize {
		size = fakeMaxPageSize
	}
	page := 1
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 1 {
			return port.ActivePage{}, fmt.Errorf("fake: geçersiz sayfa imleci %q", cursor)
		}
		page = n
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.faults.StatusFails != nil {
		// Toplu yolda da sağlayıcı kesintisi enjekte edilebilmeli.
		return port.ActivePage{}, p.faults.StatusFails
	}

	// SIRALI: sayfalama deterministik olmalı, yoksa aynı kayıt iki sayfada
	// görünüp bir başkası hiç görünmez.
	ids := make([]string, 0, len(p.orders))
	for id, o := range p.orders {
		if o.closed {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	start := (page - 1) * size
	if start > len(ids) {
		start = len(ids)
	}
	end := start + size
	if end > len(ids) {
		end = len(ids)
	}

	now := p.clock.Now()
	items := make([]port.ActiveOrder, 0, end-start)
	for _, id := range ids[start:end] {
		o := p.orders[id]
		p.deliverDue(o, now)
		msgs := make([]port.RemoteMessage, len(o.messages))
		copy(msgs, o.messages)
		items = append(items, port.ActiveOrder{
			RemoteOrderID: o.id, State: o.state, Messages: msgs,
		})
	}

	next := ""
	if end < len(ids) {
		next = strconv.Itoa(page + 1)
	}
	return port.ActivePage{Items: items, NextCursor: next}, nil
}
