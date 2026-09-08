//go:build integration

// M2'nin kabul sınavı: docs/trd.md KK-200..KK-205.
//
// Bu testler GERÇEK PostgreSQL'e karşı koşar. SQLite veya sahte bir veritabanı
// yeterli değildir: FOR UPDATE kilit davranışı ve kısıt tetikleyicileri motora
// özgüdür ve tam olarak burada doğrulamak istediğimiz şey onlar.
//
//	make up && go test -tags=integration ./internal/service/wallet/ -v
package wallet_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/ikmetrik/sms-platform/api/internal/adapter/postgres"
	"github.com/ikmetrik/sms-platform/api/internal/db"
	apperr "github.com/ikmetrik/sms-platform/api/internal/domain/errors"
	"github.com/ikmetrik/sms-platform/api/internal/domain/money"
	"github.com/ikmetrik/sms-platform/api/internal/service/wallet"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Println("DATABASE_URL tanımsız — entegrasyon testleri atlanıyor")
		os.Exit(0)
	}
	ctx := context.Background()
	p, err := postgres.NewPool(ctx, url)
	if err != nil {
		fmt.Printf("postgres'e bağlanılamadı: %v\n", err)
		os.Exit(1)
	}
	pool = p
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// newUser test için bakiyeli bir kullanıcı oluşturur.
func newUser(t *testing.T, balanceMinor int64) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	email := fmt.Sprintf("wt-%s-%d@test.local", t.Name(), balanceMinor)
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, username, password_hash, status, balance_minor)
		VALUES ($1, $2, 'x', 'ACTIVE', $3)
		RETURNING id`,
		email, fmt.Sprintf("wt_%d_%d", len(t.Name()), balanceMinor), balanceMinor,
	).Scan(&id)
	if err != nil {
		t.Fatalf("test kullanıcısı oluşturulamadı: %v", err)
	}
	t.Cleanup(func() {
		// Defter kaydı silinemez (değişmezlik tetikleyicisi); test verisini
		// temizlemek için tetikleyiciyi oturum düzeyinde devre dışı bırakırız.
		c, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer c.Release()
		// ALTER TABLE ... DISABLE TRIGGER KULLANILMAZ: test yarıda kesilirse
		// koruma KALICI olarak kapalı kalır. Bunun yerine tek transaction
		// içinde oturum kapsamlı kaçış kapısı (migration 00004) kullanılır —
		// SET LOCAL transaction bitince kendiliğinden düşer.
		tx, err := c.Begin(context.Background())
		if err != nil {
			return
		}
		_, _ = tx.Exec(context.Background(), `SET LOCAL app.allow_ledger_truncate = 'on'`)
		_, _ = tx.Exec(context.Background(), `SET LOCAL session_replication_role = 'replica'`)
		_, _ = tx.Exec(context.Background(), `DELETE FROM ledger_entries WHERE user_id = $1`, id)
		_, _ = tx.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
		_ = tx.Commit(context.Background())
	})
	return id
}

func svc() *wallet.Service { return wallet.New(postgres.NewTxRunner(pool)) }

func try(minor int64) money.Money { return money.New(minor, money.TRY) }

// ─────────────────────────── KK-201 ───────────────────────────

// Aynı idempotency anahtarıyla 100 EŞZAMANLI çağrı bakiyeyi TAM BİR KEZ artırır.
func TestKK201_IdempotencyUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	userID := newUser(t, 0)
	s := svc()

	const n = 100
	var applied, repeated atomic.Int64

	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < n; i++ {
		g.Go(func() error {
			res, err := s.ApplyTx(gctx, wallet.Input{
				UserID: userID, Amount: try(10_000), Type: db.LedgerTypeDEPOSIT,
				IdempotencyKey: "kk201-tek-anahtar",
				ReferenceType:  "deposit", ReferenceID: "d1",
			})
			if err != nil {
				return err
			}
			if res.AlreadyApplied {
				repeated.Add(1)
			} else {
				applied.Add(1)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("eşzamanlı çağrılardan biri hata verdi: %v", err)
	}

	if applied.Load() != 1 {
		t.Errorf("uygulanan hareket sayısı = %d, beklenen TAM 1", applied.Load())
	}
	if repeated.Load() != n-1 {
		t.Errorf("tekrar olarak dönen = %d, beklenen %d", repeated.Load(), n-1)
	}

	bal, err := s.Balance(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if bal.Minor() != 10_000 {
		t.Fatalf("bakiye = %s, beklenen 100,00 ₺ — %d eşzamanlı çağrıdan yalnız biri uygulanmalıydı",
			bal, n)
	}

	var cnt int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_entries WHERE user_id=$1`, userID).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("defter kaydı sayısı = %d, beklenen 1", cnt)
	}
}

