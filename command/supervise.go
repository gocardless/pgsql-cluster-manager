package command

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"

	"github.com/gocardless/pgsql-cluster-manager/etcd"
	"github.com/gocardless/pgsql-cluster-manager/migration"
	"github.com/gocardless/pgsql-cluster-manager/pacemaker"
	"github.com/gocardless/pgsql-cluster-manager/pgbouncer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewSuperviseCommand() *cobra.Command {
	sc := &cobra.Command{
		Use:   "supervise <subcommand>",
		Short: "Manage components of the Postgres cluster",
	}

	sc.AddCommand(newSuperviseProxyCommand())
	sc.AddCommand(newSuperviseClusterCommand())
	sc.AddCommand(newSuperviseMigrationCommand())

	return sc
}

func newSuperviseProxyCommand() *cobra.Command {
	sp := &cobra.Command{
		Use:   "proxy [options]",
		Short: "Manages PGBouncer proxy",
		Long: "Controls the local PGBouncer instance by managing the config file to point " +
			"PGBouncer at the host located at postgres-master-etcd-key",
		Run: superviseProxyCommandFunc,
	}

	flags := sp.Flags()

	flags.String("pgbouncer-config-file", "/etc/pgbouncer/pgbouncer.ini", "Path to PGBouncer config file")
	flags.String("pgbouncer-config-template-file", "/etc/pgbouncer/pgbouncer.ini.template", "Path to PGBouncer config template file")
	flags.String("postgres-master-etcd-key", "/master", "etcd key that stores current Postgres primary")

	viper.BindPFlag("pgbouncer-config-file", flags.Lookup("pgbouncer-config-file"))
	viper.BindPFlag("pgbouncer-config-template-file", flags.Lookup("pgbouncer-config-template-file"))
	viper.BindPFlag("postgres-master-etcd-key", flags.Lookup("postgres-master-etcd-key"))

	return sp
}

func superviseProxyCommandFunc(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	HandleQuitSignal("cleaning context and exiting...", cancel)

	client := EtcdClientOrExit()
	bouncer := PGBouncerOrExit()

	etcdHostKey := viper.GetString("postgres-master-etcd-key")

	etcd.NewSubscriber(client, etcd.WithLogger(logger)).
		AddHandler(etcdHostKey, &pgbouncer.HostChanger{bouncer, 5 * time.Second}).
		Start(ctx)
}

func newSuperviseClusterCommand() *cobra.Command {
	sc := &cobra.Command{
		Use:   "cluster [options]",
		Short: "Manage pacemaker cluster node",
		Long: "Polls pacemaker for the current master, putting that value to a specified " +
			"etcd key whenever it changes",
		Run: superviseClusterCommandFunc,
	}

	flags := sc.Flags()

	flags.String("postgres-master-etcd-key", "/master", "etcd key that stores current Postgres primary")
	flags.String("postgres-master-crm-xpath", pacemaker.MasterXPath, "XPath selector into cibadmin that finds current master")

	viper.BindPFlag("postgres-master-etcd-key", flags.Lookup("postgres-master-etcd-key"))
	viper.BindPFlag("postgres-master-crm-xpath", flags.Lookup("postgres-master-crm-xpath"))

	return sc
}

func superviseClusterCommandFunc(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	HandleQuitSignal("cleaning context and exiting...", cancel)

	client := EtcdClientOrExit()

	etcdHostKey := viper.GetString("postgres-master-etcd-key")
	masterCrmXPath := viper.GetString("postgres-master-crm-xpath")

	// Watch for changes to master node, calling the handler registered on the host key
	crmSub := pacemaker.NewSubscriber(
		pacemaker.WatchNode(etcdHostKey, masterCrmXPath, "id"),
		pacemaker.WithTransform(pacemaker.NewPacemaker().ResolveAddress),
		pacemaker.WithLogger(logger),
	)

	// We should only update the key if it's changed- Updater provides idempotent updates
	crmSub.AddHandler(etcdHostKey, &etcd.Updater{client})
	crmSub.Start(ctx)
}

var migrationLongDescription = `
Provides an API that issues migration commands, used to perform zero-downtime
migrations. The full API is specified in migration/migration.proto.

service Migration {
	// Verifies communication with service
	rpc health_check(Empty) returns (HealthCheckResponse) {}

	// Causes PGBouncer to pause on the host, cancelling the pause if we exceed
	// timeout and automatically resuming after expiry seconds.
	rpc pause(PauseRequest) returns (PauseResponse) {}

	// Causes PGBouncer to immediately resume, removing any active pauses.
	rpc resume(Empty) returns (ResumeResponse) {}

	// Creates a migration from the current Postgres primary to the eligible sync
	// node. This is achieved by issuing a crm resource migrate.
	rpc migrate(Empty) returns (MigrateResponse) {}
}
`

func newSuperviseMigrationCommand() *cobra.Command {
	sm := &cobra.Command{
		Use:   "migration [options]",
		Short: "Run on cluster node, provides migration API",
		Long:  migrationLongDescription,
		Run:   superviseMigrationCommandFunc,
	}

	flags := sm.Flags()

	flags.String("bind-address", ":8080", "Bind API to this addr")
	viper.BindPFlag("bind-address", flags.Lookup("bind-address"))

	return sm
}

func superviseMigrationCommandFunc(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bouncer := PGBouncerOrExit()
	crm := pacemaker.NewPacemaker()
	bindAddr := viper.GetString("bind-address")

	HandleQuitSignal("cleaning context and exiting...", cancel)

	listen, err := net.Listen("tcp", bindAddr)
	if err != nil {
		logger.WithError(err).WithField("addr", bindAddr).Error("Failed to bind to address")
		return
	}

	server := migration.NewServer(
		migration.WithServerLogger(logger),
		migration.WithPGBouncer(bouncer),
		migration.WithPacemaker(crm),
	)

	grpcServer := grpc.NewServer()
	migration.RegisterMigrationServer(grpcServer, server)

	go func() { <-ctx.Done(); grpcServer.GracefulStop() }()

	logger.Infof("Starting server, bound to %s", bindAddr)
	if err := grpcServer.Serve(listen); err != nil {
		logger.WithError(err).Error("Server failed with error")
	}
}
