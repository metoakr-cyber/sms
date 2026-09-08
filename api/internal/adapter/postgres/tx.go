package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ikmetrik/sms-platform/api/internal/db"
)

// TxRunner transaction sınırını yönetir.
//
// Transaction sınırı SERVİS katmanında çizilir, depo katmanında değil:
// bir kullanım senaryosu birden çok depo çağrısını atomik yapmak isteyebilir.
type TxRunner struct {
	pool *pgxpool.Pool
}

func NewTxRunner(pool *pgxpool.Pool) *TxRunner { return &TxRunner{pool: pool} }

// InTx fn'i bir transaction içinde çalıştırır.
//
// fn hata dönerse veya panic ederse transaction geri alınır. Commit hatası da
// çağırana iletilir — sessizce başarılı sayılmaz.
func (r *TxRunner) InTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: transaction açılamadı: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			// Rollback hatası yutulur: asıl hata zaten döndürülüyor ve
			// bağlantı kapanmışsa rollback zaten anlamsızdır.
			_ = tx.Rollback(ctx)
		}
	}()

	if err := fn(db.New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit başarısız: %w", err)
	}
	committed = true
	return nil
}

// InTxSerializable en katı izolasyon seviyesiyle çalıştırır.
// Para işlemlerinde satır kilidi (FOR UPDATE) yeterlidir; bu, kilitle
// çözülemeyen okuma tutarlılığı gerektiren nadir durumlar içindir.
func (r *TxRunner) InTxSerializable(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("postgres: transaction açılamadı: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit başarısız: %w", err)
	}
	committed = true
	return nil
}

// Queries transaction dışı sorgular için.
func (r *TxRunner) Queries() *db.Queries { return db.New(r.pool) }
