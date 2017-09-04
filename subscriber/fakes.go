package subscriber

import (
	"github.com/coreos/etcd/clientv3"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
)

type FakeSubscriber struct{ mock.Mock }

func (s FakeSubscriber) Start(ctx context.Context, handlers map[string]Handler) error {
	args := s.Called(ctx, handlers)
	return args.Error(0)
}

func (s FakeSubscriber) work(handler Handler, key string, value string) error {
	args := s.Called(handler, key, value)
	return args.Error(0)
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

type FakeHandler struct {
	mock.Mock
	_Run func(string, string) error
}

func (h FakeHandler) Run(key, value string) error {
	args := h.Called(key, value)
	if h._Run != nil {
		h._Run(key, value)
	}
	return args.Error(0)
}
