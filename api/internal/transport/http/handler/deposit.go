package handler

// Bakiye yükleme — KULLANICI uçları (FR-500, FR-501).
//
// Yönetim uçları (onay/red/liste) bilerek admin.go içindedir: o dosya
// "yönetim uçları" dosyasıdır ve izin gerektiren her şey orada toplanır.

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	auditsvc "github.com/ikmetrik/sms-platform/api/internal/service/audit"
	depositsvc "github.com/ikmetrik/sms-platform/api/internal/service/deposit"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// receiptField dekont dosyasının multipart alan adı.
const receiptField = "dekont"

// Deposit bakiye yükleme uç noktaları.
type Deposit struct {
	svc *depositsvc.Service
	r   Responder
}

func NewDeposit(svc *depositsvc.Service, r Responder) *Deposit {
	return &Deposit{svc: svc, r: r}
}

/* ═══════════════════════ Ödeme yöntemleri ═══════════════════════ */

// Methods GET /wallet/deposit-methods
//
// Yalnız AKTİF yöntemler döner. Pasif bir yöntemin config'i (IBAN, cüzdan
// adresi) yanıta HİÇ girmez — servis katmanı zaten pasifleri getirmez.
//
// test: deposit_integration_test.go#TestInactiveMethodIsNotVisibleToUser
func (h *Deposit) Methods(c *gin.Context) {
	rows, err := h.svc.Methods(c.Request.Context())
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	items := make([]dto.DepositMethodPublicResponse, 0, len(rows))
	for _, m := range rows {
		items = append(items, publicMethodDTO(m))
	}
	h.r.OK(c, dto.DepositMethodListResponse{Items: items})
}

/* ═══════════════════════ Talepler ═══════════════════════ */

// Create POST /wallet/deposits
func (h *Deposit) Create(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	var req dto.CreateDepositRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}
	methodID, err := uuid.Parse(strings.TrimSpace(req.MethodID))
	if err != nil {
		h.r.FailField(c, []dto.FieldError{{Field: "methodId", Message: "Geçersiz ödeme yöntemi."}})
		return
	}

	dep, err := h.svc.Create(c.Request.Context(), depositsvc.CreateInput{
		UserID:         userID,
		MethodPublicID: methodID,
		// İstemci aynı talebi iki kez göndermesin diye: ağ yanıtı yutarsa
		// kullanıcı tekrar dener ve ikinci istek AYNI talebi geri alır.
		IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")),
		AmountMinor:    req.AmountMinor,
		Reference:      req.Reference,
		Note:           req.Note,
	})
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, depositDTO(dep))
}

