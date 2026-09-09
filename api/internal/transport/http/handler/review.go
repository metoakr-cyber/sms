package handler

// Müşteri yorumları — SİTE, KULLANICI ve YÖNETİM uçları.
//
// Üçü aynı dosyadadır çünkü aynı verinin üç görünümüdür; ayırmak, birinde
// yapılan bir alan değişikliğinin diğerinde unutulmasını kolaylaştırırdı.
// Ayrım kodda değil, ROTA KAYDINDADIR: yönetim uçları izin ara katmanının
// arkasındadır, sitedeki liste ise oturumsuzdur (router.go).

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	reviewdom "github.com/ikmetrik/sms-platform/api/internal/domain/review"
	reviewsvc "github.com/ikmetrik/sms-platform/api/internal/service/review"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Review müşteri yorumu uç noktaları.
type Review struct {
	svc *reviewsvc.Service
	r   Responder
}

func NewReview(svc *reviewsvc.Service, r Responder) *Review {
	return &Review{svc: svc, r: r}
}

/* ═══════════════════════ Site ucu (oturumsuz) ═══════════════════════ */

// Public GET /catalog/reviews — ONAYLI yorumlar.
//
// Oturum GEREKTİRMEZ: sitedeki bölüm ziyaretçiye de görünür ve SEO açısından
// sunucu bileşeninden çekilebilmelidir (docs/frontend-contract.md §10).
//
// 🔴 Yanıtta kullanıcı e-postası YOKTUR; sorgu onu SEÇMEZ bile.
// test: review_integration_test.go#TestPublicReviewsNeverExposeEmail
func (h *Review) Public(c *gin.Context) {
	items, sum, err := h.svc.PublicList(c.Request.Context())
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	out := make([]dto.PublicReviewResponse, 0, len(items))
	for _, r := range items {
		out = append(out, dto.PublicReviewResponse{
			ID:          r.PublicID.String(),
			Rating:      int(r.Rating),
			Body:        r.Body,
			AuthorName:  r.AuthorName,
			PublishedAt: rfc3339OrEmpty(r.PublishedAt),
		})
	}
	h.r.OK(c, dto.PublicReviewListResponse{
		Items: out, Total: sum.Total, AverageX10: sum.AverageX10,
	})
}

/* ═══════════════════════ Kullanıcı uçları ═══════════════════════ */

// Create POST /reviews
func (h *Review) Create(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	var req dto.CreateReviewRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}

	row, err := h.svc.Submit(c.Request.Context(), reviewsvc.SubmitInput{
		UserID: userID, Rating: req.Rating, Body: req.Body,
	})
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, customerReviewDTO(row))
}

