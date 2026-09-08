package handler

// Yönetim uçları.
//
// HER UÇ AYRI BİR İZİN İSTER (router.go). "admin ise her şeyi yapabilir"
// modeli, bir hesabın ele geçirilmesini toplam kayba çevirir.
//
// 🔴 BU DOSYADAKİ HİÇBİR YANIT ŞUNLARI İÇERMEZ:
// test: admin_integration_test.go#TestListUsersNeverReturnsPasswordHash
// test: admin_integration_test.go#TestListProvidersNeverReturnsAPIKey
//
//   · parola özeti (password_hash)
//   · sağlayıcı API anahtarı — şifreli hâli bile
//   · sayısal veritabanı kimlikleri (kullanıcı/sipariş için)

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	depositdom "github.com/ikmetrik/sms-platform/api/internal/domain/deposit"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	depositsvc "github.com/ikmetrik/sms-platform/api/internal/service/deposit"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Admin yönetim uçları.
type Admin struct {
	queries *db.Queries
	secrets *crypto.SecretBox
	rules   *pricingsvc.RuleService
	sync    CatalogSyncer
	r       Responder
}

// AdminDeps yönetim işleyicisinin bağımlılıkları.
//
// Yapı ile verilir, konumsal parametrelerle değil: her yeni yönetim özelliği
// bir bağımlılık ekliyor ve altı parametreli bir kurucuda `nil` yerini
// şaşırmak sessizce çalışmayan bir uç üretiyordu.
type AdminDeps struct {
	Queries *db.Queries
	Secrets *crypto.SecretBox
	// Rules nil ise fiyat kuralı uçları 500 döner — sessizce boş liste
	// dönmezler; bağlanmamış bir servis fark edilmelidir.
	Rules *pricingsvc.RuleService
	// Sync nil ise katalog senkronu ucu 503 döner.
	Sync      CatalogSyncer
	Responder Responder
}

func NewAdmin(d AdminDeps) *Admin {
	return &Admin{
		queries: d.Queries, secrets: d.Secrets,
		rules: d.Rules, sync: d.Sync, r: d.Responder,
	}
}

/* ═══════════════════════════ Kullanıcılar ═══════════════════════════ */

