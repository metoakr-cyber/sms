package handler

// Destek talepleri (FR-600) — KULLANICI ve YÖNETİM uçları.
//
// İkisi aynı dosyadadır çünkü aynı yazışmanın iki ucudur; ayırmak, birinde
// yapılan bir alan değişikliğinin diğerinde unutulmasını kolaylaştırırdı.
// Ayrım kodda değil, ROTA KAYDINDADIR: yönetim uçları izin ara katmanının
// arkasındadır (router.go).

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	ticketdom "github.com/ikmetrik/sms-platform/api/internal/domain/ticket"
	ticketsvc "github.com/ikmetrik/sms-platform/api/internal/service/ticket"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Ticket destek talebi uç noktaları.
type Ticket struct {
	svc *ticketsvc.Service
	r   Responder
}

func NewTicket(svc *ticketsvc.Service, r Responder) *Ticket {
	return &Ticket{svc: svc, r: r}
}

/* ═══════════════════════ Kullanıcı uçları ═══════════════════════ */

// Create POST /tickets
func (h *Ticket) Create(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	var req dto.CreateTicketRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	// Doğrulama geçtiyse öncelik ayrıştırılabilir; boş girdi NORMAL olur.
	priority, _ := ticketdom.ParsePriority(req.Priority)

	t, err := h.svc.Create(c.Request.Context(), ticketsvc.CreateInput{
		UserID:   userID,
		Subject:  req.Subject,
		Priority: priority,
		Body:     req.Message,
	})
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, ticketDTO(t, 1, nil))
}

