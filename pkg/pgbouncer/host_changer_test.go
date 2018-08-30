package pgbouncer_test

import (
	"context"
	"testing"
	"time"

	"github.com/gocardless/pgsql-cluster-manager/pkg/etcd"
	"github.com/gocardless/pgsql-cluster-manager/pkg/integration"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer"
	"github.com/stretchr/testify/assert"
)

type handler interface {
	Run(string, string) error
}

func TestHostChanger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	etcdClient := integration.StartEtcd(t, ctx)
	bouncer := integration.StartPgBouncer(t, ctx)

	showDatabase := func(name string) (*pgbouncer.Database, error) {
		databases, err := bouncer.ShowDatabases(context.Background())
		if err != nil {
			return nil, err
		}

		for _, database := range databases {
			if database.Name == name {
				return &database, nil
			}
		}

		return nil, nil
	}

	t.Run("changes PgBouncer database host in response to etcd key changes", func(t *testing.T) {
		go etcd.NewSubscriber(etcdClient).
			AddHandler("/master", pgbouncer.HostChanger{bouncer, time.Second}).
			Start(ctx)

		database, err := showDatabase("postgres")
		if ok := assert.Nil(t, err) && assert.Equal(t, database.Host, "{{.Host}}"); !ok {
			return
		}

		timeout := time.After(10 * time.Second)

		// We have to retry putting the value to etcd a few times, as we don't know if the
		// subscriber will be listening when we first attempt a put. Retry the put until
		// either the host changes, or we timeout.
		for {
			select {
			case <-timeout:
				assert.Fail(t, "timed out waiting for PgBouncer host to change")
				return
			default:
				// Attempt to put a new value to etcd
				if _, err := etcdClient.Put(ctx, "/master", "pg01"); err != nil {
					assert.Nil(t, err)
					return
				}

				// Check to see if database has been updated to our desired target
				latestDb, err := showDatabase(database.Name)
				if ok := assert.Nil(t, err); !ok {
					return
				}

				if latestDb.Host == "pg01" {
					return
				}
			}
		}
	})
}