// Mine GET /reviews/mine — kullanıcının KENDİ yorumları ve durumları.
func (h *Review) Mine(c *gin.Context) {
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
	items := make([]dto.ReviewResponse, 0, len(rows))
	for _, r := range rows {
		items = append(items, customerReviewDTO(r))
	}
	h.r.OK(c, dto.ReviewListResponse{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

// Get GET /reviews/:id — kullanıcının KENDİ yorumu.
//
// Sahiplik SORGUNUN parçasıdır; başkasının yorumu 404 döner (değişmez #7).
func (h *Review) Get(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	publicID, ok := h.parseID(c)
	if !ok {
		return
	}
	row, err := h.svc.Get(c.Request.Context(), userID, publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, customerReviewDTO(row))
}

/* ═══════════════════════ Yönetim uçları ═══════════════════════ */

// AdminList GET /admin/reviews — durum süzgeci + sayfalama.
func (h *Review) AdminList(c *gin.Context) {
	var filter reviewsvc.AdminFilter
	if raw := c.Query("status"); raw != "" {
		s, ok := reviewdom.ParseStatus(raw)
		if !ok {
			h.r.FailField(c, []dto.FieldError{
				{Field: "status", Message: "Geçersiz durum süzgeci."}})
			return
		}
		filter.Status = &s
	}

	limit, offset := pagination(c, 20, 100)
	rows, total, pending, err := h.svc.AdminList(c.Request.Context(), filter, limit, offset)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	items := make([]dto.AdminReviewResponse, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.AdminReviewResponse{
			ID:              r.PublicID.String(),
			UserID:          r.UserPublicID.String(),
			UserEmail:       r.UserEmail,
			UserUsername:    r.UserUsername,
			Rating:          int(r.Rating),
			Body:            r.Body,
			Status:          string(r.Status),
			StatusLabel:     reviewStatusLabel(r.Status),
			RejectionReason: r.RejectionReason,
			CreatedAt:       r.CreatedAt.Format(time.RFC3339),
			ReviewedAt:      rfc3339OrEmpty(r.ReviewedAt),
		})
	}
	h.r.OK(c, dto.AdminReviewListResponse{
		Items: items, Total: total, PendingTotal: pending,
		Limit: limit, Offset: offset,
	})
}

// Approve POST /admin/reviews/:id/approve
//
// POST'tur çünkü durum DEĞİŞTİRİR (değişmez #8). Eski sistemde onay bir GET'ti
// ve bir <img src="…/approve"> etiketiyle tetiklenebiliyordu (design.md §11).
func (h *Review) Approve(c *gin.Context) {
	h.decide(c, reviewdom.StatusApproved, "")
}

// Reject POST /admin/reviews/:id/reject — GEREKÇE ZORUNLU.
func (h *Review) Reject(c *gin.Context) {
	var req dto.RejectReviewRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	h.decide(c, reviewdom.StatusRejected, req.Reason)
}

// decide onay ve reddin ORTAK gövdesidir; fark yalnız hedef durum ve gerekçe.
func (h *Review) decide(c *gin.Context, target reviewdom.Status, reason string) {
	staffID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	publicID, ok := h.parseID(c)
	if !ok {
		return
	}
	row, err := h.svc.Decide(c.Request.Context(), staffID, publicID, target, reason)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, customerReviewDTO(row))
}

/* ═══════════════════════ Yardımcılar ═══════════════════════ */

// parseID kimlik ayrıştırması. Bozuk kimlik ile var olmayan kimlik AYNI
// yanıtı alır: hangi UUID'lerin geçerli olduğu sızdırılmaz.
func (h *Review) parseID(c *gin.Context) (uuid.UUID, bool) {
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.r.Fail(c, reviewsvc.ErrNotFound)
		return uuid.Nil, false
	}
	return publicID, true
}

// reviewStatusLabel durumun kullanıcıya gösterilecek Türkçe adı.
//
// Etiket KULLANICI GÖZÜNDEN yazılmıştır: "APPROVED" onun için "Yayında"dır,
// "onaylandı" değil — yorumunun sitede görünüp görünmediğini merak eder.
func reviewStatusLabel(s db.ReviewStatus) string {
	switch s {
	case db.ReviewStatusPENDING:
		return "Onay bekliyor"
	case db.ReviewStatusAPPROVED:
		return "Yayında"
	case db.ReviewStatusREJECTED:
		return "Yayımlanmadı"
	default:
		return string(s)
	}
}

// customerReviewDTO kullanıcıya dönen yorum görünümü.
//
// SIZDIRILMAYANLAR: sayısal id, user_id, kararı veren yöneticinin kimliği.
// Kullanıcının "kim reddetti" bilgisine ihtiyacı yoktur ve o bilgi personeli
// hedef hâline getirir.
func customerReviewDTO(r db.Review) dto.ReviewResponse {
	return dto.ReviewResponse{
		ID:              r.PublicID.String(),
		Rating:          int(r.Rating),
		Body:            r.Body,
		Status:          string(r.Status),
		StatusLabel:     reviewStatusLabel(r.Status),
		RejectionReason: r.RejectionReason,
		CreatedAt:       r.CreatedAt.Format(time.RFC3339),
		ReviewedAt:      rfc3339OrEmpty(r.ReviewedAt),
	}
}
