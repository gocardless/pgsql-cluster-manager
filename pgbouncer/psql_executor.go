package pgbouncer

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/pkg/errors"
)

// PsqlExecutor implements the execution of SQL queries against a Postgres connection
type PsqlExecutor interface {
	Query(string, ...interface{}) (*sql.Rows, error)
}

type pgbouncerExecutor struct {
	PGBouncer
	timeout time.Duration
}

func NewPGBouncerExecutor(bouncer PGBouncer, timeout time.Duration) PsqlExecutor {
	return &pgbouncerExecutor{bouncer, timeout}
}

// Query generates a connection to PGBouncer's Postgres database and executes the given
// command
func (e pgbouncerExecutor) Query(query string, params ...interface{}) (*sql.Rows, error) {
	psql, err := e.psql()

	if err != nil {
		return nil, err
	}

	return psql.Query(query, params...)
}

func (e pgbouncerExecutor) psql() (*sql.DB, error) {
	connStr, err := e.connectionString()

	if err != nil {
		return nil, err
	}

	return sql.Open("postgres", connStr)
}

func (e pgbouncerExecutor) connectionString() (string, error) {
	var nullString string
	var config map[string]string

	config, err := e.Config()

	if err != nil {
		return nullString, err
	}

	socketDir := config["unix_socket_dir"]
	port := config["listen_port"]

	if socketDir == nullString || port == nullString {
		return nullString, errors.Errorf(
			"failed to parse config for PGBouncer: socketDir=%s, port=%s",
			socketDir, port,
		)
	}

	return fmt.Sprintf(
		"user=pgbouncer dbname=pgbouncer connect_timeout=1 host=%s port=%s",
		socketDir,
		port,
	), err
}
