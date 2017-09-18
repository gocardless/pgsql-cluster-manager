package etcd

import (
	"fmt"
	"testing"
	"time"

	"github.com/gocardless/pgsql-cluster-manager/testHelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

type fakeHandler struct {
	_Run func(string, string) error
}

func (h fakeHandler) Run(key, value string) error {
	return h._Run(key, value)
}

func TestSubscriber(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	etcd := testHelpers.StartEtcd(t, ctx)

	// Helper to put a value and bail if error
	put := func(key, value string) {
		_, err := etcd.Put(context.Background(), key, value)
		require.Nil(t, err)
	}

	// This handler will log arguments to Run into the entries slice
	entries := []string{}
	h := fakeHandler{
		_Run: func(key, value string) error {
			entries = append(entries, fmt.Sprintf("%s=%s", key, value))
			return nil
		},
	}

	// Place value in /a as we want the subscriber to trigger handlers on initialization
	put("/a", "initial")

	go func() {
		sub := NewSubscriber(etcd).
			AddHandler("/a", h).
			AddHandler("/b", h)

		sub.Start(ctx)
	}()

	// Wait for the subscriber to establish a connection
	<-time.After(100 * time.Millisecond)

	put("/a", "after-subscribe")
	put("/b", "initial-after-subscribe")

	put("/a", "final-and-third")
	put("/b", "final-and-second")

	// Wait for either a timeout, or for the expected number of log entries to appear
	func() {
		timeout := time.After(time.Second)
		for {
			select {
			case <-timeout:
				return
			default:
				if len(entries) == 5 {
					return
				}
			}
		}
	}()

	assert.EqualValues(t, []string{
		"/a=initial",
		"/a=after-subscribe",
		"/b=initial-after-subscribe",
		"/a=final-and-third",
		"/b=final-and-second",
	}, entries)
}
