package subscriber

import (
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

func (s loggingSubscriber) Start(ctx context.Context, handlers map[string]Handler) error {
	entry := s.Logger.WithField("subscriber", reflect.TypeOf(s.Subscriber).Name())

	entry.Info("Starting...")
	defer entry.Info("Finished!")

	return s.Subscriber.Start(ctx, handlers)
}

func (s loggingSubscriber) work(handler Handler, key, value string) error {
	entry := s.Logger.WithFields(logrus.Fields{
		"handler": reflect.TypeOf(handler).Name(),
		"key":     key,
		"value":   value,
	})

	entry.Info("Running...")
	defer entry.Info("Finished!")

	err := s.Subscriber.work(handler, key, value)

	if err != nil {
		if ferr, ok := err.(interface {
			Fields() map[string]interface{}
		}); ok {
			entry = entry.WithFields(ferr.Fields())
		}

		entry.Error(err.Error())
	}

	return err
}
