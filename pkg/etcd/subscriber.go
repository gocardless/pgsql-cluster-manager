package etcd

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	kitlog "github.com/go-kit/kit/log"
	"golang.org/x/net/context"
)

type subscriber struct {
	client        *clientv3.Client
	logger        kitlog.Logger
	handlers      map[string]*idempotentHandler
	retryInterval time.Duration
}

type handler interface {
	Run(string, string) error
}

type idempotentHandler struct {
	handler
	*sync.Mutex
	revision int64
}

type staleKeyError struct {
	revision int64
}

func (e staleKeyError) Error() string {
	return fmt.Sprintf("stale key revision: %d", e.revision)
}

// Run only calls the handler if the current event has a ModRevision that is greater than
// the latest this handler has successfully called before.
func (h *idempotentHandler) Run(kv *mvccpb.KeyValue) error {
	h.Lock()
	defer h.Unlock()

	if kv.ModRevision < h.revision {
		return staleKeyError{kv.ModRevision}
	}

	// Subscriber will retry handlers when they return an error, unless that error is
	// `staleKeyError`. We don't want any handler to run when a key of a higher revision has
	// been seen. Setting the latest revision to be equal to this key's ModRevision-1 means
	// any retries for keys that have lower ModRevisions will return staleKeyError, which
	// will get the subscriber to give up on the retry.
	//
	// This allows us to be retrying at-most one key at a time, where that key has the
	// highest observed revision.
	h.revision = kv.ModRevision - 1
	err := h.handler.Run(string(kv.Key), string(kv.Value))

	if err != nil {
		h.revision = kv.ModRevision
	}

	return err
}

// WithLogger creates an option to configure a subscriber with a logging handle.
func WithLogger(logger kitlog.Logger) func(*subscriber) {
	return func(s *subscriber) {
		s.logger = logger
	}
}

// WithRetryInterval configures the duration between retrying handlers that have failed
func WithRetryInterval(interval time.Duration) func(*subscriber) {
	return func(s *subscriber) {
		s.retryInterval = interval
	}
}

// NewSubscriber generates a Daemon with a watcher constructed using the given etcd config
func NewSubscriber(client *clientv3.Client, options ...func(*subscriber)) *subscriber {
	s := &subscriber{
		client:        client,
		logger:        kitlog.NewNopLogger(),
		handlers:      map[string]*idempotentHandler{},
		retryInterval: time.Second,
	}

	for _, option := range options {
		option(s)
	}

	return s
}

// AddHandler registers a new handler to be run on changes to the given key
func (s *subscriber) AddHandler(key string, h handler) *subscriber {
	s.logger.Log("event", "handler.add", "key", key, "handler", reflect.TypeOf(h).String())
	s.handlers[key] = &idempotentHandler{h, &sync.Mutex{}, 0}

	return s
}

// Start creates a new etcd watcher, and will trigger handlers that match the given key
// when values change.
func (s *subscriber) Start(ctx context.Context) {
	s.bootHandlers(ctx)
	s.watch(ctx)
}

// bootHandlers goes through each of handlers and runs them for the values in etcd. This
// is used to make sure calling Start will run handlers with the current state, even
// without observing a change in etcd.
func (s *subscriber) bootHandlers(ctx context.Context) {
	for key := range s.handlers {
		getResp, err := s.client.Get(ctx, key)

		if err != nil {
			s.logger.Log("event", "boot.error", "error", err, "key", key)
		}

		if len(getResp.Kvs) == 0 {
			s.logger.Log(
				"event", "boot.no_initial_value", "key", key,
				"msg", "key had no initial value (is supervise running?)",
			)
		}

		if len(getResp.Kvs) == 1 {
			s.eventLogger(getResp.Kvs[0]).Log("event", "boot.trigger_handler")
			s.handleEvent(getResp.Kvs[0])
		}
	}
}

func (s *subscriber) watch(ctx context.Context) {
	s.logger.Log("event", "watch.start", "msg", "starting etcd watcher")
	watcher := s.client.Watch(ctx, "/", clientv3.WithPrefix())

	for watcherResponse := range watcher {
		for _, event := range watcherResponse.Events {
			s.logger.Log("event", "watch.change", "payload", event, "msg", "observed etcd event")
			s.handleEvent(event.Kv)
		}
	}

	s.logger.Log("event", "watch.finish", "msg", "finished etcd watcher, channel closed")
}

func (s *subscriber) handleEvent(kv *mvccpb.KeyValue) {
	handler := s.handlers[string(kv.Key)]
	if handler == nil {
		return
	}

	logger := s.eventLogger(kv)
	err := handler.Run(kv)

	if err == nil {
		logger.Log("event", "handler.success")
		return
	}

	// In the case where the handler has processed seen events more recent than this, give
	// up and return.
	if _, ok := err.(staleKeyError); ok {
		logger.Log(
			"event", "handler.stale_key",
			"msg", "stale key revision, won't run handler",
		)
		return
	}

	logger.Log("event", "handler.error", "error", err,
		"msg", "failed to run handler, scheduling retry")

	// It's now the case that the handler has failed, but we don't want to give up trying.
	// We should schedule retrying this handler until the write succeeds.
	go func() {
		<-time.After(s.retryInterval)
		logger.Log("event", "handler.retry", "msg", "retrying event handler")
		s.handleEvent(kv)
	}()
}

func (s *subscriber) eventLogger(kv *mvccpb.KeyValue) kitlog.Logger {
	logger := kitlog.With(s.logger, "key", string(kv.Key), "value", string(kv.Value), "revision", kv.ModRevision)

	if handler := s.handlers[string(kv.Key)]; handler != nil {
		logger = kitlog.With(logger, "handler", reflect.TypeOf(handler.handler).String())
	}

	return logger
}
