package subscriber

import (
	"github.com/coreos/etcd/clientv3"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
)

type FakeSubscriber struct {
	_Start func(context.Context, map[string]Handler) error
}

func (s FakeSubscriber) Start(ctx context.Context, handlers map[string]Handler) error {
	return s._Start(ctx, handlers)
}

type FakeWatcher struct{ mock.Mock }

func (w FakeWatcher) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	args := w.Called(ctx, key, opts)
	return args.Get(0).(clientv3.WatchChan)
}

func (w FakeWatcher) Close() error {
	args := w.Called()
	return args.Error(0)
}

type FakeHandler struct{ mock.Mock }

func (h FakeHandler) Run(key, value string) error {
	args := h.Called(key, value)
	return args.Error(0)
}