// List GET /wallet/deposits
func (h *Deposit) List(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	limit, offset := pagination(c, 20, 100)

	// `?status=` — geçersiz değer SESSİZCE YOK SAYILMAZ: yok sayılsaydı yanlış
	// yazılmış bir süzgeç, süzgeçsiz TÜM listeyi döndürür ve kullanıcı bunu
	// "süzgeç çalıştı" sanardı. `GET /orders` ile aynı davranış.
	var status *db.DepositStatus
	if v := strings.TrimSpace(c.Query("status")); v != "" {
		var st db.DepositStatus
		if err := st.Scan(v); err != nil {
			h.r.FailField(c, []dto.FieldError{{Field: "status", Message: "Geçersiz durum süzgeci."}})
			return
		}
		status = &st
	}

	rows, total, err := h.svc.List(c.Request.Context(), userID, status, limit, offset)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	items := make([]dto.DepositResponse, 0, len(rows))
	for _, d := range rows {
		items = append(items, depositDTO(d))
	}
	h.r.OK(c, dto.DepositListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

// Get GET /wallet/deposits/:id
func (h *Deposit) Get(c *gin.Context) {
	userID, publicID, ok := h.scope(c)
	if !ok {
		return
	}
	dep, err := h.svc.Get(c.Request.Context(), userID, publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, depositDTO(dep))
}

/* ═══════════════════════ Dekont (FR-500 / KK-500) ═══════════════════════ */

// UploadReceipt POST /wallet/deposits/:id/receipt
//
// Gövde multipart/form-data; dosya alanının adı "dekont".
//
// İstemcinin gönderdiği dosya adı ve Content-Type OKUNMAZ: tip kararı
// sihirli baytlarla depo katmanında verilir. Bu yüzden burada dosya adı
// hiçbir değişkene atanmaz — atansaydı, bir gün birinin onu yola koyması
// yalnız bir satır uzaklıkta olurdu.
//
// test: deposit_integration_test.go#TestReceiptUploadRejectsFakeImage
func (h *Deposit) UploadReceipt(c *gin.Context) {
	userID, publicID, ok := h.scope(c)
	if !ok {
		return
	}

	part, err := firstFilePart(c.Request)
	if err != nil {
		h.r.FailField(c, []dto.FieldError{{
			Field: receiptField, Message: "Bir dekont dosyası seçmelisiniz (JPEG, PNG veya PDF)."}})
		return
	}
	defer func() { _ = part.Close() }()

	dep, err := h.svc.AttachReceipt(c.Request.Context(), userID, publicID, part)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, depositDTO(dep))
}

// Receipt GET /wallet/deposits/:id/receipt — kullanıcının KENDİ dekontu.
func (h *Deposit) Receipt(c *gin.Context) {
	userID, publicID, ok := h.scope(c)
	if !ok {
		return
	}
	h.serveReceipt(c, &userID, publicID)
}

// serveReceipt dosyayı yetkili çağırana akıtır.
//
// Dosya WEB KÖKÜNÜN DIŞINDA durur ve hiçbir statik sunucuya bağlı değildir;
// tek erişim yolu budur (KK-500). Content-Type SUNUCUNUN belirlediği tiptir,
// istemcinin iddia ettiği değil; `nosniff` + `attachment` ile tarayıcının
// içeriği çalıştırma ihtimali de kapatılır.
//
// test: deposit_integration_test.go#TestReceiptIsNotReachableByURL
func (h *Deposit) serveReceipt(c *gin.Context, userID *int64, publicID uuid.UUID) {
	rc, meta, err := h.svc.OpenReceipt(c.Request.Context(), userID, publicID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	defer func() { _ = rc.Close() }()

	c.Header("Content-Type", meta.MIME)
	c.Header("Content-Disposition", `attachment; filename="dekont"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-store")
	c.DataFromReader(http.StatusOK, meta.Size, meta.MIME, rc, nil)
}

// firstFilePart gövdedeki ilk dosya bölümünü AKIŞ olarak döner.
//
// ParseMultipartForm yerine MultipartReader tercih edildi: o çağrı bellekte
// tampon ayırır ve büyük gövdeleri geçici dosyalara yazar — yani sınır
// kontrolümüzden ÖNCE diske yazmış olur. MultipartReader ile bölüm doğrudan
// depo katmanına akar ve okuma sınırı (io.LimitReader) orada uygulanır.
func firstFilePart(req *http.Request) (*multipart.Part, error) {
	mr, err := req.MultipartReader()
	if err != nil {
		return nil, err
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			return nil, err
		}
		if part.FormName() == receiptField && part.FileName() != "" {
			return part, nil
		}
		_ = part.Close()
	}
}

/* ═══════════════════════ Yardımcılar ═══════════════════════ */

// scope oturum + kimlik ayrıştırması.
func (h *Deposit) scope(c *gin.Context) (int64, uuid.UUID, bool) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return 0, uuid.Nil, false
	}
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		// Bozuk kimlik ile var olmayan kimlik AYNI yanıtı alır.
		h.r.Fail(c, depositsvc.ErrNotFound)
		return 0, uuid.Nil, false
	}
	return userID, publicID, true
}

// auditMeta isteğin izlenebilirlik bilgisini servise taşır.
func auditMeta(c *gin.Context) auditsvc.Meta {
	m := auditsvc.Meta{
		UserAgent: c.Request.UserAgent(),
		RequestID: middleware.RequestIDFrom(c.Request.Context()),
	}
	if addr, err := netip.ParseAddr(c.ClientIP()); err == nil {
		m.IP = &addr
	}
	return m
}

// depositStatusLabel durumun kullanıcıya gösterilecek Türkçe adı.
func depositStatusLabel(s db.DepositStatus) string {
	switch s {
	case db.DepositStatusPENDING:
		return "Onay bekliyor"
	case db.DepositStatusCOMPLETED:
		return "Onaylandı"
	case db.DepositStatusREJECTED:
		return "Reddedildi"
	case db.DepositStatusREFUNDED:
		return "İade edildi"
	default:
		return string(s)
	}
}

// referenceLabel kullanıcıya referans alanında ne isteneceğini söyler.
func referenceLabel(kind db.DepositMethodKind) string {
	if kind == db.DepositMethodKindCRYPTO {
		return "İşlem hash'i (TX)"
	}
	return "Havale açıklaması / dekont numarası"
}

func publicMethodDTO(m db.DepositMethod) dto.DepositMethodPublicResponse {
	var cfg map[string]string
	_ = json.Unmarshal(m.Config, &cfg)
	if cfg == nil {
		cfg = map[string]string{}
	}
	return dto.DepositMethodPublicResponse{
		ID:              m.PublicID.String(),
		Code:            m.Code,
		Kind:            string(m.Kind),
		Name:            m.Name,
		Instructions:    m.Instructions,
		Config:          cfg,
		MinAmount:       moneyDTO(money.New(m.MinAmountMinor, money.TRY)),
		MaxAmount:       moneyDTO(money.New(m.MaxAmountMinor, money.TRY)),
		ReferenceLabel:  referenceLabel(m.Kind),
		ReceiptRequired: m.Kind == db.DepositMethodKindBANKTRANSFER,
	}
}

// depositDTO kullanıcıya dönen talep görünümü.
//
// SIZDIRILMAYANLAR: sayısal id, receipt_path, admin_note, reviewed_by_user_id.
// Dosya yolunu vermek KK-500'ün "doğrudan URL ile servis edilemez"
// garantisini anlamsız kılardı; admin_note ise yöneticinin iç değerlendirmesidir.
//
// test: deposit_integration_test.go#TestReceiptPathNeverLeaves
func depositDTO(d db.Deposit) dto.DepositResponse {
	resp := dto.DepositResponse{
		ID:              d.PublicID.String(),
		Method:          d.MethodName,
		Amount:          moneyDTO(money.New(d.AmountMinor, money.TRY)),
		Credited:        moneyDTO(money.New(d.CreditedMinor, money.TRY)),
		Status:          string(d.Status),
		StatusLabel:     depositStatusLabel(d.Status),
		Network:         d.Network,
		Note:            d.UserNote,
		RejectionReason: d.RejectionReason,
		HasReceipt:      d.ReceiptPath != "",
		CreatedAt:       d.CreatedAt.Format(time.RFC3339),
	}
	if d.ReviewedAt != nil {
		resp.ReviewedAt = d.ReviewedAt.Format(time.RFC3339)
	}
	return resp
}

// adminDepositDTO yönetim listesindeki talep görünümü.
func adminDepositDTO(row db.ListDepositsForAdminRow) dto.AdminDepositResponse {
	resp := dto.AdminDepositResponse{
		ID:              row.PublicID.String(),
		UserID:          row.UserPublicID.String(),
		UserEmail:       row.UserEmail,
		UserUsername:    row.UserUsername,
		Method:          row.MethodName,
		Amount:          moneyDTO(money.New(row.AmountMinor, money.TRY)),
		Credited:        moneyDTO(money.New(row.CreditedMinor, money.TRY)),
		Status:          string(row.Status),
		StatusLabel:     depositStatusLabel(row.Status),
		TxHash:          deref(row.TxHash),
		Network:         row.Network,
		UserNote:        row.UserNote,
		AdminNote:       row.AdminNote,
		RejectionReason: row.RejectionReason,
		HasReceipt:      row.ReceiptPath != "",
		CreatedAt:       row.CreatedAt.Format(time.RFC3339),
	}
	if row.ReviewedAt != nil {
		resp.ReviewedAt = row.ReviewedAt.Format(time.RFC3339)
	}
	return resp
}

// reviewDTO onay/red sonucunun yönetim görünümü.
func reviewDTO(res depositsvc.ReviewResult) dto.DepositReviewResponse {
	d := res.Deposit
	item := dto.AdminDepositResponse{
		ID:              d.PublicID.String(),
		UserID:          res.UserPublicID.String(),
		UserEmail:       res.UserEmail,
		UserUsername:    res.UserUsername,
		Method:          d.MethodName,
		Amount:          moneyDTO(money.New(d.AmountMinor, money.TRY)),
		Credited:        moneyDTO(money.New(d.CreditedMinor, money.TRY)),
		Status:          string(d.Status),
		StatusLabel:     depositStatusLabel(d.Status),
		TxHash:          deref(d.TxHash),
		Network:         d.Network,
		UserNote:        d.UserNote,
		AdminNote:       d.AdminNote,
		RejectionReason: d.RejectionReason,
		HasReceipt:      d.ReceiptPath != "",
		CreatedAt:       d.CreatedAt.Format(time.RFC3339),
	}
	if d.ReviewedAt != nil {
		item.ReviewedAt = d.ReviewedAt.Format(time.RFC3339)
	}
	return dto.DepositReviewResponse{
		Deposit:        item,
		Balance:        moneyDTO(res.NewBalance),
		AlreadyApplied: res.AlreadyApplied,
	}
}
