// Package audit yönetim işlemlerini denetim kaydına yazar.
//
// Sorgu (InsertAuditLog) ve üretilen Go kodu aylardır vardı ama HİÇBİR YERDEN
// çağrılmıyordu: "kim onayladı?" sorusunun cevabı hiçbir zaman yazılmıyordu.
// Bu paket o çağrıyı tek bir yerde toplar; sonraki yönetim işlemleri
// (kullanıcı askıya alma, sağlayıcı değişikliği) aynı yardımcıyı kullanır.
//
// 🔴 Before/After JSONB'sine KİŞİSEL VERİ VE DOSYA YOLU KONMAZ.
// Denetim kaydı "neyin nasıl değiştiği" sorusunu cevaplar; e-posta, telefon,
// dekont yolu veya tam işlem hash'i bu soruya cevap vermez ama sızdığında
// zarar verir. Kayıt yalnız durum/tutar gibi alan değişimlerini taşır.
//
// test: ../deposit/deposit_integration_test.go#TestApproveWritesAuditLog
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/ikmetrik/sms-platform/api/internal/db"
)

// Meta isteğin izlenebilirlik bilgisi.
//
// Servis katmanı gin'i TANIMAZ; bu bilgiyi handler doldurup taşır.
type Meta struct {
	IP        *netip.Addr
	UserAgent string
	RequestID string
}

// Entry bir denetim kaydı.
type Entry struct {
	// ActorUserID işlemi yapan. nil ise sistem işlemi.
	ActorUserID *int64
	// Action nokta ile ayrılmış eylem adı: "deposit.approve", "deposit.reject".
	Action string
	// EntityType ve EntityID etkilenen kayıt. EntityID her zaman public_id'dir;
	// sayısal id denetim kaydında da yer almaz.
	EntityType string
	EntityID   string
	// Before/After JSON'a çevrilebilir alan değişimleri. nil bırakılabilir.
	Before any
	After  any
	Meta   Meta
}

// Record denetim kaydını yazar.
//
// ÇAĞIRANIN TRANSACTION'I İÇİNDE çalışır (q, InTx'in verdiği Queries olmalı):
// işlem geri alınırsa denetim kaydı da geri alınmalıdır, yoksa "onaylandı"
// diyen bir kayıt ile onaylanmamış bir talep yan yana durur.
func Record(ctx context.Context, q *db.Queries, e Entry) error {
	before, err := encode(e.Before)
	if err != nil {
		return err
	}
	after, err := encode(e.After)
	if err != nil {
		return err
	}
	if err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		ActorUserID: e.ActorUserID,
		Action:      e.Action,
		EntityType:  e.EntityType,
		EntityID:    e.EntityID,
		Before:      before,
		After:       after,
		Ip:          e.Meta.IP,
		UserAgent:   e.Meta.UserAgent,
		RequestID:   e.Meta.RequestID,
	}); err != nil {
		return fmt.Errorf("audit: %s kaydı yazılamadı: %w", e.Action, err)
	}
	return nil
}

func encode(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("audit: alan değişimi JSON'a çevrilemedi: %w", err)
	}
	return b, nil
}
