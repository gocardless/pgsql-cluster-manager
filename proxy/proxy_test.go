package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/gocardless/pgsql-novips/pgbouncer"
	"github.com/gocardless/pgsql-novips/subscriber"
	"github.com/gocardless/pgsql-novips/testHelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	etcd := testHelpers.ExecEtcd(t, ctx)
	bouncer := testHelpers.ExecPGBouncer(t, ctx)

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

	waitForHostToBecome := func(db *pgbouncer.Database, host string) *pgbouncer.Database {
		timeout := time.After(5 * time.Second)
		retry := time.Tick(100 * time.Millisecond)

		for {
			select {
			case <-retry:
				if latestDb := showDatabase(db.Name); latestDb.Host == host {
					return latestDb
				}
			case <-timeout:
				require.FailNow(t, "timed out waiting for PGBouncer host to change")
			}
		}
	}

	t.Run("changes PGBouncer database host in response to etcd key changes", func(t *testing.T) {
		sub := New(
			subscriber.NewEtcd(etcd, "/postgres"),
			ProxyConfig{
				PGBouncerHostKey:        "/master",
				PGBouncerConfig:         bouncer.ConfigFile,
				PGBouncerConfigTemplate: bouncer.ConfigFileTemplate,
				PGBouncerTimeout:        time.Second,
			},
		)

		go sub.Start(ctx)

		database := showDatabase("postgres")
		require.Equal(t, database.Host, "{{.Host}}", "expected initial host to be from template")

		_, err := etcd.Put(ctx, "/postgres/master", "pg01")
		require.Nil(t, err)

		databaseAfterChange := waitForHostToBecome(database, "pg01")

		assert.Equal(t, "pg01", databaseAfterChange.Host, "expected host to match etcd key")
	})
}
