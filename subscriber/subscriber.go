package subscriber

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

type Subscriber interface {
	RegisterHandler(string, Handler)
	Start(context.Context) error
	Shutdown() error
}

func NewLoggingSubscriber(logger *logrus.Logger, subscriber Subscriber) Subscriber {
	return &loggingSubscriber{logger, subscriber}
}

type loggingSubscriber struct {
	logger *logrus.Logger
	Subscriber
}

func (s loggingSubscriber) RegisterHandler(key string, handler Handler) {
	s.Subscriber.RegisterHandler(key, newLoggingHandler(s.logger, handler))
}

func (s loggingSubscriber) Start(ctx context.Context) error {
	s.logger.Info("Starting subscriber")
	defer s.logger.Info("Finished subscriber")

	return s.Subscriber.Start(ctx)
}
