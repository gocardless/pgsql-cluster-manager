package pgbouncer

import (
	"context"
	"time"
)

type reloader interface {
	GenerateConfig(string) error
	Reload(context.Context) error
}

type HostChanger struct {
	PgBouncer reloader
	Timeout   time.Duration
}

// Run receives new PgBouncer host values and will reload PgBouncer to point at the new
// host
func (h HostChanger) Run(_, host string) error {
	err := h.PgBouncer.GenerateConfig(host)

	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.Timeout)
	defer cancel()

	return h.PgBouncer.Reload(ctx)
}
