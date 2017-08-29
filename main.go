package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/gocardless/pgsql-novips/proxy"
	"github.com/gocardless/pgsql-novips/subscriber"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	App(logrus.StandardLogger()).Run(os.Args)
}

// App generates a command-line application that is the entrypoint for pgsql-novips
func App(logger *logrus.Logger) *cli.App {
	app := cli.NewApp()

	app.Name = "pgsql-novips"
	app.Usage = "Control Postgres clusters through etcd configuration"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "client-id",
			Usage:  "Unique identifier for heartbeats (typically hostname)",
			EnvVar: "CLIENT_ID",
		},
		cli.StringFlag{
			Name:   "etcd-namespace",
			Usage:  "Prefix to all etcd accesses",
			EnvVar: "ETCD_NAMESPACE",
			Value:  "/postgres",
		},
		cli.StringFlag{
			Name:   "etcd-hosts",
			Usage:  "Comma separated list of etcd hosts",
			EnvVar: "ETCD_HOSTS",
		},
		cli.IntFlag{
			Name:   "etcd-keepalive",
			Usage:  "Keepalive in seconds when talking to etcd",
			EnvVar: "ETCD_KEEPALIVE",
			Value:  2,
		},
		cli.IntFlag{
			Name:   "etcd-timeout",
			Usage:  "Timeout in seconds when talking to etcd",
			EnvVar: "ETCD_TIMEOUT",
			Value:  5,
		},
		cli.IntFlag{
			Name:   "etcd-heartbeat-keepalive",
			Usage:  "Interval between renewing client heartbeat in etcd",
			EnvVar: "ETCD_HEARTBEAT_KEEPALIVE",
			Value:  3,
		},
		cli.IntFlag{
			Name:   "etcd-heartbeat-timeout",
			Usage:  "Period to persist client heartbeat in etcd",
			EnvVar: "ETCD_HEARTBEAT_TIMEOUT",
			Value:  10,
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "migrate",
			Aliases: []string{},
			Usage:   "Zero-downtime promotion of sync to master",
			Action: func(c *cli.Context) error {
				if err := checkMissingFlags(c); err != nil {
					return cli.NewExitError(err, 1)
				}

				return cli.NewExitError("Bailing, you haven't implemented 'migrate' yet", 1)
			},
		},
		{
			Name:    "proxy",
			Aliases: []string{},
			Usage:   "Manage PGBouncer to proxy connections to host in etcd",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "pgbouncer-config",
					Usage:  "Path to place rendered PGBouncer config",
					EnvVar: "PGBOUNCER_CONFIG",
					Value:  "/etc/pgbouncer/pgbouncer.ini",
				},
				cli.StringFlag{
					Name:   "pgbouncer-config-template",
					Usage:  "Template file for PGBouncer config",
					EnvVar: "PGBOUNCER_CONFIG_TEMPLATE",
					Value:  "/etc/pgbouncer/pgbouncer.ini.template",
				},
				cli.IntFlag{
					Name:   "pgbouncer-timeout",
					Usage:  "Timeout in seconds to wait for PGBouncer to execute statement",
					EnvVar: "PGBOUNCER_TIMEOUT",
					Value:  1,
				},
				cli.StringFlag{
					Name:   "etcd-heartbeat-path",
					Usage:  "Path in which to store client heartbeat (within namespace)",
					EnvVar: "PROXY_ETCD_HEARTBEAT_PATH",
					Value:  "/proxy",
				},
				cli.StringFlag{
					Name:   "pgbouncer-host-key",
					EnvVar: "PGBOUNCER_HOST_KEY",
					Usage:  "Proxy to host at the etcd key (within namespace)",
					Value:  "/pgbouncer",
				},
			},
			Action: func(c *cli.Context) error {
				if err := checkMissingFlags(c); err != nil {
					return cli.NewExitError(err, 1)
				}

				etcd, err := createEtcdConnection(c)

				if err != nil {
					return cli.NewExitError(err.Error(), 1)
				}

				sub := proxy.New(
					subscriber.NewLoggingSubscriber(
						logger, subscriber.NewEtcd(etcd, c.GlobalString("etcd-namespace")),
					),
					proxy.ProxyConfig{
						PGBouncerHostKey:        c.String("pgbouncer-host-key"),
						PGBouncerConfig:         c.String("pgbouncer-config"),
						PGBouncerConfigTemplate: c.String("pgbouncer-config-template"),
						PGBouncerTimeout:        time.Duration(c.Int("pgbouncer-timeout")) * time.Second,
					},
				)

				go sub.Start(context.Background())

				waitForSignal(logger, "Received %s, shutting down daemon...")
				return sub.Shutdown()
			},
		},
		{
			Name:    "cluster",
			Aliases: []string{},
			Usage:   "Manage host as part of a Postgres cluster",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "etcd-heartbeat-path",
					Usage:  "Path in which to store client heartbeat",
					EnvVar: "CLUSTER_ETCD_HEARTBEAT_PATH",
					Value:  "/postgres/cluster",
				},
				cli.StringFlag{
					Name:   "cluster-postgres-master-etcd-key",
					EnvVar: "CLUSTER_POSTGRES_MASTER_ETCD_KEY",
					Usage:  "Proxy to host at the etcd key",
					Value:  "/postgres/master",
				},
				cli.IntFlag{
					Name:   "cluster-pgbouncer-pause-timeout",
					EnvVar: "CLUSTER_PGBOUNCER_PAUSE_TIMEOUT",
					Value:  3,
					Usage:  "Wait `TIMEOUT` seconds for PGBouncer to pause before giving up",
				},
			},
			Action: func(c *cli.Context) error {
				if err := checkMissingFlags(c); err != nil {
					return cli.NewExitError(err, 1)
				}

				return cli.NewExitError("Bailing, you haven't implemented 'cluster' yet", 1)
			},
		},
	}

	return app
}

func waitForSignal(logger *logrus.Logger, template string) os.Signal {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	recv := <-sigc

	logger.Info(fmt.Sprintf(template, recv))

	return recv
}

func createEtcdConnection(c *cli.Context) (*clientv3.Client, error) {
	hosts := c.GlobalString("etcd-hosts")
	timeout := c.GlobalInt("etcd-timeout")

	client, err := clientv3.New(
		clientv3.Config{
			Endpoints:   strings.Split(hosts, ","),
			DialTimeout: time.Duration(timeout) * time.Second,
		},
	)

	if err == nil {
		return client, err
	}

	return client, fmt.Errorf("Failed to connect to etcd: %v", hosts)
}

func checkMissingFlags(c *cli.Context) error {
	var err error
	var missing []string
	var nullString string

	for _, flag := range append(c.FlagNames(), c.GlobalFlagNames()...) {
		if c.String(flag) == nullString && c.GlobalString(flag) == nullString {
			missing = append(missing, flag)
		}
	}

	if len(missing) > 0 {
		err = fmt.Errorf("Missing configuration flags: %v", missing)
	}

	return err
}
