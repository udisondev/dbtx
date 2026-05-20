// Long-lived cqrs.EventBus over a watermill-sql v4 outbox driven by a dbtx
// *SQLDBExecutor.
//
// The publisher chain is built once at wiring time:
//
//	cqrs.EventBus → forwarder.Publisher → watermill-sql.Publisher → dbtx executor
//
// At call sites the service publishes typed events through the bus
// (bus.Publish(ctx, OrderPlaced{...})). The bus marshals the event, attaches
// the caller's ctx to the resulting message, and the SQL publisher hands that
// ctx to dbtx's ExecContext. dbtx sees the active transaction in ctx and
// routes the outbox INSERT through it — the row commits atomically with the
// rest of the InTx closure.
//
// Run against a Postgres reachable at $DATABASE_URL (default
// postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable).
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/ThreeDotsLabs/watermill"
	watermillSQL "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/components/cqrs"
	"github.com/ThreeDotsLabs/watermill/components/forwarder"
	_ "github.com/jackc/pgx/v5/stdlib"
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
// *dbtx.SQLDBExecutor satisfies it (so does the raw *sql.DB).
type OrderExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type OrderRepo struct {
	db OrderExecer
}

func (r *OrderRepo) Save(ctx context.Context, o Order) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO orders (id, total) VALUES ($1, $2)", o.ID, o.Total)
	return err
}

// TxRunner is the tx-boundary contract the service depends on. Defined as a
// plain function type — the service takes a callable, no interface required.
// *dbtx.SQLDBExecutor's InTx method matches this signature, so pass it as a
// method value at wiring time. The service stays free of dbtx / database/sql
// types in its dependencies.
type TxRunner func(ctx context.Context, fn func(ctx context.Context) error) error

type EventPublisher interface {
	Publish(ctx context.Context, event any) error
}

type OrderService struct {
	inTx   TxRunner
	orders *OrderRepo
	events EventPublisher
}

func (s *OrderService) Place(ctx context.Context, o Order) error {
	return s.inTx(ctx, func(ctx context.Context) error {
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

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := setupSchema(ctx, db); err != nil {
		log.Fatalf("setup: %v", err)
	}

	sqlExec := dbtx.NewSQLDBExecutor(db)

	logger := watermill.NewSlogLogger(slog.Default())

	sqlPub, err := watermillSQL.NewPublisher(
		watermillSQL.BeginnerFromStdSQL(sqlExec),
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
		inTx:   sqlExec.InTx,
		orders: &OrderRepo{db: sqlExec},
		events: bus,
	}

	if err := svc.Place(ctx, Order{ID: "o-1", Total: 4200}); err != nil {
		log.Fatalf("place: %v", err)
	}
	fmt.Println("placed o-1 + outbox row in one tx")
}

func setupSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS orders (id text PRIMARY KEY, total bigint NOT NULL)")
	return err
}
