package subscriber

import (
	"github.com/coreos/etcd/clientv3"
	"golang.org/x/net/context"
)

type etcd struct {
	watcher  clientv3.Watcher
	handlers map[string]Handler
}

// NewEtcd generates a Daemon with a watcher constructed using the given etcd config
func NewEtcd(watcher clientv3.Watcher) Subscriber {
	return &etcd{
		watcher:  watcher,
		handlers: make(map[string]Handler),
	}
}

// Start creates a new etcd watcher, and will trigger handlers that match the given key
// when values change.
func (s etcd) Start(ctx context.Context, handlers map[string]Handler) error {
	watcher := s.watcher.Watch(ctx, "/", clientv3.WithPrefix())

	for watcherResponse := range watcher {
		for _, event := range watcherResponse.Events {
			key := string(event.Kv.Key)
			handler := handlers[key]

			if handler != nil {
				handler.Run(key, string(event.Kv.Value))
			}
		}
	}

	return nil
}
