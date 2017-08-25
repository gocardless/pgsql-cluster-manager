package pgbouncer

import (
	"time"

	"github.com/gocardless/pgsql-novips/util"
)

type fieldError interface {
	Field(byte) string
}

// Pause causes PGBouncer to buffer incoming queries while waiting for those currently
// processing to finish executing. The supplied timeout is applied to the Postgres
// connection.
func Pause(b PGBouncer, timeout time.Duration) error {
	psql, err := b.Psql(timeout)
	if _, err = psql.Exec(`PAUSE;`); err == nil {
		return nil
	}

	if ferr, ok := err.(fieldError); ok {
		// We get this when PGBouncer tells us we're already paused
		if ferr.Field('C') == "08P01" {
			return nil // ignore the error, as the pause was not required
		}
	}

	return util.NewErrorWithFields(
		"failed to pause PGBouncer",
		&map[string]interface{}{
			"error": err.Error(),
		},
	)
}
