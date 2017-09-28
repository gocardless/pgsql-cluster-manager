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
Talk to the migration API- hosted on the Postgres nodes- in order to cause the
Postgres primary to be migrated to the current sync node.

This migration is performed by first pausing PGBouncer, waiting until all
queries have finished, then performing a pacemaker resource migration. Once the
migration is complete, we issue a PGBouncer resume, restoring traffic to the
cluster.
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
	flags.Duration("pause-timeout", 5*time.Second, "Timeout for all nodes to pause PGBouncer")
	flags.Duration("pause-expiry", 25*time.Second, "Time after which PGBouncer will automatically lift pause")
	flags.String("postgres-master-etcd-key", "/master", "etcd key that stores current Postgres primary")

	viper.BindPFlag("migration-api-endpoints", flags.Lookup("migration-api-endpoints"))
	viper.BindPFlag("pause-timeout", flags.Lookup("pause-timeout"))
	viper.BindPFlag("pause-expiry", flags.Lookup("pause-expiry"))
	viper.BindPFlag("postgres-master-etcd-key", flags.Lookup("postgres-master-etcd-key"))

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
		Logger:        logger,
		Etcd:          client,
		EtcdMasterKey: etcdMasterKey,
		Clients:       clients,
		Locker:        locker,
		PauseTimeout:  viper.GetDuration("pause-timeout"),
		PauseExpiry:   viper.GetDuration("pause-expiry"),
	}

	if err := migration.NewMigration(cfg).Run(ctx); err != nil {
		logger.WithError(err).Error("Migration failed")
	}
}
