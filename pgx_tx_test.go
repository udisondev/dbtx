package dbtx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"github.com/udisondev/dbtx"
)

type PgxSuite struct {
	suite.Suite
	ctx  context.Context
	pool *pgxpool.Pool
	exec *dbtx.PgxPoolExecutor
}

func TestPgxSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(PgxSuite))
}

func (s *PgxSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := getSharedDSN(s.T())
	pool, err := pgxpool.New(s.ctx, dsn)
	s.Require().NoError(err)
	_, err = pool.Exec(s.ctx,
		"CREATE TABLE IF NOT EXISTS accounts_pgx (id text primary key, balance int not null)")
	s.Require().NoError(err)
	s.pool = pool
	s.exec = dbtx.NewPgxPoolExecutor(pool)
}

func (s *PgxSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *PgxSuite) SetupTest() {
	_, err := s.pool.Exec(s.ctx, "TRUNCATE accounts_pgx")
	s.Require().NoError(err)
}

func (s *PgxSuite) TestInTxCommit() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx VALUES ('A', 100)")
		return execErr
	})
	s.Require().NoError(err)

	var balance int
	err = s.pool.QueryRow(s.ctx, "SELECT balance FROM accounts_pgx WHERE id='A'").Scan(&balance)
	s.Require().NoError(err)
	s.Equal(100, balance)
}

func (s *PgxSuite) TestInTxRollbackOnError() {
	sentinel := errors.New("boom")
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx VALUES ('A', 100)")
		s.Require().NoError(execErr)
		return sentinel
	})
	s.Require().ErrorIs(err, sentinel)

	var count int
	err = s.pool.QueryRow(s.ctx, "SELECT COUNT(*) FROM accounts_pgx").Scan(&count)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *PgxSuite) TestNestedInTx_OuterCommitsAfterInnerRollback() {
	innerErr := errors.New("inner")
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx VALUES ('A', 100)")
		s.Require().NoError(execErr)

		innerRet := s.exec.InTx(ctx, func(ctx context.Context) error {
			_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx VALUES ('B', 200)")
			s.Require().NoError(execErr)
			return innerErr
		})
		s.Require().ErrorIs(innerRet, innerErr)
		return nil
	})
	s.Require().NoError(err)

	rows, err := s.pool.Query(s.ctx, "SELECT id FROM accounts_pgx ORDER BY id")
	s.Require().NoError(err)
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		s.Require().NoError(rows.Scan(&id))
		ids = append(ids, id)
	}
	s.Equal([]string{"A"}, ids)
}

func (s *PgxSuite) TestNestedInTx_OuterRollbackDiscardsBoth() {
	outerErr := errors.New("outer")
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx VALUES ('A', 100)")
		s.Require().NoError(execErr)

		innerRet := s.exec.InTx(ctx, func(ctx context.Context) error {
			_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx VALUES ('B', 200)")
			return execErr
		})
		s.Require().NoError(innerRet)
		return outerErr
	})
	s.Require().ErrorIs(err, outerErr)

	var count int
	err = s.pool.QueryRow(s.ctx, "SELECT COUNT(*) FROM accounts_pgx").Scan(&count)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *PgxSuite) TestExecRoutedThroughTx() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.Exec(ctx, "INSERT INTO accounts_pgx VALUES ('A', 100)")
		s.Require().NoError(execErr)

		var inside int
		insideErr := s.exec.QueryRow(ctx, "SELECT balance FROM accounts_pgx WHERE id='A'").Scan(&inside)
		s.Require().NoError(insideErr)
		s.Equal(100, inside)

		var outside int
		outsideErr := s.pool.QueryRow(s.ctx, "SELECT COUNT(*) FROM accounts_pgx WHERE id='A'").Scan(&outside)
		s.Require().NoError(outsideErr)
		s.Equal(0, outside)
		return nil
	})
	s.Require().NoError(err)
}

func (s *PgxSuite) TestIsolationOption() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		var iso string
		scanErr := s.exec.QueryRow(ctx, "SHOW transaction_isolation").Scan(&iso)
		s.Require().NoError(scanErr)
		s.Equal("serializable", iso)
		return nil
	}, dbtx.WithIsolationLevel(pgx.Serializable))
	s.Require().NoError(err)
}
