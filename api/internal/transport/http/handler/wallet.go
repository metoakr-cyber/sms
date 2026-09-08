package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/dto"
	"github.com/ikmetrik/sms-platform/api/internal/transport/http/middleware"
)

// Wallet cüzdan uç noktalarını yönetir.
type Wallet struct {
	svc     *walletsvc.Service
	queries *db.Queries
	r       Responder
}

func NewWallet(svc *walletsvc.Service, q *db.Queries, r Responder) *Wallet {
	return &Wallet{svc: svc, queries: q, r: r}
}

// Balance GET /wallet/balance
func (h *Wallet) Balance(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}
	bal, err := h.svc.Balance(c.Request.Context(), userID)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, dto.BalanceResponse{Balance: moneyDTO(bal)})
}

// Statement GET /wallet/entries
//
// Sahiplik SORGUNUN PARÇASIDIR: kullanıcı hiçbir parametreyle başkasının
// hareketlerini göremez (docs/trd.md KK-206).
func (h *Wallet) Statement(c *gin.Context) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}

	f := walletsvc.StatementFilter{
		Limit:  int32(queryInt(c, "limit", 20)),
		Offset: int32(queryInt(c, "offset", 0)),
	}
	if t := c.Query("type"); t != "" {
		f.Type = db.LedgerType(t)
	}
	if v := c.Query("from"); v != "" {
		if ts, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = &ts
		}
	}
	if v := c.Query("to"); v != "" {
		if ts, err := time.Parse(time.RFC3339, v); err == nil {
			f.To = &ts
		}
	}
	f.Normalize()

	rows, total, err := h.svc.Statement(c.Request.Context(), userID, f)
	if err != nil {
		h.r.Fail(c, err)
		return
	}

	items := make([]dto.LedgerEntryResponse, 0, len(rows))
	for _, e := range rows {
		items = append(items, dto.LedgerEntryResponse{
			ID:           strconv.FormatInt(e.ID, 10),
			Type:         string(e.EntryType),
			TypeLabel:    ledgerTypeLabel(e.EntryType),
			Amount:       moneyDTO(money.New(e.AmountMinor, money.TRY)),
			BalanceAfter: moneyDTO(money.New(e.BalanceAfterMinor, money.TRY)),
			Reference:    deref(e.ReferenceType),
			Note:         deref(e.Note),
			CreatedAt:    e.CreatedAt.Format(time.RFC3339),
		})
	}
	h.r.OK(c, dto.StatementResponse{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset})
}

// AdjustBalance POST /admin/users/:id/balance — 'users:write' izni gerektirir.
func (h *Wallet) AdjustBalance(c *gin.Context) {
	adminID, ok := middleware.UserIDFrom(c)
	if !ok {
		h.r.Fail(c, apperr.ErrUnauthenticated)
		return
	}

	targetUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		h.r.Fail(c, apperr.ErrNotFound)
		return
	}
	target, err := h.queries.GetUserByPublicID(c.Request.Context(), targetUUID)
	if err != nil {
		h.r.Fail(c, apperr.ErrNotFound)
		return
	}

	var req dto.AdjustBalanceRequest
	if !bindJSON(c, h.r, &req) {
		return
	}
	if errs := req.Validate(); len(errs) > 0 {
		h.r.FailField(c, errs)
		return
	}

	// Anahtar İSTEĞİN İÇERİĞİNDEN türer, taşıyıcısından değil.
	//
	// Hedef kullanıcı kapsama dahildir: aynı anahtarın farklı bir kullanıcı
	// için kullanılması artık çakışma değil, ayrı bir işlemdir.
	//
	// test: handler/auth_wallet_integration_test.go#TestAdjustIsIdempotent
	idem := fmt.Sprintf("manual:%s:%s", target.PublicID, strings.TrimSpace(req.IdempotencyKey))

	res, err := h.svc.Adjust(c.Request.Context(), adminID, target.ID,
		money.New(req.AmountMinor, money.TRY), req.Note, idem)
	if err != nil {
		h.r.Fail(c, err)
		return
	}
	h.r.OK(c, dto.BalanceResponse{
		Balance:        moneyDTO(res.NewBalance),
		AlreadyApplied: res.AlreadyApplied,
	})
}

// ledgerTypeLabel hareket tipinin kullanıcıya gösterilecek Türkçe adı.
func ledgerTypeLabel(t db.LedgerType) string {
	switch t {
	case db.LedgerTypeDEPOSIT:
		return "Bakiye yükleme"
	case db.LedgerTypePURCHASE:
		return "Numara satın alma"
	case db.LedgerTypeREFUND:
		return "İade"
	case db.LedgerTypeADJUSTMENT:
		return "Düzeltme"
	case db.LedgerTypeCOMMISSION:
		return "Referans komisyonu"
	case db.LedgerTypeCHARGEBACK:
		return "Ödeme itirazı"
	default:
		return string(t)
	}
}

func queryInt(c *gin.Context, key string, def int) int {
	if v := c.Query(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
