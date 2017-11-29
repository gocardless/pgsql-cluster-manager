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
	PGBouncer reloader
	Timeout   time.Duration
}

// Run receives new PGBouncer host values and will reload PGBouncer to point at the new
// host
func (h HostChanger) Run(_, host string) error {
	err := h.PGBouncer.GenerateConfig(host)

	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.Timeout)
	defer cancel()

	return h.PGBouncer.Reload(ctx)
}
