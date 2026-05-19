package dbtx

import "context"

type TxExecutor interface {
	InTx(
		ctx context.Context,
		fn func(ctx context.Context) error,
		opts ...PgxOpt,
	) error
}
