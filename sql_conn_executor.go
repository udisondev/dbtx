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
	txOpts sql.TxOptions
}

var _ SQLConn = (*SQLConnExecutor)(nil)

// NewSQLConnExecutor wraps conn. The opts configure the sql.TxOptions used
// for every top-level transaction opened via InTx / WithTx (nested calls
// reuse the outer tx and ignore options). Default isolation is
// sql.LevelReadCommitted.
func NewSQLConnExecutor(conn *sql.Conn, opts ...SQLOpt) *SQLConnExecutor {
	return &SQLConnExecutor{Conn: conn, txOpts: buildSQLTxOptions(opts)}
}

// InTx runs fn in a transaction. If ctx carries a tx, fn reuses it;
// otherwise a new top-level tx is opened with the options the executor was
// configured with. fn's error triggers rollback, nil triggers commit. Nested
// InTx does not create a savepoint.
func (e *SQLConnExecutor) InTx(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	return sqlInTx(ctx, e.Conn.BeginTx, e.txOpts, fn)
}

// WithTx is InTx that also hands the active *sql.Tx to fn — the one it just
// stashed in ctx, or the existing one when nested (database/sql has no
// portable savepoint API, so a nested call reuses the outer tx). Use it when
// fn must pass the raw tx to a third-party component that takes *sql.Tx
// directly; otherwise prefer InTx.
func (e *SQLConnExecutor) WithTx(
	ctx context.Context,
	fn func(ctx context.Context, tx *sql.Tx) error,
) error {
	return sqlWithTx(ctx, e.Conn.BeginTx, e.txOpts, fn)
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

// Exec is the database/sql legacy API. It runs against the embedded *sql.Conn
// with context.Background() and does NOT route through a tx stashed in
// context — there is no ctx to read from. Use it only for code that needs a
// *sql.DB-shaped surface and is not transactional. Inside an InTx/WithTx,
// prefer ExecContext, or hand the *sql.Tx from WithTx straight to the
// caller.
func (e *SQLConnExecutor) Exec(query string, args ...any) (sql.Result, error) {
	return e.Conn.ExecContext(context.Background(), query, args...)
}

// Query is the database/sql legacy API. Same caveat as Exec: no ctx, no tx
// routing.
func (e *SQLConnExecutor) Query(query string, args ...any) (*sql.Rows, error) {
	return e.Conn.QueryContext(context.Background(), query, args...)
}

// QueryRow is the database/sql legacy API. Same caveat as Exec: no ctx, no
// tx routing.
func (e *SQLConnExecutor) QueryRow(query string, args ...any) *sql.Row {
	return e.Conn.QueryRowContext(context.Background(), query, args...)
}
