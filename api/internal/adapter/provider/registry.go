// Package provider sağlayıcı adaptörlerini protokole göre çözer.
package provider

import (
	"fmt"
	"sync"

	"github.com/ikmetrik/sms-platform/api/internal/port"
)

// Registry protokol adından adaptöre eşleme yapar.
//
// Adaptör seçimi sağlayıcının ADINDAN değil PROTOKOLÜNDEN yapılır.
// Eski prototip name.toLowerCase().includes('hero') kullanıyordu; sağlayıcının
// adını panelden düzenlemek sistemi sessizce bozuyordu (docs/design.md ADR-009).
type Registry struct {
	mu         sync.RWMutex
	byProtocol map[string]port.ProviderPort
}

func NewRegistry() *Registry {
	return &Registry{byProtocol: make(map[string]port.ProviderPort)}
}

// Register bir adaptörü kaydeder.
func (r *Registry) Register(p port.ProviderPort) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byProtocol[p.Protocol()] = p
}

// Resolve protokole karşılık gelen adaptörü döner.
//
// Bilinmeyen protokol SESSİZCE yok sayılmaz: veritabanında tanımlı ama kodda
// karşılığı olmayan bir sağlayıcı, sipariş akışında gizemli bir "sağlayıcı yok"
// hatasına dönüşürdü.
func (r *Registry) Resolve(protocol string) (port.ProviderPort, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byProtocol[protocol]
	if !ok {
		return nil, fmt.Errorf("provider: %q protokolü için adaptör kayıtlı değil", protocol)
	}
	return p, nil
}

// Protocols kayıtlı protokolleri döner (tanı ve sağlık kontrolü için).
func (r *Registry) Protocols() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byProtocol))
	for k := range r.byProtocol {
		out = append(out, k)
	}
	return out
}

// Supports bir adaptörün belirli bir ürün tipini destekleyip desteklemediğini söyler.
func Supports(p port.ProviderPort, kind port.ProductKind) bool {
	for _, k := range p.Capabilities() {
		if k == kind {
			return true
		}
	}
	return false
}