// ListUsers GET /admin/users — izin: users:read
func (a *Admin) ListUsers(c *gin.Context) {
	limit, offset := pagination(c, 25, 100)

	var q *string
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		q = &s
	}
	var status *db.UserStatus
	if s := c.Query("status"); s != "" {
		var st db.UserStatus
		if err := st.Scan(s); err != nil {
			a.r.FailField(c, []dto.FieldError{{Field: "status", Message: "Geçersiz durum."}})
			return
		}
		status = &st
	}

	rows, err := a.queries.ListUsersForAdmin(c.Request.Context(), db.ListUsersForAdminParams{
		Q: q, Status: status, Lim: limit, Off: offset,
	})
	if err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}
	total, err := a.queries.CountUsersForAdmin(c.Request.Context(), db.CountUsersForAdminParams{
		Q: q, Status: status,
	})
	if err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}

	items := make([]dto.AdminUserResponse, 0, len(rows))
	for _, u := range rows {
		// BOŞ DİZİ, null DEĞİL. `null` gelen bir dizi alanı istemcide
		// `.map is not a function` hatası üretir.
		roles := []string{}
		if u.Roles != "" {
			roles = strings.Split(u.Roles, ",")
		}
		items = append(items, dto.AdminUserResponse{
			ID:            u.PublicID.String(),
			Email:         u.Email,
			Username:      u.Username,
			Status:        string(u.Status),
			EmailVerified: u.EmailVerifiedAt != nil,
			Balance:       moneyDTO(money.New(u.BalanceMinor, money.TRY)),
			Roles:         roles,
			OrderCount:    u.OrderCount,
			CreatedAt:     u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	a.r.OK(c, dto.AdminUserListResponse{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

// SetUserStatus PATCH /admin/users/:id/status — izin: users:write
func (a *Admin) SetUserStatus(c *gin.Context) {
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	var req dto.SetUserStatusRequest
	if !bindJSON(c, a.r, &req) {
		return
	}
	var st db.UserStatus
	if err := st.Scan(req.Status); err != nil {
		a.r.FailField(c, []dto.FieldError{{Field: "status", Message: "Geçersiz durum."}})
		return
	}

	// KENDİNİ ASKIYA ALMA KORUMASI.
	//
	// Bir yönetici kendi hesabını askıya alırsa panele erişimi biter ve geri
	// alacak kimse kalmayabilir. Ucuz bir kontrol, pahalı bir kurtarma
	// operasyonunu önler.
	if actor, ok := middleware.UserIDFrom(c); ok {
		self, err := a.queries.GetUserByPublicID(c.Request.Context(), publicID)
		if err == nil && self.ID == actor && st != db.UserStatusACTIVE {
			a.r.Fail(c, apperr.ErrValidation.WithMessage(
				"Kendi hesabınızın durumunu değiştiremezsiniz."))
			return
		}
	}

	u, err := a.queries.SetUserStatusByPublicID(c.Request.Context(), db.SetUserStatusByPublicIDParams{
		PublicID: publicID, Status: st,
	})
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	a.r.OK(c, gin.H{"id": u.PublicID.String(), "status": string(u.Status)})
}

/* ═══════════════════════════ Sağlayıcılar ═══════════════════════════ */

// ListProviders GET /admin/providers — izin: providers:read
func (a *Admin) ListProviders(c *gin.Context) {
	rows, err := a.queries.ListProvidersForAdmin(c.Request.Context())
	if err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}
	items := make([]dto.AdminProviderResponse, 0, len(rows))
	for _, p := range rows {
		var caps []string
		_ = json.Unmarshal(p.Capabilities, &caps)
		items = append(items, dto.AdminProviderResponse{
			ID:       strconv.FormatInt(p.ID, 10),
			Name:     p.Name,
			Protocol: string(p.Protocol),
			BaseURL:  p.BaseUrl,
			IsActive: p.IsActive,
			Priority: p.Priority,
			// 🔴 Anahtarın KENDİSİ değil, VARLIĞI bildirilir.
			HasAPIKey:      p.HasApiKey,
			CostMultiplier: numericString(p.CostMultiplier),
			Capabilities:   caps,
			Balance:        moneyDTO(money.New(p.AccountBalanceMicro, money.USD)),
		})
		// Senkron durumu listeyle birlikte döner: panel tetikleme ucunu
		// yoklayarak durum öğrenemez — her yoklama yeni bir tur başlatırdı.
		if a.sync != nil {
			st := syncDTO(a.sync.State(p.ID))
			items[len(items)-1].Sync = &st
		}
	}
	a.r.OK(c, gin.H{"items": items})
}

// UpdateProvider PATCH /admin/providers/:id — izin: providers:write
func (a *Admin) UpdateProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	var req dto.UpdateProviderRequest
	if !bindJSON(c, a.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		a.r.FailField(c, errs)
		return
	}
	var mult pgtype.Numeric
	if err := mult.Scan(req.CostMultiplier); err != nil {
		a.r.FailField(c, []dto.FieldError{
			{Field: "costMultiplier", Message: "Geçersiz çarpan."}})
		return
	}

	p, err := a.queries.UpdateProviderSettings(c.Request.Context(), db.UpdateProviderSettingsParams{
		ID: id, BaseUrl: req.BaseURL, IsActive: req.IsActive,
		Priority: req.Priority, CostMultiplier: mult,
	})
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	a.r.OK(c, gin.H{"id": strconv.FormatInt(p.ID, 10), "name": p.Name, "isActive": p.IsActive})
}

// SetProviderAPIKey PUT /admin/providers/:id/api-key — izin: providers:write
//
// 🔴 AYRI BİR UÇ, bilinçli olarak.
//
// Diğer ayarlarla aynı uçta olsaydı, ayar değiştiren her istek anahtarı da
// taşırdı; boş bir alan anahtarı SİLERDİ ve sağlayıcı sessizce çalışmaz
// hâle gelirdi. Ayrı uç, "kaydet"e basmanın anahtarı yok etmesini imkânsız
// kılar.
//
// test: admin_integration_test.go#TestSavingProviderSettingsDoesNotEraseAPIKey
//
// Anahtar YANITTA DÖNMEZ; yalnız maskeli bir önizleme döner.
func (a *Admin) SetProviderAPIKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	var req dto.SetAPIKeyRequest
	if !bindJSON(c, a.r, &req) {
		return
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		a.r.FailField(c, []dto.FieldError{
			{Field: "apiKey", Message: "Anahtar boş olamaz. Silmek için sağlayıcıyı pasifleştirin."}})
		return
	}

	enc, err := a.secrets.SealString(key)
	if err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}
	if err := a.queries.UpdateProviderAPIKey(c.Request.Context(), db.UpdateProviderAPIKeyParams{
		ID: id, ApiKeyEnc: enc,
	}); err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}
	// Maskeli önizleme: yöneticinin doğru anahtarı yapıştırdığını
	// doğrulayabilmesi için yeterli, anahtarı ele vermek için değil.
	a.r.OK(c, gin.H{"masked": crypto.Mask(key)})
}

/* ═══════════════════════ Ödeme yöntemleri ═══════════════════════ */

// ListDepositMethods GET /admin/deposit-methods — izin: deposits:read
func (a *Admin) ListDepositMethods(c *gin.Context) {
	rows, err := a.queries.ListDepositMethods(c.Request.Context())
	if err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}
	items := make([]dto.DepositMethodResponse, 0, len(rows))
	for _, m := range rows {
		items = append(items, depositMethodDTO(m))
	}
	a.r.OK(c, gin.H{"items": items})
}

