package handlers

import (
	"testing"

	"github.com/gocardless/pgsql-novips/util"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type FakeHandler struct{ mock.Mock }

func (h FakeHandler) Run(key, value string) error {
	args := h.Called(key, value)
	return args.Error(0)
}

type logEntry struct {
	Message string
	Data    logrus.Fields
}

func TestLoggingHandler(t *testing.T) {
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
					Message: "Running handler...",
					Data: logrus.Fields{
						"handler": "FakeHandler",
						"key":     "key",
						"value":   "value",
					},
				},
				logEntry{
					Message: "Finished running handler",
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
			util.NewErrorWithFields(
				"uh oh",
				map[string]interface{}{"error": "spaghettios"},
			),
			[]logEntry{
				logEntry{
					Message: "Running handler...",
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
					Message: "Finished running handler",
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
			handler := FakeHandler{}

			handler.On("Run", "key", "value").Return(tc.handlerError).Once()

			loggingHandler := NewLoggingHandler(logger, handler)
			loggingHandler.Run("key", "value")

			handler.AssertExpectations(t)

			for idx, expected := range tc.logEntries {
				assert.Equal(t, logEntry{hook.Entries[idx].Message, hook.Entries[idx].Data}, expected)
			}
		})
	}
}