// ─────────────────────────── KK-202 ───────────────────────────

// 100,00 ₺ bakiyeyle 50,00 ₺'lik 10 EŞZAMANLI harcamadan TAM 2'si geçer.
// Kilit olmasaydı hepsi aynı bakiyeyi okuyup geçerdi ve bakiye negatife düşerdi.
func TestKK202_ConcurrentSpendingRespectsBalance(t *testing.T) {
	ctx := context.Background()
	userID := newUser(t, 10_000) // 100,00 ₺
	s := svc()

	const attempts = 10
	var ok, insufficient atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.ApplyTx(ctx, wallet.Input{
				UserID: userID, Amount: try(-5_000), Type: db.LedgerTypePURCHASE,
				IdempotencyKey: fmt.Sprintf("kk202-harcama-%d", i), // her biri AYRI işlem
				ReferenceType:  "order", ReferenceID: fmt.Sprintf("o%d", i),
			})
			switch {
			case err == nil:
				ok.Add(1)
			case apperrIs(err, apperr.ErrInsufficientBalance):
				insufficient.Add(1)
			default:
				t.Errorf("beklenmeyen hata: %v", err)
			}
		}(i)
	}
	wg.Wait()

	if ok.Load() != 2 {
		t.Errorf("başarılı harcama = %d, beklenen TAM 2", ok.Load())
	}
	if insufficient.Load() != attempts-2 {
		t.Errorf("yetersiz bakiye = %d, beklenen %d", insufficient.Load(), attempts-2)
	}

	bal, err := s.Balance(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if bal.Minor() != 0 {
		t.Fatalf("bakiye = %s, beklenen 0,00 ₺", bal)
	}
	if bal.IsNegative() {
		t.Fatal("BAKİYE NEGATİFE DÜŞTÜ — kilit çalışmıyor")
	}
}

// ─────────────────────────── KK-200 ───────────────────────────

// Rastgele bir hareket dizisinden sonra defter toplamı ile önbelleklenmiş
// bakiye HER ZAMAN eşit olmalıdır.
func TestKK200_LedgerMatchesCachedBalance(t *testing.T) {
	ctx := context.Background()
	userID := newUser(t, 0)
	s := svc()

	moves := []struct {
		amount int64
		typ    db.LedgerType
	}{
		{50_000, db.LedgerTypeDEPOSIT},
		{-1_250, db.LedgerTypePURCHASE},
		{1_250, db.LedgerTypeREFUND},
		{-3_333, db.LedgerTypePURCHASE},
		{25_000, db.LedgerTypeDEPOSIT},
		{-7, db.LedgerTypePURCHASE},
		{-19_999, db.LedgerTypePURCHASE},
		{500, db.LedgerTypeADJUSTMENT},
	}
	var want int64
	for i, m := range moves {
		if _, err := s.ApplyTx(ctx, wallet.Input{
			UserID: userID, Amount: try(m.amount), Type: m.typ,
			IdempotencyKey: fmt.Sprintf("kk200-%d", i),
		}); err != nil {
			t.Fatalf("%d. hareket: %v", i, err)
		}
		want += m.amount
	}

	var cached, ledger, drift int64
	err := pool.QueryRow(ctx,
		`SELECT cached_balance, ledger_balance, drift FROM ledger_reconciliation WHERE user_id=$1`,
		userID).Scan(&cached, &ledger, &drift)
	if err != nil {
		t.Fatal(err)
	}
	if drift != 0 {
		t.Fatalf("MUTABAKAT SAPMASI: önbellek=%d defter=%d sapma=%d", cached, ledger, drift)
	}
	if cached != want {
		t.Fatalf("bakiye = %d, beklenen %d", cached, want)
	}
}

