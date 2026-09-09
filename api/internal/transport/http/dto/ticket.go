package dto

// Destek talebi DTO'ları (FR-600).
//
// Dışa açık her kimlik public_id (UUID) stringidir; sayısal veritabanı
// kimliği hiçbir yanıtta yer almaz (değişmez #10).

import (
	"strings"
	"unicode/utf8"

	ticketdom "github.com/ikmetrik/sms-platform/api/internal/domain/ticket"
)

/* ═══════════════════════ İstekler ═══════════════════════ */

// CreateTicketRequest yeni talep.
//
// Durum İSTEMCİDEN ALINMAZ: yeni talep her zaman OPEN'dır ve durum yalnız
// durum makinesi fonksiyonu üzerinden değişir (değişmez #13).
type CreateTicketRequest struct {
	Subject string `json:"subject"`
	// Priority boş bırakılabilir; sunucu NORMAL varsayar.
	Priority string `json:"priority"`
	Message  string `json:"message"`
}

func (r *CreateTicketRequest) Validate() []FieldError {
	var errs []FieldError
	errs = appendSubjectErrors(errs, r.Subject)
	errs = appendBodyErrors(errs, "message", r.Message)
	if _, ok := ticketdom.ParsePriority(r.Priority); !ok {
		errs = append(errs, FieldError{"priority", "Geçersiz öncelik."})
	}
	return errs
}

// CreateTicketMessageRequest yazışmaya mesaj ekler.
type CreateTicketMessageRequest struct {
	Message string `json:"message"`
}

func (r *CreateTicketMessageRequest) Validate() []FieldError {
	return appendBodyErrors(nil, "message", r.Message)
}

// UpdateTicketStatusRequest yöneticinin durum değişikliği.
//
// Kabul edilen değerler domain'de tanımlıdır (OPEN / CLOSED). ANSWERED ve
// USER_REPLIED elle atanamaz — ikisi de bir mesajın sonucudur.
type UpdateTicketStatusRequest struct {
	Status string `json:"status"`
}

func (r *UpdateTicketStatusRequest) Validate() []FieldError {
	s, ok := ticketdom.ParseStatus(r.Status)
	if !ok || !ticketdom.AdminCanSet(s) {
		return []FieldError{{"status", "Durum yalnız 'OPEN' veya 'CLOSED' olabilir."}}
	}
	return nil
}

/* ═══════════════════════ Doğrulama yardımcıları ═══════════════════════ */

// UZUNLUK RUNE İLE ÖLÇÜLÜR, bayt ile değil.
//
// `len(string)` Türkçe metinde bayt sayar: "ş" iki bayttır. 4000 baytlık
// sınır, tamamı Türkçe karakterlerden oluşan bir mesajda ~2000 karakterde
// dolardı ve kullanıcı "4000 karakter" yazan uyarıyla çelişen bir hata görürdü.
// Veritabanındaki char_length() de KARAKTER sayar; iki taraf aynı şeyi ölçmeli.
func appendSubjectErrors(errs []FieldError, raw string) []FieldError {
	n := utf8.RuneCountInString(strings.TrimSpace(raw))
	switch {
	case n < ticketdom.MinSubjectLen:
		return append(errs, FieldError{"subject",
			"Konu en az 5 karakter olmalıdır."})
	case n > ticketdom.MaxSubjectLen:
		return append(errs, FieldError{"subject",
			"Konu en fazla 120 karakter olabilir."})
	}
	return errs
}

func appendBodyErrors(errs []FieldError, field, raw string) []FieldError {
	n := utf8.RuneCountInString(strings.TrimSpace(raw))
	switch {
	case n < ticketdom.MinBodyLen:
		return append(errs, FieldError{field, "Mesaj boş olamaz."})
	case n > ticketdom.MaxBodyLen:
		return append(errs, FieldError{field,
			"Mesaj en fazla 4000 karakter olabilir."})
	}
	return errs
}

/* ═══════════════════════ Yanıtlar ═══════════════════════ */

// TicketMessageResponse yazışmadaki tek mesaj.
//
// 🔴 `authorUsername` KULLANICI yanıtında BOŞ kalır ve `omitempty` ile hiç
// görünmez: personelin kimliği kullanıcıya açılmaz. Sorgu (queries/tickets.sql,
// ListTicketMessagesForUser) o sütunu zaten seçmez — burada ikinci kez
// güvence altındadır.
//
// test: ticket_integration_test.go#TestStaffUsernameNeverLeaksToUser
type TicketMessageResponse struct {
	ID   string `json:"id"`
	Body string `json:"body"`
	// IsStaff true ise mesajı destek ekibi yazdı (KK-600).
	IsStaff bool `json:"isStaff"`
	// AuthorLabel arayüzde gösterilecek Türkçe etiket.
	AuthorLabel    string `json:"authorLabel"`
	AuthorUsername string `json:"authorUsername,omitempty"`
	CreatedAt      string `json:"createdAt"`
}

// TicketResponse kullanıcıya dönen talep.
type TicketResponse struct {
	ID            string `json:"id"`
	Subject       string `json:"subject"`
	Priority      string `json:"priority"`
	PriorityLabel string `json:"priorityLabel"`
	Status        string `json:"status"`
	StatusLabel   string `json:"statusLabel"`
	// MessageCount liste ekranında gösterilir; detayda 0 gelir.
	MessageCount int64  `json:"messageCount"`
	CreatedAt    string `json:"createdAt"`
	LastReplyAt  string `json:"lastReplyAt"`
	ClosedAt     string `json:"closedAt,omitempty"`
	// Messages yalnız DETAY yanıtında dolar.
	Messages []TicketMessageResponse `json:"messages,omitempty"`
}

type TicketListResponse struct {
	Items  []TicketResponse `json:"items"`
	Total  int64            `json:"total"`
	Limit  int32            `json:"limit"`
	Offset int32            `json:"offset"`
}

// AdminTicketResponse yönetim listesindeki talep.
//
// Kullanıcı bilgisi (e-posta, kullanıcı adı) YALNIZ bu yapıdadır ve log'a
// yazılmaz.
type AdminTicketResponse struct {
	ID            string `json:"id"`
	UserID        string `json:"userId"` // users.public_id
	UserEmail     string `json:"userEmail"`
	UserUsername  string `json:"userUsername"`
	Subject       string `json:"subject"`
	Priority      string `json:"priority"`
	PriorityLabel string `json:"priorityLabel"`
	Status        string `json:"status"`
	StatusLabel   string `json:"statusLabel"`
	MessageCount  int64  `json:"messageCount"`
	CreatedAt     string `json:"createdAt"`
	LastReplyAt   string `json:"lastReplyAt"`
	ClosedAt      string `json:"closedAt,omitempty"`

	Messages []TicketMessageResponse `json:"messages,omitempty"`
}

type AdminTicketListResponse struct {
	Items  []AdminTicketResponse `json:"items"`
	Total  int64                 `json:"total"`
	Limit  int32                 `json:"limit"`
	Offset int32                 `json:"offset"`
}