// CreateDepositMethod POST /admin/deposit-methods — izin: deposits:approve
func (a *Admin) CreateDepositMethod(c *gin.Context) {
	var req dto.DepositMethodRequest
	if !bindJSON(c, a.r, &req) {
		return
	}
	if errs := req.Validate(true); len(errs) > 0 {
		a.r.FailField(c, errs)
		return
	}
	var kind db.DepositMethodKind
	if err := kind.Scan(req.Kind); err != nil {
		a.r.FailField(c, []dto.FieldError{{Field: "kind", Message: "Geçersiz yöntem tipi."}})
		return
	}
	cfg, err := json.Marshal(req.Config)
	if err != nil {
		a.r.FailField(c, []dto.FieldError{{Field: "config", Message: "Geçersiz yapılandırma."}})
		return
	}

	m, err := a.queries.CreateDepositMethod(c.Request.Context(), db.CreateDepositMethodParams{
		Code: req.Code, Kind: kind, Name: req.Name, Instructions: req.Instructions,
		Config: cfg, MinAmountMinor: req.MinAmountMinor, MaxAmountMinor: req.MaxAmountMinor,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		// Aynı kod ikinci kez eklenemez.
		a.r.Fail(c, apperr.ErrValidation.WithMessage(
			"Bu kod zaten kullanılıyor veya bilgiler geçersiz."))
		return
	}
	// YENİ YÖNTEM PASİF DOĞAR (şema varsayılanı): bilgileri doldurulmadan
	// kullanıcıya görünmemeli. Boş IBAN'lı bir "aktif" yöntem, paranın
	// hiçbir yere gitmemesi demektir.
	c.Status(http.StatusCreated)
	a.r.OK(c, depositMethodDTO(m))
}

// UpdateDepositMethod PATCH /admin/deposit-methods/:id — izin: deposits:approve
func (a *Admin) UpdateDepositMethod(c *gin.Context) {
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	var req dto.DepositMethodRequest
	if !bindJSON(c, a.r, &req) {
		return
	}
	if errs := req.Validate(false); len(errs) > 0 {
		a.r.FailField(c, errs)
		return
	}
	cfg, err := json.Marshal(req.Config)
	if err != nil {
		a.r.FailField(c, []dto.FieldError{{Field: "config", Message: "Geçersiz yapılandırma."}})
		return
	}

	m, err := a.queries.UpdateDepositMethod(c.Request.Context(), db.UpdateDepositMethodParams{
		PublicID: publicID, Name: req.Name, Instructions: req.Instructions,
		Config: cfg, MinAmountMinor: req.MinAmountMinor,
		MaxAmountMinor: req.MaxAmountMinor, SortOrder: req.SortOrder,
	})
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	a.r.OK(c, depositMethodDTO(m))
}

// SetDepositMethodActive PATCH /admin/deposit-methods/:id/active — izin: deposits:approve
func (a *Admin) SetDepositMethodActive(c *gin.Context) {
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	var req dto.SetActiveRequest
	if !bindJSON(c, a.r, &req) {
		return
	}

	// AKTİFLEŞTİRME ÖN KOŞULU: yapılandırma dolu olmalı.
	//
	// Boş IBAN'lı bir yöntemi aktifleştirmek, kullanıcının parayı hiçbir
	// yere göndermemesi demektir. Bu kontrol olmadan tek bir düğme
	// tıklaması ödemeleri sessizce boşluğa yönlendirir.
	if req.IsActive {
		m, err := a.queries.GetDepositMethod(c.Request.Context(), publicID)
		if err != nil {
			a.r.Fail(c, apperr.ErrNotFound)
			return
		}
		if missing := missingConfigFields(m); len(missing) > 0 {
			a.r.Fail(c, apperr.ErrValidation.WithMessage(
				"Aktifleştirmeden önce şu alanlar doldurulmalı: "+strings.Join(missing, ", ")))
			return
		}
	}

	m, err := a.queries.SetDepositMethodActive(c.Request.Context(), db.SetDepositMethodActiveParams{
		PublicID: publicID, IsActive: req.IsActive,
	})
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	a.r.OK(c, depositMethodDTO(m))
}

// DeleteDepositMethod DELETE /admin/deposit-methods/:id — izin: deposits:approve
func (a *Admin) DeleteDepositMethod(c *gin.Context) {
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	// Geçmiş yükleme kayıtları KALIR: deposits.method_id ON DELETE SET NULL
	// ve method_name anlık görüntü olarak saklanıyor. Kullanıcı bir yıl sonra
	// "hangi yolla yatırmıştım?" diye baktığında cevabı görebilmeli.
	if err := a.queries.DeleteDepositMethod(c.Request.Context(), publicID); err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}
	a.r.NoContent(c)
}

/* ═══════════════════════════ Yardımcılar ═══════════════════════════ */

// missingConfigFields yöntemin eksik zorunlu alanlarını döner.
//
// Zorunlu alanlar TİPE göre değişir: havalede IBAN, kriptoda cüzdan adresi.
// Tek bir liste kullanmak, birinde anlamsız alan zorunlu kılardı.
func missingConfigFields(m db.DepositMethod) []string {
	var cfg map[string]string
	_ = json.Unmarshal(m.Config, &cfg)

	required := map[db.DepositMethodKind][]string{
		db.DepositMethodKindBANKTRANSFER: {"iban", "hesapAdi"},
		db.DepositMethodKindCRYPTO:       {"cuzdanAdresi", "ag"},
	}
	labels := map[string]string{
		"iban": "IBAN", "hesapAdi": "Hesap adı",
		"cuzdanAdresi": "Cüzdan adresi", "ag": "Ağ",
	}

	var missing []string
	for _, f := range required[m.Kind] {
		if strings.TrimSpace(cfg[f]) == "" {
			missing = append(missing, labels[f])
		}
	}
	return missing
}

