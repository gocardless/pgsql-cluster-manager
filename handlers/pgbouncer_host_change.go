package handlers

import (
	"time"

	"github.com/gocardless/pgsql-novips/pgbouncer"
)

// PGBouncerHostChange manages PGBouncer config
type PGBouncerHostChange struct {
	Timeout time.Duration
	pgbouncer.PGBouncer
}

// Run receives new PGBouncer host values and will reload PGBouncer to point at the new
// host
func (h PGBouncerHostChange) Run(_, host string) error {
	err := h.PGBouncer.GenerateConfig(host)

	if err != nil {
		return err
	}

	return pgbouncer.Pause(h.PGBouncer, h.Timeout)
}
