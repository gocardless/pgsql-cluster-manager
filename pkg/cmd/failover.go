package cmd

import (
	"context"
	"time"

	"google.golang.org/grpc"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/failover"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var failoverLongDescription = `
Talk to the migration API- hosted on the Postgres nodes- in order to
cause the Postgres primary to be failoverd to the current sync node.

This migration is performed by first pausing PGBouncer, waiting until
all queries have finished, then performing a pacemaker resource
migration. Once the migration is complete, we issue a PGBouncer resume,
restoring traffic to the cluster.

# pause-timeout

This timeout bounds the amount of time we'll delay query execution, and
directly affects every API request that touches the database. This
timeout is critical as the moment we attempt to pause PGBouncer, we'll
queue new queries.

# pause-expiry

This expiry should be used as a safety belt. It's never the case that we
want PGBouncer to remain paused indefinitely, as this means we're down.
When a pause request is made with an expiry the server will schedule a
resume command to be run n seconds in the future, ensuring that we
resume traffic regardless of client failure.

# pacemaker-timeout

Timeout on API requests that hit endpoints that will execute pacemaker
commands. This timeout DOES NOT apply to the affect being seen from
executing such a command- as an example, a failover command may take
anywhere up to 20s to complete but applying the failover constraint may
succeed instantly. This timeout applies to the latter only.
`

func NewFailoverCommand(ctx context.Context) *cobra.Command {
	c := &cobra.Command{
		Use:   "failover",
		Short: "Run a zero-downtime failover of the Postgres primary",
		Long:  failoverLongDescription,
		RunE: func(_ *cobra.Command, _ []string) error {
			failover := &failoverCommand{
				client:             mustEtcdClient(),
				endpoints:          viper.GetStringSlice("failover-api-endpoints"),
				etcdHostKey:        viper.GetString("etcd-postgres-master-key"),
				healthCheckTimeout: viper.GetDuration("health-check-timeout"),
				lockTimeout:        viper.GetDuration("lock-timeout"),
				pauseTimeout:       viper.GetDuration("pause-timeout"),
				pauseExpiry:        viper.GetDuration("pause-expiry"),
				pacemakerTimeout:   viper.GetDuration("pacemaker-timeout"),
			}

			return failover.Run(ctx, logger)
		},
	}

	addFailoverFlags(c.Flags())
	viper.BindPFlags(c.Flags())

	return c
}

type failoverCommand struct {
	client             *clientv3.Client
	endpoints          []string
	etcdHostKey        string
	healthCheckTimeout time.Duration
	lockTimeout        time.Duration
	pauseTimeout       time.Duration
	pauseExpiry        time.Duration
	pacemakerTimeout   time.Duration
}

func (f *failoverCommand) Run(ctx context.Context, logger kitlog.Logger) error {
	session, err := concurrency.NewSession(f.client)
	if err != nil {
		return err
	}

	locker := concurrency.NewMutex(session, f.etcdHostKey)

	clients := make([]failover.FailoverClient, len(f.endpoints))
	for idx, endpoint := range f.endpoints {
		logger.Log("event", "client.connecting", "endpoint", endpoint)
		conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
		if err != nil {
			return errors.Wrapf(err, "failed to connect to endpoint %s", endpoint)
		}

		clients[idx] = failover.NewFailoverClient(conn)
	}

	process := &failover.Failover{
		Logger:             logger,
		Clients:            clients,
		Locker:             locker,
		Etcd:               f.client,
		EtcdHostKey:        f.etcdHostKey,
		HealthCheckTimeout: f.healthCheckTimeout,
		LockTimeout:        f.lockTimeout,
		PauseTimeout:       f.pauseTimeout,
		PauseExpiry:        f.pauseExpiry,
		PacemakerTimeout:   f.pacemakerTimeout,
	}

	err = process.Run(ctx)

	if err != nil {
		logger.Log("event", "failover.error", "error", err,
			"msg", "failed to run migration, PgBouncers have been resumed")
	}

	return err
}
