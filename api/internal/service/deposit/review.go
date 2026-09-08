package deposit

// Yönetici incelemesi: onay (FR-502) ve red (FR-503).
//
// ÇİFT ONAYA KARŞI ÜÇ SAVUNMA vardır ve üçü de zorunludur:
//
//  1. LockDepositForUpdate — satır kilidi, InTx İÇİNDE. 100 eşzamanlı istek
//     burada sıraya girer.
//  2. ApproveDeposit'in `AND status = 'PENDING'` koşulu — yarışta ikinci
//     çağrıya sıfır satır döner.
//  3. wallet.Apply'a verilen DETERMİNİSTİK anahtar `deposit:<public_id>` —
//     ledger_entries.idempotency_key UNIQUE kısıtı son sözü söyler.
//
// Yalnız (3)'e güvenmek yetmez: kilit olmadan iki istek aynı satırı okuyup
// ikisi de "PENDING" görür, biri UNIQUE ihlaliyle 500 alır ve yönetici
// "onay çalışmıyor" der.
//
// test: deposit_integration_test.go#TestApproveRepeatedReturnsAlreadyApplied

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ikmetrik/sms-platform/api/internal/db"
	depositdom "github.com/ikmetrik/sms-platform/api/internal/domain/deposit"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	auditsvc "github.com/ikmetrik/sms-platform/api/internal/service/audit"
	walletsvc "github.com/ikmetrik/sms-platform/api/internal/service/wallet"
)

// ApproveInput onay isteği.
type ApproveInput struct {
	AdminID  int64
	PublicID uuid.UUID
	// CreditedMinor bakiyeye YAZILACAK tutar. 0 ise kullanıcının bildirdiği
	// tutar (amount_minor) kullanılır.
	//
	// amount_minor ile credited_minor AYRI kavramlardır: kripto yatırımlarda
	// ağ ücreti sonrası gelen tutar farklı olabilir. Defter'e yazılan
	// credited_minor'dır.
	CreditedMinor int64
	AdminNote     string
	Audit         auditsvc.Meta
}

// RejectInput red isteği.
type RejectInput struct {
	AdminID  int64
	PublicID uuid.UUID
	// Reason ZORUNLUDUR ve KULLANICIYA GÖSTERİLİR (FR-503).
	Reason    string
	AdminNote string
	Audit     auditsvc.Meta
}

// ReviewResult inceleme sonucu.
type ReviewResult struct {
	Deposit db.Deposit
	// Talep SAHİBİNİN kimliği — yanıtta sayısal id yerine bunlar kullanılır.
	//
	// Tüm `db.User` satırı taşınmaz: parola özeti gibi alanların servis
	// sınırından çıkması, bir gün birinin onları yanıta koymasını kolaylaştırır.
	UserPublicID uuid.UUID
	UserEmail    string
	UserUsername string
	// NewBalance talep sahibinin güncel bakiyesi (yöneticinin değil).
	NewBalance money.Money
	// AlreadyApplied true ise bu çağrı bir TEKRAR'dı; hiçbir şey yazılmadı.
	AlreadyApplied bool
}

// withOwner sonucu talep sahibinin kimliğiyle tamamlar.
func withOwner(ctx context.Context, q *db.Queries, res ReviewResult, userID int64) (ReviewResult, error) {
	u, err := q.GetUserByID(ctx, userID)
	if err != nil {
		return res, apperr.Internal(err)
	}
	res.UserPublicID, res.UserEmail, res.UserUsername = u.PublicID, u.Email, u.Username
	return res, nil
}

/* ═══════════════════════ Onay (FR-502) ═══════════════════════ */

