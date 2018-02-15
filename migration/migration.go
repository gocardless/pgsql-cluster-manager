//go:generate protoc --go_out=plugins=grpc:. migration.proto

package migration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
)

type migration struct {
	MigrationConfig
}

type MigrationConfig struct {
	*logrus.Logger
	Etcd               etcdClient
	EtcdMasterKey      string
	Clients            []MigrationClient
	Locker             locker
	HealthCheckTimeout time.Duration
	LockTimeout        time.Duration
	PauseTimeout       time.Duration
	PauseExpiry        time.Duration
	PacemakerTimeout   time.Duration
}

func NewMigration(cfg MigrationConfig) *migration {
	return &migration{cfg}
}

type etcdClient interface {
	Get(context.Context, string, ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Watch(context.Context, string, ...clientv3.OpOption) clientv3.WatchChan
}

type locker interface {
	Lock(context.Context) error
	Unlock(context.Context) error
}

func iso3339(t time.Time) string {
	return t.Format("2006-01-02T15:04:05-0700")
}

func (m *migration) Run(ctx context.Context) error {
	timeout := func(t time.Duration) context.Context {
		ctx, _ := context.WithTimeout(ctx, t)
		return ctx
	}

	m.Info("Health checking clients")
	if err := m.HealthCheck(timeout(m.HealthCheckTimeout)); err != nil {
		return err
	}

	m.Info("Acquiring etcd migration lock")
	if err := m.Locker.Lock(timeout(m.LockTimeout)); err != nil {
		return err
	}

	defer func() { m.Info("Releasing etcd migration lock"); m.Locker.Unlock(ctx) }()

	defer m.Resume(ctx)

	m.Info("Pausing all clients")
	if err := m.Pause(ctx); err != nil {
		return err
	}

	m.Info("Running crm resource migrate")
	resp, err := m.Clients[0].Migrate(timeout(m.PacemakerTimeout), &Empty{})

	// We should schedule a resource unmigrate regardless of if there was an error, as we
	// don't know after timing out whether the migration constraint has been applied.
	defer func() {
		m.Info("Running crm resource unmigrate")
		if _, err := m.Clients[0].Unmigrate(ctx, &Empty{}); err != nil {
			m.WithError(err).Error("Failed to unmigrate, manual action required")
		}
	}()

	if err != nil {
		return err
	}

	select {
	case <-time.After(m.PauseExpiry):
		return fmt.Errorf("Timed out waiting for %s to become master", resp.MigratingTo)
	case <-m.HasBecomeMaster(ctx, resp.Address):
		m.WithField("master", resp.MigratingTo).Info("Successfully migrated!")
	}

	return nil
}

func (m *migration) HealthCheck(ctx context.Context) error {
	return m.Batch(func(client MigrationClient) error {
		resp, err := client.HealthCheck(ctx, &Empty{})

		if err != nil {
			return err
		}

		if status := resp.GetStatus(); status != HealthCheckResponse_HEALTHY {
			return fmt.Errorf("Received non-healthy response: %s", status.String())
		}

		return nil
	})
}

func (m *migration) Pause(ctx context.Context) error {
	return m.Batch(func(client MigrationClient) error {
		_, err := client.Pause(ctx, &PauseRequest{
			Timeout: int32(m.PauseTimeout / time.Second),
			Expiry:  int32(m.PauseExpiry / time.Second),
		})
		return err
	})
}

func (m *migration) Resume(ctx context.Context) error {
	return m.Batch(func(client MigrationClient) error {
		_, err := client.Resume(ctx, &Empty{})
		return err
	})
}

func (m *migration) HasBecomeMaster(ctx context.Context, migratingToAddress string) chan interface{} {
	contextLogger := m.WithField("key", m.EtcdMasterKey).WithField("target", migratingToAddress)
	contextLogger.Info("Watching for etcd key to update with master IP address")

	watchChan := m.Etcd.Watch(ctx, m.EtcdMasterKey)
	notify := make(chan interface{}, 1)

	currentValue, err := m.Etcd.Get(ctx, m.EtcdMasterKey)
	if err == nil && len(currentValue.Kvs) == 1 && string(currentValue.Kvs[0].Value) == migratingToAddress {
		notify <- struct{}{}
		close(notify)

		return notify // we're already master, so we can return immediately
	}

	go func() {
		defer close(notify)

		for resp := range watchChan {
			for _, event := range resp.Events {
				contextLogger.WithField("value", string(event.Kv.Value)).
					Debug("etcd master key has been updated")
				if string(event.Kv.Value) == migratingToAddress {
					notify <- struct{}{}
				}
			}
		}
	}()

	return notify
}

func (m *migration) Batch(op func(MigrationClient) error) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(m.Clients))

	for _, client := range m.Clients {
		wg.Add(1)
		go func(client MigrationClient) {
			if err := op(client); err != nil {
				m.WithError(err).Error("Client operation failed")
				errChan <- err
			}

			wg.Done()
		}(client)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) == 0 {
		return nil
	}

	return fmt.Errorf("%d clients responded with errors", len(errChan))
}
