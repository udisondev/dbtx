package dbtx

import (
	"context"
	"database/sql"
)

// SQLConn is the surface a database/sql-backed repository depends on.
// *sql.DB, *sql.Conn, *SQLDBExecutor and *SQLConnExecutor all satisfy it.
type SQLConn interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	PingContext(ctx context.Context) error
}

// SqlTxExecutor is the narrow surface a service holds to manage transaction
// boundaries. Repositories use SQLConn instead.
type SqlTxExecutor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error, opts ...SQLOpt) error
	WithTx(ctx context.Context, fn func(ctx context.Context, tx *sql.Tx) error, opts ...SQLOpt) error
}

var (
	_ SQLConn       = (*sql.DB)(nil)
	_ SQLConn       = (*sql.Conn)(nil)
	_ SqlTxExecutor = (*SQLDBExecutor)(nil)
	_ SqlTxExecutor = (*SQLConnExecutor)(nil)
)
