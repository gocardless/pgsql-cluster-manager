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
			"located at --etcd-postgres-master-etcd-key",
		RunE: func(_ *cobra.Command, _ []string) error {
			proxy := &ProxyOptions{
				etcd.StreamOptions{
					Ctx:          ctx,
					GetTimeout:   viper.GetDuration("etcd-get-timeout"),
					PollInterval: viper.GetDuration("etcd-stream-poll-interval"),
					Keys: []string{
						viper.GetString("etcd-postgres-master-key"),
					},
				},
				streams.RetryFoldOptions{
					Ctx:      ctx,
					Interval: viper.GetDuration("pgbouncer-retry-timeout"),
					Timeout:  viper.GetDuration("pgbouncer-timeout"),
				},
			}

			return proxy.Run(ctx, mustEtcdClient(), mustPgBouncer())
		},
	}

	addPgBouncerFlags(c.Flags())

	c.Flags().Duration("etcd-stream-poll-interval", time.Minute, "poll etcd on this interval")
	c.Flags().Duration("etcd-get-timeout", 5*time.Second, "timeout for etcd get operation")
	c.Flags().Duration("pgbouncer-retry-interval", 5*time.Second, "retry failed PgBouncer operations at this interval")

	viper.BindPFlags(c.Flags())

	return c
}

type ProxyOptions struct {
	etcd.StreamOptions
	streams.RetryFoldOptions
}

func (opt *ProxyOptions) Run(ctx context.Context, client *clientv3.Client, pgBouncer *pgbouncer.PgBouncer) (err error) {
	kvs, _ := etcd.NewStream(logger, client, opt.StreamOptions)

	// etcd provides events out-of-order, and potentially duplicated. We need to use the
	// RevisionFilter to ensure we only fold our events in their logical order, without
	// duplicates.
	kvs = streams.RevisionFilter(logger, kvs)

	err = streams.RetryFold(
		logger, kvs, opt.RetryFoldOptions,
		func(ctx context.Context, kv *mvccpb.KeyValue) error {
			logger.Log("event", "pgbouncer.reload_configuration", "host", string(kv.Value))
			if err := pgBouncer.GenerateConfig(string(kv.Value)); err != nil {
				return err
			}

			return pgBouncer.Reload(ctx)
		},
	)

	if err != nil {
		logger.Log("event", "proxy.error", "error", err.Error(),
			"msg", "proxy failed, exiting with error")
	}

	logger.Log("event", "proxy.shutdown")
	return err
}
