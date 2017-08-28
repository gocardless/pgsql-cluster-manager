package daemon

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/etcd/clientv3"
	"github.com/gocardless/pgsql-novips/handlers"
	"github.com/gocardless/pgsql-novips/util"
	"github.com/sirupsen/logrus"
)

// Daemon provides the ability to watch for etcd event changes and dispatch events to
// handlers
type Daemon struct {
	watcher   clientv3.Watcher
	handlers  map[string]handlers.Handler
	namespace string
	logger    *logrus.Logger
}

// New generates a Daemon with a watcher constructed using the given etcd config
func New(cfg clientv3.Config, namespace string, logger *logrus.Logger) (*Daemon, error) {
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

	return &Daemon{
		watcher:   watcher,
		namespace: namespace,
		handlers:  make(map[string]handlers.Handler),
		logger:    logger,
	}, nil
}

// RegisterHandler assigns a handler to an etcd key. When the daemon observes this key
// change within the configured namespace,
func (d Daemon) RegisterHandler(key string, handler handlers.Handler) {
	d.handlers[key] = handlers.NewLoggingHandler(d.logger, handler)
}

// Start creates a new etcd watcher, subscribed to keys within namespace, and will trigger
// handlers that match the given key when values change.
func (d Daemon) Start(ctx context.Context) error {
	watcher := d.watcher.Watch(ctx, d.namespace, clientv3.WithPrefix())

	for watcherResponse := range watcher {
		for _, event := range watcherResponse.Events {
			key := strings.TrimPrefix(string(event.Kv.Key), d.namespace)
			value := string(event.Kv.Value)
			handler := d.handlers[key]

			d.logger.
				WithFields(logrus.Fields{"namespace": d.namespace, "key": key, "value": value}).
				Info("Received etcd event")

			if handler != nil {
				handler.Run(key, value)
			}
		}
	}

	return nil
}

// Shutdown closes the etcd watcher, ending the processing of all handlers
func (d Daemon) Shutdown() error {
	return d.watcher.Close()
}
