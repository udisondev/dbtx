module github.com/udisondev/dbtx/examples/watermill-sql-v4

go 1.26.2

replace github.com/udisondev/dbtx => ../..

require (
	github.com/ThreeDotsLabs/watermill v1.5.1
	github.com/ThreeDotsLabs/watermill-sql/v4 v4.1.2
	github.com/jackc/pgx/v5 v5.9.2
	github.com/udisondev/dbtx v0.0.0
)

require (
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/lithammer/shortuuid/v3 v3.0.7 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)
