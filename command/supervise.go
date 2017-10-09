package command

import (
	"context"
	"net/http"
	"time"

	"github.com/gocardless/pgsql-cluster-manager/etcd"
	"github.com/gocardless/pgsql-cluster-manager/pacemaker"
	"github.com/gocardless/pgsql-cluster-manager/pgbouncer"
	"github.com/gocardless/pgsql-cluster-manager/routes"
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
		pacemaker.WatchNode(etcdHostKey, masterCrmXPath, "uname"),
		pacemaker.WithLogger(logger),
	)

	// We should only update the key if it's changed- Updater provides idempotent updates
	crmSub.AddHandler(etcdHostKey, &etcd.Updater{client})
	crmSub.Start(ctx)
}

var migrationLongDescription = `
Provides an API that issues migration commands, used to perform zero-downtime
migrations.

  POST /pause?timeout&expiry
  Causes PGBouncer to pause on the host, cancelling the pause if we exceed
  timeout and automatically resuming after expiry seconds.

  POST /resume
  Causes PGBouncer to immediately resume, removing any active pauses.

  POST /migrate
  Creates a migration from the current Postgres primary to the eligible sync
  node. This is achieved by issuing a crm resource migrate.
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
	cib := pacemaker.NewCib()
	bindAddr := viper.GetString("bind-address")

	HandleQuitSignal("cleaning context and exiting...", cancel)

	server := &http.Server{
		Addr: bindAddr,
		Handler: routes.Route(
			routes.WithLogger(logger),
			routes.WithPGBouncer(bouncer),
			routes.WithCib(cib),
		),
	}

	go func() { <-ctx.Done(); server.Shutdown(context.Background()) }()

	logger.Infof("Starting server, bound to %s", bindAddr)
	if err := server.ListenAndServe(); err != nil {
		logger.WithError(err).Error("Server failed with error")
	}
}
