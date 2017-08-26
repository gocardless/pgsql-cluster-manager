package daemon

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/etcd/clientv3"
	"github.com/gocardless/pgsql-novips/util"
)

// Daemon provides the ability to watch for etcd event changes and dispatch events to
// handlers
type Daemon struct {
	watcher clientv3.Watcher
}

// HandlerMap is used to configure mapping from etcd key changes to handler
// functions
type HandlerMap map[string]Handler

// Handler is a function that receives an string value of an etcd key and takes
// action on it
type Handler func(string) error

// New generates a Daemon with a watcher constructed using the given etcd config
func New(cfg clientv3.Config) (*Daemon, error) {
	watcher, err := clientv3.New(cfg)

	if err != nil {
		return nil, util.NewErrorWithFields(
			"failed to connect to etcd",
			map[string]interface{}{
				"error":  err.Error(),
				"config": fmt.Sprintf("%v", cfg),
			},
		)
	}

	return &Daemon{watcher: watcher}, nil
}

// Start creates a new etcd watcher, subscribed to keys within namespace, and will trigger
// handlers that match the given key when values change.
func (d Daemon) Start(ctx context.Context, namespace string, handlers HandlerMap) error {
	watcher := d.watcher.Watch(ctx, namespace, clientv3.WithPrefix())

	for watcherResponse := range watcher {
		for _, event := range watcherResponse.Events {
			key := strings.TrimPrefix(string(event.Kv.Key), namespace)

			if handler := handlers[key]; handler != nil {
				handler(string(event.Kv.Value))
			}
		}
	}

	return nil
}

// Shutdown closes the etcd watcher, ending the processing of all handlers
func (d Daemon) Shutdown() error {
	return d.watcher.Close()
}
