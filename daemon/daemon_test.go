package daemon

import (
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/stretchr/testify/assert"
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

	watchChan := make(chan clientv3.WatchResponse, 1)
	watchChan <- clientv3.WatchResponse{
		Events: []*clientv3.Event{mockEvent("/postgres/master", "pg01")},
	}

	values := make(chan string, 1)

	defer close(watchChan)
	defer close(values)

	watcher.
		On("Watch", ctx, "/postgres", mock.AnythingOfType("[]clientv3.OpOption")).
		Return((clientv3.WatchChan)(watchChan))

	go func() {
		Daemon{watcher}.Start(ctx, "/postgres", HandlerMap{
			"/master": func(value string) error {
				values <- value
				return nil
			},
		})
	}()

	select {
	case value := <-values:
		watcher.AssertExpectations(t)
		assert.Equal(t, value, "pg01", "expected to receive value in watched event")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler to be executed")
	}
}
