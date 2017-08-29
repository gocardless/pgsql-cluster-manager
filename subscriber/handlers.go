package subscriber

import (
	"reflect"

	"github.com/sirupsen/logrus"
)

// Handler is the minimal interface that should respond to Subscriber events
type Handler interface {
	Run(string, string) error
}

type loggingHandler struct {
	logger *logrus.Logger
	name   string
	Handler
}

func newLoggingHandler(logger *logrus.Logger, handler Handler) Handler {
	return &loggingHandler{logger, reflect.TypeOf(handler).Name(), handler}
}

func (h loggingHandler) Run(key, value string) error {
	logger := h.logger.WithFields(logrus.Fields{
		"key":     key,
		"value":   value,
		"handler": h.name,
	})

	logger.Info("Running handler...")
	defer logger.Info("Finished running handler")

	err := h.Handler.Run(key, value)

	if err != nil {
		if ferr, ok := err.(interface {
			Fields() map[string]interface{}
		}); ok {
			logger = logger.WithFields(ferr.Fields())
		}

		logger.Error(err.Error())
	}

	return err
}
