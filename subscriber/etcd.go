package subscriber

import (
	"strings"

	"github.com/coreos/etcd/clientv3"
	"golang.org/x/net/context"
)

type etcd struct {
	watcher   clientv3.Watcher
	handlers  map[string]Handler
	namespace string
}

// NewEtcd generates a Daemon with a watcher constructed using the given etcd config
func NewEtcd(watcher clientv3.Watcher, namespace string) Subscriber {
	return &etcd{
		watcher:   watcher,
		namespace: namespace,
		handlers:  make(map[string]Handler),
	}
}

// Start creates a new etcd watcher, subscribed to keys within namespace, and will trigger
// handlers that match the given key when values change.
func (s etcd) Start(ctx context.Context, handlers map[string]Handler) error {
	watcher := s.watcher.Watch(ctx, s.namespace, clientv3.WithPrefix())

	for watcherResponse := range watcher {
		for _, event := range watcherResponse.Events {
			key := strings.TrimPrefix(string(event.Kv.Key), s.namespace)
			value := string(event.Kv.Value)
			handler := handlers[key]

			if handler != nil {
				handler.Run(key, value)
			}
		}
	}

	return nil
}
