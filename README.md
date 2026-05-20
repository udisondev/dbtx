# dbtx

Thin transaction-routing wrapper for [pgx](https://github.com/jackc/pgx) and `database/sql`. Open `InTx` at the business-logic layer; let your repositories stay transaction-agnostic; let an orchestrator compose several services into a single atomic operation without changing a single signature along the way.

```
go get github.com/udisondev/dbtx
```

## The problem dbtx solves

In Go, threading a transaction from a use-case down through several repositories is awkward. You have a few options, each with a tax:

1. **Pass `*sql.Tx` / `pgx.Tx` as a parameter** through every layer. Repository methods grow a `tx` argument, half of which is `nil` when you call them outside a transaction.
2. **Hand-roll a Unit of Work / repository factory** that mints transactional repos. More machinery, more types.
3. **Stash the tx in `context.Context`** manually. Then every repository has to remember to read it out and choose between `tx.ExecContext` and `db.ExecContext`. Easy to forget, easy to leak.

dbtx is option 3, **productized and invisible to the caller**:

- Repositories accept a `dbtx.PgxConn` (or `dbtx.SQLConn`) interface — exactly the surface `*pgxpool.Pool` / `*sql.DB` already expose. **No `tx` parameter, no transaction-aware code in the repo.**
- The dbtx wrapper transparently routes `Exec` / `Query` / `QueryRow` through a transaction stashed in `context.Context` when one is present.
- The same use-case opens that transaction by calling `executor.InTx(ctx, fn)`.
- A nested `InTx` joins the outer transaction (savepoint for pgx, tx reuse for `database/sql`) — so an orchestrator can wrap several already-transactional services into a single atomic call.

The result: repository signatures don't know transactions exist, service code reads naturally (`db.InTx(ctx, func(ctx) error { ... })`), and composition just works.

## How it fits together

```
┌──────────────────────────┐
│ use-case / orchestrator  │  ── opens InTx (top-level tx)
└─────────────┬────────────┘
              │ ctx carries pgx.Tx
              ▼
       ┌──────────────┐
       │   service    │  ── opens InTx (joins via savepoint)
       └──────┬───────┘
              │ ctx still carries the tx
              ▼
        ┌───────────┐
        │   repo    │  ── exec/query via the executor → tx
        └───────────┘
```

Three rules to remember:

1. Repositories take `PgxConn` / `SQLConn`. They never see a `tx`.
2. Services take a `PgxTxExecutor` / `SqlTxExecutor` interface — a single-method surface (`InTx`) decoupled from data access. In production it's a `*PgxPoolExecutor` / `*SQLDBExecutor` (or a `*Conn` variant); in tests it can be a fake that just runs `fn(ctx)` directly.
3. The concrete executor (`*PgxPoolExecutor` etc.) implements both interfaces — it's both a `PgxTxExecutor` (`InTx`) and a `PgxConn` (`Exec`/`Query`/`QueryRow`/...). So at wiring time you construct it once and hand the same value to the service (typed as `PgxTxExecutor`) and the repository (typed as `PgxConn`).

## Quick example (pgx)

```go
package main

import (
    "context"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/udisondev/dbtx"
)

// ── Repository ─────────────────────────────────────────
// Takes the interface, not the concrete pool. Never sees a tx.

type UserRepo struct {
    db dbtx.PgxConn
}

func (r *UserRepo) Insert(ctx context.Context, id, name string) error {
    _, err := r.db.Exec(ctx, "INSERT INTO users (id, name) VALUES ($1, $2)", id, name)
    return err
}

// ── Service ────────────────────────────────────────────
// Owns transactional boundaries via InTx.

type UserService struct {
    db    dbtx.PgxTxExecutor
    users *UserRepo
}

func (s *UserService) Create(ctx context.Context, id, name string) error {
    return s.db.InTx(ctx, func(ctx context.Context) error {
        return s.users.Insert(ctx, id, name)
        // any other repo call here joins the same tx automatically
    })
}

// ── Wiring ─────────────────────────────────────────────

func main() {
    pool, _ := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
    defer pool.Close()

    db := dbtx.NewPgxPoolExecutor(pool) // implements PgxConn and PgxTxExecutor

    svc := &UserService{
        db:    db,                 // service sees only the InTx surface
        users: &UserRepo{db: db},  // repo sees the data-access surface
    }

    _ = svc.Create(context.Background(), "u-1", "Alice")
}
```

The same `db` is wired into `UserService` (as an executor, for `InTx`) and into `UserRepo` (as a `PgxConn`). In tests you can replace it with any other `PgxConn` — a different executor, a raw pool, a test wrapper.

## Nested transactions across multiple services

This is what dbtx is really for. Suppose a money transfer touches two services:

- `WalletService` updates balances.
- `LedgerService` appends an immutable journal entry.

Each service is independently useful and is itself transactional — when called directly it opens its own `InTx`. But a `TransferUseCase` on top needs **all of it in one atomic operation**: a partial transfer is unacceptable.

With dbtx the orchestrator just wraps both services in another `InTx`. The inner `InTx`-es detect the outer tx in `ctx` and join it (via savepoint).

```go
// ── Wallet ─────────────────────────────────────────────

type WalletRepo struct{ db dbtx.PgxConn }

func (r *WalletRepo) Balance(ctx context.Context, id string) (int64, error) {
    var v int64
    err := r.db.QueryRow(ctx, "SELECT balance FROM wallets WHERE id=$1", id).Scan(&v)
    return v, err
}

func (r *WalletRepo) SetBalance(ctx context.Context, id string, v int64) error {
    _, err := r.db.Exec(ctx, "UPDATE wallets SET balance=$2 WHERE id=$1", id, v)
    return err
}

type WalletService struct {
    db      dbtx.PgxTxExecutor
    wallets *WalletRepo
}

func (s *WalletService) Debit(ctx context.Context, id string, amount int64) error {
    return s.db.InTx(ctx, func(ctx context.Context) error {
        bal, err := s.wallets.Balance(ctx, id)
        if err != nil {
            return err
        }
        if bal < amount {
            return ErrInsufficientFunds
        }
        return s.wallets.SetBalance(ctx, id, bal-amount)
    })
}

func (s *WalletService) Credit(ctx context.Context, id string, amount int64) error {
    return s.db.InTx(ctx, func(ctx context.Context) error {
        bal, err := s.wallets.Balance(ctx, id)
        if err != nil {
            return err
        }
        return s.wallets.SetBalance(ctx, id, bal+amount)
    })
}

// ── Ledger ─────────────────────────────────────────────

type LedgerRepo struct{ db dbtx.PgxConn }

func (r *LedgerRepo) Append(ctx context.Context, from, to string, amount int64) error {
    _, err := r.db.Exec(ctx,
        "INSERT INTO ledger (src, dst, amount) VALUES ($1, $2, $3)",
        from, to, amount)
    return err
}

type LedgerService struct {
    db      dbtx.PgxTxExecutor
    entries *LedgerRepo
}

func (s *LedgerService) Record(ctx context.Context, from, to string, amount int64) error {
    return s.db.InTx(ctx, func(ctx context.Context) error {
        return s.entries.Append(ctx, from, to, amount)
    })
}

// ── Orchestrator: one tx across both services ──────────

type TransferUseCase struct {
    db      dbtx.PgxTxExecutor
    wallets *WalletService
    ledger  *LedgerService
}

func (uc *TransferUseCase) Transfer(ctx context.Context, from, to string, amount int64) error {
    return uc.db.InTx(ctx, func(ctx context.Context) error {
        if err := uc.wallets.Debit(ctx, from, amount); err != nil {
            return err
        }
        if err := uc.wallets.Credit(ctx, to, amount); err != nil {
            return err
        }
        return uc.ledger.Record(ctx, from, to, amount)
    })
}
```

### What happens at runtime

1. `Transfer` calls `InTx`. No tx in `ctx` yet → a **top-level** transaction is opened. The pgx.Tx is stashed in the new `ctx` passed to the closure.
2. `WalletService.Debit` calls `InTx`. The outer tx is already in `ctx` → a **savepoint** is opened on it. `Balance`/`SetBalance` inside automatically route through that savepoint.
3. `Debit` returns nil → the savepoint commits. Outer tx is still open.
4. `WalletService.Credit` → another savepoint, same story.
5. `LedgerService.Record` → another savepoint.
6. `Transfer` returns nil → the top-level tx commits. All three savepoints become permanent atomically.

If any of the inner calls returns an error:

- That savepoint rolls back (its own writes erased).
- The error bubbles up to `Transfer`'s closure.
- `Transfer` returns the error → the **top-level tx rolls back**, undoing every committed savepoint above it.

Either everything commits or nothing does. Each service is still callable on its own — when invoked outside an orchestrator, the very first `InTx` becomes top-level and the same code commits a self-contained tx.

## `database/sql` version

Same shape, prefixed names. Wrapper is `*dbtx.SQLDBExecutor`, interface is `dbtx.SQLConn`, helpers are `SQLWithTx` / `SQLFromCtx`, options are `SQLWithIsolationLevel` / `SQLWithReadOnly`.

```go
import (
    "database/sql"

    _ "github.com/jackc/pgx/v5/stdlib" // or any other driver
    "github.com/udisondev/dbtx"
)

db, _ := sql.Open("pgx", dsn)
exec := dbtx.NewSQLDBExecutor(db) // implements SQLConn and SqlTxExecutor

err := exec.InTx(ctx, func(ctx context.Context) error {
    _, err := exec.ExecContext(ctx, "INSERT INTO users (id, name) VALUES ($1, $2)", "u-1", "Alice")
    return err
}, dbtx.SQLWithIsolationLevel(sql.LevelSerializable))
```

One semantic difference: `database/sql` has no portable savepoint API, so a nested `InTx` does **not** open a savepoint — it reuses the existing transaction. The orchestrator pattern still works (everything still ends up in one tx), but you lose the ability to roll back just the inner step. An error from any inner `InTx` aborts the whole tx.

## Transaction options

pgx — wraps `pgx.TxOptions`:

```go
exec.InTx(ctx, fn,
    dbtx.WithIsolationLevel(pgx.Serializable),
    dbtx.WithAccessMode(pgx.ReadWrite),
    dbtx.WithDeferrableMode(pgx.Deferrable),
)
```

`database/sql` — wraps `sql.TxOptions`:

```go
exec.InTx(ctx, fn,
    dbtx.SQLWithIsolationLevel(sql.LevelSerializable),
    dbtx.SQLWithReadOnly(true),
)
```

Default isolation is `ReadCommitted` on both sides. Options passed to a **nested** `InTx` are ignored — for pgx, savepoints don't accept `TxOptions`; for `database/sql`, the outer tx is already open.

## Why `PgxConn` / `SQLConn` exist (and what they let you do)

Go doesn't have a standard "query executor" interface. `*pgxpool.Pool`, `*pgx.Conn`, `pgx.Tx`, `*sql.DB`, `*sql.Tx`, `*sql.Conn` are all concrete types with no common ancestor.

`dbtx.PgxConn` is an interface whose signature matches `*pgxpool.Pool` exactly. `*dbtx.PgxPoolExecutor` embeds `*pgxpool.Pool` and overrides only the data-access methods, so it satisfies `PgxConn` automatically — including any method pgx adds to the pool surface in the future. `dbtx.SQLConn` mirrors the same idea for `*sql.DB`. Compile-time guards in the source enforce this:

```go
var _ dbtx.PgxConn = (*pgxpool.Pool)(nil)
var _ dbtx.PgxConn = (*dbtx.PgxPoolExecutor)(nil)

var _ dbtx.SQLConn = (*sql.DB)(nil)
var _ dbtx.SQLConn = (*dbtx.SQLDBExecutor)(nil)
```

The embedded pool/DB stays accessible: `executor.Pool.Acquire(ctx)`, `executor.DB.Stats()` etc. work as on the raw types and bypass tx routing.

So when you declare your repository as

```go
type UserRepo struct {
    db dbtx.PgxConn
}
```

you can plug in either the raw pool or the executor without changing the type. No magic, no codegen — just an interface picked to fit existing surfaces.

## Direct tx access

`dbtx.FromCtx(ctx)` / `dbtx.SQLFromCtx(ctx)` return the active transaction stored in context if you need to call methods that aren't exposed through `PgxConn` / `SQLConn` (e.g. `pgx.Tx.LargeObjects`, `pgx.Tx.Prepare`). Use this sparingly — the whole point of the library is that you rarely need it.

```go
exec.InTx(ctx, func(ctx context.Context) error {
    tx, _ := dbtx.FromCtx(ctx)
    return tx.SendBatch(ctx, batch).Close()
})
```

### `WithTx`: same as `InTx`, but hands the tx to the closure

When you need to pass the raw transaction to a third-party component that takes a `pgx.Tx` / `*sql.Tx` directly (an outbox publisher, a migration runner, a query builder bound to a tx, …), use `WithTx`. It's `InTx` with one extra parameter: the closure receives `(ctx, tx)` instead of just `(ctx)`. The `tx` is the same one that's been stashed in `ctx` — either the top-level tx just opened, or, when nested, the savepoint just opened on the outer pgx tx (for `database/sql`, the existing outer tx, since there's no portable savepoint).

```go
err := exec.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
    if err := outbox.Publish(ctx, tx, event); err != nil { // third-party API wants pgx.Tx
        return err
    }
    return repo.Insert(ctx, row) // repo still uses ctx-routed access, same tx
})
```

Same semantics as `InTx`: nested calls join the outer tx, fn's error rolls back, nil commits, options on a nested call are ignored. Reach for `WithTx` only when the explicit `tx` argument is required; otherwise stick to `InTx` and keep call sites tx-agnostic.

The `database/sql` form mirrors it:

```go
err := exec.WithTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
    return thirdPartyThatNeedsSQLTx(ctx, tx)
})
```

`WithTx` is part of the `PgxTxExecutor` / `SqlTxExecutor` interfaces, so services can depend on the narrow surface and still hand a tx to whatever needs one.

## Testing

The library is tested against a real Postgres via [testcontainers-go](https://golang.testcontainers.org/) using `testify/suite`. See `pgx_tx_test.go` and `sql_tx_test.go` for the layout: one shared `postgres:16-alpine` container per test package, separate tables per suite for parallel safety, `TRUNCATE` between tests.

Run locally:

```bash
go test ./... -v -count=1
```

If Docker isn't available, both suites self-skip with a clear message instead of failing.

## Status

Single-purpose library, small surface, no external moving parts beyond pgx / `database/sql`. The shape is set; future changes will be additive only.

## License

MIT — see [LICENSE](LICENSE).
