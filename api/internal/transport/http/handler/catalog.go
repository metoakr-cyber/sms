package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
)

// Catalog katalog uç noktalarını yönetir.
type Catalog struct {
	queries *db.Queries
	r       Responder
}

func NewCatalog(q *db.Queries, r Responder) *Catalog { return &Catalog{queries: q, r: r} }

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
