package subscriber

import (
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
)

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
		Events: []*clientv3.Event{mockEvent("/master", "pg01")},
	}

	done := make(chan interface{}, 1)

	defer close(watchChan)
	defer close(done)

	watcher.
		On("Watch", ctx, "/", mock.AnythingOfType("[]clientv3.OpOption")).
		Return((clientv3.WatchChan)(watchChan))

	handler := FakeHandler{}

	// Expect that we receive the key without the namespace prefix
	handler.On("Run", "/master", "pg01").Return(nil).Run(func(args mock.Arguments) {
		done <- struct{}{}
	})

	go etcd{watcher: watcher}.Start(ctx, map[string]Handler{
		"/master": handler,
	})

	select {
	case <-done:
		watcher.AssertExpectations(t)
		handler.AssertExpectations(t)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler to be executed")
	}
}
