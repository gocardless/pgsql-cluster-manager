package cmd

import (
	"context"
	"time"

	"github.com/gocardless/pgsql-cluster-manager/supervise"
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
	flags.Duration("pgbouncer-timeout", time.Second, "Timeout when executing queries against PGBouncer")
	flags.String("postgres-master-etcd-key", "/master", "etcd key that stores current Postgres primary")

	viper.BindPFlag("pgbouncer-config-file", flags.Lookup("pgbouncer-config-file"))
	viper.BindPFlag("pgbouncer-config-template-file", flags.Lookup("pgbouncer-config-template-file"))
	viper.BindPFlag("pgbouncer-timeout", flags.Lookup("pgbouncer-timeout"))
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

	supervise.Proxy(
		ctx, logger,
		client, bouncer,
		etcdHostKey,
	)
}

// This will select the current master node for standard install of the pgsql OCF resource
var defaultMasterCrmXPath = "crm_mon/resources/clone[@id='msPostgresql']/resource[@role='Master']/node[@name]"

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
	flags.String("postgres-master-crm-xpath", defaultMasterCrmXPath, "XPath selector into crm_mon --as-xml that finds current master")

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

	supervise.Cluster(
		ctx,
		logger,
		client,
		etcdHostKey,
		masterCrmXPath,
	)
}