func depositMethodDTO(m db.DepositMethod) dto.DepositMethodResponse {
	var cfg map[string]string
	_ = json.Unmarshal(m.Config, &cfg)
	if cfg == nil {
		cfg = map[string]string{}
	}
	return dto.DepositMethodResponse{
		ID:            m.PublicID.String(),
		Code:          m.Code,
		Kind:          string(m.Kind),
		Name:          m.Name,
		Instructions:  m.Instructions,
		Config:        cfg,
		MinAmount:     moneyDTO(money.New(m.MinAmountMinor, money.TRY)),
		MaxAmount:     moneyDTO(money.New(m.MaxAmountMinor, money.TRY)),
		IsActive:      m.IsActive,
		SortOrder:     m.SortOrder,
		MissingFields: missingConfigFields(m),
	}
}

func numericString(n pgtype.Numeric) string {
	if !n.Valid {
		return "1.0"
	}
	b, err := n.MarshalJSON()
	if err != nil {
		return "1.0"
	}
	return strings.Trim(string(b), `"`)
}

/* ═══════════════════ Bakiye yükleme talepleri (FR-502 / FR-503) ═══════════════════ */

// Bu bölümdeki işleyiciler *Deposit tipine aittir (tip handler/deposit.go'da
// tanımlıdır) ama BURADA durur: admin.go yönetim uçlarının dosyasıdır ve
// izin gerektiren her uç burada aranır.
//
// 🔴 ONAY VE RED **POST**'TUR, GET DEĞİL (değişmez #8). Eski sistemde onay bir
// GET'ti ve bir <img src="..."> etiketiyle tetiklenebiliyordu.

// AdminList GET /admin/deposits — izin: deposits:read
func (h *Deposit) AdminList(c *gin.Context) {
	limit, offset := pagination(c, 20, 100)

	// Durum süzgeci ENUM'a KARŞI doğrulanır. db.DepositStatus.Scan (üretilen
	// kod) her dizeyi kabul eder; doğrulamadan geçirilen bir değer Postgres'e
	// gider ve 22P02 ile 500 üretirdi — kullanıcı hatası 500 olarak görünmez.
	var status *db.DepositStatus
	if s := strings.TrimSpace(c.Query("status")); s != "" {
		if !depositdom.Status(s).Valid() {
			h.r.FailField(c, []dto.FieldError{{Field: "status", Message: "Geçersiz durum."}})
			return
		}
		st := db.DepositStatus(s)
		status = &st
	}

	rows, total, err := h.svc.ListForAdmin(c.Request.Context(), status, limit, offset)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	items := make([]dto.AdminDepositResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminDepositDTO(row))
	}
	h.r.OK(c, dto.AdminDepositListResponse{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

// Approve POST /admin/deposits/:id/approve — izin: deposits:approve
//
// İdempotency anahtarı İSTEKTEN ALINMAZ: sunucuda talebin public_id'sinden
// türer (servis katmanı). Bu yüzden gövde yalnız `creditedMinor` ve
// `adminNote` taşır.
//
// test: deposit_integration_test.go#TestApproveIsIdempotentUnderConcurrency
func (h *Deposit) Approve(c *gin.Context) {
	adminID, publicID, ok := h.adminScope(c)
	if !ok {
		return
	}
	var req dto.ApproveDepositRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}

	res, err := h.svc.Approve(c.Request.Context(), depositsvc.ApproveInput{
		AdminID:       adminID,
		PublicID:      publicID,
		CreditedMinor: req.CreditedMinor,
		AdminNote:     strings.TrimSpace(req.AdminNote),
		Audit:         auditMeta(c),
	})
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, reviewDTO(res))
}

// Reject POST /admin/deposits/:id/reject — izin: deposits:approve
//
// Red nedeni ZORUNLUDUR ve kullanıcıya gösterilir (FR-503); bakiye değişmez.
//
// test: deposit_integration_test.go#TestRejectReasonRequired
func (h *Deposit) Reject(c *gin.Context) {
	adminID, publicID, ok := h.adminScope(c)
	if !ok {
		return
	}
	var req dto.RejectDepositRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}

	res, err := h.svc.Reject(c.Request.Context(), depositsvc.RejectInput{
		AdminID:   adminID,
		PublicID:  publicID,
		Reason:    strings.TrimSpace(req.Reason),
		AdminNote: strings.TrimSpace(req.AdminNote),
		Audit:     auditMeta(c),
	})
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, reviewDTO(res))
}

// AdminReceipt GET /admin/deposits/:id/receipt — izin: deposits:read
//
// Yönetici havale dekontunu görmeden onay veremez. Dosya yine yalnız bu uçtan
// akar; hiçbir statik dosya yolundan erişilemez (KK-500).
//
// test: deposit_integration_test.go#TestReceiptIsNotReachableByURL
func (h *Deposit) AdminReceipt(c *gin.Context) {
	_, publicID, ok := h.adminScope(c)
	if !ok {
		return
	}
	// userID = nil → sahiplik kısıtı YOK; yetki middleware'de doğrulandı.
	h.serveReceipt(c, nil, publicID)
}

// adminScope yöneticinin kimliğini ve hedef talebi çözer.
func (h *Deposit) adminScope(c *gin.Context) (int64, uuid.UUID, bool) {
	adminID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return 0, uuid.Nil, false
	}
	publicID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.r.Fail(c, depositsvc.ErrNotFound)
		return 0, uuid.Nil, false
	}
	return adminID, publicID, true
}

