package migration

import (
	"context"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func DefaultConfig(client etcdClient) MigrationConfig {
	return MigrationConfig{
		Logger:        debugLogger(),
		Etcd:          client,
		EtcdMasterKey: "/master",
		Clients:       []MigrationClient{},
		PauseTimeout:  5 * time.Second,
		PauseExpiry:   5 * time.Second,
	}
}

func debugLogger() *logrus.Logger {
	logger := logrus.StandardLogger()
	logger.SetLevel(logrus.DebugLevel)

	return logger
}

func makeKv(key, value string) *mvccpb.KeyValue {
	return &mvccpb.KeyValue{
		Key:   []byte(key),
		Value: []byte(value),
	}
}

type fakeEtcdClient struct{ mock.Mock }

func (c *fakeEtcdClient) Get(ctx context.Context, key string, options ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	params := []interface{}{ctx, key}
	for _, option := range options {
		params = append(params, option)
	}

	args := c.Called(params...)
	return args.Get(0).(*clientv3.GetResponse), args.Error(1)
}

func (c *fakeEtcdClient) Watch(ctx context.Context, key string, options ...clientv3.OpOption) clientv3.WatchChan {
	params := []interface{}{ctx, key}
	for _, option := range options {
		params = append(params, option)
	}

	args := c.Called(params...)
	return args.Get(0).(clientv3.WatchChan)
}

func TestMigrationHasBecomeMaster(t *testing.T) {
	testCases := []struct {
		name          string
		initialMaster string
		keyValueFeed  []*mvccpb.KeyValue
	}{
		{
			name:          "changes to master after watch",
			initialMaster: "pg01",
			keyValueFeed: []*mvccpb.KeyValue{
				makeKv("/master", "pg02"),
				makeKv("/master", "pg03"),
			},
		},
		{
			name:          "already target master",
			initialMaster: "pg03",
			keyValueFeed:  []*mvccpb.KeyValue{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockContext := mock.AnythingOfType("*context.emptyCtx")

			client := &fakeEtcdClient{}
			client.On("Get", mockContext, "/master").Return(&clientv3.GetResponse{
				Kvs: []*mvccpb.KeyValue{makeKv("/master", tc.initialMaster)},
			}, nil).Once()

			watchChan := make(chan clientv3.WatchResponse, len(tc.keyValueFeed))
			defer close(watchChan)

			// Use this function to cast the channel to the correct type
			func(c clientv3.WatchChan) { client.On("Watch", mockContext, "/master").Return(c) }(watchChan)

			// Push the fixture key value pairs down the watch channel
			for _, kv := range tc.keyValueFeed {
				watchChan <- clientv3.WatchResponse{
					Events: []*clientv3.Event{&clientv3.Event{Kv: kv}},
				}
			}

			m := NewMigration(DefaultConfig(client))

			select {
			case <-m.HasBecomeMaster(context.Background(), "pg03"):
				return // success!
			case <-time.After(time.Second):
				assert.Fail(t, "timed out waiting to detect change to master")
			}
		})
	}
}
