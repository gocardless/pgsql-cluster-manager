package pgbouncer

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/jackc/pgx"
	"github.com/jackc/pgx/pgtype"
	"github.com/jackc/pgx/stdlib"
	"github.com/pkg/errors"
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
	port, err := strconv.Atoi(e.Port)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse valid port number")
	}

	driverConfig := stdlib.DriverConfig{
		ConnConfig: pgx.ConnConfig{
			Database:      e.Database,
			User:          e.User,
			Password:      e.Password,
			Host:          e.SocketDir,
			Port:          uint16(port),
			RuntimeParams: map[string]string{"client_encoding": "UTF8"},
			// We need to use SimpleProtocol in order to communicate with PgBouncer
			PreferSimpleProtocol: true,
			CustomConnInfo: func(_ *pgx.Conn) (*pgtype.ConnInfo, error) {
				connInfo := pgtype.NewConnInfo()
				connInfo.InitializeDataTypes(map[string]pgtype.OID{
					"int4":    pgtype.Int4OID,
					"name":    pgtype.NameOID,
					"oid":     pgtype.OIDOID,
					"text":    pgtype.TextOID,
					"varchar": pgtype.VarcharOID,
				})

				return connInfo, nil
			},
		},
	}

	stdlib.RegisterDriverConfig(&driverConfig)

	return sql.Open("pgx", driverConfig.ConnectionString("connect_timeout=1"))
}
