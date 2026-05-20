// Long-lived cqrs.EventBus over a watermill-sql v4 outbox driven by a dbtx
// *PgxPoolExecutor.
//
// Same wiring as the stdsql variant — the difference is the pgx pool, which
// gives real savepoints for nested InTx via dbtx.
//
//	cqrs.EventBus → forwarder.Publisher → watermill-sql.Publisher → dbtx executor
//
// Run against a Postgres reachable at $DATABASE_URL (default
// postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable).
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/ThreeDotsLabs/watermill"
	watermillSQL "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/components/cqrs"
	"github.com/ThreeDotsLabs/watermill/components/forwarder"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/udisondev/dbtx"
)

const outboxTopic = "outbox"

type Order struct {
	ID    string
	Total int64
}

type OrderPlaced struct {
	OrderID string
	Total   int64
}

// OrderExecer is the narrow query surface the repo actually uses.
// *dbtx.PgxPoolExecutor satisfies it (so does the raw *pgxpool.Pool).
type OrderExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type OrderRepo struct {
	db OrderExecer
}

func (r *OrderRepo) Save(ctx context.Context, o Order) error {
	_, err := r.db.Exec(ctx,
		"INSERT INTO orders (id, total) VALUES ($1, $2)", o.ID, o.Total)
	return err
}

// TxRunner is the narrow tx-boundary surface the service depends on.
// *dbtx.PgxPoolExecutor satisfies it. Note: no pgx / dbtx types in the
// signature — the service is decoupled from the underlying driver.
type TxRunner interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type EventPublisher interface {
	Publish(ctx context.Context, event any) error
}

type OrderService struct {
	tx     TxRunner
	orders *OrderRepo
	events EventPublisher
}

func (s *OrderService) Place(ctx context.Context, o Order) error {
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.orders.Save(ctx, o); err != nil {
			return err
		}
		return s.events.Publish(ctx, OrderPlaced{OrderID: o.ID, Total: o.Total})
	})
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()

	if err := setupSchema(ctx, pool); err != nil {
		log.Fatalf("setup: %v", err)
	}

	pgxExec := dbtx.NewPgxPoolExecutor(pool)

	logger := watermill.NewSlogLogger(slog.Default())

	sqlPub, err := watermillSQL.NewPublisher(
		watermillSQL.BeginnerFromPgx(pgxExec),
		watermillSQL.PublisherConfig{
			SchemaAdapter:        watermillSQL.DefaultPostgreSQLSchema{},
			AutoInitializeSchema: true,
		},
		logger,
	)
	if err != nil {
		log.Fatalf("sql publisher: %v", err)
	}
	defer sqlPub.Close()

	fwdPub := forwarder.NewPublisher(sqlPub, forwarder.PublisherConfig{
		ForwarderTopic: outboxTopic,
	})

	bus, err := cqrs.NewEventBusWithConfig(fwdPub, cqrs.EventBusConfig{
		GeneratePublishTopic: func(p cqrs.GenerateEventPublishTopicParams) (string, error) {
			return p.EventName, nil
		},
		Marshaler: cqrs.JSONMarshaler{GenerateName: cqrs.StructName},
		Logger:    logger,
	})
	if err != nil {
		log.Fatalf("event bus: %v", err)
	}

	svc := &OrderService{
		tx:     pgxExec,
		orders: &OrderRepo{db: pgxExec},
		events: bus,
	}

	if err := svc.Place(ctx, Order{ID: "o-1", Total: 4200}); err != nil {
		log.Fatalf("place: %v", err)
	}
	fmt.Println("placed o-1 + outbox row in one tx")
}

func setupSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		"CREATE TABLE IF NOT EXISTS orders (id text PRIMARY KEY, total bigint NOT NULL)")
	return err
}
