package daemon

import (
	"testing"
	"time"

	"github.com/Sirupsen/logrus/hooks/test"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/gocardless/pgsql-novips/handlers"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
)

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
	h._Run(key, value)
	return args.Error(0)
}

func mockEvent(key, value string) *clientv3.Event {
	return &clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte(key),
			Value: []byte(value),
		},
	}
}

func TestStart_CallsHandlersOnEvents(t *testing.T) {
	watcher := FakeWatcher{}
	ctx := context.Background()
	logger, _ := test.NewNullLogger()

	watchChan := make(chan clientv3.WatchResponse, 1)
	watchChan <- clientv3.WatchResponse{
		Events: []*clientv3.Event{mockEvent("/postgres/master", "pg01")},
	}

	done := make(chan interface{}, 1)

	defer close(watchChan)
	defer close(done)

	watcher.
		On("Watch", ctx, "/postgres", mock.AnythingOfType("[]clientv3.OpOption")).
		Return((clientv3.WatchChan)(watchChan))

	handler := FakeHandler{
		_Run: func(key, value string) error {
			done <- struct{}{}
			return nil
		},
	}

	// Expect that we receive the key without the namespace prefix
	handler.On("Run", "/master", "pg01").Return(nil)

	go Daemon{
		watcher:   watcher,
		namespace: "/postgres",
		logger:    logger,
		handlers: map[string]handlers.Handler{
			"/master": handler,
		},
	}.Start(ctx)

	select {
	case <-done:
		watcher.AssertExpectations(t)
		handler.AssertExpectations(t)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler to be executed")
	}
}
