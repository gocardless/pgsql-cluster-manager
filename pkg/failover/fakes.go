package failover

import (
	"context"
	"time"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/mock"
)

type fakePauser struct{ mock.Mock }

func (p fakePauser) Pause(ctx context.Context) error {
	args := p.Called(ctx)
	return args.Error(0)
}

func (p fakePauser) Resume(ctx context.Context) error {
	args := p.Called(ctx)
	return args.Error(0)
}

type fakeClock struct{ mock.Mock }

func (c fakeClock) Now() time.Time {
	args := c.Called()
	return args.Get(0).(time.Time)
}

func (c fakeClock) Until(t time.Time) time.Duration {
	args := c.Called(t)
	return args.Get(0).(time.Duration)
}

type fakeCrm struct{ mock.Mock }

func (c fakeCrm) Get(ctx context.Context, xpaths ...string) ([]*etree.Element, error) {
	args := c.Called(ctx, xpaths)
	return args.Get(0).([]*etree.Element), args.Error(1)
}

func (c fakeCrm) ResolveAddress(ctx context.Context, nodeID string) (string, error) {
	args := c.Called(ctx, nodeID)
	return args.String(0), args.Error(1)
}

func (c fakeCrm) Migrate(ctx context.Context, to string) error {
	args := c.Called(ctx, to)
	return args.Error(0)
}

func (c fakeCrm) Unmigrate(ctx context.Context) error {
	args := c.Called(ctx)
	return args.Error(0)
}
