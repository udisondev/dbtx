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
	txOpts pgx.TxOptions
}

var _ PgxConn = (*PgxPoolExecutor)(nil)

// NewPgxPoolExecutor wraps pool. The opts configure the pgx.TxOptions used
// for every top-level transaction opened via InTx / WithTx (nested calls open
// a savepoint and ignore options). Default isolation is pgx.ReadCommitted.
func NewPgxPoolExecutor(pool *pgxpool.Pool, opts ...PgxOpt) *PgxPoolExecutor {
	return &PgxPoolExecutor{Pool: pool, txOpts: buildPgxTxOptions(opts)}
}

// InTx runs fn in a transaction. If ctx carries a tx, fn runs in a savepoint
// on it; otherwise a new top-level tx is opened with the options the executor
// was configured with. fn's error triggers rollback, nil triggers commit.
func (e *PgxPoolExecutor) InTx(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	return pgxInTx(ctx, e.Pool.BeginTx, e.txOpts, fn)
}

// WithTx is InTx that also hands the active pgx.Tx to fn — the same one it
// just stashed in ctx (or, when nested, the savepoint just opened on the
// outer tx). Use it when fn must pass the raw tx to a third-party component
// that takes a pgx.Tx directly; otherwise prefer InTx.
func (e *PgxPoolExecutor) WithTx(
	ctx context.Context,
	fn func(ctx context.Context, tx pgx.Tx) error,
) error {
	return pgxWithTx(ctx, e.Pool.BeginTx, e.txOpts, fn)
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
