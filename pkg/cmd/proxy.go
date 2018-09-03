package cmd

import (
	"context"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/gocardless/pgsql-cluster-manager/pkg/etcd"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer"
	"github.com/gocardless/pgsql-cluster-manager/pkg/streams"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewProxyCommand(ctx context.Context) *cobra.Command {
	c := &cobra.Command{
		Use:   "proxy",
		Short: "Manages PgBouncer proxy",
		Long: "Controls a PgBouncer by templating its config file to point pools at the host " +
			"located at --postgres-master-etcd-key",
		RunE: func(_ *cobra.Command, _ []string) error {
			proxy := &proxyCommand{
				client:      mustEtcdClient(),
				pgBouncer:   mustPgBouncer(),
				etcdHostKey: viper.GetString("etcd-postgres-master-key"),
				timeout:     viper.GetDuration("pgbouncer-timeout"),
			}

			return proxy.Run(ctx)
		},
	}

	addPgBouncerFlags(c.Flags())
	viper.BindPFlags(c.Flags())

	return c
}

type proxyCommand struct {
	client      *clientv3.Client
	pgBouncer   *pgbouncer.PgBouncer
	etcdHostKey string
	timeout     time.Duration
}

func (p *proxyCommand) Run(ctx context.Context) (err error) {
	kvs, _ := etcd.NewStream(
		logger,
		p.client,
		etcd.StreamOptions{
			Ctx:          ctx,
			Keys:         []string{p.etcdHostKey},
			PollInterval: 5 * time.Second,
			GetTimeout:   5 * time.Second,
		},
	)

	// etcd provides events out-of-order, and potentially duplicated. We need to use the
	// RevisionFilter to ensure we only fold our events in their logical order, without
	// duplicates.
	kvs = streams.RevisionFilter(logger, kvs)

	opt := streams.RetryFoldOptions{
		Ctx:      ctx,
		Interval: time.Second,
		Timeout:  5 * time.Second,
	}

	err = streams.RetryFold(
		logger, kvs, opt,
		func(ctx context.Context, kv *mvccpb.KeyValue) error {
			logger.Log("event", "pgbouncer.reload_configuration", "host", string(kv.Value))
			if err := p.pgBouncer.GenerateConfig(string(kv.Value)); err != nil {
				return err
			}

			return p.pgBouncer.Reload(ctx)
		},
	)

	if err != nil {
		logger.Log("event", "proxy.error", "error", err.Error(),
			"msg", "proxy failed, exiting with error")
	}

	logger.Log("event", "proxy.shutdown")
	return err
}
