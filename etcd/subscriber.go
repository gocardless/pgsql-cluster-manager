package etcd

import (
	"io/ioutil"
	"reflect"

	"github.com/coreos/etcd/clientv3"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

type subscriber struct {
	client   *clientv3.Client
	logger   *logrus.Logger
	handlers map[string]handler
}

type handler interface {
	Run(string, string) error
}

// WithLogger creates an option to configure a subscriber with a logging handle.
func WithLogger(logger *logrus.Logger) func(*subscriber) {
	return func(s *subscriber) {
		s.logger = logger
	}
}

// NewSubscriber generates a Daemon with a watcher constructed using the given etcd config
func NewSubscriber(client *clientv3.Client, options ...func(*subscriber)) *subscriber {
	nullLogger := logrus.New()
	nullLogger.Out = ioutil.Discard // for ease of use, default to using a null logger

	s := &subscriber{
		client:   client,
		logger:   nullLogger,
		handlers: map[string]handler{},
	}

	for _, option := range options {
		option(s)
	}

	return s
}

// AddHandler registers a new handler to be run on changes to the given key
func (s *subscriber) AddHandler(key string, h handler) *subscriber {
	s.logger.WithFields(map[string]interface{}{
		"key": key, "handler": reflect.TypeOf(h).String(),
	}).Info("Registering handler")

	s.handlers[key] = h
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
	for key, handler := range s.handlers {
		getResp, err := s.client.Get(ctx, key)

		if err != nil {
			s.logger.WithField("key", key).WithError(err).Errorf("etcd get failed")
		}

		if len(getResp.Kvs) == 1 {
			value := string(getResp.Kvs[0].Value)

			s.logger.WithFields(map[string]interface{}{
				"key": key, "value": value,
				"handler": reflect.TypeOf(handler).String(),
			}).Info("Triggering handler with initial etcd key value")

			handler.Run(key, string(getResp.Kvs[0].Value))
		}
	}
}

func (s subscriber) watch(ctx context.Context) {
	s.logger.Info("Starting etcd watcher")
	watcher := s.client.Watch(ctx, "/", clientv3.WithPrefix())

	for watcherResponse := range watcher {
		for _, event := range watcherResponse.Events {
			s.processEvent(event)
		}
	}

	s.logger.Info("Finished etcd watcher, channel closed")
}

func (s *subscriber) processEvent(event *clientv3.Event) {
	key := string(event.Kv.Key)
	value := string(event.Kv.Value)
	handler := s.handlers[key]

	contextLogger := s.logger.WithFields(map[string]interface{}{
		"key": key, "value": value,
	})

	contextLogger.Debug("Observed etcd value change")
	if handler == nil {
		contextLogger.Debug("No handler")
		return
	}

	contextLogger = contextLogger.WithField("handler", reflect.TypeOf(handler).String())
	contextLogger.Info("Running handler")

	if err := handler.Run(key, value); err != nil {
		contextLogger.WithError(err).Error("failed to run handler")
	}
}
