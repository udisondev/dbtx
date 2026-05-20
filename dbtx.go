package dbtx

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type TxExecutor interface {
	InTx(
		ctx context.Context,
		fn func(ctx context.Context) error,
		opts ...PgxOpt,
	) error
	WithTx(
		ctx context.Context,
		fn func(ctx context.Context, tx pgx.Tx) error,
		opts ...PgxOpt,
	) error
}
