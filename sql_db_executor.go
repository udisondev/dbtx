package dbtx

import (
	"context"
	"database/sql"
)

// SQLDBExecutor is a *sql.DB whose data-access methods route through the
// *sql.Tx in ctx when present. The embedded DB is accessible via .DB for
// explicit bypass.
type SQLDBExecutor struct {
	*sql.DB
}

var _ SQLConn = (*SQLDBExecutor)(nil)

// NewSQLDBExecutor wraps db.
func NewSQLDBExecutor(db *sql.DB) *SQLDBExecutor {
	return &SQLDBExecutor{DB: db}
}

// InTx runs fn in a transaction. If ctx carries a tx, fn reuses it;
// otherwise a new top-level tx is opened with opts. fn's error triggers
// rollback, nil triggers commit. Nested InTx does not create a savepoint;
// options on a nested call are ignored.
func (e *SQLDBExecutor) InTx(
	ctx context.Context,
	fn func(ctx context.Context) error,
	opts ...SQLOpt,
) error {
	return sqlInTx(ctx, e.DB.BeginTx, fn, opts...)
}

// WithTx is InTx that also hands the active *sql.Tx to fn — the one it just
// stashed in ctx, or the existing one when nested (database/sql has no
// portable savepoint API, so a nested call reuses the outer tx). Use it when
// fn must pass the raw tx to a third-party component that takes *sql.Tx
// directly; otherwise prefer InTx.
func (e *SQLDBExecutor) WithTx(
	ctx context.Context,
	fn func(ctx context.Context, tx *sql.Tx) error,
	opts ...SQLOpt,
) error {
	return sqlWithTx(ctx, e.DB.BeginTx, fn, opts...)
}

func (e *SQLDBExecutor) ExecContext(
	ctx context.Context,
	query string,
	args ...any,
) (sql.Result, error) {
	if tx, ok := SQLFromCtx(ctx); ok {
		return tx.ExecContext(ctx, query, args...)
	}
	return e.DB.ExecContext(ctx, query, args...)
}

func (e *SQLDBExecutor) QueryContext(
	ctx context.Context,
	query string,
	args ...any,
) (*sql.Rows, error) {
	if tx, ok := SQLFromCtx(ctx); ok {
		return tx.QueryContext(ctx, query, args...)
	}
	return e.DB.QueryContext(ctx, query, args...)
}

func (e *SQLDBExecutor) QueryRowContext(
	ctx context.Context,
	query string,
	args ...any,
) *sql.Row {
	if tx, ok := SQLFromCtx(ctx); ok {
		return tx.QueryRowContext(ctx, query, args...)
	}
	return e.DB.QueryRowContext(ctx, query, args...)
}

func (e *SQLDBExecutor) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	if tx, ok := SQLFromCtx(ctx); ok {
		return tx.PrepareContext(ctx, query)
	}
	return e.DB.PrepareContext(ctx, query)
}
