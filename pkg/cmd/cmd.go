package cmd

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"io/ioutil"
	stdlog "log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"

	kitlog "github.com/go-kit/kit/log"
	level "github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logger kitlog.Logger

	configHash = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pgcm_config_hash",
		Help: "Hash of the currently loaded pgsql-cluster-manager configuration file.",
	})
)

func init() {
	prometheus.MustRegister(configHash)

	// Configure viper so that command-line flags are used as a priority, followed by
	// environment variables, followed by the supplied defaults
	viper.AutomaticEnv()
	viper.SetEnvPrefix("pgcm")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	logOutput := kitlog.NewSyncWriter(os.Stderr)

	// We should default to JSON logging if we think we're probably capturing logs, like
	// when we can't detect a terminal.
	if terminal.IsTerminal(int(os.Stderr.Fd())) {
		logger = kitlog.NewLogfmtLogger(logOutput)
	} else {
		logger = kitlog.NewJSONLogger(logOutput)
	}

	logger = level.NewFilter(logger, level.AllowInfo())
	logger = kitlog.With(logger, "ts", kitlog.DefaultTimestampUTC, "caller", kitlog.DefaultCaller)
	stdlog.SetOutput(kitlog.NewStdlibAdapter(logger))
}

// Execute is the top-level function that should be called to run the pgcm binary.
func Execute(ctx context.Context) {
	if err := NewPgcmCommand(ctx).Execute(); err != nil {
		logger.Log("event", "execute.error", "error", err, "msg", "execution failed, exiting with error")
		os.Exit(1)
	}
}

func NewPgcmCommand(ctx context.Context) *cobra.Command {
	pgcm := &pgcmCommand{}
	c := &cobra.Command{
		Use:           "pgcm",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return pgcm.loadConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("no command given")
		},
	}

	c.PersistentFlags().StringVar(&pgcm.ConfigFile, "config-file", "", "Load configuration from confile file")
	addEtcdFlags(c.PersistentFlags())
	viper.BindPFlags(c.PersistentFlags())

	// Automatically clean-up resources when we receive a quit signal
	ctx, cancel := context.WithCancel(ctx)
	handleQuitSignal(func() {
		logger.Log("event", "shutdown.start", "msg", "received signal, shutting down")
		cancel()
	})

	c.AddCommand(NewConfigCommand(ctx))
	c.AddCommand(NewFailoverCommand(ctx))
	c.AddCommand(NewProxyCommand(ctx))
	c.AddCommand(NewSuperviseCommand(ctx))

	return c
}

type pgcmCommand struct {
	ConfigFile string
}

func (c *pgcmCommand) loadConfig() error {
	if c.ConfigFile == "" {
		logger.Log("event", "config_file.none_given")
		configHash.Set(0)

		return nil
	}

	logger := kitlog.With(logger, "config_file", c.ConfigFile)
	logger.Log("event", "config_file.loading")
	content, err := ioutil.ReadFile(c.ConfigFile)

	if err == nil {
		configHash.Set(computeConfigHash(content))
		err = viper.ReadConfig(bytes.NewReader(content))
		if err == nil {
			logger.Log("event", "config_file.loaded", "hash", configHash)
			return nil
		}
	}

	logger.Log("event", "config_file.error", "error", err)
	return errors.Wrap(err, "failed to load config file")
}

func handleQuitSignal(handler func()) func() {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	go func() {
		if recv := <-sigc; recv != nil {
			handler()
			close(sigc)
		}
	}()

	return func() { close(sigc) }
}

// computeConfigHash generates a float64 from the md5 hash of the contents in our config
// file. We use the first 48 bits of the md5 hash as the float64 specification has a 53
// bit mantissa.
func computeConfigHash(content []byte) float64 {
	sum := md5.Sum(content)
	var bytes = make([]byte, 8)
	copy(bytes, sum[0:6])
	return float64(binary.LittleEndian.Uint64(bytes))
}
