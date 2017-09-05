package subscriber

import (
	"fmt"
	"reflect"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

func Log(logger *logrus.Logger, subscriber Subscriber) Subscriber {
	return &loggingSubscriber{logger, subscriber}
}

type loggingSubscriber struct {
	*logrus.Logger
	Subscriber
}

type loggingHandler struct {
	loggingSubscriber
	Handler
}

func (s loggingSubscriber) Start(ctx context.Context, handlers map[string]Handler) error {
	entry := s.Logger.WithFields(map[string]interface{}{
		"subscriber":  reflect.TypeOf(s.Subscriber).String(),
		"handlerKeys": fmt.Sprintf("%v", reflect.ValueOf(handlers).MapKeys()),
	})

	entry.Info("Starting...")

	loggingHandlers := make(map[string]Handler)
	for key, handler := range handlers {
		loggingHandlers[key] = loggingHandler{s, handler}
	}

	err := s.Subscriber.Start(ctx, loggingHandlers)

	if err != nil {
		entry.WithFields(extractFields(err)).Error(err.Error())
	} else {
		entry.Info("Finished!")
	}

	return err
}

func (h loggingHandler) Run(key, value string) error {
	entry := h.Logger.WithFields(logrus.Fields{
		"subscriber": reflect.TypeOf(h.loggingSubscriber.Subscriber).String(),
		"handler":    reflect.TypeOf(h.Handler).String(),
		"key":        key,
		"value":      value,
	})

	entry.Info("Running...")

	err := h.Handler.Run(key, value)

	if err != nil {
		entry.WithFields(extractFields(err)).Error(err.Error())
	} else {
		entry.Info("Finished!")
	}

	return err
}

func extractFields(err error) map[string]interface{} {
	if ferr, ok := err.(interface {
		Fields() map[string]interface{}
	}); ok {
		return ferr.Fields()
	}

	return make(map[string]interface{})
}
