package pgbouncer_test

import (
	"context"
	"testing"
	"time"

	"github.com/gocardless/pgsql-cluster-manager/etcd"
	"github.com/gocardless/pgsql-cluster-manager/integration"
	"github.com/gocardless/pgsql-cluster-manager/pgbouncer"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type handler interface {
	Run(string, string) error
}

func TestHostChanger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	etcdClient := integration.StartEtcd(t, ctx)
	bouncer := integration.StartPGBouncer(t, ctx)

	// Use a debug logger for the etcd subscriber
	logger := logrus.StandardLogger()
	logger.Level = logrus.DebugLevel

	showDatabase := func(name string) *pgbouncer.Database {
		databases, err := bouncer.ShowDatabases()
		if err != nil {
			require.FailNow(t, "failed to query pgbouncer: %s", err.Error())
		}

		for _, database := range databases {
			if database.Name == name {
				return &database
			}
		}

		return nil
	}

	t.Run("changes PGBouncer database host in response to etcd key changes", func(t *testing.T) {
		go etcd.NewSubscriber(etcdClient, etcd.WithLogger(logger)).
			AddHandler("/master", pgbouncer.HostChanger{bouncer}).
			Start(ctx)

		database := showDatabase("postgres")
		require.Equal(t, database.Host, "{{.Host}}", "expected initial host to be from template")

		timeout := time.After(2 * time.Second)

		// We have to retry putting the value to etcd a few times, as we don't know if the
		// subscriber will be listening when we first attempt a put. Retry the put until
		// either the host changes, or we timeout.
		for {
			select {
			case <-timeout:
				require.FailNow(t, "timed out waiting for PGBouncer host to change")
			default:
				// Attempt to put a new value to etcd
				_, err := etcdClient.Put(ctx, "/master", "pg01")
				require.Nil(t, err)

				// Check to see if database has been updated to our desired target
				if latestDb := showDatabase(database.Name); latestDb.Host == "pg01" {
					return
				}
			}
		}
	})
}
