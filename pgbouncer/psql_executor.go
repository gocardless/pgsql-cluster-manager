package pgbouncer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
)

// PsqlExecutor implements the execution of SQL queries against a Postgres connection
type PsqlExecutor interface {
	Exec(interface{}, ...interface{}) (orm.Result, error)
}

type pgbouncerExecutor struct {
	PGBouncer
	timeout time.Duration
}

func NewPGBouncerExecutor(bouncer PGBouncer, timeout time.Duration) PsqlExecutor {
	return &pgbouncerExecutor{bouncer, timeout}
}

// Exec generates a connection to PGBouncer's Postgres database and executes the given
// command
func (e pgbouncerExecutor) Exec(query interface{}, params ...interface{}) (orm.Result, error) {
	psqlOptions, err := e.psqlOptions()

	if err != nil {
		return nil, err
	}

	return pg.Connect(psqlOptions).WithTimeout(e.timeout).Exec(query, params)
}

func (e pgbouncerExecutor) psqlOptions() (*pg.Options, error) {
	var nullString string
	var config map[string]string

	config, err := e.Config()

	if err != nil {
		return nil, err
	}

	socketDir := config["unix_socket_dir"]
	portStr := config["listen_port"]
	port, _ := strconv.Atoi(strings.TrimSpace(portStr))

	if socketDir == nullString || portStr == nullString {
		return nil, errorWithFields{
			errors.New("Failed to parse required config from PGBouncer config template"),
			map[string]interface{}{
				"socketDir": socketDir,
				"portStr":   portStr,
				"port":      port,
			},
		}
	}

	return &pg.Options{
		Network:     "unix",
		User:        "pgbouncer",
		Database:    "pgbouncer",
		Addr:        fmt.Sprintf("%s/.s.PGSQL.%d", socketDir, port),
		ReadTimeout: time.Second,
	}, nil
}
