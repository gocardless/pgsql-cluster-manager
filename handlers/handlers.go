package handlers

import (
	"reflect"

	"github.com/gocardless/pgsql-novips/util"
	"github.com/sirupsen/logrus"
)

// Handler is the minimal interface for components that should respond to etcd key changes
type Handler interface {
	Run(string, string) error
}

type loggingHandler struct {
	logger *logrus.Logger
	name   string
	Handler
}

// NewLoggingHandler decorates a Handler to enable logging of start/end of Run
func NewLoggingHandler(logger *logrus.Logger, handler Handler) Handler {
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
		if ferr, ok := err.(util.ErrorWithFields); ok {
			logger = logger.WithFields(ferr.Fields)
		}

		logger.Error(err.Error())
	}

	return err
}
