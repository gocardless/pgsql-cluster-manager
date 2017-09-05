package subscriber

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

type logEntry struct {
	Message string
	Data    logrus.Fields
}

type errorWithFields struct {
	error
	fields map[string]interface{}
}

func (e errorWithFields) Fields() map[string]interface{} {
	return e.fields
}

func TestLog(t *testing.T) {
	testCases := []struct {
		name         string
		handlerError error
		logEntries   []logEntry
	}{
		{
			"log when successful",
			nil,
			[]logEntry{
				logEntry{
					Message: "Starting...",
					Data: logrus.Fields{
						"subscriber":  "subscriber.FakeSubscriber",
						"handlerKeys": "[key]",
					},
				},
				logEntry{
					Message: "Running...",
					Data: logrus.Fields{
						"handler":    "subscriber.FakeHandler",
						"subscriber": "subscriber.FakeSubscriber",
						"key":        "key",
						"value":      "value",
					},
				},
				logEntry{
					Message: "Finished!",
					Data: logrus.Fields{
						"handler":    "subscriber.FakeHandler",
						"subscriber": "subscriber.FakeSubscriber",
						"key":        "key",
						"value":      "value",
					},
				},
				logEntry{
					Message: "Finished!",
					Data: logrus.Fields{
						"subscriber":  "subscriber.FakeSubscriber",
						"handlerKeys": "[key]",
					},
				},
			},
		},
		{
			"log when fails",
			errorWithFields{
				errors.New("uh oh"),
				map[string]interface{}{"error": "spaghettios"},
			},
			[]logEntry{
				logEntry{
					Message: "Starting...",
					Data: logrus.Fields{
						"subscriber":  "subscriber.FakeSubscriber",
						"handlerKeys": "[key]",
					},
				},
				logEntry{
					Message: "Running...",
					Data: logrus.Fields{
						"handler":    "subscriber.FakeHandler",
						"subscriber": "subscriber.FakeSubscriber",
						"key":        "key",
						"value":      "value",
					},
				},
				logEntry{
					Message: "uh oh",
					Data: logrus.Fields{
						"handler":    "subscriber.FakeHandler",
						"subscriber": "subscriber.FakeSubscriber",
						"key":        "key",
						"value":      "value",
						"error":      "spaghettios",
					},
				},
				logEntry{
					Message: "Finished!",
					Data: logrus.Fields{
						"subscriber":  "subscriber.FakeSubscriber",
						"handlerKeys": "[key]",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger, hook := test.NewNullLogger()
			handler := FakeHandler{}

			sub := FakeSubscriber{
				_Start: func(ctx context.Context, handlers map[string]Handler) error {
					handlers["key"].Run("key", "value")
					return nil
				},
			}

			handler.On("Run", "key", "value").Return(tc.handlerError).Once()

			Log(logger, sub).Start(context.Background(), map[string]Handler{"key": handler})

			handler.AssertExpectations(t)

			for _, e := range hook.Entries {
				fmt.Printf("%#v\n", e.Message)
			}

			for idx, expected := range tc.logEntries {
				assert.Equal(t, expected, logEntry{hook.Entries[idx].Message, hook.Entries[idx].Data})
			}
		})
	}
}
