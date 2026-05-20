# Interop example: watermill-sql v4 outbox

Two minimal programs showing that a dbtx executor can serve as the long-lived database handle for a [watermill-sql v4](https://github.com/ThreeDotsLabs/watermill-sql) publisher — no per-transaction publisher reconstruction required, with typed events through the watermill CQRS bus.

This is an example, not a recommendation. The point is to document one concrete interop shape.

## The chain

```
cqrs.EventBus → forwarder.Publisher → watermill-sql.Publisher → dbtx executor → Postgres
```

Built once at wiring time. At call sites the service uses the high-level CQRS surface:

```go
return s.exec.InTx(ctx, func(ctx context.Context) error {
    if err := s.orders.Save(ctx, order); err != nil {
        return err
    }
    return s.events.Publish(ctx, OrderPlaced{...}) // typed event
})
```

How `ctx` flows:

1. `dbtx.InTx(ctx, fn)` opens a tx and stashes it in the ctx handed to `fn` (savepoint on the pgx side for nested calls).
2. `cqrs.EventBus.Publish(ctx, event)` marshals the event to a `*message.Message` and calls `msg.SetContext(ctx)` internally.
3. `forwarder.Publisher.Publish` wraps the message in an envelope (preserving ctx) and forwards to the wrapped publisher.
4. `watermill-sql.Publisher.Publish` reads `messages[0].Context()` and passes it to `db.ExecContext`.
5. The dbtx executor's `ExecContext` sees the tx in ctx and routes the INSERT through it.
6. The outbox row commits atomically with the rest of `fn`'s work.

## Why this is only possible with watermill-sql v4

v3's publisher hardcodes `context.Background()` inside `Publish`, so no executor — dbtx or otherwise — can intercept it. The v3 way is the classic per-tx publisher.

v4 changed one line: the publisher now reads ctx from the message (`messages[0].Context()`). Combined with a `ContextExecutor` that routes through a tx stashed in ctx — which is what `*dbtx.SQLDBExecutor` and `*dbtx.PgxPoolExecutor` already do — the publisher's `INSERT` lands in whatever transaction the caller is currently in. `BeginnerFromStdSQL` / `BeginnerFromPgx` adapt the dbtx executors to v4's `Beginner` interface.

## Variants

- [`stdsql/`](./stdsql) — `*sql.DB` via `pgx/v5/stdlib`, wired with `dbtx.NewSQLDBExecutor`.
- [`pgx/`](./pgx) — `*pgxpool.Pool`, wired with `dbtx.NewPgxPoolExecutor`. Nested `InTx` becomes a real savepoint on the pgx side.

## Running

Each variant expects a reachable Postgres in `DATABASE_URL` (defaults to `postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable`):

```sh
DATABASE_URL=postgres://... go run ./stdsql
DATABASE_URL=postgres://... go run ./pgx
```

Both programs create an `orders` table, run one `InTx` that inserts an order and publishes an `OrderPlaced` event through the bus, then exit. The watermill outbox tables are created automatically (`AutoInitializeSchema: true`). Topic naming and JSON marshaling are handled by `cqrs.JSONMarshaler{GenerateName: cqrs.StructName}`.

## Things to keep in mind

- **Use the high-level surface.** The service depends on `Publish(ctx, any) error`, not on `message.Publisher`. Going through `cqrs.EventBus` means `msg.SetContext(ctx)` is automatic; reaching for the raw `Publisher.Publish(topic, *Message)` API would put the burden of `SetContext` back on the caller (forget it and the publish silently bypasses the tx).
- **`cqrs.EventBus.Publish` takes a single event** (`Publish(ctx, event any) error`). For multiple events, call it multiple times — each invocation carries its own ctx. There is no batch-ctx foot-gun like in the raw `Publisher.Publish(topic, msg1, msg2, …)` API, where the SQL publisher only reads `messages[0].Context()`.
- **`forwarder.NewPublisher` is a transparent decorator** — only rewrites topic/envelope, preserves ctx via `wrappedMsg.SetContext(msg.Context())`.

## Module layout

This directory is a **separate Go module** (`go.mod` with a `replace` pointing at `../..`), so the dbtx root module does not pick up watermill or any of its transitive dependencies.
