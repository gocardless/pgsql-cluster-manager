//go:generate protoc --go_out=plugins=grpc:. failover.proto

package failover

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coreos/etcd/clientv3"
	kitlog "github.com/go-kit/kit/log"
)

type Failover struct {
	Logger             kitlog.Logger
	Etcd               etcdClient
	EtcdHostKey        string
	Clients            []FailoverClient
	Locker             locker
	HealthCheckTimeout time.Duration
	LockTimeout        time.Duration
	PauseTimeout       time.Duration
	PauseExpiry        time.Duration
	PacemakerTimeout   time.Duration
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

func (f *Failover) Run(ctx context.Context) error {
	timeout := func(t time.Duration) context.Context {
		ctx, _ := context.WithTimeout(ctx, t)
		return ctx
	}

	f.Logger.Log("event", "clients.health_check", "clients", f.Clients)
	if err := f.HealthCheck(timeout(f.HealthCheckTimeout)); err != nil {
		return err
	}

	f.Logger.Log("event", "etcd.acquire_lock")
	if err := f.Locker.Lock(timeout(f.LockTimeout)); err != nil {
		return err
	}

	defer func() {
		f.Logger.Log("event", "etcd.release_lock")
		f.Locker.Unlock(ctx)
	}()

	defer f.Resume(ctx)

	f.Logger.Log("event", "clients.pause")
	if err := f.Pause(ctx); err != nil {
		return err
	}

	// Use one pacemaker client throughout our interaction
	pacemakerClient := f.Clients[0]

	f.Logger.Log("event", "crm_resource.migrate", "client", pacemakerClient)
	resp, err := pacemakerClient.Migrate(timeout(f.PacemakerTimeout), &Empty{})

	// We should schedule a resource unmigrate regardless of if there was an error, as we
	// don't know after timing out whether the migration constraint has been applied.
	defer func() {
		f.Logger.Log("event", "crm_resource.unmigrate", "client", pacemakerClient)
		if _, err := f.Clients[0].Unmigrate(ctx, &Empty{}); err != nil {
			f.Logger.Log("event", "crm_resource.unmigrate", "error", err,
				"msg", "failed to unmigrate, manual action required to unmigrate cluster")
		}
	}()

	if err != nil {
		return err
	}

	select {
	case <-time.After(f.PauseExpiry):
		return fmt.Errorf("timed out waiting for %s to become master", resp.MigratingTo)
	case <-f.HasBecomeMaster(ctx, resp.Address):
		f.Logger.Log("event", "crm_resource.migrate_success", "master", resp.MigratingTo,
			"msg", "successfully migrated Postgres master")
	}

	return nil
}

func (f *Failover) HealthCheck(ctx context.Context) error {
	return f.Batch(func(client FailoverClient) error {
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

func (f *Failover) Pause(ctx context.Context) error {
	return f.Batch(func(client FailoverClient) error {
		_, err := client.Pause(ctx, &PauseRequest{
			Timeout: int32(f.PauseTimeout / time.Second),
			Expiry:  int32(f.PauseExpiry / time.Second),
		})
		return err
	})
}

func (f *Failover) Resume(ctx context.Context) error {
	return f.Batch(func(client FailoverClient) error {
		_, err := client.Resume(ctx, &Empty{})
		return err
	})
}

func (f *Failover) HasBecomeMaster(ctx context.Context, migratingToAddress string) chan interface{} {
	logger := kitlog.With(f.Logger, "key", f.EtcdHostKey, "target", migratingToAddress)
	logger.Log("event", "etcd.wait_for_update")

	watchChan := f.Etcd.Watch(ctx, f.EtcdHostKey)
	notify := make(chan interface{}, 1)

	currentValue, err := f.Etcd.Get(ctx, f.EtcdHostKey)
	if err == nil && len(currentValue.Kvs) == 1 && string(currentValue.Kvs[0].Value) == migratingToAddress {
		notify <- struct{}{}
		close(notify)

		return notify // we're already master, so we can return immediately
	}

	go func() {
		defer close(notify)

		for resp := range watchChan {
			for _, event := range resp.Events {
				logger.Log("event", "etcd.observed_update", "value", string(event.Kv.Value))
				if string(event.Kv.Value) == migratingToAddress {
					notify <- struct{}{}
				}
			}
		}
	}()

	return notify
}

func (f *Failover) Batch(op func(FailoverClient) error) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(f.Clients))

	for _, client := range f.Clients {
		wg.Add(1)
		go func(client FailoverClient) {
			if err := op(client); err != nil {
				f.Logger.Log("event", "clients.operation", "error", err)
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
