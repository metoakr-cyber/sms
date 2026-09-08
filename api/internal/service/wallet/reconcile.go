package wallet

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
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

// Adjust admin tarafından manuel bakiye düzeltmesi (FR-504).
//
// Not ZORUNLUDUR: sebepsiz bir bakiye değişikliği denetlenemez.
func (s *Service) Adjust(ctx context.Context, adminID, userID int64, amount money.Money, note, idemKey string) (Result, error) {
	if note == "" {
		return Result{}, apperr.ErrValidation.WithMessage("Düzeltme için açıklama zorunludur.")
	}
	if idemKey == "" {
		return Result{}, apperr.Internal(fmt.Errorf("wallet: düzeltme için idempotency anahtarı zorunlu"))
	}
	return s.ApplyTx(ctx, Input{
		UserID:          userID,
		Amount:          amount,
		Type:            db.LedgerTypeADJUSTMENT,
		IdempotencyKey:  idemKey,
		ReferenceType:   "manual",
		ReferenceID:     idemKey,
		CreatedByUserID: &adminID,
		Note:            note,
	})
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
		FromTs:    filter.From,
		ToTs:      filter.To,
		Lim:       filter.Limit,
		Off:       filter.Offset,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}

	total, err := q.CountLedgerEntries(ctx, db.CountLedgerEntriesParams{
		UserID:    userID,
		EntryType: typeFilter,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return rows, total, nil
}
