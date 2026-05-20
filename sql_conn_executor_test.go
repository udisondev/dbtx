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

type SQLConnSuite struct {
	suite.Suite
	ctx  context.Context
	db   *sql.DB
	conn *sql.Conn
	exec *dbtx.SQLConnExecutor
}

func TestSQLConnSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(SQLConnSuite))
}

func (s *SQLConnSuite) SetupSuite() {
	s.ctx = context.Background()
	dsn := getSharedDSN(s.T())
	db, err := sql.Open("pgx", dsn)
	s.Require().NoError(err)
	s.Require().NoError(db.PingContext(s.ctx))
	conn, err := db.Conn(s.ctx)
	s.Require().NoError(err)
	_, err = conn.ExecContext(s.ctx,
		"CREATE TABLE IF NOT EXISTS accounts_sql_conn (id text primary key, balance int not null)")
	s.Require().NoError(err)
	s.db = db
	s.conn = conn
	s.exec = dbtx.NewSQLConnExecutor(conn)
}

func (s *SQLConnSuite) TearDownSuite() {
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.db != nil {
		s.db.Close()
	}
}

func (s *SQLConnSuite) SetupTest() {
	_, err := s.conn.ExecContext(s.ctx, "TRUNCATE accounts_sql_conn")
	s.Require().NoError(err)
}

func (s *SQLConnSuite) TestInTxCommit() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql_conn VALUES ('A', 100)")
		return execErr
	})
	s.Require().NoError(err)

	var balance int
	err = s.conn.QueryRowContext(s.ctx, "SELECT balance FROM accounts_sql_conn WHERE id='A'").Scan(&balance)
	s.Require().NoError(err)
	s.Equal(100, balance)
}

func (s *SQLConnSuite) TestInTxRollbackOnError() {
	sentinel := errors.New("boom")
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql_conn VALUES ('A', 100)")
		s.Require().NoError(execErr)
		return sentinel
	})
	s.Require().ErrorIs(err, sentinel)

	var count int
	err = s.conn.QueryRowContext(s.ctx, "SELECT COUNT(*) FROM accounts_sql_conn").Scan(&count)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *SQLConnSuite) TestExecRoutedThroughTx() {
	err := s.exec.InTx(s.ctx, func(ctx context.Context) error {
		_, execErr := s.exec.ExecContext(ctx, "INSERT INTO accounts_sql_conn VALUES ('A', 100)")
		s.Require().NoError(execErr)

		var inside int
		insideErr := s.exec.QueryRowContext(ctx, "SELECT balance FROM accounts_sql_conn WHERE id='A'").Scan(&inside)
		s.Require().NoError(insideErr)
		s.Equal(100, inside)
		return nil
	})
	s.Require().NoError(err)
}

func (s *SQLConnSuite) TestWithTx_TxMatchesCtx() {
	err := s.exec.WithTx(s.ctx, func(ctx context.Context, tx *sql.Tx) error {
		fromCtx, ok := dbtx.SQLFromCtx(ctx)
		s.Require().True(ok)
		s.Same(tx, fromCtx)
		_, execErr := tx.ExecContext(ctx, "INSERT INTO accounts_sql_conn VALUES ('A', 100)")
		return execErr
	})
	s.Require().NoError(err)

	var balance int
	err = s.conn.QueryRowContext(s.ctx, "SELECT balance FROM accounts_sql_conn WHERE id='A'").Scan(&balance)
	s.Require().NoError(err)
	s.Equal(100, balance)
}
