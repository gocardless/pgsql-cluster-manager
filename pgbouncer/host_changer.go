package pgbouncer

import (
	"context"
	"time"
)

type HostChanger struct {
	PGBouncer
	Timeout time.Duration
}

// Run receives new PGBouncer host values and will reload PGBouncer to point at the new
// host
func (h HostChanger) Run(_, host string) error {
	err := h.GenerateConfig(host)

	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.Timeout)
	defer cancel()

	return h.Reload(ctx)
}
