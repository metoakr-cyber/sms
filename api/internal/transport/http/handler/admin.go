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
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/crypto"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Admin yönetim uçları.
type Admin struct {
	queries *db.Queries
	secrets *crypto.SecretBox
	r       Responder
}

func NewAdmin(q *db.Queries, secrets *crypto.SecretBox, r Responder) *Admin {
	return &Admin{queries: q, secrets: secrets, r: r}
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
