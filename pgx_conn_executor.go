package dbtx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PgxConnExecutor is a *pgx.Conn whose data-access methods route through the
// pgx.Tx in ctx when present. The embedded Conn is accessible via .Conn for
// explicit bypass.
type PgxConnExecutor struct {
	*pgx.Conn
}

var _ PgxConn = (*PgxConnExecutor)(nil)

// NewPgxConnExecutor wraps conn.
func NewPgxConnExecutor(conn *pgx.Conn) *PgxConnExecutor {
	return &PgxConnExecutor{Conn: conn}
}

// InTx runs fn in a transaction. If ctx carries a tx, fn runs in a savepoint
// on it (opts ignored); otherwise a new top-level tx is opened with opts.
// Default isolation is pgx.ReadCommitted. fn's error triggers rollback, nil
// triggers commit.
func (e *PgxConnExecutor) InTx(
	ctx context.Context,
	fn func(ctx context.Context) error,
	opts ...PgxOpt,
) error {
	return pgxInTx(ctx, e.Conn.BeginTx, buildPgxTxOptions(opts), fn)
}

// WithTx is InTx that also hands the active pgx.Tx to fn — the same one it
// just stashed in ctx (or, when nested, the savepoint just opened on the
// outer tx). Use it when fn must pass the raw tx to a third-party component
// that takes a pgx.Tx directly; otherwise prefer InTx.
func (e *PgxConnExecutor) WithTx(
	ctx context.Context,
	fn func(ctx context.Context, tx pgx.Tx) error,
	opts ...PgxOpt,
) error {
	return pgxWithTx(ctx, e.Conn.BeginTx, buildPgxTxOptions(opts), fn)
}

func (e *PgxConnExecutor) Exec(
	ctx context.Context,
	sql string,
	args ...any,
) (pgconn.CommandTag, error) {
	if tx, ok := FromCtx(ctx); ok {
		return tx.Exec(ctx, sql, args...)
	}
	return e.Conn.Exec(ctx, sql, args...)
}

func (e *PgxConnExecutor) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx, ok := FromCtx(ctx); ok {
		return tx.Query(ctx, sql, args...)
	}
	return e.Conn.Query(ctx, sql, args...)
}

func (e *PgxConnExecutor) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx, ok := FromCtx(ctx); ok {
		return tx.QueryRow(ctx, sql, args...)
	}
	return e.Conn.QueryRow(ctx, sql, args...)
}

func (e *PgxConnExecutor) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	if tx, ok := FromCtx(ctx); ok {
		return tx.SendBatch(ctx, b)
	}
	return e.Conn.SendBatch(ctx, b)
}

func (e *PgxConnExecutor) CopyFrom(
	ctx context.Context,
	tableName pgx.Identifier,
	columnNames []string,
	rowSrc pgx.CopyFromSource,
) (int64, error) {
	if tx, ok := FromCtx(ctx); ok {
		return tx.CopyFrom(ctx, tableName, columnNames, rowSrc)
	}
	return e.Conn.CopyFrom(ctx, tableName, columnNames, rowSrc)
}

// Begin returns Conn.Begin (top-level tx) or, if ctx carries a tx, a
// savepoint via Tx.Begin.
func (e *PgxConnExecutor) Begin(ctx context.Context) (pgx.Tx, error) {
	if tx, ok := FromCtx(ctx); ok {
		return tx.Begin(ctx)
	}
	return e.Conn.Begin(ctx)
}