/* ═══════════════════ Katalog senkronu tetikleme ═══════════════════ */

// CatalogSyncer katalog senkronunu ASENKRON çalıştırır.
//
// Arayüz BURADA tanımlıdır (tüketici tarafında), uygulaması `cmd/server`
// tarafından verilir — webhook kuyruğuyla aynı desen. Transport katmanı
// katalog servisini tanımaz.
type CatalogSyncer interface {
	// Start senkronu başlatır. Aynı sağlayıcı için bir senkron zaten
	// koşuyorsa started=false döner ve YENİ BİR TUR BAŞLATILMAZ.
	// test: admin_sync_test.go#TestSyncRunsInBackgroundAndIsSingleFlight
	Start(providerID int64) (state SyncState, started bool)
	// State son bilinen durumu döner.
	State(providerID int64) SyncState
}

// SyncState bir sağlayıcının senkron durumu.
type SyncState struct {
	Running    bool
	StartedAt  time.Time
	FinishedAt time.Time
	Failed     bool
}

// SyncRunner katalog senkronunu arka planda, sağlayıcı başına TEK UÇUŞLA
// çalıştırır.
//
// NEDEN İSTEK İÇİNDE KOŞMAZ: gerçek katalogda senkron dakikalarca sürer.
// İstek içinde koşsaydı ters vekil zaman aşımına düşer, istemci bağlantıyı
// kapatır, `c.Request.Context()` iptal olur ve iş YARIDA KALIRDI — yarısı
// güncellenmiş bir katalog, hiç güncellenmemişten kötüdür (bayat teklifler
// "yok" işaretlenmeden kalır).
//
// 🔴 İSTEK BAĞLAMI KULLANILMAZ: arka plan turu context.Background()'dan
// türeyen kendi bağlamıyla koşar; yanıt yazıldığında iptal olmaz.
// test: admin_sync_test.go#TestSyncSurvivesRequestContextCancellation
//
// TEK UÇUŞ YALNIZ BU SÜREÇ İÇİNDEDİR. İkinci bir sunucu örneği eklendiğinde
// Redis kilidi gerekir; bugün tek örnek varsayımı operasyoneldir
// (bkz. internal/worker/worker.go aynı kısıt).
type SyncRunner struct {
	mu    sync.Mutex
	state map[int64]*SyncState
	run   func(ctx context.Context, providerID int64) error
	limit time.Duration
}

// syncTimeout tek bir senkron turuna tanınan süre.
//
// Sınırsız bırakılsaydı, yanıt vermeyen bir sağlayıcı yüzünden `Running`
// sonsuza kadar true kalır ve o sağlayıcı bir daha HİÇ senkronlanamazdı.
const syncTimeout = 30 * time.Minute

// NewSyncRunner verilen senkron işini saran koşucuyu üretir.
func NewSyncRunner(run func(ctx context.Context, providerID int64) error) *SyncRunner {
	return &SyncRunner{state: map[int64]*SyncState{}, run: run, limit: syncTimeout}
}

func (r *SyncRunner) Start(providerID int64) (SyncState, bool) {
	r.mu.Lock()
	st, ok := r.state[providerID]
	if !ok {
		st = &SyncState{}
		r.state[providerID] = st
	}
	if st.Running {
		cur := *st
		r.mu.Unlock()
		return cur, false
	}
	st.Running = true
	st.StartedAt = time.Now()
	st.FinishedAt = time.Time{}
	st.Failed = false
	cur := *st
	r.mu.Unlock()

	go r.once(providerID, st)
	return cur, true
}

func (r *SyncRunner) once(providerID int64, st *SyncState) {
	// PANİK YAKALANIR: bir adaptör hatası tüm sunucuyu düşürmemeli ve
	// `Running` bayrağını sonsuza kadar açık bırakmamalı.
	defer func() {
		if p := recover(); p != nil {
			slog.Error("katalog senkronu panik verdi — sunucu ayakta kalıyor",
				"provider_id", providerID, "panic", fmt.Sprint(p))
			r.finish(st, true)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), r.limit)
	defer cancel()

	err := r.run(ctx, providerID)
	if err != nil {
		// HAM HATA KULLANICIYA GİTMEZ (Değişmez #12); yalnız günlüğe.
		slog.Error("katalog senkronu başarısız", "provider_id", providerID, "err", err)
	}
	r.finish(st, err != nil)
}

func (r *SyncRunner) finish(st *SyncState, failed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st.Running = false
	st.FinishedAt = time.Now()
	st.Failed = failed
}

func (r *SyncRunner) State(providerID int64) SyncState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.state[providerID]; ok {
		return *st
	}
	return SyncState{}
}

// errSyncInProgress aynı sağlayıcı için ikinci bir senkron denemesi.
var errSyncInProgress = apperr.NewStatus(apperr.KindDomain, "SYNC_IN_PROGRESS",
	"Bu sağlayıcı için bir katalog senkronu zaten çalışıyor. Bitmesini bekleyin.",
	http.StatusConflict)

// errSyncUnavailable senkron koşucusu bağlanmamış.
var errSyncUnavailable = apperr.NewStatus(apperr.KindInfra, "SYNC_UNAVAILABLE",
	"Katalog senkronu bu sunucuda yapılandırılmamış.", http.StatusServiceUnavailable)

