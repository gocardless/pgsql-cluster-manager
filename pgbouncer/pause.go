package pgbouncer

import (
	"time"

	"github.com/gocardless/pgsql-novips/util"
)

type fieldError interface {
	Field(byte) string
}

// AlreadyPausedError is the field returned as the error code when PGBouncer is already
// paused, and you issue a PAUSE;
const AlreadyPausedError string = "08P01"

// Pause causes PGBouncer to buffer incoming queries while waiting for those currently
// processing to finish executing. The supplied timeout is applied to the Postgres
// connection.
func Pause(b PGBouncer, timeout time.Duration) error {
	psql, err := b.Psql(timeout)

	if err != nil {
		return err
	}

	_, err = psql.Exec(`PAUSE;`)

	if err == nil {
		return nil
	}

	if ferr, ok := err.(fieldError); ok {
		// We get this when PGBouncer tells us we're already paused
		if ferr.Field('C') == AlreadyPausedError {
			return nil // ignore the error, as the pause was not required
		}
	}

	return util.NewErrorWithFields(
		"Failed to pause PGBouncer",
		map[string]interface{}{
			"error": err.Error(),
		},
	)
}
