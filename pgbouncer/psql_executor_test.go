package pgbouncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPGBouncerExecutor_connectionString(t *testing.T) {
	executor := pgbouncerExecutor{
		PGBouncer: pgBouncer{
			ConfigFile:         "/etc/pgbouncer/pgbouncer.ini",
			ConfigFileTemplate: "./fixtures/pgbouncer.ini.template",
		},
	}

	connStr, err := executor.connectionString()

	if ok := assert.Nil(t, err, "expected no error"); !ok {
		return
	}

	assert.Contains(t, connStr, "port=6432")
	assert.Contains(t, connStr, "host=/var/run/postgresql")
}