// List GET /tickets — kullanıcının KENDİ talepleri.
func (h *Ticket) List(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	limit, offset := pagination(c, 20, 100)

	// `?status=` — geçersiz değer SESSİZCE YOK SAYILMAZ; yok sayılsaydı yanlış
	// yazılmış bir süzgeç TÜM listeyi döndürür ve kullanıcı bunu "süzgeç
	// çalıştı" sanardı. `AdminList` ile aynı davranış.
	var status *ticketdom.Status
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		st, ok := ticketdom.ParseStatus(raw)
		if !ok {
			h.r.FailField(c, []dto.FieldError{{Field: "status", Message: "Geçersiz durum süzgeci."}})
			return
		}
		status = &st
	}

	var ara *string
	if v := strings.TrimSpace(c.Query("q")); v != "" {
		ara = &v
	}

	rows, total, err := h.svc.List(c.Request.Context(), userID, status, ara, limit, offset)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	items := make([]dto.TicketResponse, 0, len(rows))
	for _, t := range rows {
		items = append(items, dto.TicketResponse{
			ID:            t.PublicID.String(),
			Subject:       t.Subject,
			Priority:      string(t.Priority),
			PriorityLabel: ticketPriorityLabel(t.Priority),
			Status:        string(t.Status),
			StatusLabel:   ticketStatusLabel(t.Status),
			MessageCount:  t.MessageCount,
			CreatedAt:     t.CreatedAt.Format(time.RFC3339),
			LastReplyAt:   t.LastReplyAt.Format(time.RFC3339),
			ClosedAt:      rfc3339OrEmpty(t.ClosedAt),
		})
	}
	h.r.OK(c, dto.TicketListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

// Get GET /tickets/:id — detay + yazışma.
func (h *Ticket) Get(c *gin.Context) {
	userID, publicID, ok := h.scope(c)
	if !ok {
		return
	}
	t, msgs, err := h.svc.Get(c.Request.Context(), userID, publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, ticketDTO(t, int64(len(msgs)), msgs))
}

// AddMessage POST /tickets/:id/messages
//
// POST'tur çünkü durum değiştirir (değişmez #8).
func (h *Ticket) AddMessage(c *gin.Context) {
	userID, publicID, ok := h.scope(c)
	if !ok {
		return
	}
	var req dto.CreateTicketMessageRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	if _, err := h.svc.AddMessage(c.Request.Context(), userID, publicID, req.Message); err != nil {
		h.r.Fail(c, err)
		return
	}
	// Yazışmanın TAMAMI geri döner (yönetim yolundaki gibi): kullanıcı mesajı
	// gönderdikten sonra ayrı bir istekle listeyi tazelemek zorunda kalmasın.
	// Yalnız talep başlığını dönmek, mesaj sayısını da yanlış göstermek olurdu.
	t, msgs, err := h.svc.Get(c.Request.Context(), userID, publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, ticketDTO(t, int64(len(msgs)), msgs))
}

/* ═══════════════════════ Yönetim uçları ═══════════════════════ */

// AdminList GET /admin/tickets — durum süzgeci + sayfalama.
//
// İki süzgeç vardır:
//
//	?status=OPEN      tam durum eşleşmesi
//	?pending=true     personel müdahalesi bekleyenler (OPEN + USER_REPLIED)
//
// Yönetim ekranının varsayılanı ikincisidir: "açık talepler" tek bir durum
// değildir ve yalnız OPEN süzmek, kullanıcının yanıt yazdığı talepleri
// varsayılan ekranda gizlerdi.
func (h *Ticket) AdminList(c *gin.Context) {
	var filter ticketsvc.AdminFilter
	if raw := c.Query("status"); raw != "" {
		s, ok := ticketdom.ParseStatus(raw)
		if !ok {
			h.r.FailField(c, []dto.FieldError{{Field: "status", Message: "Geçersiz durum süzgeci."}})
			return
		}
		filter.Status = &s
	}
	filter.OnlyPending = c.Query("pending") == "true"
	if v := strings.TrimSpace(c.Query("q")); v != "" {
		filter.Q = &v
	}

	limit, offset := pagination(c, 20, 100)
	rows, total, err := h.svc.AdminList(c.Request.Context(), filter, limit, offset)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	items := make([]dto.AdminTicketResponse, 0, len(rows))
	for _, t := range rows {
		items = append(items, dto.AdminTicketResponse{
			ID:            t.PublicID.String(),
			UserID:        t.UserPublicID.String(),
			UserEmail:     t.UserEmail,
			UserUsername:  t.UserUsername,
			Subject:       t.Subject,
			Priority:      string(t.Priority),
			PriorityLabel: ticketPriorityLabel(t.Priority),
			Status:        string(t.Status),
			StatusLabel:   ticketStatusLabel(t.Status),
			MessageCount:  t.MessageCount,
			CreatedAt:     t.CreatedAt.Format(time.RFC3339),
			LastReplyAt:   t.LastReplyAt.Format(time.RFC3339),
			ClosedAt:      rfc3339OrEmpty(t.ClosedAt),
		})
	}
	h.r.OK(c, dto.AdminTicketListResponse{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

// AdminGet GET /admin/tickets/:id
func (h *Ticket) AdminGet(c *gin.Context) {
	publicID, ok := h.adminScope(c)
	if !ok {
		return
	}
	row, msgs, err := h.svc.AdminGet(c.Request.Context(), publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, adminTicketDTO(row, msgs))
}

// AdminAddMessage POST /admin/tickets/:id/messages — personel yanıtı.
func (h *Ticket) AdminAddMessage(c *gin.Context) {
	staffID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	publicID, ok := h.adminScope(c)
	if !ok {
		return
	}
	var req dto.CreateTicketMessageRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	if _, err := h.svc.AdminAddMessage(c.Request.Context(), staffID, publicID, req.Message); err != nil {
		h.r.Fail(c, err)
		return
	}
	// Yazışmanın tamamı geri döner: yönetici mesajı gönderdikten sonra ayrı
	// bir istekle listeyi tazelemek zorunda kalmasın.
	row, msgs, err := h.svc.AdminGet(c.Request.Context(), publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, adminTicketDTO(row, msgs))
}

// AdminSetStatus PATCH /admin/tickets/:id/status
func (h *Ticket) AdminSetStatus(c *gin.Context) {
	publicID, ok := h.adminScope(c)
	if !ok {
		return
	}
	var req dto.UpdateTicketStatusRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	target, _ := ticketdom.ParseStatus(req.Status)

	if _, err := h.svc.AdminSetStatus(c.Request.Context(), publicID, target); err != nil {
		h.r.Fail(c, err)
		return
	}
	row, msgs, err := h.svc.AdminGet(c.Request.Context(), publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, adminTicketDTO(row, msgs))
}

/* ═══════════════════════ Yardımcılar ═══════════════════════ */

// scope oturum + kimlik ayrıştırması (kullanıcı yolu).
func (h *Ticket) scope(c *gin.Context) (int64, uuid.UUID, bool) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return 0, uuid.Nil, false
	}
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		// Bozuk kimlik ile var olmayan kimlik AYNI yanıtı alır.
		h.r.Fail(c, ticketsvc.ErrNotFound)
		return 0, uuid.Nil, false
	}
	return userID, publicID, true
}

// adminScope kimlik ayrıştırması (yönetim yolu; yetki middleware'dedir).
func (h *Ticket) adminScope(c *gin.Context) (uuid.UUID, bool) {
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.r.Fail(c, ticketsvc.ErrNotFound)
		return uuid.Nil, false
	}
	return publicID, true
}