// ─────────────────────────── KK-203 ───────────────────────────

func TestKK203_CannotGoNegative(t *testing.T) {
	ctx := context.Background()
	userID := newUser(t, 1_000)
	s := svc()

	_, err := s.ApplyTx(ctx, wallet.Input{
		UserID: userID, Amount: try(-1_001), Type: db.LedgerTypePURCHASE,
		IdempotencyKey: "kk203-asiri",
	})
	if !apperrIs(err, apperr.ErrInsufficientBalance) {
		t.Fatalf("hata = %v, ErrInsufficientBalance bekleniyordu", err)
	}

	bal, _ := s.Balance(ctx, userID)
	if bal.Minor() != 1_000 {
		t.Fatalf("başarısız harcama bakiyeyi değiştirdi: %s", bal)
	}

	// Tam bakiye kadar harcama GEÇMELİ (sınır dahil).
	if _, err := s.ApplyTx(ctx, wallet.Input{
		UserID: userID, Amount: try(-1_000), Type: db.LedgerTypePURCHASE,
		IdempotencyKey: "kk203-tam",
	}); err != nil {
		t.Fatalf("tam bakiye harcaması reddedildi: %v", err)
	}
}

// ─────────────────────────── Girdi doğrulaması ───────────────────────────

func TestRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	userID := newUser(t, 1_000)
	s := svc()

	cases := map[string]wallet.Input{
		"idempotency anahtarı yok": {UserID: userID, Amount: try(100), Type: db.LedgerTypeDEPOSIT},
		"sıfır tutar":              {UserID: userID, Amount: try(0), Type: db.LedgerTypeDEPOSIT, IdempotencyKey: "k1"},
		"kullanıcı yok":            {Amount: try(100), Type: db.LedgerTypeDEPOSIT, IdempotencyKey: "k2"},
		"USD tutar":                {UserID: userID, Amount: money.New(100, money.USD), Type: db.LedgerTypeDEPOSIT, IdempotencyKey: "k3"},
		"tip yok":                  {UserID: userID, Amount: try(100), IdempotencyKey: "k4"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.ApplyTx(ctx, in); err == nil {
				t.Fatal("geçersiz girdi kabul edildi")
			}
		})
	}
}

// Aynı anahtarın FARKLI bir işlem için kullanılması sessizce başarılı olmamalı.
func TestIdempotencyKeyReuseWithDifferentPayloadIsRejected(t *testing.T) {
	ctx := context.Background()
	userID := newUser(t, 0)
	s := svc()

	if _, err := s.ApplyTx(ctx, wallet.Input{
		UserID: userID, Amount: try(5_000), Type: db.LedgerTypeDEPOSIT,
		IdempotencyKey: "paylasilan-anahtar",
	}); err != nil {
		t.Fatal(err)
	}

	// Aynı anahtar, FARKLI tutar → sessizce "başarılı" dönmek veriyi bozar.
	_, err := s.ApplyTx(ctx, wallet.Input{
		UserID: userID, Amount: try(9_999), Type: db.LedgerTypeDEPOSIT,
		IdempotencyKey: "paylasilan-anahtar",
	})
	if err == nil {
		t.Fatal("aynı anahtar farklı tutarla kabul edildi — çift işlem koruması yanıltıcı")
	}

	bal, _ := s.Balance(ctx, userID)
	if bal.Minor() != 5_000 {
		t.Fatalf("bakiye bozuldu: %s", bal)
	}
}

func apperrIs(err error, target *apperr.Error) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == target.Code
}
