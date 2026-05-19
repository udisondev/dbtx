package dbtx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxPoolExecutor is a *pgxpool.Pool whose data-access methods route through
// the pgx.Tx in ctx when present. The embedded Pool is accessible via .Pool
// for explicit bypass.
type PgxPoolExecutor struct {
	*pgxpool.Pool
}

var _ PgxConn = (*PgxPoolExecutor)(nil)

// NewPgxPoolExecutor wraps pool.
func NewPgxPoolExecutor(pool *pgxpool.Pool) *PgxPoolExecutor {
	return &PgxPoolExecutor{Pool: pool}
}

// InTx runs fn in a transaction. If ctx carries a tx, fn runs in a savepoint
// on it; otherwise a new top-level tx is opened with opts. fn's error
// triggers rollback, nil triggers commit. Options on a nested InTx are
// ignored.
func (e *PgxPoolExecutor) InTx(
	ctx context.Context,
	fn func(ctx context.Context) error,
	opts ...PgxOpt,
) error {
	return pgxInTx(ctx, e.Pool.BeginTx, fn, opts...)
}

func (e *PgxPoolExecutor) Exec(
	ctx context.Context,
	sql string,
	args ...any,
) (pgconn.CommandTag, error) {
	if tx, ok := FromCtx(ctx); ok {
		return tx.Exec(ctx, sql, args...)
	}
	return e.Pool.Exec(ctx, sql, args...)
}

func (e *PgxPoolExecutor) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx, ok := FromCtx(ctx); ok {
		return tx.Query(ctx, sql, args...)
	}
	return e.Pool.Query(ctx, sql, args...)
}

func (e *PgxPoolExecutor) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx, ok := FromCtx(ctx); ok {
		return tx.QueryRow(ctx, sql, args...)
	}
	return e.Pool.QueryRow(ctx, sql, args...)
}

func (e *PgxPoolExecutor) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	if tx, ok := FromCtx(ctx); ok {
		return tx.SendBatch(ctx, b)
	}
	return e.Pool.SendBatch(ctx, b)
}

func (e *PgxPoolExecutor) CopyFrom(
	ctx context.Context,
	tableName pgx.Identifier,
	columnNames []string,
	rowSrc pgx.CopyFromSource,
) (int64, error) {
	if tx, ok := FromCtx(ctx); ok {
		return tx.CopyFrom(ctx, tableName, columnNames, rowSrc)
	}
	return e.Pool.CopyFrom(ctx, tableName, columnNames, rowSrc)
}

// Begin returns Pool.Begin (top-level tx) or, if ctx carries a tx, a
// savepoint via Tx.Begin.
func (e *PgxPoolExecutor) Begin(ctx context.Context) (pgx.Tx, error) {
	if tx, ok := FromCtx(ctx); ok {
		return tx.Begin(ctx)
	}
	return e.Pool.Begin(ctx)
}