// Approve talebi onaylar ve bakiyeyi TEK TRANSACTION içinde artırır.
//
// Adım sırası KRİTİKTİR ve değiştirilemez:
//
//  1. satırı KİLİTLE          → eşzamanlı istekler burada bekler
//  2. zaten COMPLETED mi      → hiçbir yazım yapmadan erken dön
//  3. PENDING değilse         → 409 (REJECTED/REFUNDED onaylanamaz)
//  4. durum makinesi          → status yalnız bu doğrulamadan sonra yazılır
//  5. koşullu UPDATE          → ikinci savunma
//  6. wallet.Apply(ÇAĞIRANIN q'su)  → defter + bakiye, aynı transaction
//  7. denetim kaydı           → aynı transaction
//
// test: deposit_integration_test.go#TestApproveIsIdempotentUnderConcurrency
func (s *Service) Approve(ctx context.Context, in ApproveInput) (ReviewResult, error) {
	var res ReviewResult

	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		// ── 1. Kilit ──
		dep, err := q.LockDepositForUpdate(ctx, in.PublicID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return apperr.Internal(err)
		}

		// ── 2. Tekrar mı? ──
		//
		// Tekrarlanan onay bir HATA DEĞİL, bir TEKRAR'dır: yönetici ağ hatası
		// sonrası düğmeye ikinci kez basmış olabilir. 409 dönmek onu "acaba
		// geçti mi?" sorusuyla baş başa bırakır.
		if dep.Status == db.DepositStatusCOMPLETED {
			bal, err := q.GetUserBalance(ctx, dep.UserID)
			if err != nil {
				return apperr.Internal(err)
			}
			res, err = withOwner(ctx, q, ReviewResult{
				Deposit:        dep,
				NewBalance:     money.New(bal, money.TRY),
				AlreadyApplied: true,
			}, dep.UserID)
			return err
		}

		// ── 3. Sonuçlandırılmış mı? ──
		if dep.Status != db.DepositStatusPENDING {
			return ErrNotPending
		}

		// ── 4. Durum makinesi (değişmez #13) ──
		if err := depositdom.Transition(
			depositdom.Status(dep.Status), depositdom.StatusCompleted,
		); err != nil {
			return apperr.Internal(err)
		}

		credited := in.CreditedMinor
		if credited == 0 {
			credited = dep.AmountMinor
		}
		if credited <= 0 {
			return ErrAmountRange
		}

		// ── 5. Koşullu UPDATE (ikinci savunma) ──
		now := s.clock.Now()
		updated, err := q.ApproveDeposit(ctx, db.ApproveDepositParams{
			PublicID:         in.PublicID,
			CreditedMinor:    credited,
			AdminNote:        in.AdminNote,
			ReviewedByUserID: &in.AdminID,
			ReviewedAt:       &now,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotPending
			}
			return mapDBErr(err)
		}

		// ── 6. Bakiye — ÇAĞIRANIN transaction'ında ──
		//
		// 🔴 ApplyTx DEĞİL: ApplyTx kendi transaction'ını açardı ve deposits
		// UPDATE'i ile defter kaydı AYRI transaction'lara düşerdi. Süreç arada
		// ölürse "onaylanmış ama para yatmamış" bir talep kalırdı.
		//
		// 🔴 Anahtar SUNUCUDA, talebin public_id'sinden türer. Admin isteğinden
		// alınsaydı iki yönetici (veya bir yöneticinin iki sekmesi) aynı talebi
		// farklı anahtarlarla onaylayıp bakiyeyi iki kez artırabilirdi.
		w, err := s.wallet.Apply(ctx, q, walletsvc.Input{
			UserID:          dep.UserID,
			Amount:          money.New(credited, money.TRY),
			Type:            db.LedgerTypeDEPOSIT,
			IdempotencyKey:  idempotencyKey(dep.PublicID),
			ReferenceType:   "deposit",
			ReferenceID:     dep.PublicID.String(),
			CreatedByUserID: &in.AdminID,
			Note:            "Bakiye yükleme — " + dep.MethodName,
		})
		if err != nil {
			return err
		}

		// ── 7. Denetim kaydı — aynı transaction ──
		if err := auditsvc.Record(ctx, q, auditsvc.Entry{
			ActorUserID: &in.AdminID,
			Action:      "deposit.approve",
			EntityType:  "deposit",
			EntityID:    dep.PublicID.String(),
			Before:      snapshot(dep),
			After:       snapshot(updated),
			Meta:        in.Audit,
		}); err != nil {
			return apperr.Internal(err)
		}

		res, err = withOwner(ctx, q, ReviewResult{
			Deposit: updated, NewBalance: w.NewBalance, AlreadyApplied: w.AlreadyApplied,
		}, dep.UserID)
		return err
	})
	if err != nil {
		return ReviewResult{}, err
	}
	return res, nil
}

/* ═══════════════════════ Red (FR-503) ═══════════════════════ */

