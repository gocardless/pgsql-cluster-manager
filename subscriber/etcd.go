package subscriber

import (
	"fmt"
	"strings"

	"github.com/coreos/etcd/clientv3"
	"github.com/gocardless/pgsql-novips/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

type etcd struct {
	watcher   clientv3.Watcher
	handlers  map[string]Handler
	namespace string
	logger    *logrus.Logger
}

// New generates a Daemon with a watcher constructed using the given etcd config
func NewEtcd(cfg clientv3.Config, namespace string, logger *logrus.Logger) (Subscriber, error) {
	watcher, err := clientv3.New(cfg)

	if err != nil {
		return nil, util.NewErrorWithFields(
			"Failed to connect to etcd",
			map[string]interface{}{
				"error":  err.Error(),
				"config": fmt.Sprintf("%v", cfg),
			},
		)
	}

	return &etcd{
		watcher:   watcher,
		namespace: namespace,
		handlers:  make(map[string]Handler),
		logger:    logger,
	}, nil
}

// RegisterHandler assigns a handler to an etcd key. When the daemon observes this key
// change within the configured namespace,
func (s etcd) RegisterHandler(key string, handler Handler) {
	s.handlers[key] = newLoggingHandler(s.logger, handler)
}

// Start creates a new etcd watcher, subscribed to keys within namespace, and will trigger
// handlers that match the given key when values change.
func (s etcd) Start(ctx context.Context) error {
	watcher := s.watcher.Watch(ctx, s.namespace, clientv3.WithPrefix())

	for watcherResponse := range watcher {
		for _, event := range watcherResponse.Events {
			key := strings.TrimPrefix(string(event.Kv.Key), s.namespace)
			value := string(event.Kv.Value)
			handler := s.handlers[key]

			s.logger.
				WithFields(logrus.Fields{"namespace": s.namespace, "key": key, "value": value}).
				Info("Received etcd event")

			if handler != nil {
				handler.Run(key, value)
			}
		}
	}

	return nil
}

// Shutdown closes the etcd watcher, ending the processing of all handlers
func (s etcd) Shutdown() error {
	return s.watcher.Close()
}
