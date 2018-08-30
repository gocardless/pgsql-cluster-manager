package cmd

import (
	"context"
	"time"

	"github.com/coreos/etcd/clientv3"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/etcd"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer"
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

func (p *proxyCommand) Run(ctx context.Context) error {
	etcd.NewSubscriber(
		p.client, etcd.WithLogger(
			kitlog.With(logger, "role", "proxy"),
		)).
		AddHandler(
			p.etcdHostKey,
			&pgbouncer.HostChanger{
				PgBouncer: p.pgBouncer,
				Timeout:   p.timeout,
			}).
		Start(ctx)

	return nil
}