// Reject talebi reddeder. BAKİYE HİÇ DEĞİŞMEZ.
//
// Cüzdan servisi bu yolda HİÇ ÇAĞRILMAZ; red bir para hareketi değildir.
//
// test: deposit_integration_test.go#TestRejectDoesNotChangeBalance
func (s *Service) Reject(ctx context.Context, in RejectInput) (ReviewResult, error) {
	var res ReviewResult

	err := s.tx.InTx(ctx, func(q *db.Queries) error {
		dep, err := q.LockDepositForUpdate(ctx, in.PublicID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return apperr.Internal(err)
		}

		if dep.Status == db.DepositStatusREJECTED {
			bal, err := q.GetUserBalance(ctx, dep.UserID)
			if err != nil {
				return apperr.Internal(err)
			}
			res, err = withOwner(ctx, q, ReviewResult{
				Deposit: dep, NewBalance: money.New(bal, money.TRY), AlreadyApplied: true,
			}, dep.UserID)
			return err
		}
		if dep.Status != db.DepositStatusPENDING {
			// Onaylanmış bir talep reddedilemez: para çoktan yazıldı, geri
			// alma ayrı bir akıştır (REFUNDED, v1 kapsamı dışında).
			return ErrNotPending
		}
		if err := depositdom.Transition(
			depositdom.Status(dep.Status), depositdom.StatusRejected,
		); err != nil {
			return apperr.Internal(err)
		}

		now := s.clock.Now()
		updated, err := q.RejectDeposit(ctx, db.RejectDepositParams{
			PublicID:         in.PublicID,
			RejectionReason:  in.Reason,
			AdminNote:        in.AdminNote,
			ReviewedByUserID: &in.AdminID,
			ReviewedAt:       &now,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotPending
			}
			return mapDBErr(err)
		}

		if err := auditsvc.Record(ctx, q, auditsvc.Entry{
			ActorUserID: &in.AdminID,
			Action:      "deposit.reject",
			EntityType:  "deposit",
			EntityID:    dep.PublicID.String(),
			Before:      snapshot(dep),
			After:       snapshot(updated),
			Meta:        in.Audit,
		}); err != nil {
			return apperr.Internal(err)
		}

		bal, err := q.GetUserBalance(ctx, dep.UserID)
		if err != nil {
			return apperr.Internal(err)
		}
		res, err = withOwner(ctx, q, ReviewResult{
			Deposit: updated, NewBalance: money.New(bal, money.TRY),
		}, dep.UserID)
		return err
	})
	if err != nil {
		return ReviewResult{}, err
	}
	return res, nil
}

/* ═══════════════════════ Yönetim listesi ═══════════════════════ */

// ListForAdmin talepleri duruma göre süzer ve sayfalar.
func (s *Service) ListForAdmin(ctx context.Context, status *db.DepositStatus, limit, offset int32) ([]db.ListDepositsForAdminRow, int64, error) {
	q := s.tx.Queries()
	rows, err := q.ListDepositsForAdmin(ctx, db.ListDepositsForAdminParams{
		Status: status, Lim: limit, Off: offset,
	})
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	total, err := q.CountDepositsForAdmin(ctx, status)
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return rows, total, nil
}

/* ═══════════════════════ Yardımcılar ═══════════════════════ */

// idempotencyKey defter anahtarı — docs/design.md §8.5'te yazılı şema.
func idempotencyKey(publicID uuid.UUID) string {
	return fmt.Sprintf("deposit:%s", publicID)
}

// snapshot denetim kaydına giden alan görüntüsü.
//
// 🔴 BURAYA KİŞİSEL VERİ VE DEKONT YOLU KONMAZ: e-posta, kullanıcı adı,
// receipt_path, tam tx_hash ve notlar bilerek dışarıda bırakıldı. Denetim
// kaydı "durum ve tutar nasıl değişti" sorusuna cevap verir; kimlik bilgisi
// aynı satırdaki actor_user_id ve entity_id üzerinden zaten izlenebilir.
//
// test: deposit_integration_test.go#TestApproveWritesAuditLog
func snapshot(d db.Deposit) map[string]any {
	return map[string]any{
		"status":        string(d.Status),
		"amountMinor":   d.AmountMinor,
		"creditedMinor": d.CreditedMinor,
		"methodName":    d.MethodName,
		"hasReceipt":    d.ReceiptPath != "",
	}
}
