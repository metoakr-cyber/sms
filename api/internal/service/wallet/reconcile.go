package wallet

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/service/audit"
)

// Drift bir kullanıcının defter toplamı ile önbelleklenmiş bakiyesi arasındaki fark.
type Drift struct {
	UserID        int64
	Email         string
	CachedBalance money.Money
	LedgerBalance money.Money
	Amount        money.Money
}

// ReconcileReport günlük mutabakatın sonucu.
type ReconcileReport struct {
	CheckedUsers int64
	Drifts       []Drift
}

// HasDrift sapma bulunup bulunmadığını söyler.
func (r ReconcileReport) HasDrift() bool { return len(r.Drifts) > 0 }

// Reconcile defter toplamı ile önbelleklenmiş bakiyeyi karşılaştırır (FR-205).
//
// SAPMA OTOMATİK DÜZELTİLMEZ. Sapma bir hatanın BELİRTİSİDİR — bakiyeyi
// sessizce düzeltmek belirtiyi gizler ve asıl hata (yanlış bir kod yolu,
// eksik transaction, elle müdahale) fark edilmeden devam eder.
// Doğru davranış: alarm üretmek ve insanın incelemesini sağlamak.
func (s *Service) Reconcile(ctx context.Context, limit int32) (ReconcileReport, error) {
	q := s.tx.Queries()

	rows, err := q.ListReconciliationDrift(ctx, limit)
	if err != nil {
		return ReconcileReport{}, apperr.Internal(err)
	}

	rep := ReconcileReport{Drifts: make([]Drift, 0, len(rows))}
	for _, r := range rows {
		rep.Drifts = append(rep.Drifts, Drift{
			UserID:        r.UserID,
			Email:         r.Email,
			CachedBalance: money.New(r.CachedBalance, money.TRY),
			LedgerBalance: money.New(r.LedgerBalance, money.TRY),
			Amount:        money.New(r.Drift, money.TRY),
		})
	}

	if rep.HasDrift() {
		// En kritik alarmımız. Sıfır olmayan her sapma incelenmelidir.
		slog.Error("MUTABAKAT SAPMASI — defter ile bakiye uyuşmuyor",
			"drift_count", len(rep.Drifts),
			"first_user_id", rep.Drifts[0].UserID,
			"first_drift", rep.Drifts[0].Amount.String(),
		)
	} else {
		slog.Info("mutabakat tamam", "drift_count", 0)
	}
	return rep, nil
}

// AdjustInput manuel bakiye düzeltmesinin girdisi.
type AdjustInput struct {
	AdminID int64
	UserID  int64
	// UserPublicID denetim kaydına yazılır: sayısal kimlik oraya da girmez.
	UserPublicID string
	Amount       money.Money
	Note         string
	IdemKey      string
	Audit        audit.Meta
}

// Adjust admin tarafından manuel bakiye düzeltmesi (FR-504).
//
// Not ZORUNLUDUR: sebepsiz bir bakiye değişikliği denetlenemez.
//
// 🔴 DENETİM KAYDI DEFTERLE AYNI TRANSACTION'DA YAZILIR. Ayrı yazılsaydı ikisi
// ayrışabilirdi: para değişir, izi kalmaz. Ele geçirilmiş bir `users:write`
// hesabı `AmountMinor` üst sınırı olmadığı için istediği kadar bakiye basabilir;
// tek caydırıcı ve tek kanıt bu satırdır.
//
// test: ../../transport/http/handler/auth_wallet_integration_test.go#TestAdjustIsAudited
func (s *Service) Adjust(ctx context.Context, in AdjustInput) (Result, error) {
	if in.Note == "" {
		return Result{}, apperr.ErrValidation.WithMessage("Düzeltme için açıklama zorunludur.")
	}
	if in.IdemKey == "" {
		return Result{}, apperr.Internal(fmt.Errorf("wallet: düzeltme için idempotency anahtarı zorunlu"))
	}

	var out Result
	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		res, err := s.Apply(ctx, q, Input{
			UserID:          in.UserID,
			Amount:          in.Amount,
			Type:            db.LedgerTypeADJUSTMENT,
			IdempotencyKey:  in.IdemKey,
			ReferenceType:   "manual",
			ReferenceID:     in.IdemKey,
			CreatedByUserID: &in.AdminID,
			Note:            in.Note,
		})
		if err != nil {
			return err
		}
		out = res

		// Tekrarlanan çağrıda ikinci bir denetim satırı yazılmaz: işlem
		// gerçekten olmadı, yalnız önceki sonuç döndürüldü.
		if res.AlreadyApplied {
			return nil
		}
		admin := in.AdminID
		return audit.Record(ctx, q, audit.Entry{
			ActorUserID: &admin,
			Action:      "wallet.adjust",
			EntityType:  "user",
			EntityID:    in.UserPublicID,
			After: map[string]any{
				"amountMinor":  in.Amount.Minor(),
				"balanceMinor": res.NewBalance.Minor(),
				"note":         in.Note,
			},
			Meta: in.Audit,
		})
	})
	return out, err
}

// Statement kullanıcının hareket dökümünü döner (FR-206).
//
// Sahiplik sorgunun PARÇASIDIR: userID bir WHERE koşuludur, ayrı bir if değil.
// Böylece "sahiplik kontrolünü unutmak" yapısal olarak zorlaşır.
func (s *Service) Statement(ctx context.Context, userID int64, filter StatementFilter) ([]db.LedgerEntry, int64, error) {
	q := s.tx.Queries()

	var typeFilter *db.LedgerType
	if filter.Type != "" {
		typeFilter = &filter.Type
	}

	rows, err := q.ListLedgerEntries(ctx, db.ListLedgerEntriesParams{
		UserID:    userID,
		EntryType: typeFilter,
		Q:         filter.Q,
		FromTs:    filter.From,
		ToTs:      filter.To,
		Lim:       filter.Limit,
		Off:       filter.Offset,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}

	// 🔴 SAYIM LİSTEYLE AYNI SÜZGECİ ALIR. Önceden `FromTs`/`ToTs` burada
	// YOKTU: tarih aralığı verildiğinde sayfalama yanlış toplam gösterirdi.
	total, err := q.CountLedgerEntries(ctx, db.CountLedgerEntriesParams{
		UserID:    userID,
		EntryType: typeFilter,
		Q:         filter.Q,
		FromTs:    filter.From,
		ToTs:      filter.To,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return rows, total, nil
}
