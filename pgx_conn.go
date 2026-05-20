package dbtx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxConn is the surface a pgx-backed repository depends on.
// *pgxpool.Pool, *pgx.Conn, *PgxPoolExecutor and *PgxConnExecutor all satisfy it.
type PgxConn interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Ping(ctx context.Context) error
}

// PgxTxExecutor is the narrow surface a service holds to manage transaction
// boundaries. Repositories use PgxConn instead. Transaction options are baked
// into the executor at construction time (see NewPgxPoolExecutor /
// NewPgxConnExecutor), so this interface stays free of pgx-specific types.
type PgxTxExecutor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
	WithTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error
}

var (
	_ PgxConn       = (*pgxpool.Pool)(nil)
	_ PgxConn       = (*pgx.Conn)(nil)
	_ PgxTxExecutor = (*PgxPoolExecutor)(nil)
	_ PgxTxExecutor = (*PgxConnExecutor)(nil)
)
