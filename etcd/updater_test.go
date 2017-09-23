package etcd

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/gocardless/pgsql-cluster-manager/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var seededRandom = rand.New(rand.NewSource(time.Now().UnixNano()))
var charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// generateKey will create a key that can be used in each of our etcd tests, ensuring we
// test against different keys for each test even if re-using the same etcd instance.
func generateKey() string {
	keyBytes := make([]byte, 20)
	for idx := range keyBytes {
		keyBytes[idx] = charset[seededRandom.Intn(len(charset))]
	}

	return fmt.Sprintf("/%s", keyBytes)
}

func TestUpdater(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := integration.StartEtcd(t, ctx)
	updater := Updater{client}

	put := func(key, value string) *clientv3.PutResponse {
		resp, err := client.Put(context.Background(), key, value)
		require.Nil(t, err)

		return resp
	}

	get := func(key string) *clientv3.GetResponse {
		resp, err := client.Get(context.Background(), key)
		require.Nil(t, err)

		return resp
	}

	testCases := []struct {
		name         string
		initialValue string
		runValue     string
		revisionDiff int
	}{
		{
			"when etcd has different value, updates etcd",
			"value",
			"new-value",
			1,
		},
		{
			"when etcd matches value, does not update etcd",
			"value",
			"value",
			0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := generateKey()

			put(key, tc.initialValue)
			originalRevision := get(key).Kvs[0].ModRevision

			err := updater.Run(key, tc.runValue)
			keyAfterHandler := get(key).Kvs[0]

			// In all cases, we should find that etcd is updated with the value that the handler
			// has been run with.
			afterValue := tc.runValue

			assert.Nil(t, err)
			assert.EqualValues(t, afterValue, keyAfterHandler.Value)
			assert.EqualValues(t, tc.revisionDiff, keyAfterHandler.ModRevision-originalRevision)
		})
	}
}