// ticketStatusLabel durumun kullanıcıya gösterilecek Türkçe adı.
func ticketStatusLabel(s db.TicketStatus) string {
	switch s {
	case db.TicketStatusOPEN:
		return "Açık"
	case db.TicketStatusANSWERED:
		return "Yanıtlandı"
	case db.TicketStatusUSERREPLIED:
		return "Yanıtınız iletildi"
	case db.TicketStatusCLOSED:
		return "Kapatıldı"
	default:
		return string(s)
	}
}

func ticketPriorityLabel(p db.TicketPriority) string {
	switch p {
	case db.TicketPriorityLOW:
		return "Düşük"
	case db.TicketPriorityHIGH:
		return "Yüksek"
	case db.TicketPriorityNORMAL:
		return "Normal"
	default:
		return string(p)
	}
}

// ticketAuthorLabel yazar etiketi.
//
// 🔴 Personelin kullanıcı adı BU ETİKETTE KULLANILMAZ: kullanıcı "Destek
// ekibi" görür. Kim yanıtladığı destek ekibinin iç bilgisidir ve yönetim
// yanıtındaki ayrı `authorUsername` alanından okunur.
//
// test: ticket_integration_test.go#TestStaffUsernameNeverLeaksToUser
func ticketAuthorLabel(isStaff bool) string {
	if isStaff {
		return "Destek ekibi"
	}
	return "Siz"
}

func rfc3339OrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func ticketMessagesDTO(msgs []ticketsvc.Message) []dto.TicketMessageResponse {
	out := make([]dto.TicketMessageResponse, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, dto.TicketMessageResponse{
			ID:             m.PublicID.String(),
			Body:           m.Body,
			IsStaff:        m.IsStaff,
			AuthorLabel:    ticketAuthorLabel(m.IsStaff),
			AuthorUsername: m.AuthorUsername,
			CreatedAt:      m.CreatedAt.Format(time.RFC3339),
		})
	}
	return out
}

// ticketDTO kullanıcıya dönen talep görünümü.
//
// SIZDIRILMAYANLAR: sayısal id, user_id, personelin kullanıcı adı.
func ticketDTO(t db.Ticket, messageCount int64, msgs []ticketsvc.Message) dto.TicketResponse {
	resp := dto.TicketResponse{
		ID:            t.PublicID.String(),
		Subject:       t.Subject,
		Priority:      string(t.Priority),
		PriorityLabel: ticketPriorityLabel(t.Priority),
		Status:        string(t.Status),
		StatusLabel:   ticketStatusLabel(t.Status),
		MessageCount:  messageCount,
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
		LastReplyAt:   t.LastReplyAt.Format(time.RFC3339),
		ClosedAt:      rfc3339OrEmpty(t.ClosedAt),
	}
	if msgs != nil {
		resp.Messages = ticketMessagesDTO(msgs)
	}
	return resp
}

// adminTicketDTO yönetim görünümü — sahibi ve yazarların adları dâhil.
func adminTicketDTO(t db.GetTicketForAdminRow, msgs []ticketsvc.Message) dto.AdminTicketResponse {
	return dto.AdminTicketResponse{
		ID:            t.PublicID.String(),
		UserID:        t.UserPublicID.String(),
		UserEmail:     t.UserEmail,
		UserUsername:  t.UserUsername,
		Subject:       t.Subject,
		Priority:      string(t.Priority),
		PriorityLabel: ticketPriorityLabel(t.Priority),
		Status:        string(t.Status),
		StatusLabel:   ticketStatusLabel(t.Status),
		MessageCount:  int64(len(msgs)),
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
		LastReplyAt:   t.LastReplyAt.Format(time.RFC3339),
		ClosedAt:      rfc3339OrEmpty(t.ClosedAt),
		Messages:      ticketMessagesDTO(msgs),
	}
}
