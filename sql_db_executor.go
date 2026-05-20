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
	txOpts sql.TxOptions
}

var _ SQLConn = (*SQLDBExecutor)(nil)

// NewSQLDBExecutor wraps db. The opts configure the sql.TxOptions used for
// every top-level transaction opened via InTx / WithTx (nested calls reuse
// the outer tx and ignore options). Default isolation is
// sql.LevelReadCommitted.
func NewSQLDBExecutor(db *sql.DB, opts ...SQLOpt) *SQLDBExecutor {
	return &SQLDBExecutor{DB: db, txOpts: buildSQLTxOptions(opts)}
}

// InTx runs fn in a transaction. If ctx carries a tx, fn reuses it;
// otherwise a new top-level tx is opened with the options the executor was
// configured with. fn's error triggers rollback, nil triggers commit. Nested
// InTx does not create a savepoint.
func (e *SQLDBExecutor) InTx(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	return sqlInTx(ctx, e.DB.BeginTx, e.txOpts, fn)
}

// WithTx is InTx that also hands the active *sql.Tx to fn — the one it just
// stashed in ctx, or the existing one when nested (database/sql has no
// portable savepoint API, so a nested call reuses the outer tx). Use it when
// fn must pass the raw tx to a third-party component that takes *sql.Tx
// directly; otherwise prefer InTx.
func (e *SQLDBExecutor) WithTx(
	ctx context.Context,
	fn func(ctx context.Context, tx *sql.Tx) error,
) error {
	return sqlWithTx(ctx, e.DB.BeginTx, e.txOpts, fn)
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

// Exec is the database/sql legacy API. It runs against the embedded *sql.DB
// with context.Background() and does NOT route through a tx stashed in
// context — there is no ctx to read from. Use it only for code that needs a
// *sql.DB-shaped surface and is not transactional. Inside an InTx/WithTx,
// prefer ExecContext, or hand the *sql.Tx from WithTx straight to the
// caller.
func (e *SQLDBExecutor) Exec(query string, args ...any) (sql.Result, error) {
	return e.DB.Exec(query, args...)
}

// Query is the database/sql legacy API. Same caveat as Exec: no ctx, no tx
// routing.
func (e *SQLDBExecutor) Query(query string, args ...any) (*sql.Rows, error) {
	return e.DB.Query(query, args...)
}

// QueryRow is the database/sql legacy API. Same caveat as Exec: no ctx, no
// tx routing.
func (e *SQLDBExecutor) QueryRow(query string, args ...any) *sql.Row {
	return e.DB.QueryRow(query, args...)
}
