package routes

import (
	"context"
	"time"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/mock"
)

type fakeClock struct{ mock.Mock }

func (c fakeClock) Now() time.Time {
	args := c.Called()
	return args.Get(0).(time.Time)
}

func (c fakeClock) Until(t time.Time) time.Duration {
	args := c.Called(t)
	return args.Get(0).(time.Duration)
}

type fakePGBouncerPauser struct{ mock.Mock }

func (b fakePGBouncerPauser) Pause(ctx context.Context) error {
	args := b.Called(ctx)
	return args.Error(0)
}

func (b fakePGBouncerPauser) Resume(ctx context.Context) error {
	args := b.Called(ctx)
	return args.Error(0)
}

type fakeCib struct{ mock.Mock }

func (c fakeCib) Get(xpaths ...string) ([]*etree.Element, error) {
	args := c.Called(xpaths)
	return args.Get(0).([]*etree.Element), args.Error(1)
}

func (c fakeCib) Migrate(to string) error {
	args := c.Called(to)
	return args.Error(0)
}
