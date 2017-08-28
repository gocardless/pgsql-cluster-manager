package handlers

import (
	"github.com/gocardless/pgsql-novips/pgbouncer"
)

// PGBouncerHostChange manages PGBouncer config
type PGBouncerHostChange struct {
	pgbouncer.PGBouncer
}

// Run receives new PGBouncer host values and will reload PGBouncer to point at the new
// host
func (h PGBouncerHostChange) Run(_, host string) error {
	return collectError(
		func() error { return h.GenerateConfig(host) },
		func() error { return h.Reload() },
	)
}
