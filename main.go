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
	"github.com/gocardless/pgsql-novips/daemon"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	App(logrus.StandardLogger()).Run(os.Args)
}

// App generates a command-line application that is the entrypoint for pgsql-novips
func App(log *logrus.Logger) *cli.App {
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
					Value:  "/etc/pgbouncer/config.ini",
				},
				cli.StringFlag{
					Name:   "pgbouncer-config-template",
					Usage:  "Template file for PGBouncer config",
					EnvVar: "PGBOUNCER_CONFIG_TEMPLATE",
					Value:  "/etc/pgbouncer/config.ini.template",
				},
				cli.StringFlag{
					Name:   "etcd-heartbeat-path",
					Usage:  "Path in which to store client heartbeat",
					EnvVar: "PROXY_ETCD_HEARTBEAT_PATH",
					Value:  "/postgres/proxy",
				},
				cli.StringFlag{
					Name:   "proxy-postgres-etcd-key",
					EnvVar: "PROXY_POSTGRES_ETCD_KEY",
					Usage:  "Proxy to host at the etcd key",
					Value:  "/postgres/pgbouncer",
				},
			},
			Action: func(c *cli.Context) error {
				if err := checkMissingFlags(c); err != nil {
					return cli.NewExitError(err, 1)
				}

				cfg := newEtcdConfig(c)
				d, err := daemon.New(cfg)

				if err != nil {
					cli.NewExitError("Failed to initialize daemon", 1)
				}

				log.Info("Starting daemon...")

				go d.Start(context.Background(), c.GlobalString("etcd-namespace"),
					daemon.HandlerMap{
						"/master": func(value string) error {
							log.WithField("value", value).Info("/master")
							return nil
						},
					},
				)

				waitForSignal(log, "Received %s, shutting down daemon...")
				return d.Shutdown()
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

func waitForSignal(log *logrus.Logger, template string) os.Signal {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	recv := <-sigc

	log.Info(fmt.Sprintf(template, recv))

	return recv
}

func newEtcdConfig(c *cli.Context) clientv3.Config {
	hosts := c.GlobalString("etcd-hosts")
	timeout := c.GlobalInt("etcd-timeout")

	return clientv3.Config{
		Endpoints:   strings.Split(hosts, ","),
		DialTimeout: time.Duration(timeout) * time.Second,
	}
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
