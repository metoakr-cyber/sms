// Package wallet kullanıcı bakiyesini yönetir.
//
// TEK GEÇİT KURALI: Bakiye yalnız bu paketteki Apply üzerinden değişir.
// Başka hiçbir yerde "UPDATE users SET balance_minor" yazılmaz.
// Bu kural CLAUDE.md'de değişmez #2 olarak kayıtlıdır.
//
// Bkz. docs/design.md §5, docs/trd.md FR-200..FR-206.
package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
)

type Service struct {
	tx *postgres.TxRunner
}

func New(tx *postgres.TxRunner) *Service { return &Service{tx: tx} }

// Input bir defter hareketinin girdisi.
type Input struct {
	UserID int64
	// Amount pozitifse alacak, negatifse borç. Sıfır olamaz.
	Amount money.Money
	Type   db.LedgerType

	// IdempotencyKey ZORUNLUDUR ve deterministik olmalıdır.
	// Örn. "order:<uuid>", "order:<uuid>:refund", "deposit:<uuid>".
	// Aynı anahtarla ikinci çağrı YENİ kayıt üretmez, eski sonucu döner.
	IdempotencyKey string

	ReferenceType string
	ReferenceID   string

	// CreatedByUserID admin işlemlerinde işlemi yapanı kaydeder.
	CreatedByUserID *int64
	Note            string
}

// Result hareketin sonucu.
type Result struct {
	EntryID    int64
	NewBalance money.Money
	// AlreadyApplied true ise bu çağrı bir TEKRAR'dı; yeni kayıt oluşmadı.
	AlreadyApplied bool
}

func (in Input) validate() error {
	if in.UserID == 0 {
		return fmt.Errorf("wallet: UserID zorunlu")
	}
	if in.IdempotencyKey == "" {
		return fmt.Errorf("wallet: IdempotencyKey zorunlu — çift işlem koruması olmadan bakiye değiştirilemez")
	}
	if in.Amount.IsZero() {
		return fmt.Errorf("wallet: sıfır tutarlı hareket anlamsız")
	}
	if in.Amount.Currency() != money.TRY {
		return fmt.Errorf("wallet: cüzdan yalnız TRY tutar (verilen: %s)", in.Amount.Currency())
	}
	if in.Type == "" {
		return fmt.Errorf("wallet: hareket tipi zorunlu")
	}
	return nil
}

// Apply bir defter hareketi uygular. ÇAĞIRANIN TRANSACTION'I İÇİNDE çalışır.
//
// Sipariş akışı gibi senaryolarda teklifin tüketilmesi ile bakiyenin düşülmesi
// aynı transaction'da olmalıdır; bu yüzden burada yeni transaction açılmaz.
//
// Adım sırası KRİTİKTİR:
//  1. İdempotency kontrolü — tekrarlanan çağrı erken döner
//  2. Kullanıcı satırını KİLİTLE (FOR UPDATE)
//  3. Yeterlilik kontrolü — KİLİT ALTINDA
//  4. Defter kaydı + bakiye güncellemesi
//
// 2. adım olmadan iki eşzamanlı istek aynı bakiyeyi okuyup ikisi de yeterli
// sanabilir; sonuç negatif bakiye veya kayıp güncellemedir.
func (s *Service) Apply(ctx context.Context, q *db.Queries, in Input) (Result, error) {
	if err := in.validate(); err != nil {
		return Result{}, apperr.Internal(err)
	}

	// ── 1. İdempotency ──
	if prev, err := q.FindLedgerEntryByKey(ctx, in.IdempotencyKey); err == nil {
		// Aynı anahtar farklı bir kullanıcı veya tutar için kullanılmışsa bu bir
		// programlama hatasıdır; sessizce "başarılı" dönmek veriyi bozar.
		if prev.UserID != in.UserID || prev.AmountMinor != in.Amount.Minor() {
			return Result{}, apperr.Internal(fmt.Errorf(
				"wallet: idempotency anahtarı %q farklı bir işlem için kullanılmış "+
					"(kayıtlı: user=%d amount=%d, istenen: user=%d amount=%d)",
				in.IdempotencyKey, prev.UserID, prev.AmountMinor, in.UserID, in.Amount.Minor()))
		}
		return Result{
			EntryID:        prev.ID,
			NewBalance:     money.New(prev.BalanceAfterMinor, money.TRY),
			AlreadyApplied: true,
		}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, apperr.Internal(err)
	}

	// ── 2. Kilit ──
	locked, err := q.LockUserForUpdate(ctx, in.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, apperr.ErrNotFound
	}
	if err != nil {
		return Result{}, apperr.Internal(err)
	}

	// ── 3. Yeterlilik (kilit altında) ──
	current := money.New(locked.BalanceMinor, money.TRY)
	next, err := current.Add(in.Amount)
	if err != nil {
		return Result{}, apperr.Internal(err)
	}
	if next.IsNegative() {
		return Result{}, apperr.ErrInsufficientBalance
	}

	// ── 4. Yazım ──
	entry, err := q.InsertLedgerEntry(ctx, db.InsertLedgerEntryParams{
		UserID:            in.UserID,
		AmountMinor:       in.Amount.Minor(),
		Currency:          db.CurrencyCodeTRY,
		EntryType:         in.Type,
		ReferenceType:     nullStr(in.ReferenceType),
		ReferenceID:       nullStr(in.ReferenceID),
		BalanceAfterMinor: next.Minor(),
		IdempotencyKey:    in.IdempotencyKey,
		CreatedByUserID:   in.CreatedByUserID,
		Note:              nullStr(in.Note),
	})
	if err != nil {
		return Result{}, mapLedgerErr(err)
	}

	if err := q.SetUserBalance(ctx, db.SetUserBalanceParams{
		ID: in.UserID, BalanceMinor: next.Minor(),
	}); err != nil {
		return Result{}, apperr.Internal(err)
	}

	return Result{EntryID: entry.ID, NewBalance: next}, nil
}

// ApplyTx tek başına bir hareket uygular (kendi transaction'ını açar).
//
// Eşzamanlı tekrar durumunda BİR KEZ yeniden dener: iki istek idempotency
// kontrolünü aynı anda geçip ikisi de INSERT denerse biri UNIQUE ihlali alır ve
// transaction'ı iptal olur. Yeniden denemede 1. adım artık diğerinin yazdığı
// kaydı görür ve AlreadyApplied=true döner.
func (s *Service) ApplyTx(ctx context.Context, in Input) (Result, error) {
	const maxAttempts = 2
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var res Result
		err := s.tx.InTx(ctx, func(q *db.Queries) error {
			r, err := s.Apply(ctx, q, in)
			if err != nil {
				return err
			}
			res = r
			return nil
		})
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !IsDuplicate(err) {
			return Result{}, err
		}
		// Yarışı kaybettik; yeniden denemede idempotency kaydı bulunacak.
	}
	return Result{}, apperr.Internal(fmt.Errorf(
		"wallet: %d denemede uygulanamadı: %w", maxAttempts, lastErr))
}

// Balance kullanıcının güncel bakiyesini döner.
func (s *Service) Balance(ctx context.Context, userID int64) (money.Money, error) {
	v, err := s.tx.Queries().GetUserBalance(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return money.Zero(money.TRY), apperr.ErrNotFound
	}
	if err != nil {
		return money.Zero(money.TRY), apperr.Internal(err)
	}
	return money.New(v, money.TRY), nil
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
