package pgbouncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPGBouncerExecutor_psqlOptions(t *testing.T) {
	executor := pgbouncerExecutor{
		PGBouncer: pgBouncer{
			ConfigFile:         "/etc/pgbouncer/pgbouncer.ini",
			ConfigFileTemplate: "./fixtures/pgbouncer.ini.template",
		},
	}

	options, err := executor.psqlOptions()

	assert.Nil(t, err, "expected no error")
	assert.Equal(t, options.Addr, "/var/run/postgresql/.s.PGSQL.6432")
}
