package subscriber

import (
	"errors"
	"testing"

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
					Message: "Running...",
					Data: logrus.Fields{
						"handler": "FakeHandler",
						"key":     "key",
						"value":   "value",
					},
				},
				logEntry{
					Message: "Finished!",
					Data: logrus.Fields{
						"handler": "FakeHandler",
						"key":     "key",
						"value":   "value",
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
					Message: "Running...",
					Data: logrus.Fields{
						"handler": "FakeHandler",
						"key":     "key",
						"value":   "value",
					},
				},
				logEntry{
					Message: "uh oh",
					Data: logrus.Fields{
						"handler": "FakeHandler",
						"key":     "key",
						"value":   "value",
						"error":   "spaghettios",
					},
				},
				logEntry{
					Message: "Finished!",
					Data: logrus.Fields{
						"handler": "FakeHandler",
						"key":     "key",
						"value":   "value",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger, hook := test.NewNullLogger()
			sub := new(FakeSubscriber)
			handler := FakeHandler{}

			sub.On("work", handler, "key", "value").Return(tc.handlerError).Once()
			Log(logger, sub).work(handler, "key", "value")
			sub.AssertExpectations(t)

			for idx, expected := range tc.logEntries {
				assert.Equal(t, logEntry{hook.Entries[idx].Message, hook.Entries[idx].Data}, expected)
			}
		})
	}
}
