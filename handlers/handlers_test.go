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

func TestLoggingHandler_WhenSuccessful(t *testing.T) {
	logger, hook := test.NewNullLogger()
	handler := FakeHandler{}

	handler.On("Run", "key", "value").Return(nil)

	loggingHandler := NewLoggingHandler(logger, handler)
	loggingHandler.Run("key", "value")

	handler.AssertExpectations(t)

	expectedEntries := []logEntry{
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
	}

	for idx, expected := range expectedEntries {
		assert.Equal(t, logEntry{hook.Entries[idx].Message, hook.Entries[idx].Data}, expected)
	}
}

func TestLoggingHandler_WhenErrors(t *testing.T) {
	logger, hook := test.NewNullLogger()
	handler := FakeHandler{}

	handler.
		On("Run", "key", "value").
		Return(util.NewErrorWithFields(
			"uh oh",
			map[string]interface{}{"error": "spaghettios"},
		))

	loggingHandler := NewLoggingHandler(logger, handler)
	loggingHandler.Run("key", "value")

	handler.AssertExpectations(t)

	expectedEntries := []logEntry{
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
	}

	for idx, expected := range expectedEntries {
		assert.Equal(t, logEntry{hook.Entries[idx].Message, hook.Entries[idx].Data}, expected)
	}
}
