package dbtx

import (
	"context"
	"database/sql"
)

// SQLConnExecutor is a *sql.Conn whose data-access methods route through the
// *sql.Tx in ctx when present. The embedded Conn is accessible via .Conn for
// explicit bypass.
type SQLConnExecutor struct {
	*sql.Conn
}

var _ SQLConn = (*SQLConnExecutor)(nil)

// NewSQLConnExecutor wraps conn.
func NewSQLConnExecutor(conn *sql.Conn) *SQLConnExecutor {
	return &SQLConnExecutor{Conn: conn}
}

// InTx runs fn in a transaction. If ctx carries a tx, fn reuses it;
// otherwise a new top-level tx is opened with opts. fn's error triggers
// rollback, nil triggers commit. Nested InTx does not create a savepoint;
// options on a nested call are ignored.
func (e *SQLConnExecutor) InTx(
	ctx context.Context,
	fn func(ctx context.Context) error,
	opts ...SQLOpt,
) error {
	return sqlInTx(ctx, e.Conn.BeginTx, fn, opts...)
}

// WithTx is InTx that also hands the active *sql.Tx to fn — the one it just
// stashed in ctx, or the existing one when nested (database/sql has no
// portable savepoint API, so a nested call reuses the outer tx). Use it when
// fn must pass the raw tx to a third-party component that takes *sql.Tx
// directly; otherwise prefer InTx.
func (e *SQLConnExecutor) WithTx(
	ctx context.Context,
	fn func(ctx context.Context, tx *sql.Tx) error,
	opts ...SQLOpt,
) error {
	return sqlWithTx(ctx, e.Conn.BeginTx, fn, opts...)
}

func (e *SQLConnExecutor) ExecContext(
	ctx context.Context,
	query string,
	args ...any,
) (sql.Result, error) {
	if tx, ok := SQLFromCtx(ctx); ok {
		return tx.ExecContext(ctx, query, args...)
	}
	return e.Conn.ExecContext(ctx, query, args...)
}

func (e *SQLConnExecutor) QueryContext(
	ctx context.Context,
	query string,
	args ...any,
) (*sql.Rows, error) {
	if tx, ok := SQLFromCtx(ctx); ok {
		return tx.QueryContext(ctx, query, args...)
	}
	return e.Conn.QueryContext(ctx, query, args...)
}

func (e *SQLConnExecutor) QueryRowContext(
	ctx context.Context,
	query string,
	args ...any,
) *sql.Row {
	if tx, ok := SQLFromCtx(ctx); ok {
		return tx.QueryRowContext(ctx, query, args...)
	}
	return e.Conn.QueryRowContext(ctx, query, args...)
}

func (e *SQLConnExecutor) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	if tx, ok := SQLFromCtx(ctx); ok {
		return tx.PrepareContext(ctx, query)
	}
	return e.Conn.PrepareContext(ctx, query)
}
