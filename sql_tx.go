package dbtx

import (
	"context"
	"database/sql"
	"fmt"
)

// SQLOpt configures the database/sql transaction options held by an executor.
// Apply at construction time (NewSQLDBExecutor / NewSQLConnExecutor); the same
// options are reused on every top-level InTx / WithTx opened through the
// executor.
type SQLOpt func(*sql.TxOptions)

type sqlTxKey struct{}

type sqlBeginTxFn func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)

// SQLFromCtx returns the active *sql.Tx attached to ctx.
func SQLFromCtx(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(sqlTxKey{}).(*sql.Tx)
	return tx, ok
}

// SQLWithTx puts tx into ctx.
func SQLWithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, sqlTxKey{}, tx)
}

// SQLWithIsolationLevel sets sql.TxOptions.Isolation.
func SQLWithIsolationLevel(l sql.IsolationLevel) SQLOpt {
	return func(o *sql.TxOptions) { o.Isolation = l }
}

// SQLWithReadOnly sets sql.TxOptions.ReadOnly.
func SQLWithReadOnly(ro bool) SQLOpt {
	return func(o *sql.TxOptions) { o.ReadOnly = ro }
}

func buildSQLTxOptions(opts []SQLOpt) sql.TxOptions {
	o := sql.TxOptions{Isolation: sql.LevelReadCommitted}
	for _, apply := range opts {
		apply(&o)
	}
	return o
}

func sqlWithTx(
	ctx context.Context,
	begin sqlBeginTxFn,
	txOpts sql.TxOptions,
	fn func(ctx context.Context, tx *sql.Tx) error,
) error {
	if outer, ok := SQLFromCtx(ctx); ok {
		return fn(ctx, outer)
	}

	tx, err := begin(ctx, &txOpts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := fn(SQLWithTx(ctx, tx), tx); err != nil {
		return err
	}
	return tx.Commit()
}

func sqlInTx(
	ctx context.Context,
	begin sqlBeginTxFn,
	txOpts sql.TxOptions,
	fn func(ctx context.Context) error,
) error {
	return sqlWithTx(ctx, begin, txOpts, func(ctx context.Context, _ *sql.Tx) error {
		return fn(ctx)
	})
}
