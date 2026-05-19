package dbtx_test

import (
	"context"
	"log"
	"os"
	"sync"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var (
	sharedOnce      sync.Once
	sharedContainer *postgres.PostgresContainer
	sharedDSN       string
	sharedErr       error
)

func getSharedDSN(t *testing.T) string {
	t.Helper()
	sharedOnce.Do(func() {
		ctx := context.Background()
		c, err := postgres.Run(ctx,
			"postgres:16-alpine",
			postgres.WithDatabase("dbtx"),
			postgres.WithUsername("dbtx"),
			postgres.WithPassword("dbtx"),
			postgres.BasicWaitStrategies(),
		)
		if err != nil {
			sharedErr = err
			return
		}
		dsn, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			sharedErr = err
			return
		}
		sharedContainer = c
		sharedDSN = dsn
	})
	if sharedErr != nil {
		t.Skipf("docker required to run dbtx tests: %v", sharedErr)
	}
	return sharedDSN
}

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedContainer != nil {
		if err := sharedContainer.Terminate(context.Background()); err != nil {
			log.Printf("terminate testcontainer: %v", err)
		}
	}
	os.Exit(code)
}
