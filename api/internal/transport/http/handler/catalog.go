package handler

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	pricingsvc "github.com/ikmetrik/sms-platform/api/internal/service/pricing"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Catalog katalog uç noktalarını yönetir.
type Catalog struct {
	queries *db.Queries
	quotes  *pricingsvc.QuoteService
	r       Responder
}

func NewCatalog(q *db.Queries, quotes *pricingsvc.QuoteService, r Responder) *Catalog {
	return &Catalog{queries: q, quotes: quotes, r: r}
}

// Services GET /catalog/services
func (h *Catalog) Services(c *gin.Context) {
	rows, err := h.queries.ListVisibleServices(c.Request.Context())
	if err != nil {
		h.r.Fail(c, apperr.Internal(err))
		return
	}
	out := make([]dto.ServiceResponse, 0, len(rows))
	for _, s := range rows {
		out = append(out, dto.ServiceResponse{
			Code: s.Code, Name: displayName(s.NameTr, s.Name), IconURL: s.IconUrl,
		})
	}
	h.r.OK(c, gin.H{"items": out})
}

// ServicesWithStock GET /catalog/services-in-stock
//
// Izgara için: yalnız stoklu servisler + stoklu ülke sayısı.
// /catalog/availability tüm matrisi döner (gerçek katalogda ~1 MB) ve mobil
// için uygun değildir; ülkeler servis seçildikten sonra ayrıca çekilir.
func (h *Catalog) ServicesWithStock(c *gin.Context) {
	rows, err := h.queries.ListServicesWithStock(c.Request.Context())
	if err != nil {
		h.r.Fail(c, apperr.Internal(err))
		return
	}
	out := make([]dto.ServiceSummaryResponse, 0, len(rows))
	for _, s := range rows {
		out = append(out, dto.ServiceSummaryResponse{
			Code:         s.ServiceCode,
			Name:         displayName(s.ServiceNameTr, s.ServiceName),
			IconURL:      s.IconUrl,
			CountryCount: s.CountryCount,
		})
	}
	h.r.OK(c, gin.H{"items": out})
}

// RentalServices GET /catalog/rental/services
func (h *Catalog) RentalServices(c *gin.Context) {
	rows, err := h.queries.ListRentalServicesWithStock(c.Request.Context())
	if err != nil {
		h.r.Fail(c, apperr.Internal(err))
		return
	}
	out := make([]dto.RentalServiceResponse, 0, len(rows))
	for _, s := range rows {
		out = append(out, dto.RentalServiceResponse{
			Code: s.ServiceCode, Name: displayName(s.ServiceNameTr, s.ServiceName),
			IconURL: s.IconUrl, CountryCount: s.CountryCount, DurationCount: s.DurationCount,
		})
	}
	h.r.OK(c, gin.H{"items": out})
}

// RentalCountries GET /catalog/rental/countries?serviceCode=
func (h *Catalog) RentalCountries(c *gin.Context) {
	code := c.Query("serviceCode")
	if code == "" {
		h.r.FailField(c, []dto.FieldError{{Field: "serviceCode", Message: "Servis seçilmelidir."}})
		return
	}
	rows, err := h.queries.ListRentalCountriesForService(c.Request.Context(), code)
	if err != nil {
		h.r.Fail(c, apperr.Internal(err))
		return
	}
	out := make([]dto.CountryResponse, 0, len(rows))
	for _, x := range rows {
		out = append(out, dto.CountryResponse{
			ISO2: x.CountryIso2, Name: displayName(x.CountryNameTr, x.CountryName),
			PhoneCode: x.PhoneCode,
		})
	}
	h.r.OK(c, gin.H{"items": out})
}

// RentalDurations GET /catalog/rental/durations?serviceCode=&countryIso=
func (h *Catalog) RentalDurations(c *gin.Context) {
	svc, ctry := c.Query("serviceCode"), c.Query("countryIso")
	if svc == "" || ctry == "" {
		h.r.FailField(c, []dto.FieldError{
			{Field: "serviceCode", Message: "Servis ve ülke seçilmelidir."}})
		return
	}
	rows, err := h.queries.ListRentalDurationsForCatalog(c.Request.Context(),
		db.ListRentalDurationsForCatalogParams{ServiceCode: svc, CountryIso: ctry})
	if err != nil {
		h.r.Fail(c, apperr.Internal(err))
		return
	}
	out := make([]dto.RentalDurationResponse, 0, len(rows))
	for _, r := range rows {
		if r.DurationMinutes == nil {
			continue
		}
		m := *r.DurationMinutes
		out = append(out, dto.RentalDurationResponse{
			Minutes: m, Hours: m / 60, Days: m / 1440,
			Label:   durationLabel(m),
			InStock: r.TotalStock > 0,
		})
	}
	h.r.OK(c, gin.H{"items": out})
}

