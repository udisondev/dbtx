package dbtx

import (
	"context"
	"database/sql"
	"fmt"
)

// SQLOpt configures a database/sql transaction opened by InTx.
type SQLOpt func(*sqlOpt)

type sqlTxKey struct{}

type sqlOpt struct {
	sql.TxOptions
}

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
	return func(o *sqlOpt) { o.Isolation = l }
}

// SQLWithReadOnly sets sql.TxOptions.ReadOnly.
func SQLWithReadOnly(ro bool) SQLOpt {
	return func(o *sqlOpt) { o.ReadOnly = ro }
}

func sqlInTx(
	ctx context.Context,
	begin sqlBeginTxFn,
	fn func(ctx context.Context) error,
	opts ...SQLOpt,
) error {
	if _, ok := SQLFromCtx(ctx); ok {
		return fn(ctx)
	}

	opt := sqlOpt{TxOptions: sql.TxOptions{Isolation: sql.LevelReadCommitted}}
	for _, apply := range opts {
		apply(&opt)
	}

	tx, err := begin(ctx, &opt.TxOptions)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := fn(SQLWithTx(ctx, tx)); err != nil {
		return err
	}
	return tx.Commit()
}
