package dbtx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/suite"
	"github.com/udisondev/dbtx"
)

type PgxConnSuite struct {
	suite.Suite
	ctx  context.Context
	conn *pgx.Conn
	exec *dbtx.PgxConnExecutor
}

func TestPgxConnSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(PgxConnSuite))
}

func (s *PgxConnSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := getSharedDSN(s.T())
	conn, err := pgx.Connect(s.ctx, dsn)
	s.Require().NoError(err)
	_, err = conn.Exec(s.ctx,
		"CREATE TABLE IF NOT EXISTS accounts_pgx_conn (id text primary key, balance int not null)")
	s.Require().NoError(err)
	s.conn = conn
	s.exec = dbtx.NewPgxConnExecutor(conn)
}

func (s *PgxConnSuite) TearDownSuite() {
	if s.conn != nil {
		_ = s.conn.Close(s.ctx)
	}
}

func (s *PgxConnSuite) SetupTest() {
	_, err := s.conn.Exec(s.ctx, "TRUNCATE accounts_pgx_conn")
	s.Require().NoError(err)
}

func (s *PgxConnSuite) TestInTxCommit() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx_conn VALUES ('A', 100)")
		return execErr
	})
	s.Require().NoError(err)

	var balance int
	err = s.conn.QueryRow(s.ctx, "SELECT balance FROM accounts_pgx_conn WHERE id='A'").Scan(&balance)
	s.Require().NoError(err)
	s.Equal(100, balance)
}

func (s *PgxConnSuite) TestInTxRollbackOnError() {
	sentinel := errors.New("boom")
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx_conn VALUES ('A', 100)")
		s.Require().NoError(execErr)
		return sentinel
	})
	s.Require().ErrorIs(err, sentinel)

	var count int
	err = s.conn.QueryRow(s.ctx, "SELECT COUNT(*) FROM accounts_pgx_conn").Scan(&count)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *PgxConnSuite) TestExecRoutedThroughTx() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx_conn VALUES ('A', 100)")
		s.Require().NoError(execErr)

		var inside int
		insideErr := s.exec.QueryRow(ctx, "SELECT balance FROM accounts_pgx_conn WHERE id='A'").Scan(&inside)
		s.Require().NoError(insideErr)
		s.Equal(100, inside)
		return nil
	})
	s.Require().NoError(err)
}
