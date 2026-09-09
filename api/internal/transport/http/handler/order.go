package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	ordersvc "github.com/ikmetrik/sms-platform/api/internal/service/order"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// OrderStream sipariş olaylarına abone olur.
type OrderStream interface {
	Subscribe(ctx context.Context, orderPublicID string) (<-chan []byte, error)
}

// Order sipariş uç noktaları.
type Order struct {
	svc    *ordersvc.Service
	stream OrderStream
	r      Responder
}

func NewOrder(svc *ordersvc.Service, stream OrderStream, r Responder) *Order {
	return &Order{svc: svc, stream: stream, r: r}
}

/* ═══════════════════════════ Satın alma ═══════════════════════════ */

// Create POST /orders
//
// GÖVDE YALNIZ {quoteId} ALIR (CLAUDE.md değişmez #9). Fiyat, sağlayıcı ve
// maliyet istemciden gelmez; eski prototipte kullanıcı istediği fiyatı
// gövdede gönderebiliyordu.
func (h *Order) Create(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	var req dto.CreateOrderRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	quoteID, err := uuid.Parse(req.QuoteID)
	if err != nil {
		h.r.FailField(c, []dto.FieldError{{Field: "quoteId", Message: "Geçersiz teklif kimliği."}})
		return
	}

	ord, err := h.svc.Create(c.Request.Context(), ordersvc.CreateInput{
		UserID: userID, QuoteID: quoteID,
	})
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, orderDTO(ord, nil))
}

/* ═══════════════════════════ Görüntüleme ═══════════════════════════ */

// Get GET /orders/:id
func (h *Order) Get(c *gin.Context) {
	userID, publicID, ok := h.scope(c)
	if !ok {
		return
	}
	d, err := h.svc.Get(c.Request.Context(), userID, publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, orderDTO(d.Order, d.Messages))
}

// List GET /orders
func (h *Order) List(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	limit, offset := pagination(c, 20, 100)
	rows, total, err := h.svc.List(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	items := make([]dto.OrderResponse, 0, len(rows))
	for _, r := range rows {
		d := orderDTO(r.Order, nil)
		d.IconURL = r.IconURL
		items = append(items, d)
	}
	h.r.OK(c, dto.OrderListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

// Cancel DELETE /orders/:id
//
// DELETE kullanılır çünkü durum DEĞİŞTİRİR. GET ile durum değiştirmek
// yasaktır (değişmez #8).
func (h *Order) Cancel(c *gin.Context) {
	userID, publicID, ok := h.scope(c)
	if !ok {
		return
	}
	ord, err := h.svc.Cancel(c.Request.Context(), userID, publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, orderDTO(ord, nil))
}

/* ═══════════════════════════ SSE ═══════════════════════════ */

// sseKeepalive yorum satırı aralığı.
//
// Vekiller ve mobil operatörler sessiz bağlantıları 30–60 saniyede kapatır.
// Keepalive olmadan kullanıcı kodu beklerken bağlantı sessizce ölür ve
// istemci bunu ancak yoklama yedeğiyle fark eder.
const sseKeepalive = 20 * time.Second

// Stream GET /orders/:id/stream — Server-Sent Events.
//
// AKIŞ İLK KODDA KAPANMAZ (FR-415): ikinci doğrulama kodu da gelebilir.
// Yalnız TERMİNAL durumda kapanır.
func (h *Order) Stream(c *gin.Context) {
	userID, publicID, ok := h.scope(c)
	if !ok {
		return
	}

	// Sahiplik ÖNCE doğrulanır: başkasının akışına bağlanma denemesi 404
	// almalıdır (KK-403). Aboneliği önce kurup sonra kontrol etmek, kanal
	// adının varlığını sızdırır.
	d, err := h.svc.Get(c.Request.Context(), userID, publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}

	ctx := c.Request.Context()
	events, err := h.stream.Subscribe(ctx, publicID.String())
	if err != nil {
		h.r.Fail(c, apperr.Internal(err))
		return
	}

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache, no-transform")
	hdr.Set("Connection", "keep-alive")
	// Nginx/Caddy ara belleklemesini kapat. Bu başlık olmadan olaylar
	// vekilde birikir ve kullanıcı kodu 30 saniye geç görür.
	hdr.Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, canFlush := c.Writer.(http.Flusher)
	if !canFlush {
		// Flush edemeyen bir yazıcıda SSE ÇALIŞMAZ. Sessizce akış açmış gibi
		// yapmak, kullanıcıyı hiç gelmeyecek bir olayı beklerken bırakır.
		h.r.Fail(c, apperr.Internal(errors.New("sse: akış desteklenmiyor")))
		return
	}

	// AÇILIŞ OLAYI: istemci bağlandığı anda mevcut durumu alır.
	// Beklemezse, bağlanmadan hemen önce gelmiş bir kodu asla göremez.
	// test: scripts/smoke-auth.sh (SSE açılış olayı)
	h.sendEvent(c, flusher, "status", statusEvent(d))

	if isTerminal(d.Order.Status) {
		h.sendEvent(c, flusher, "close", gin.H{"reason": "terminal"})
		return
	}

	ticker := time.NewTicker(sseKeepalive)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			// Yorum satırı: olay değildir, istemci tarafında hiçbir dinleyici
			// tetiklemez ama bağlantıyı canlı tutar.
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case raw, open := <-events:
			if !open {
				return
			}
			// Olay adı gövdedeki `type` alanından okunur: SSE'de `event:`
			// satırı istemcinin hangi dinleyiciyi tetikleyeceğini belirler.
			var ev struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(raw, &ev); err != nil {
				continue
			}
			name := ev.Type
			if name == "" {
				name = "status"
			}
			h.sendRaw(c, flusher, name, raw)

			if isTerminal(db.OrderStatus(ev.Status)) {
				h.sendEvent(c, flusher, "close", gin.H{"reason": "terminal"})
				return
			}
		}
	}
}

func (h *Order) sendEvent(c *gin.Context, f http.Flusher, name string, payload any) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.sendRaw(c, f, name, buf)
}

