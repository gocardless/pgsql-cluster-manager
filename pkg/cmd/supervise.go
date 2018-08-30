package cmd

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"

	"github.com/coreos/etcd/clientv3"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/etcd"
	"github.com/gocardless/pgsql-cluster-manager/pkg/failover"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pacemaker"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewSuperviseCommand(ctx context.Context) *cobra.Command {
	c := &cobra.Command{
		Use:   "supervise",
		Short: "Supervise a cluster member",
		Long:  "Sync pacemaker state to etcd and expose a failover API",
		RunE: func(_ *cobra.Command, _ []string) error {
			supervise := &SuperviseCommand{
				client:         mustEtcdClient(),
				pgBouncer:      mustPgBouncer(),
				crm:            pacemaker.NewPacemaker(),
				etcdHostKey:    viper.GetString("etcd-postgres-master-key"),
				etcdTimeout:    viper.GetDuration("etcd-timeout"),
				masterCrmXPath: viper.GetString("postgres-master-crm-xpath"),
				bindAddress:    viper.GetString("bind-address"),
			}

			return supervise.Run(ctx)
		},
	}

	c.Flags().String("postgres-master-crm-xpath", pacemaker.MasterXPath, "XPath selector into cibadmin that finds current master")
	c.Flags().String("bind-address", ":8080", "Bind API to this address")

	viper.BindPFlags(c.Flags())

	return c
}

type SuperviseCommand struct {
	client         *clientv3.Client
	pgBouncer      *pgbouncer.PgBouncer
	crm            *pacemaker.Pacemaker
	etcdHostKey    string
	etcdTimeout    time.Duration
	masterCrmXPath string
	bindAddress    string
}

func (c *SuperviseCommand) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	var g run.Group

	{
		// Watch for changes to master node, calling the handler registered on the host key
		crmSub := pacemaker.NewSubscriber(
			pacemaker.WatchNode(c.etcdHostKey, c.masterCrmXPath, "id"),
			pacemaker.WithTransform(pacemaker.NewPacemaker().ResolveAddress),
			pacemaker.WithLogger(kitlog.With(
				logger, "role", "supervise", "component", "pacemaker.subscriber"),
			),
		)

		// We should only update the key if it's changed- Updater provides idempotent updates
		crmSub.AddHandler(c.etcdHostKey, &etcd.Updater{c.client, c.etcdTimeout})

		g.Add(
			func() error { crmSub.Start(ctx); return nil },
			func(error) { cancel() },
		)
	}

	{
		listen, err := net.Listen("tcp", c.bindAddress)
		if err != nil {
			return errors.Wrap(err, "failed to bind to address")
		}

		server := failover.NewServer(logger, c.pgBouncer, c.crm)
		grpcServer := grpc.NewServer()
		failover.RegisterFailoverServer(grpcServer, server)

		g.Add(
			func() error {
				logger.Log("event", "supervise.server.listen", "address", c.bindAddress)
				return grpcServer.Serve(listen)
			},
			func(err error) {
				logger.Log("event", "supervise.server.shutdown", "error", err)
				grpcServer.GracefulStop()
			},
		)
	}

	if err := g.Run(); err != nil {
		logger.Log("event", "supervise.finish", "error", err)
		return err
	}

	return nil
}
