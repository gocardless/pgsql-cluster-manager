package proxy

import (
	"time"

	"github.com/gocardless/pgsql-novips/pgbouncer"
	"github.com/gocardless/pgsql-novips/subscriber"
)

type ProxyConfig struct {
	PGBouncerHostKey        string
	PGBouncerConfig         string
	PGBouncerConfigTemplate string
	PGBouncerTimeout        time.Duration
}

func New(sub subscriber.Subscriber, cfg ProxyConfig) subscriber.Subscriber {
	sub.RegisterHandler(
		cfg.PGBouncerHostKey,
		pgbouncer.HostChange{
			PGBouncer: pgbouncer.NewPGBouncer(
				cfg.PGBouncerConfig,
				cfg.PGBouncerConfigTemplate,
				cfg.PGBouncerTimeout,
			),
		},
	)

	return sub
}
