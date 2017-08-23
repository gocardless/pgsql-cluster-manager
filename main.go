package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
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
	}

	app.Commands = []cli.Command{
		{
			Name:    "migrate",
			Aliases: []string{},
			Usage:   "Zero-downtime promotion of sync to master",
			Action: func(c *cli.Context) error {
				log.Info("Bailing, you haven't implemented 'migrate' yet")
				return nil
			},
		},
		{
			Name:    "proxy",
			Aliases: []string{},
			Usage:   "Manage PGBouncer to proxy connections to host in etcd",
			Flags: []cli.Flag{
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
				log.Info("Bailing, you haven't implemented 'proxy' yet")
				return nil
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
				assertFlagsValue(c, c.GlobalFlagNames()...)
				assertFlagsValue(c, c.FlagNames()...)

				log.Info("Bailing, you haven't implemented 'proxy' yet")
				return nil
			},
		},
	}

	app.Run(os.Args)
}

func assertFlagsValue(c *cli.Context, flags ...string) {
	var missing []string
	var nullString string

	for _, flag := range flags {
		if c.String(flag) == nullString && c.GlobalString(flag) == nullString {
			missing = append(missing, flag)
		}
	}

	if len(missing) > 0 {
		log.WithFields(log.Fields{
			"missing": fmt.Sprintf("%v", missing),
		}).Fatal("Exiting with error, missing configuration")
	}
}