// SyncProvider POST /admin/providers/:id/sync — izin: providers:write
//
// Senkronu BAŞLATIR ve hemen döner. İş arka planda sürer; ilerleme
// `GET /admin/providers` yanıtındaki `sync` alanından okunur.
func (a *Admin) SyncProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound)
		return
	}
	// Var olmayan sağlayıcı 404 döner — senkron kuyruğuna atılmadan ÖNCE.
	if _, err := a.queries.GetProvider(c.Request.Context(), id); err != nil {
		a.r.Fail(c, apperr.ErrNotFound.WithMessage("Sağlayıcı bulunamadı."))
		return
	}
	if a.sync == nil {
		a.r.Fail(c, errSyncUnavailable)
		return
	}

	st, started := a.sync.Start(id)
	if !started {
		a.r.Fail(c, errSyncInProgress)
		return
	}
	a.r.OK(c, dto.ProviderSyncResponse{
		ProviderID: strconv.FormatInt(id, 10),
		Started:    true,
		Sync:       syncDTO(st),
	})
}

func syncDTO(st SyncState) dto.ProviderSyncDTO {
	out := dto.ProviderSyncDTO{Running: st.Running, Failed: st.Failed}
	if !st.StartedAt.IsZero() {
		out.StartedAt = st.StartedAt.Format(time.RFC3339)
	}
	if !st.FinishedAt.IsZero() {
		out.FinishedAt = st.FinishedAt.Format(time.RFC3339)
	}
	return out
}

/* ═══════════════════ Sağlayıcı ekleme (FR-701) ═══════════════════ */

// CreateProvider POST /admin/providers — izin: providers:write
//
// 🔴 API ANAHTARI BU UÇTAN ALINMAZ. Gövdede bir `apiKey` alanı gelse bile
// çözümlenmez ve hiçbir yere yazılmaz; anahtar ayrı uçtan konur.
// test: admin_provider_integration_test.go#TestCreateProviderIgnoresAPIKeyInBody
//
// YENİ SAĞLAYICI PASİF DOĞAR (istekteki değere BAKILMAZ): anahtarı ve boyut
// eşleştirmeleri yokken etkin olsaydı teklif yolunda aday sayılır, her
// çağrıda zaman aşımına düşer ve kullanıcıya yansıyan tek şey yavaşlık
// olurdu. Etkinleştirme PATCH ile, anahtar konduktan sonra yapılır.
// test: admin_provider_integration_test.go#TestNewProviderIsCreatedInactive
func (a *Admin) CreateProvider(c *gin.Context) {
	var req dto.CreateProviderRequest
	if !bindJSON(c, a.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		a.r.FailField(c, errs)
		return
	}

	var mult pgtype.Numeric
	if err := mult.Scan(strings.TrimSpace(req.CostMultiplier)); err != nil {
		a.r.FailField(c, []dto.FieldError{
			{Field: "costMultiplier", Message: "Geçersiz çarpan."}})
		return
	}

	name := strings.TrimSpace(req.Name)
	if _, err := a.queries.GetProviderByName(c.Request.Context(), name); err == nil {
		a.r.Fail(c, apperr.ErrValidation.WithMessage("Bu adda bir sağlayıcı zaten var."))
		return
	}

	caps := req.Capabilities
	if len(caps) == 0 {
		caps = []string{"SMS_ACTIVATION"}
	}
	capsJSON, err := json.Marshal(caps)
	if err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}

	p, err := a.queries.CreateProvider(c.Request.Context(), db.CreateProviderParams{
		Name:     name,
		Protocol: db.ProviderProtocol(req.Protocol),
		BaseUrl:  strings.TrimSpace(req.BaseURL),
		// ApiKeyEnc BİLEREK boş: anahtar ayrı uçtan yazılır.
		ApiKeyEnc:      nil,
		IsActive:       false,
		Priority:       req.Priority,
		CostMultiplier: mult,
		Capabilities:   capsJSON,
	})
	if err != nil {
		a.r.Fail(c, apperr.ErrValidation.WithMessage(
			"Sağlayıcı eklenemedi: bilgiler geçersiz veya bu ad kullanılıyor."))
		return
	}

	a.r.OK(c, dto.AdminProviderResponse{
		ID:             strconv.FormatInt(p.ID, 10),
		Name:           p.Name,
		Protocol:       string(p.Protocol),
		BaseURL:        p.BaseUrl,
		IsActive:       p.IsActive,
		Priority:       p.Priority,
		HasAPIKey:      false,
		CostMultiplier: numericString(p.CostMultiplier),
		Capabilities:   caps,
		Balance:        moneyDTO(money.New(p.AccountBalanceMicro, money.USD)),
	})
}

/* ═══════════════════ Fiyat kuralları (FR-703) ═══════════════════ */

// ListPricingRules GET /admin/pricing-rules — izin: pricing:read
func (a *Admin) ListPricingRules(c *gin.Context) {
	if a.rules == nil {
		a.r.Fail(c, apperr.Internal(errors.New("fiyat kuralı servisi bağlanmamış")))
		return
	}
	views, err := a.rules.List(c.Request.Context())
	if err != nil {
		a.r.Fail(c, err)
		return
	}
	items := make([]dto.PricingRuleResponse, 0, len(views))
	for _, v := range views {
		items = append(items, pricingRuleDTO(v))
	}
	a.r.OK(c, dto.PricingRuleListResponse{Items: items})
}

