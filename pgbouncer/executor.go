package pgbouncer

import (
	"context"
	"database/sql"
	"fmt"
)

type executor interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

type AuthorizedExecutor struct {
	User, Password, Database, SocketDir, Port string
}

func (e AuthorizedExecutor) QueryContext(ctx context.Context, query string, params ...interface{}) (*sql.Rows, error) {
	conn, err := e.connection()

	if err != nil {
		return nil, err
	}

	return conn.QueryContext(ctx, query, params...)
}

func (e AuthorizedExecutor) ExecContext(ctx context.Context, query string, params ...interface{}) (sql.Result, error) {
	conn, err := e.connection()

	if err != nil {
		return nil, err
	}

	return conn.ExecContext(ctx, query, params...)
}

func (e AuthorizedExecutor) connection() (*sql.DB, error) {
	connStr := fmt.Sprintf(
		"user=%s dbname=%s host=%s port=%s connect_timeout=1",
		e.User,
		e.Database,
		e.SocketDir,
		e.Port,
	)

	if e.Password != "" {
		connStr = fmt.Sprintf("%s password=%s", connStr, e.Password)
	}

	return sql.Open("postgres", connStr)
}
