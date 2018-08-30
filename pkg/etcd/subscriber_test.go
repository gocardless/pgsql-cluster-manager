package etcd

import (
	"errors"
	"testing"
	"time"

	"github.com/gocardless/pgsql-cluster-manager/pkg/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

type fakeHandler struct{ mock.Mock }

func (h fakeHandler) Run(key, value string) error {
	args := h.Called(key, value)
	return args.Error(0)
}

func TestSubscriber(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := integration.StartEtcd(t, ctx)

	// Helper to put a value and bail if error
	put := func(key, value string) {
		_, err := client.Put(context.Background(), key, value)
		require.Nil(t, err)
	}

	// Returns true if timed-out waiting for condition to become true
	timeoutUnless := func(condition func() bool) bool {
		timeout := time.After(10 * time.Second)

		for {
			select {
			case <-timeout:
				return true
			default:
				if condition() {
					return false
				}

				<-time.After(25 * time.Millisecond)
			}
		}
	}

	// This handler will log arguments to Run into the entries slice
	arguments := []mock.Arguments{}
	appendArguments := func(args mock.Arguments) {
		arguments = append(arguments, args)
	}

	handler := new(fakeHandler)
	handler.On("Run", "/key", "initial-value").Return(nil).Once().Run(appendArguments)
	handler.On("Run", "/key", "value-after-subscribe").Return(nil).Once().Run(appendArguments)
	handler.On("Run", "/key", "dubious").Return(errors.New("transient error")).Times(3).Run(appendArguments)
	handler.On("Run", "/key", "dubious").Return(nil).Run(appendArguments)

	// Place initial value as we need the subscriber to trigger handlers on initialization
	put("/key", "initial-value")

	sub := NewSubscriber(client, WithRetryInterval(25*time.Millisecond))
	sub.AddHandler("/key", handler)

	go sub.Start(ctx)

	if timeoutUnless(func() bool { return len(arguments) == 1 }) {
		t.Error("timed out waiting for handler to run with initial value")
		return
	}

	put("/key", "value-after-subscribe")

	if timeoutUnless(func() bool { return len(arguments) == 2 }) {
		t.Error("timed out waiting for response to first watched event")
		return
	}

	put("/key", "dubious") // this should cause transient errors

	if timeoutUnless(func() bool { return len(arguments) == 6 }) {
		t.Error("timed out waiting for 'dubious' handler to be retried")
		return
	}

	assert.EqualValues(t, []mock.Arguments{
		mock.Arguments{"/key", "initial-value"},
		mock.Arguments{"/key", "value-after-subscribe"},
		mock.Arguments{"/key", "dubious"}, // fail
		mock.Arguments{"/key", "dubious"}, // fail
		mock.Arguments{"/key", "dubious"}, // fail
		mock.Arguments{"/key", "dubious"}, // success
	}, arguments)
}