// CreatePricingRule POST /admin/pricing-rules — izin: pricing:write
//
// Aynı kapsamdaki etkin kural OTOMATİK olarak devreden çıkar; kapsam bir an
// için bile kuralsız kalmaz (tek transaction).
func (a *Admin) CreatePricingRule(c *gin.Context) {
	if a.rules == nil {
		a.r.Fail(c, apperr.Internal(errors.New("fiyat kuralı servisi bağlanmamış")))
		return
	}
	actor, ok := middleware.UserIDFrom(c)
	if !ok {
		a.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	var req dto.CreatePricingRuleRequest
	if !bindJSON(c, a.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		a.r.FailField(c, errs)
		return
	}

	view, err := a.rules.Create(c.Request.Context(), pricingsvc.CreateRuleInput{
		Target: pricingsvc.RuleTarget{
			Scope:           req.Scope,
			ServiceCode:     req.ServiceCode,
			CountryISO:      req.CountryISO,
			DurationMinutes: req.DurationMinutes,
		},
		MarginPercent: req.MarginPercent,
		FixedFeeMinor: req.FixedFeeMinor,
		MinPriceMinor: req.MinPriceMinor,
		Note:          req.Note,
		ActorUserID:   actor,
		Audit:         auditMeta(c),
	})
	if err != nil {
		a.r.Fail(c, err)
		return
	}
	a.r.OK(c, pricingRuleDTO(view))
}

// DeactivatePricingRule DELETE /admin/pricing-rules/:id — izin: pricing:write
//
// Kural SİLİNMEZ, pasifleşir: geçmiş siparişlerin hangi kuralla fiyatlandığı
// (price_quotes.pricing_rule_id) izlenebilir kalmalı.
//
// SON GLOBAL KURAL PASİFLEŞTİRİLEMEZ — sistem satılamaz hâle gelirdi.
// test: ../../../service/pricing/rules_integration_test.go#TestDeactivatingLastGlobalRuleIsRefused
func (a *Admin) DeactivatePricingRule(c *gin.Context) {
	if a.rules == nil {
		a.r.Fail(c, apperr.Internal(errors.New("fiyat kuralı servisi bağlanmamış")))
		return
	}
	actor, ok := middleware.UserIDFrom(c)
	if !ok {
		a.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		a.r.Fail(c, apperr.ErrNotFound.WithMessage("Fiyat kuralı bulunamadı."))
		return
	}
	if err := a.rules.Deactivate(c.Request.Context(), pricingsvc.DeactivateRuleInput{
		RuleID: id, ActorUserID: actor, Audit: auditMeta(c),
	}); err != nil {
		a.r.Fail(c, err)
		return
	}
	a.r.NoContent(c)
}

// PreviewPricing POST /admin/pricing-rules/preview — izin: pricing:read
//
// Durum DEĞİŞTİRMEZ; teklif de üretmez. Gövdesi aday kural taşıdığı için
// POST'tur (docs/trd.md API haritası).
func (a *Admin) PreviewPricing(c *gin.Context) {
	if a.rules == nil {
		a.r.Fail(c, apperr.Internal(errors.New("fiyat kuralı servisi bağlanmamış")))
		return
	}
	var req dto.PricingPreviewRequest
	if !bindJSON(c, a.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		a.r.FailField(c, errs)
		return
	}

	in := pricingsvc.PreviewInput{
		ServiceCode:     req.ServiceCode,
		CountryISO:      req.CountryISO,
		DurationMinutes: req.DurationMinutes,
	}
	if req.Candidate != nil {
		in.Candidate = &pricingsvc.CandidateRule{
			MarginPercent: req.Candidate.MarginPercent,
			FixedFeeMinor: req.Candidate.FixedFeeMinor,
			MinPriceMinor: req.Candidate.MinPriceMinor,
		}
	}

	res, err := a.rules.Preview(c.Request.Context(), in)
	if err != nil {
		a.r.Fail(c, err)
		return
	}
	a.r.OK(c, dto.PricingPreviewResponse{
		SellPrice:     moneyDTO(res.SellPrice),
		Cost:          moneyDTO(res.Cost),
		CostInTRY:     moneyDTO(res.CostInTRY),
		FXRate:        res.FXRate,
		FXFetchedAt:   res.FXFetchedAt.Format(time.RFC3339),
		ProviderName:  res.ProviderName,
		Stock:         res.Stock,
		InStock:       res.Stock > 0,
		RuleSource:    res.RuleSource,
		RuleScope:     res.RuleScope,
		MarginPercent: res.MarginPercent,
		HitMinimum:    res.HitMinimum,
		CostSource:    "CACHE",
	})
}

func pricingRuleDTO(v pricingsvc.RuleView) dto.PricingRuleResponse {
	return dto.PricingRuleResponse{
		ID:              v.ID,
		Scope:           v.Scope,
		ServiceCode:     v.ServiceCode,
		CountryISO:      v.CountryISO,
		DurationMinutes: v.DurationMinutes,
		MarginPercent:   v.MarginPercent,
		FixedFee:        moneyDTO(v.FixedFee),
		MinPrice:        moneyDTO(v.MinPrice),
		Note:            v.Note,
		CreatedAt:       v.CreatedAt.Format(time.RFC3339),
		Replaced:        v.Replaced,
	}
}

/* ═══════════════════ Denetim kaydı (FR-705) ═══════════════════ */

// auditVisibleFields denetim kaydının before/after JSONB'sinden DIŞARI VERİLEN
// alanlar.
//
// 🔴 İZİN LİSTESİ, YASAK LİSTESİ DEĞİL. Denetim kaydını yazan her yeni çağrı
// kendi alanlarını seçer; yasak listesi tutulsaydı, yarın eklenen bir
// `email` veya `receiptPath` alanı listeye eklenmediği için SESSİZCE dışarı
// akardı. Tanınmayan alan geçmez, ama varlığı `redactedFields` ile bildirilir:
// denetçi bir şeyin saklandığını bilmeli.
// test: admin_audit_integration_test.go#TestAuditPayloadIsFieldFiltered
var auditVisibleFields = map[string]bool{
	// deposit.approve / deposit.reject
	"status": true, "amountMinor": true, "creditedMinor": true,
	"methodName": true, "hasReceipt": true,
	// pricing.rule.*
	"scope": true, "marginPercent": true, "fixedFeeMinor": true,
	"minPriceMinor": true, "isActive": true,
}

// ListAuditLogs GET /admin/audit-logs — izin: audit:read
func (a *Admin) ListAuditLogs(c *gin.Context) {
	limit, offset := pagination(c, 25, 100)

	params := db.ListAuditLogsForAdminParams{Lim: limit, Off: offset}
	countParams := db.CountAuditLogsForAdminParams{}

	if v := strings.TrimSpace(c.Query("entityType")); v != "" {
		params.EntityType, countParams.EntityType = &v, &v
	}
	if v := strings.TrimSpace(c.Query("entityId")); v != "" {
		params.EntityID, countParams.EntityID = &v, &v
	}
	if v := strings.TrimSpace(c.Query("action")); v != "" {
		params.Action, countParams.Action = &v, &v
	}
	if v := strings.TrimSpace(c.Query("actorId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			a.r.FailField(c, []dto.FieldError{{Field: "actorId", Message: "Geçersiz kullanıcı kimliği."}})
			return
		}
		pg := pgtype.UUID{Bytes: id, Valid: true}
		params.Actor, countParams.Actor = pg, pg
	}
	// Tarih süzgeci YALNIZ RFC 3339 kabul eder (Değişmez #19): boşluklu
	// biçimler tarayıcıdan tarayıcıya farklı ayrıştırılır.
	from, ok := a.parseWhen(c, "from")
	if !ok {
		return
	}
	until, ok := a.parseWhen(c, "until")
	if !ok {
		return
	}
	params.From, countParams.From = from, from
	params.Until, countParams.Until = until, until

	rows, err := a.queries.ListAuditLogsForAdmin(c.Request.Context(), params)
	if err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}
	total, err := a.queries.CountAuditLogsForAdmin(c.Request.Context(), countParams)
	if err != nil {
		a.r.Fail(c, apperr.Internal(err))
		return
	}

	items := make([]dto.AuditLogResponse, 0, len(rows))
	for _, r := range rows {
		before, redBefore := filterAuditPayload(r.Before)
		after, redAfter := filterAuditPayload(r.After)
		item := dto.AuditLogResponse{
			ID:             r.ID,
			Action:         r.Action,
			EntityType:     r.EntityType,
			EntityID:       r.EntityID,
			ActorUsername:  r.ActorUsername,
			Before:         before,
			After:          after,
			RedactedFields: mergeRedacted(redBefore, redAfter),
			IP:             ipString(r.Ip),
			RequestID:      r.RequestID,
			CreatedAt:      r.CreatedAt.Format(time.RFC3339),
		}
		if r.ActorPublicID.Valid {
			item.ActorID = uuid.UUID(r.ActorPublicID.Bytes).String()
		}
		items = append(items, item)
	}
	a.r.OK(c, dto.AuditLogListResponse{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

// parseWhen bir sorgu parametresini RFC 3339 zaman olarak okur.
func (a *Admin) parseWhen(c *gin.Context, key string) (*time.Time, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		a.r.FailField(c, []dto.FieldError{
			{Field: key, Message: "Tarih RFC 3339 biçiminde olmalıdır (2026-01-31T00:00:00Z)."}})
		return nil, false
	}
	return &t, true
}

// filterAuditPayload izin listesindeki alanları geçirir, kalanların ADINI
// döner.
func filterAuditPayload(raw []byte) (map[string]any, []string) {
	if len(raw) == 0 {
		return nil, nil
	}
	var all map[string]any
	if err := json.Unmarshal(raw, &all); err != nil {
		// Çözülemeyen gövde HİÇ gösterilmez: ne olduğu bilinmeyen bir metni
		// panele basmak, en iyi ihtimalle gürültü, en kötüsü sızıntıdır.
		return nil, []string{"(çözümlenemedi)"}
	}
	out := map[string]any{}
	var redacted []string
	for k, v := range all {
		if auditVisibleFields[k] {
			out[k] = v
			continue
		}
		redacted = append(redacted, k)
	}
	if len(out) == 0 {
		out = nil
	}
	sort.Strings(redacted)
	return out, redacted
}

func mergeRedacted(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
