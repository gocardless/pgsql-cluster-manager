//go:generate protoc --go_out=plugins=grpc:. failover.proto

package failover

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coreos/etcd/clientv3"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/etcd"
	"github.com/gocardless/pgsql-cluster-manager/pkg/streams"
	"github.com/pkg/errors"
)

type FailoverOptions struct {
	EtcdHostKey        string
	HealthCheckTimeout time.Duration
	LockTimeout        time.Duration
	PauseTimeout       time.Duration
	PauseExpiry        time.Duration
	ResumeTimeout      time.Duration
	PacemakerTimeout   time.Duration
}

type Failover struct {
	logger  kitlog.Logger
	client  etcdGetter
	clients map[string]FailoverClient
	locker  locker
	opt     FailoverOptions
}

type etcdGetter interface {
	Get(context.Context, string, ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Watch(context.Context, string, ...clientv3.OpOption) clientv3.WatchChan
}

type locker interface {
	Lock(context.Context) error
	Unlock(context.Context) error
}

func NewFailover(logger kitlog.Logger, client etcdGetter, clients map[string]FailoverClient, locker locker, opt FailoverOptions) *Failover {
	return &Failover{
		logger:  logger,
		client:  client,
		clients: clients,
		locker:  locker,
		opt:     opt,
	}
}

// Run triggers the failover process. We model this as a Pipeline of steps, where each
// step has associated deferred actions that must be scheduled before the primary
// operation ever takes place.
//
// This has the benefit of clearly expressing the steps required to perform a failover,
// tidying up some of the error handling and logging noise that would otherwise be
// present.
func (f *Failover) Run(ctx context.Context, deferCtx context.Context) error {
	return Pipeline(
		Step(f.HealthCheckClients),
		Step(f.AcquireLock).Defer(f.ReleaseLock),
		Step(f.Pause).Defer(f.Resume),
		Step(f.Migrate).Defer(f.Unmigrate),
	)(
		ctx, deferCtx,
	)
}

func (f *Failover) HealthCheckClients(ctx context.Context) error {
	f.logger.Log("event", "clients.health_check", "msg", "health checking all clients")
	for endpoint, client := range f.clients {
		ctx, cancel := context.WithTimeout(ctx, f.opt.HealthCheckTimeout)
		defer cancel()

		resp, err := client.HealthCheck(ctx, &Empty{})
		if err != nil {
			return errors.Wrapf(err, "client %s failed health check", endpoint)
		}

		if status := resp.GetStatus(); status != HealthCheckResponse_HEALTHY {
			return fmt.Errorf("client %s received non-healthy response: %s", endpoint, status.String())
		}
	}

	return nil
}

func (f *Failover) AcquireLock(ctx context.Context) error {
	f.logger.Log("event", "etcd.lock.acquire", "msg", "acquiring failover lock in etcd")
	ctx, cancel := context.WithTimeout(ctx, f.opt.LockTimeout)
	defer cancel()

	return f.locker.Lock(ctx)
}

func (f *Failover) ReleaseLock(ctx context.Context) error {
	f.logger.Log("event", "etcd.lock.release", "msg", "releasing failover lock in etcd")
	ctx, cancel := context.WithTimeout(ctx, f.opt.LockTimeout)
	defer cancel()

	return f.locker.Unlock(ctx)
}

func (f *Failover) Pause(ctx context.Context) error {
	logger := kitlog.With(f.logger, "event", "clients.pgbouncer.pause")
	logger.Log("msg", "requesting all pgbouncers pause")

	// Allow an additional second for network round-trip. We should have terminated this
	// request far before this context is expired.
	ctx, cancel := context.WithTimeout(ctx, f.opt.PauseExpiry+time.Second)
	defer cancel()

	err := f.EachClient(logger, func(endpoint string, client FailoverClient) error {
		_, err := client.Pause(
			ctx, &PauseRequest{
				Timeout: int32(f.opt.PauseTimeout / time.Second),
				Expiry:  int32(f.opt.PauseExpiry / time.Second),
			},
		)

		return err
	})

	if err != nil {
		return fmt.Errorf("failed to pause pgbouncers")
	}

	return nil
}

func (f *Failover) Resume(ctx context.Context) error {
	logger := kitlog.With(f.logger, "event", "clients.pgbouncer.resume")
	logger.Log("msg", "requesting all pgbouncers resume")

	ctx, cancel := context.WithTimeout(ctx, f.opt.ResumeTimeout)
	defer cancel()

	err := f.EachClient(logger, func(endpoint string, client FailoverClient) error {
		_, err := client.Resume(ctx, &Empty{})
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to resume pgbouncers")
	}

	return nil
}

// EachClient provides a helper to perform actions on all the failover clients, in
// parallel. For some operations where there is a penalty for extended running time (such
// as pause) it's important that each request occurs in parallel.
func (f *Failover) EachClient(logger kitlog.Logger, action func(string, FailoverClient) error) (result error) {
	var wg sync.WaitGroup
	for endpoint, client := range f.clients {
		wg.Add(1)

		go func(endpoint string, client FailoverClient) {
			defer func(begin time.Time) {
				logger.Log("endpoint", endpoint, "elapsed", time.Since(begin).Seconds())
				wg.Done()
			}(time.Now())

			if err := action(endpoint, client); err != nil {
				logger.Log("endpoint", endpoint, "error", err.Error())
				result = err
			}
		}(endpoint, client)
	}

	wg.Wait()
	return result
}

// Migrate attempts to run a pacemaker migration, using a single FailoverClient.
func (f *Failover) Migrate(ctx context.Context) error {
	// Use one pacemaker client throughout our interaction
	endpoint, client := f.getClient()

	logger := kitlog.With(f.logger, "event", "clients.pacemaker.migrate", "endpoint", endpoint)
	logger.Log("msg", "requesting pacemaker migration")

	migrateCtx, cancel := context.WithTimeout(ctx, f.opt.PacemakerTimeout)
	defer cancel()

	resp, err := client.Migrate(migrateCtx, &Empty{})
	if err != nil {
		logger.Log("error", err.Error(),
			"msg", "failed to migrate, manual inspection of cluster state is recommended")
		return errors.Wrapf(err, "failed to migrate client %s", endpoint)
	}

	select {
	case <-time.After(f.opt.PauseExpiry):
		return fmt.Errorf("timed out waiting for %s to become master", resp.MigratingTo)
	case <-f.NotifyWhenMaster(ctx, logger, resp.Address):
		logger.Log("msg", "observed successful migration", "master", resp.MigratingTo)
	}

	return nil
}

func (f *Failover) Unmigrate(ctx context.Context) error {
	// Use one pacemaker client throughout our interaction
	endpoint, client := f.getClient()

	logger := kitlog.With(f.logger, "event", "clients.pacemaker.unmigrate", "endpoint", endpoint)
	logger.Log("msg", "requesting pacemaker unmigrate")

	ctx, cancel := context.WithTimeout(ctx, f.opt.PacemakerTimeout)
	defer cancel()

	if _, err := client.Unmigrate(ctx, &Empty{}); err != nil {
		logger.Log("error", err.Error(),
			"msg", "failed to unmigrate, manual action required to unmigrate cluster")
		return errors.Wrapf(err, "failed to unmigrate client %s", endpoint)
	}

	return nil
}

// NotifyWhenMaster returns a channel that receives an empty struct when the given
// targetAddr is updated in etcd.
func (f *Failover) NotifyWhenMaster(ctx context.Context, logger kitlog.Logger, targetAddr string) chan interface{} {
	logger = kitlog.With(logger, "key", f.opt.EtcdHostKey, "target", targetAddr)
	logger.Log("msg", "waiting for etcd to update with master key")

	kvs, _ := etcd.NewStream(
		f.logger,
		f.client,
		etcd.StreamOptions{
			Ctx:          ctx,
			Keys:         []string{f.opt.EtcdHostKey},
			PollInterval: time.Second,
			GetTimeout:   time.Second,
		},
	)

	kvs = streams.RevisionFilter(f.logger, kvs)

	notify := make(chan interface{})
	go func() {
		for kv := range kvs {
			if string(kv.Key) == f.opt.EtcdHostKey && string(kv.Value) == targetAddr {
				notify <- struct{}{}
				close(notify)
			}
		}
	}()

	return notify
}

// getClient returns a random (endpoint, client) pairing, enabling easy targeting of
// requests to a single client.
func (f *Failover) getClient() (string, FailoverClient) {
	for endpoint, client := range f.clients {
		return endpoint, client
	}

	return "", nil
}

func Pipeline(steps ...*pStep) func(context.Context, context.Context) error {
	return func(ctx context.Context, deferCtx context.Context) error {
		for _, step := range steps {
			// Defer first, ensuring we always attempt our defer steps, even if the primary
			// action fails.
			for _, deferAction := range step.deferred {
				defer deferAction(deferCtx)
			}

			if err := step.action(ctx); err != nil {
				return err
			}
		}

		return nil
	}
}

type pStep struct {
	action   func(context.Context) error
	deferred []func(context.Context) error
}

func Step(action func(context.Context) error) *pStep {
	return &pStep{action: action, deferred: []func(context.Context) error{}}
}

func (s *pStep) Defer(deferred ...func(context.Context) error) *pStep {
	s.deferred = deferred
	return s
}

func iso3339(t time.Time) string {
	return t.Format("2006-01-02T15:04:05-0700")
}
