package command

import (
	"context"
	"time"

	"google.golang.org/grpc"

	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/gocardless/pgsql-cluster-manager/migration"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var migrateLongDescription = `
Talk to the migration API- hosted on the Postgres nodes- in order to
cause the Postgres primary to be migrated to the current sync node.

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
executing such a command- as an example, a migrate command may take
anywhere up to 20s to complete but applying the migrate constraint may
succeed instantly. This timeout applies to the latter only.
`

func NewMigrateCommand() *cobra.Command {
	cm := &cobra.Command{
		Use:   "migrate",
		Short: "Run a zero-downtime migration of Postgres primary",
		Long:  migrateLongDescription,
		Run:   migrateCommandFunc,
	}

	flags := cm.Flags()

	flags.StringSlice("migration-api-endpoints", []string{"pg01:8080", "pg02:8080", "pg03:8080"}, "All Postgres node API endpoints")
	flags.Duration("health-check-timeout", 2*time.Second, "Timeout to health check each node")
	flags.Duration("lock-timeout", 5*time.Second, "Timeout to acquire exclusive migration lock in etcd")
	flags.Duration("pause-timeout", 5*time.Second, "Timeout for all nodes to pause PGBouncer")
	flags.Duration("pause-expiry", 25*time.Second, "Time after which PGBouncer will automatically lift pause")
	flags.Duration("pacemaker-timeout", 20*time.Second, "Timeout for executing (not necessarily to completion) pacemaker commands")

	viper.BindPFlag("migration-api-endpoints", flags.Lookup("migration-api-endpoints"))
	viper.BindPFlag("health-check-timeout", flags.Lookup("health-check-timeout"))
	viper.BindPFlag("lock-timeout", flags.Lookup("lock-timeout"))
	viper.BindPFlag("pause-timeout", flags.Lookup("pause-timeout"))
	viper.BindPFlag("pause-expiry", flags.Lookup("pause-expiry"))
	viper.BindPFlag("pacemaker-timeout", flags.Lookup("pacemaker-timeout"))

	return cm
}

func migrateCommandFunc(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	HandleQuitSignal("cleaning context and exiting...", cancel)

	client := EtcdClientOrExit()
	session := EtcdSessionOrExit(client)
	etcdMasterKey := viper.GetString("postgres-master-etcd-key")
	locker := concurrency.NewMutex(session, etcdMasterKey)

	endpoints := viper.GetStringSlice("migration-api-endpoints")
	clients := make([]migration.MigrationClient, len(endpoints))

	for idx, endpoint := range endpoints {
		conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
		if err != nil {
			logger.WithField("endpoint", endpoint).Fatal("Failed to connect")
		}

		clients[idx] = migration.NewMigrationClient(conn)
	}

	cfg := migration.MigrationConfig{
		Logger:             logger,
		Etcd:               client,
		EtcdMasterKey:      etcdMasterKey,
		Clients:            clients,
		Locker:             locker,
		HealthCheckTimeout: viper.GetDuration("health-check-timeout"),
		LockTimeout:        viper.GetDuration("lock-timeout"),
		PauseTimeout:       viper.GetDuration("pause-timeout"),
		PauseExpiry:        viper.GetDuration("pause-expiry"),
		PacemakerTimeout:   viper.GetDuration("pacemaker-timeout"),
	}

	if err := migration.NewMigration(cfg).Run(ctx); err != nil {
		logger.WithError(err).Fatal("Failed to run migration, PGBouncers have been resumed")
	}
}
