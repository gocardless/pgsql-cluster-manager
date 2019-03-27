package cmd

import (
	"os"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/namespace"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func addPgBouncerFlags(flags *pflag.FlagSet) {
	flags.String("pgbouncer-user", "pgbouncer", "Admin user of PgBouncer")
	flags.String("pgbouncer-password", "", "Password for admin user")
	flags.String("pgbouncer-database", "pgbouncer", "PgBouncer special database (inadvisable to change)")
	flags.String("pgbouncer-socket-dir", "/var/run/postgresql", "Directory in which the unix socket resides")
	flags.String("pgbouncer-port", "6432", "Port that PgBouncer is listening on")
	flags.String("pgbouncer-config-file", "/etc/pgbouncer/pgbouncer.ini", "Path to PgBouncer config file")
	flags.String("pgbouncer-config-template-file", "/etc/pgbouncer/pgbouncer.ini.template", "Path to PgBouncer config template file")
	flags.Duration("pgbouncer-timeout", 5*time.Second, "Timeout for PgBouncer operations")
}

func mustPgBouncer() *pgbouncer.PgBouncer {
	return &pgbouncer.PgBouncer{
		ConfigFile:         viper.GetString("pgbouncer-config-file"),
		ConfigTemplateFile: viper.GetString("pgbouncer-config-template-file"),
		Executor: pgbouncer.AuthorizedExecutor{
			User:      viper.GetString("pgbouncer-user"),
			Password:  viper.GetString("pgbouncer-password"),
			Database:  viper.GetString("pgbouncer-database"),
			SocketDir: viper.GetString("pgbouncer-socket-dir"),
			Port:      viper.GetString("pgbouncer-port"),
		},
	}
}

func addEtcdFlags(flags *pflag.FlagSet) {
	flags.String("etcd-namespace", "", "Namespace all requests to etcd under this value")
	flags.StringSlice("etcd-endpoints", []string{"http://127.0.0.1:2379"}, "gRPC etcd endpoints")
	flags.Duration("etcd-timeout", 3*time.Second, "Timeout for etcd operations")
	flags.Duration("etcd-dial-timeout", 3*time.Second, "Timeout when connecting to etcd")
	flags.Duration("etcd-keep-alive-time", 30*time.Second, "Time after which client pings server to check transport")
	flags.Duration("etcd-keep-alive-timeout", 5*time.Second, "Timeout for the keep alive probe")
	flags.String("etcd-postgres-master-key", "/master", "etcd key that stores current Postgres primary")
}

func mustEtcdClient() *clientv3.Client {
	client, err := clientv3.New(
		clientv3.Config{
			Endpoints:            viper.GetStringSlice("etcd-endpoints"),
			DialTimeout:          viper.GetDuration("etcd-dial-timeout"),
			DialKeepAliveTime:    viper.GetDuration("etcd-dial-keep-alive-time"),
			DialKeepAliveTimeout: viper.GetDuration("etcd-dial-keep-alive-timeout"),
		},
	)

	if err != nil {
		logger.Log("event", "etcd.failed", "error", err)
		os.Exit(1)
	}

	// We should namespace all our etcd queries, to scope what we'll receive from watchers
	ns := viper.GetString("etcd-namespace")

	client.KV = namespace.NewKV(client.KV, ns)
	client.Watcher = namespace.NewWatcher(client.Watcher, ns)
	client.Lease = namespace.NewLease(client.Lease, ns)

	return client
}

func addFailoverFlags(flags *pflag.FlagSet) {
	flags.StringSlice("failover-api-endpoints", []string{"pg01:8080", "pg02:8080", "pg03:8080"}, "All Postgres node API endpoints")
	flags.Duration("health-check-timeout", 2*time.Second, "Timeout to health check each node")
	flags.Duration("lock-timeout", 5*time.Second, "Timeout to acquire exclusive failover lock in etcd")
	flags.Duration("pause-timeout", 5*time.Second, "Timeout for all nodes to pause PgBouncer")
	flags.Duration("pause-expiry", 25*time.Second, "Time after which PgBouncer will automatically lift pause")
	flags.Duration("resume-timeout", 5*time.Second, "Timeout for PgBouncer resume operations")
	flags.Duration("pacemaker-timeout", 20*time.Second, "Timeout for executing (not necessarily to completion) pacemaker commands")
}
