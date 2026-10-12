package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultIsoLevel = pgx.ReadCommitted

type txKey struct{}

type TxManager struct {
	pool         *pgxpool.Pool
	isoLevel     pgx.TxIsoLevel
	queryTimeout time.Duration
}

func NewTxManager(pool *pgxpool.Pool, queryTimeout time.Duration) *TxManager {
	return &TxManager{pool: pool, isoLevel: defaultIsoLevel, queryTimeout: queryTimeout}
}

func (m *TxManager) Executor(ctx context.Context) Querier {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}

	return m.pool
}

func (m *TxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}

	tx, err := m.begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)

	defer func() {
		if p := recover(); p != nil {
			_ = m.rollback(ctx, tx)
			panic(p)
		}
	}()

	if err := fn(txCtx); err != nil {
		if rbErr := m.rollback(ctx, tx); rbErr != nil {
			return errors.Join(err, rbErr)
		}

		return err
	}

	if err := m.commit(ctx, tx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (m *TxManager) begin(ctx context.Context) (pgx.Tx, error) {
	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()

	return m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: m.isoLevel})
}

func (m *TxManager) commit(ctx context.Context, tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(ctx, m.queryTimeout)
	defer cancel()

	return tx.Commit(ctx)
}

func (m *TxManager) rollback(ctx context.Context, tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.queryTimeout)
	defer cancel()

	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("rollback tx: %w", err)
	}

	return nil
}

func txFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)

	return tx, ok
}