func (h *Order) sendRaw(c *gin.Context, f http.Flusher, name string, payload []byte) {
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", name, payload); err != nil {
		return
	}
	f.Flush()
}

func statusEvent(d ordersvc.Detail) gin.H {
	return gin.H{
		"orderId":   d.Order.PublicID.String(),
		"status":    string(d.Order.Status),
		"expiresAt": d.Order.ExpiresAt.Format(time.RFC3339),
		// SSE yolu da SÜZÜLÜR: kullanıcıya iki kanal var (GET ve akış),
		// yalnız birini süzmek sızıntıyı kapatmaz.
		"messages": visibleMessages(d.Order, d.Messages),
	}
}

func isTerminal(s db.OrderStatus) bool {
	return s == db.OrderStatusCOMPLETED || s == db.OrderStatusREFUNDED
}

/* ═══════════════════════════ Yardımcılar ═══════════════════════════ */

func (h *Order) scope(c *gin.Context) (int64, uuid.UUID, bool) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return 0, uuid.Nil, false
	}
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		// Bozuk kimlik ile var olmayan kimlik AYNI yanıtı alır.
		h.r.Fail(c, apperr.ErrOrderNotFound)
		return 0, uuid.Nil, false
	}
	return userID, publicID, true
}

func pagination(c *gin.Context, def, max int32) (int32, int32) {
	limit := def
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = int32(v)
		if limit > max {
			limit = max
		}
	}
	var offset int32
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v > 0 {
		offset = int32(v)
	}
	return limit, offset
}

func messageViews(msgs []db.OrderMessage) []dto.OrderMessageResponse {
	out := make([]dto.OrderMessageResponse, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, dto.OrderMessageResponse{
			Code: m.Code, Body: m.Body, Sender: m.Sender,
			ReceivedAt: m.ReceivedAt.Format(time.RFC3339),
		})
	}
	return out
}

// orderDTO sipariş satırını dışa açık biçime çevirir.
//
// SIZDIRILMAYANLAR: sayısal id, provider_id, cost_micro, fx_rate.
// Maliyetimizi ve hangi sağlayıcıyı kullandığımızı kullanıcıya söylemek,
// hem ticari bilgidir hem de sağlayıcı seçimini manipüle etme denemelerine
// zemin hazırlar.
// visibleMessages kullanıcıya GÖSTERİLECEK mesajları döner.
//
// 🔴 İADE EDİLMİŞ SİPARİŞİN KODU GÖSTERİLMEZ. Süre dolumu ya da iptal sonrası
// (durum CANCELLED/REFUNDED, para GERİ VERİLMİŞ) sağlayıcıdan gecikmeli bir
// SMS gelebilir — CloseAtProvider bunu kaydeder ama açmaz. Göstermek,
// kullanıcıya hem parayı hem numarayı vermek olurdu.
//
// Mesaj veritabanında DURUR: "kod gelmedi diye iade ettik ama aslında gelmişti"
// tartışmasının tek kanıtı odur. Süzgeç sunumda, saklamada değil.
//
// test: ../../../service/order/order_integration_test.go#TestRefundedOrderDoesNotLeakCode
func visibleMessages(o db.Order, msgs []db.OrderMessage) []dto.OrderMessageResponse {
	switch o.Status {
	case db.OrderStatusCANCELLED, db.OrderStatusREFUNDED:
		return []dto.OrderMessageResponse{}
	}
	return messageViews(msgs)
}

func orderDTO(o db.Order, msgs []db.OrderMessage) dto.OrderResponse {
	resp := dto.OrderResponse{
		ID:          o.PublicID.String(),
		Status:      string(o.Status),
		PhoneNumber: o.PhoneNumber,
		ServiceCode: o.ServiceCode,
		ServiceName: o.ServiceName,
		CountryISO2: o.CountryIso2,
		CountryName: o.CountryName,
		PhoneCode:   o.PhoneCode,
		Price:       moneyDTO(money.New(o.PricePaidMinor, money.TRY)),
		ExpiresAt:   o.ExpiresAt.Format(time.RFC3339),
		// Kalan süre SUNUCUDAN gider: istemcinin saati yanlış olabilir ve
		// yerel sayaç sekme donduğunda durur (frontend-contract.md §2.6).
		ExpiresIn:     int(time.Until(o.ExpiresAt).Seconds()),
		CancellableAt: o.CancellableAt.Format(time.RFC3339),
		CancellableIn: int(time.Until(o.CancellableAt).Seconds()),
		CreatedAt:     o.CreatedAt.Format(time.RFC3339),
		Messages:      visibleMessages(o, msgs),
	}
	if resp.ExpiresIn < 0 {
		resp.ExpiresIn = 0
	}
	if resp.CancellableIn < 0 {
		resp.CancellableIn = 0
	}
	return resp
}
