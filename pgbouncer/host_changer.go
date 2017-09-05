package pgbouncer

import (
	"github.com/gocardless/pgsql-cluster-manager/monad"
)

type HostChanger struct {
	PGBouncer
}

// Run receives new PGBouncer host values and will reload PGBouncer to point at the new
// host
func (h HostChanger) Run(_, host string) error {
	return monad.CollectError(
		func() error { return h.GenerateConfig(host) },
		func() error { return h.Reload() },
	)
}
