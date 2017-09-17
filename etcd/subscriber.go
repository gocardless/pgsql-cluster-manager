package etcd

import (
	"github.com/coreos/etcd/clientv3"
	"golang.org/x/net/context"
)

type subscriber struct {
	client   *clientv3.Client
	handlers map[string]Handler
}

type Handler interface {
	Run(string, string) error
}

// NewSubscriber generates a Daemon with a watcher constructed using the given etcd config
func NewSubscriber(client *clientv3.Client) *subscriber {
	return &subscriber{
		client:   client,
		handlers: make(map[string]Handler),
	}
}

// Start creates a new etcd watcher, and will trigger handlers that match the given key
// when values change.
func (s subscriber) Start(ctx context.Context, handlers map[string]Handler) error {
	watcher := s.client.Watch(ctx, "/", clientv3.WithPrefix())

	// Make sure we trigger handlers for initial key values
	for key, handler := range handlers {
		getResp, err := s.client.Get(ctx, key)

		if err != nil {
			return err
		}

		if len(getResp.Kvs) == 1 {
			handler.Run(key, string(getResp.Kvs[0].Value))
		}
	}

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