// durationLabel süreyi Türkçe, okunur bir etikete çevirir.
//
// "720 dakika" demek kullanıcıya hiçbir şey ifade etmez; "30 gün (1 ay)" eder.
func durationLabel(minutes int32) string {
	days := minutes / 1440
	switch {
	case days >= 30 && days%30 == 0:
		months := days / 30
		if months == 1 {
			return "30 gün (1 ay)"
		}
		return fmt.Sprintf("%d gün (%d ay)", days, months)
	case days >= 7 && days%7 == 0:
		weeks := days / 7
		return fmt.Sprintf("%d gün (%d hafta)", days, weeks)
	case days >= 1:
		return fmt.Sprintf("%d gün", days)
	default:
		return fmt.Sprintf("%d saat", minutes/60)
	}
}

// Countries GET /catalog/countries
func (h *Catalog) Countries(c *gin.Context) {
	rows, err := h.queries.ListVisibleCountries(c.Request.Context())
	if err != nil {
		h.r.Fail(c, apperr.Internal(err))
		return
	}
	out := make([]dto.CountryResponse, 0, len(rows))
	for _, x := range rows {
		out = append(out, dto.CountryResponse{
			ISO2: x.Iso2, Name: displayName(x.NameTr, x.Name), PhoneCode: x.PhoneCode,
		})
	}
	h.r.OK(c, gin.H{"items": out})
}

// Availability GET /catalog/availability?serviceCode=&countryIso=
//
// Yalnız STOKLU kombinasyonları döner. Stoksuz bir kombinasyonu listede
// göstermek, kullanıcıyı parasının çekileceği ama numara gelmeyeceği bir
// akışa sokmak demektir (docs/trd.md FR-306).
func (h *Catalog) Availability(c *gin.Context) {
	var params db.ListAvailableProductsForCatalogParams
	if v := c.Query("serviceCode"); v != "" {
		params.ServiceCode = &v
	}
	if v := c.Query("countryIso"); v != "" {
		params.CountryIso = &v
	}

	rows, err := h.queries.ListAvailableProductsForCatalog(c.Request.Context(), params)
	if err != nil {
		h.r.Fail(c, apperr.Internal(err))
		return
	}
	out := make([]dto.CatalogItemResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.CatalogItemResponse{
			ServiceCode: r.ServiceCode,
			ServiceName: displayName(r.ServiceNameTr, r.ServiceName),
			IconURL:     r.IconUrl,
			CountryISO2: r.CountryIso2,
			CountryName: r.CountryNameTr,
			PhoneCode:   r.PhoneCode,
			InStock:     r.TotalStock > 0,
		})
	}
	h.r.OK(c, dto.CatalogResponse{Items: out})
}

// displayName Türkçe ad varsa onu, yoksa özgün adı döner.
func displayName(tr, fallback string) string {
	if tr != "" {
		return tr
	}
	return fallback
}

// Quote GET /catalog/quote?serviceCode=&countryIso=
//
// Oturum GEREKTİRİR: teklif kullanıcıya bağlıdır ve yalnız onun tarafından
// tüketilebilir.
func (h *Catalog) Quote(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	serviceCode := c.Query("serviceCode")
	countryISO := c.Query("countryIso")
	if serviceCode == "" || countryISO == "" {
		h.r.FailField(c, []dto.FieldError{
			{Field: "serviceCode", Message: "Servis ve ülke seçilmelidir."},
		})
		return
	}

	// Süre verilmişse KİRALIK teklif. Doğrulama katı: uydurma bir süre
	// sessizce aktivasyon teklifi döndürmemeli — kullanıcı 30 günlük numara
	// sanıp 20 dakikalık alırdı.
	var durationMinutes int32
	if v := c.Query("durationMinutes"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			h.r.FailField(c, []dto.FieldError{
				{Field: "durationMinutes", Message: "Geçersiz kiralama süresi."}})
			return
		}
		durationMinutes = int32(n)
	}

	q, err := h.quotes.Create(c.Request.Context(), pricingsvc.QuoteRequest{
		UserID: userID, ServiceCode: serviceCode, CountryISO: countryISO,
		DurationMinutes: durationMinutes,
	})
	if err != nil {
		h.r.Fail(c, err)
		return
	}

	h.r.OK(c, dto.QuoteResponse{
		QuoteID:   q.QuoteID.String(),
		Price:     moneyDTO(q.SellPrice),
		Stock:     q.Stock,
		ExpiresAt: q.ExpiresAt.Format(time.RFC3339),
		ExpiresIn: int(time.Until(q.ExpiresAt).Seconds()),
	})
}
