package etcd

import (
	"io/ioutil"
	"reflect"

	"github.com/coreos/etcd/clientv3"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

type subscriber struct {
	client *clientv3.Client
	logger *logrus.Logger
}

// Handler exports the interface we expect subscriber handlers to implement. This is only
// exported because we have to create a map outside of the package.
type Handler interface {
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
		client: client,
		logger: nullLogger,
	}

	for _, option := range options {
		option(s)
	}

	return s
}

// Start creates a new etcd watcher, and will trigger handlers that match the given key
// when values change.
func (s subscriber) Start(ctx context.Context, handlers map[string]Handler) error {
	// Make sure we trigger handlers for initial key values
	for key, handler := range handlers {
		getResp, err := s.client.Get(ctx, key)

		if err != nil {
			s.logger.WithField("key", key).WithError(err).Errorf("etcd get failed")
			return err
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

	s.logger.Info("Starting etcd watcher")
	watcher := s.client.Watch(ctx, "/", clientv3.WithPrefix())

	for watcherResponse := range watcher {
		for _, event := range watcherResponse.Events {
			s.processEvent(handlers, event)
		}
	}

	s.logger.Info("Finished etcd subscriber, watch channel closed")
	return nil
}

func (s subscriber) processEvent(handlers map[string]Handler, event *clientv3.Event) {
	key := string(event.Kv.Key)
	value := string(event.Kv.Value)
	handler := handlers[key]

	contextLogger := s.logger.WithFields(map[string]interface{}{
		"key": key, "value": value,
	})

	contextLogger.Debug("Observed etcd value change")

	if handler == nil {
		contextLogger.Debug("No handler")
	}

	contextLogger = contextLogger.WithField("handler", reflect.TypeOf(handler).String())
	contextLogger.Info("Running handler")

	handler.Run(key, value)
}
