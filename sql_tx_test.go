package dbtx_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/suite"
	"github.com/udisondev/dbtx"
)

type SQLSuite struct {
	suite.Suite
	ctx  context.Context
	db   *sql.DB
	exec *dbtx.SQLDBExecutor
}

func TestSQLSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(SQLSuite))
}

func (s *SQLSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := getSharedDSN(s.T())
	db, err := sql.Open("pgx", dsn)
	s.Require().NoError(err)
	s.Require().NoError(db.PingContext(s.ctx))
	_, err = db.ExecContext(s.ctx,
		"CREATE TABLE IF NOT EXISTS accounts_sql (id text primary key, balance int not null)")
	s.Require().NoError(err)
	s.db = db
	s.exec = dbtx.NewSQLDBExecutor(db)
}

func (s *SQLSuite) TearDownSuite() {
	if s.db != nil {
		s.db.Close()
	}
}

func (s *SQLSuite) SetupTest() {
	_, err := s.db.ExecContext(s.ctx, "TRUNCATE accounts_sql")
	s.Require().NoError(err)
}

func (s *SQLSuite) TestInTxCommit() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql VALUES ('A', 100)")
		return execErr
	})
	s.Require().NoError(err)

	var balance int
	err = s.db.QueryRowContext(s.ctx, "SELECT balance FROM accounts_sql WHERE id='A'").Scan(&balance)
	s.Require().NoError(err)
	s.Equal(100, balance)
}

func (s *SQLSuite) TestInTxRollbackOnError() {
	sentinel := errors.New("boom")
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql VALUES ('A', 100)")
		s.Require().NoError(execErr)
		return sentinel
	})
	s.Require().ErrorIs(err, sentinel)

	var count int
	err = s.db.QueryRowContext(s.ctx, "SELECT COUNT(*) FROM accounts_sql").Scan(&count)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *SQLSuite) TestNestedInTx_ReusesSameTx() {
	var outerTx *sql.Tx
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		var ok bool
		outerTx, ok = dbtx.SQLFromCtx(ctx)
		s.Require().True(ok)
		_, execErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql VALUES ('A', 100)")
		s.Require().NoError(execErr)

		return s.exec.InTx(ctx, func(ctx context.Context) error {
			innerTx, ok := dbtx.SQLFromCtx(ctx)
			s.Require().True(ok)
			s.Same(outerTx, innerTx)
			_, innerExecErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql VALUES ('B', 200)")
			return innerExecErr
		})
	})
	s.Require().NoError(err)

	var count int
	err = s.db.QueryRowContext(s.ctx, "SELECT COUNT(*) FROM accounts_sql").Scan(&count)
	s.Require().NoError(err)
	s.Equal(2, count)
}

func (s *SQLSuite) TestNestedInTx_InnerErrorAbortsAll() {
	innerErr := errors.New("inner")
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql VALUES ('A', 100)")
		s.Require().NoError(execErr)
		return s.exec.InTx(ctx, func(ctx context.Context) error {
			_, innerExecErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql VALUES ('B', 200)")
			s.Require().NoError(innerExecErr)
			return innerErr
		})
	})
	s.Require().ErrorIs(err, innerErr)

	var count int
	err = s.db.QueryRowContext(s.ctx, "SELECT COUNT(*) FROM accounts_sql").Scan(&count)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *SQLSuite) TestExecRoutedThroughTx() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql VALUES ('A', 100)")
		s.Require().NoError(execErr)

		var inside int
		insideErr := s.exec.QueryRowContext(ctx, "SELECT balance FROM accounts_sql WHERE id='A'").Scan(&inside)
		s.Require().NoError(insideErr)
		s.Equal(100, inside)

		var outside int
		outsideErr := s.db.QueryRowContext(s.ctx, "SELECT COUNT(*) FROM accounts_sql WHERE id='A'").Scan(&outside)
		s.Require().NoError(outsideErr)
		s.Equal(0, outside)
		return nil
	})
	s.Require().NoError(err)
}

func (s *SQLSuite) TestIsolationOption() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		var iso string
		scanErr := s.exec.QueryRowContext(ctx, "SHOW transaction_isolation").Scan(&iso)
		s.Require().NoError(scanErr)
		s.Equal("serializable", iso)
		return nil
	}, dbtx.SQLWithIsolationLevel(sql.LevelSerializable))
	s.Require().NoError(err)
}
