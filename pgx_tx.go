// Package dbtx provides InTx for pgx and database/sql: open a transaction at
// the call site, let nested calls join it via context.Context.
package dbtx

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// PgxOpt configures the pgx transaction opened by InTx.
type PgxOpt func(*pgxOpt)

type pgxTxKey struct{}

type pgxOpt struct {
	pgx.TxOptions
}

type pgxBeginTxFn func(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)

// FromCtx returns the active pgx.Tx attached to ctx.
func FromCtx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(pgxTxKey{}).(pgx.Tx)
	return tx, ok
}

// WithTx puts tx into ctx.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, pgxTxKey{}, tx)
}

// WithIsolationLevel sets pgx.TxOptions.IsoLevel.
func WithIsolationLevel(l pgx.TxIsoLevel) PgxOpt {
	return func(o *pgxOpt) { o.IsoLevel = l }
}

// WithAccessMode sets pgx.TxOptions.AccessMode.
func WithAccessMode(m pgx.TxAccessMode) PgxOpt {
	return func(o *pgxOpt) { o.AccessMode = m }
}

// WithBeginQuery sets pgx.TxOptions.BeginQuery.
func WithBeginQuery(q string) PgxOpt {
	return func(o *pgxOpt) { o.BeginQuery = q }
}

// WithDeferrableMode sets pgx.TxOptions.DeferrableMode.
func WithDeferrableMode(m pgx.TxDeferrableMode) PgxOpt {
	return func(o *pgxOpt) { o.DeferrableMode = m }
}

func pgxInTx(
	ctx context.Context,
	begin pgxBeginTxFn,
	fn func(ctx context.Context) error,
	opts ...PgxOpt,
) error {
	if outer, ok := FromCtx(ctx); ok {
		sp, err := outer.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin savepoint: %w", err)
		}
		defer sp.Rollback(ctx) //nolint:errcheck

		if err := fn(WithTx(ctx, sp)); err != nil {
			return err
		}
		return sp.Commit(ctx)
	}

	opt := pgxOpt{TxOptions: pgx.TxOptions{IsoLevel: pgx.ReadCommitted}}
	for _, apply := range opts {
		apply(&opt)
	}

	tx, err := begin(ctx, opt.TxOptions)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := fn(WithTx(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
