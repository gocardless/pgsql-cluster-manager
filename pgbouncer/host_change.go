package pgbouncer

import (
	"github.com/gocardless/pgsql-novips/monad"
)

// PGBouncerHostChange manages PGBouncer config
type HostChange struct {
	PGBouncer
}

// Run receives new PGBouncer host values and will reload PGBouncer to point at the new
// host
func (h HostChange) Run(_, host string) error {
	return monad.CollectError(
		func() error { return h.GenerateConfig(host) },
		func() error { return h.Reload() },
	)
}
