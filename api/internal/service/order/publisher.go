package order

import "context"

// rawBus tip bilmeyen bir yayın taşıyıcısı (örn. Redis Pub/Sub).
type rawBus interface {
	Publish(ctx context.Context, key string, payload any) error
}

// BusPublisher genel bir taşıyıcıyı Publisher arayüzüne uydurur.
//
// NEDEN ADAPTÖR: `adapter/redis` paketi servis tiplerini TANIMAMALIDIR
// (katman kuralı: adapter → port, adapter ↛ service). Servis de Redis'i
// tanımaz. İkisini bağlayan ince katman burada durur; tip güvenliği servis
// tarafında korunur.
type BusPublisher struct{ bus rawBus }

func NewBusPublisher(bus rawBus) *BusPublisher { return &BusPublisher{bus: bus} }

func (p *BusPublisher) Publish(ctx context.Context, orderPublicID string, ev Event) error {
	// Olay tipi JSON'a `-` ile işaretli olduğu için gövdede taşınmaz;
	// taşıyıcıya ayrı bir sarmalayıcıyla veririz ki SSE tarafı event adını
	// bilsin.
	return p.bus.Publish(ctx, orderPublicID, struct {
		Type string `json:"type"`
		Event
	}{Type: ev.Type, Event: ev})
}
