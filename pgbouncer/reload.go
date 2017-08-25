package pgbouncer

import (
	"time"

	"github.com/gocardless/pgsql-novips/util"
)

// Reload will cause PGBouncer to reload configuration and live apply setting changes
func Reload(b PGBouncer, timeout time.Duration) error {
	psql, err := b.Psql(timeout)

	if err != nil {
		return err
	}

	_, err = psql.Exec(`RELOAD;`)

	if err != nil {
		return util.NewErrorWithFields(
			"failed to reload PGBouncer",
			map[string]interface{}{
				"error": err.Error(),
			},
		)
	}

	return nil
}
