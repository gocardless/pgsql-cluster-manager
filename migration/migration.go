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
	Etcd          etcdClient
	EtcdMasterKey string
	Clients       []MigrationClient
	Locker        locker
	PauseTimeout  time.Duration
	PauseExpiry   time.Duration
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
	if err := m.HealthCheck(timeout(5 * time.Second)); err != nil {
		return err
	}

	m.Info("Acquiring lock")
	if err := m.Locker.Lock(timeout(10 * time.Second)); err != nil {
		return err
	}

	defer func() { m.Info("Releasing lock"); m.Locker.Unlock(ctx) }()

	m.Info("Pausing all clients")
	if err := m.Pause(ctx); err != nil {
		return err
	}

	defer m.Resume(ctx)

	m.Info("Running crm resource migrate")
	resp, err := m.Clients[0].Migrate(timeout(5*time.Second), &Empty{})

	if err != nil {
		return err
	}

	defer func() {
		m.Info("Running crm resource unmigrate")
		if _, err := m.Clients[0].Unmigrate(ctx, &Empty{}); err != nil {
			m.WithError(err).Error("Failed to migrate, manual action required")
		}
	}()

	select {
	case <-time.After(m.PauseExpiry):
		return fmt.Errorf("Timed out waiting for %s to become master", resp.MigratingTo)
	case <-m.HasBecomeMaster(ctx, resp.MigratingTo):
		m.WithField("master", resp.MigratingTo).Info("Successfully migrated!")
	}

	return nil
}

func (m *migration) HealthCheck(ctx context.Context) error {
	return m.Batch(func(client MigrationClient) error {
		_, err := client.HealthCheck(ctx, &Empty{})
		return err
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

func (m *migration) HasBecomeMaster(ctx context.Context, migratingTo string) chan interface{} {
	contextLogger := m.WithField("key", m.EtcdMasterKey).WithField("target", migratingTo)
	contextLogger.Info("Watching for etcd key to update with target")

	watchChan := m.Etcd.Watch(ctx, m.EtcdMasterKey)
	notify := make(chan interface{}, 1)

	currentValue, err := m.Etcd.Get(ctx, m.EtcdMasterKey)
	if err == nil && len(currentValue.Kvs) == 1 && string(currentValue.Kvs[0].Value) == migratingTo {
		notify <- struct{}{}
		close(notify)

		return notify // we're already master, so we can return immediately
	}

	go func() {
		defer close(notify)

		for resp := range watchChan {
			for _, event := range resp.Events {
				contextLogger.WithField("value", string(event.Kv.Value)).
					Debug("Observed change to etcd key")
				if string(event.Kv.Value) == migratingTo {
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
