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
	"github.com/coreos/etcd/clientv3/namespace"
	"github.com/gocardless/pgsql-cluster-manager/etcd"
	"github.com/gocardless/pgsql-cluster-manager/pacemaker"
	"github.com/gocardless/pgsql-cluster-manager/pgbouncer"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/crypto/ssh/terminal"
)

var version string
var iso3339Timestamp = "2006-01-02T15:04:05-0700"

func main() {
	logger := logrus.StandardLogger()

	// We should default to JSON logging if we think we're probably capturing logs, like
	// when we can't detect a terminal.
	if !terminal.IsTerminal(int(os.Stderr.Fd())) {
		logger.Formatter = &logrus.JSONFormatter{
			TimestampFormat: iso3339Timestamp,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyMsg:   "message",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyTime:  "timestamp",
			},
		}
	}

	App(logger).Run(os.Args)
}

type handler interface {
	Run(string, string) error
}

// App generates a command-line application that is the entrypoint for pgsql-cluster-manager
func App(logger *logrus.Logger) *cli.App {
	app := cli.NewApp()

	app.Name = "pgsql-cluster-manager"
	app.Usage = "Control Postgres clusters through etcd configuration"
	app.Version = version

	app.Flags = []cli.Flag{
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
			Name:   "etcd-timeout",
			Usage:  "Timeout in seconds when talking to etcd",
			EnvVar: "ETCD_TIMEOUT",
			Value:  5,
		},
	}

	pgbouncerFlags := []cli.Flag{
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
			Flags: append(
				pgbouncerFlags,
				[]cli.Flag{
					cli.StringFlag{
						Name:   "pgbouncer-host-key",
						EnvVar: "PGBOUNCER_HOST_KEY",
						Usage:  "Proxy to host at the etcd key (within namespace)",
						Value:  "/pgbouncer",
					},
				}...,
			),
			Action: func(c *cli.Context) error {
				if err := checkMissingFlags(c); err != nil {
					return cli.NewExitError(err, 1)
				}

				etcdClient, err := createEtcdConnection(c)

				if err != nil {
					return cli.NewExitError(err.Error(), 1)
				}

				ctx, cancel := context.WithCancel(context.Background())
				sub := etcd.NewSubscriber(etcdClient, etcd.WithLogger(logger))

				go sub.Start(
					ctx, map[string]etcd.Handler{
						// Listen for changes to pgbouncer-host-key, and reload pgbouncer
						c.String("pgbouncer-host-key"): &pgbouncer.HostChanger{
							createPGBouncer(c),
						},
					},
				)

				return cancelOnSignal(cancel, logger, "Received %s, shutting down proxy daemon...")
			},
		},
		{
			Name:    "cluster",
			Aliases: []string{},
			Usage:   "Manage host as part of a Postgres cluster",
			Flags: append(
				pgbouncerFlags,
				[]cli.Flag{
					cli.StringFlag{
						Name:   "postgres-master-etcd-key",
						EnvVar: "POSTGRES_MASTER_ETCD_KEY",
						Usage:  "(namespaced) etcd key that specifies the Postgres primary",
						Value:  "/master",
					},
					cli.StringFlag{
						Name:   "postgres-master-crm-xpath",
						EnvVar: "POSTGRES_MASTER_CRM_XPATH",
						Usage:  "XPath query into crm_mon's XML output for the Postgres master",
						Value:  "crm_mon/resources/clone[@id='msPostgresql']/resource[@role='Master']/node[@name]",
					},
				}...,
			),
			Action: func(c *cli.Context) error {
				if err := checkMissingFlags(c); err != nil {
					return cli.NewExitError(err, 1)
				}

				etcdClient, err := createEtcdConnection(c)

				if err != nil {
					return cli.NewExitError(err.Error(), 1)
				}

				crmSub := pacemaker.NewSubscriber(
					// Watch for changes to master node, calling the handler on the master etcd key
					pacemaker.WatchNode(
						c.String("postgres-master-etcd-key"),
						c.String("postgres-master-crm-xpath"),
						"name",
					),
				)

				ctx, cancel := context.WithCancel(context.Background())
				etcdUpdater := etcd.Updater{etcdClient}

				go subscriber.Log(logger, crmSub).Start(
					ctx, map[string]handler{
						c.String("postgres-master-etcd-key"): &etcdUpdater,
					},
				)

				return cancelOnSignal(cancel, logger, "Received %s, shutting down cluster daemon...")
			},
		},
	}

	return app
}

func cancelOnSignal(cancel func(), logger *logrus.Logger, template string) error {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	recv := <-sigc

	logger.Info(fmt.Sprintf(template, recv))
	cancel()

	return nil
}

func createPGBouncer(c *cli.Context) pgbouncer.PGBouncer {
	return pgbouncer.NewPGBouncer(
		c.String("pgbouncer-config"),
		c.String("pgbouncer-config-template"),
		time.Duration(c.Int("pgbouncer-timeout"))*time.Second,
	)
}

func createEtcdConnection(c *cli.Context) (*clientv3.Client, error) {
	hosts := c.GlobalString("etcd-hosts")
	timeout := c.GlobalInt("etcd-timeout")
	etcdNamespace := c.GlobalString("etcd-namespace")

	client, err := clientv3.New(
		clientv3.Config{
			Endpoints:   strings.Split(hosts, ","),
			DialTimeout: time.Duration(timeout) * time.Second,
		},
	)

	if err != nil {
		return client, err
	}

	// We should namespace all our etcd queries, so that we can run assuming we have our own
	// private etcd instance.
	client.KV = namespace.NewKV(client.KV, etcdNamespace)
	client.Watcher = namespace.NewWatcher(client.Watcher, etcdNamespace)
	client.Lease = namespace.NewLease(client.Lease, etcdNamespace)

	return client, err
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
