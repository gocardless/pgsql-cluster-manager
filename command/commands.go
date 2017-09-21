package command

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/namespace"
	"github.com/gocardless/pgsql-cluster-manager/pgbouncer"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	logger = logrus.StandardLogger()
	// Go's native ISO3339 format doesn't play nice with the rest of the world
	iso3339Timestamp = "2006-01-02T15:04:05-0700"

	PgsqlCommand = &cobra.Command{
		Use: "pgsql-cluster-manager",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
	}
)

func init() {
	flags := PgsqlCommand.PersistentFlags()

	// We always need an etcd connection, so these flags are for all commands
	flags.String("etcd-namespace", "", "Namespace all requests to etcd under this value")
	flags.StringSlice("etcd-endpoints", []string{"http://127.0.0.1:2379"}, "gRPC etcd endpoints")
	flags.Duration("etcd-dial-timeout", 3*time.Second, "Timeout when connecting to etcd")
	flags.String("log-level", "info", "Log level, one of [debug,info,warning,error,fatal,panic]")

	// Bind flag value into Viper configuration
	viper.BindPFlag("etcd-namespace", flags.Lookup("etcd-namespace"))
	viper.BindPFlag("etcd-endpoints", flags.Lookup("etcd-endpoints"))
	viper.BindPFlag("etcd-dial-timeout", flags.Lookup("etcd-dial-timeout"))
	viper.BindPFlag("log-level", flags.Lookup("log-level"))

	PgsqlCommand.AddCommand(NewSuperviseCommand())
	PgsqlCommand.AddCommand(NewVersionCommand())

	cobra.OnInitialize(ConfigureLogger)
}

func ConfigureLogger() {
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

	level, err := logrus.ParseLevel(viper.GetString("log-level"))
	if err != nil {
		logger.WithError(err).Fatal("Invalid log level!")
	}

	logger.Level = level
}

func EtcdClientOrExit() *clientv3.Client {
	client, err := clientv3.New(
		clientv3.Config{
			Endpoints:   viper.GetStringSlice("etcd-endpoints"),
			DialTimeout: viper.GetDuration("etcd-dial-timeout"),
		},
	)

	if err != nil {
		logger.WithError(err).Fatal("Failed to connect to etcd")
	}

	// We should namespace all our etcd queries, to scope what we'll receive from watchers
	ns := viper.GetString("etcd-namespace")

	client.KV = namespace.NewKV(client.KV, ns)
	client.Watcher = namespace.NewWatcher(client.Watcher, ns)
	client.Lease = namespace.NewLease(client.Lease, ns)

	return client
}

func PGBouncerOrExit() pgbouncer.PGBouncer {
	return pgbouncer.NewPGBouncer(
		viper.GetString("pgbouncer-config-file"),
		viper.GetString("pgbouncer-config-template-file"),
		viper.GetDuration("pgbouncer-timeout"),
	)
}

func HandleQuitSignal(message string, handler func()) func() {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	go func() {
		if s := <-sigc; s != nil {
			logger.Infof("Received %s: %s", s, message)
			handler()
		}
	}()

	return func() { close(sigc) }
}
