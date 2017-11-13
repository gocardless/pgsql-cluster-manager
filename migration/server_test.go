package migration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/beevik/etree"
	"github.com/gocardless/pgsql-cluster-manager/pacemaker"
	"github.com/stretchr/testify/assert"
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

func TestServerPause(t *testing.T) {
	testCases := []struct {
		name          string
		request       *PauseRequest
		pauserError   error
		waitForResume bool
		responseError error
	}{
		{
			name:          "when pause is successful",
			request:       &PauseRequest{Timeout: 5, Expiry: 0},
			pauserError:   nil,
			waitForResume: false,
			responseError: nil,
		},
		{
			name:          "when pause is successful and expiry elapses, we resume",
			request:       &PauseRequest{Timeout: 5, Expiry: 20},
			pauserError:   nil,
			waitForResume: true,
			responseError: nil,
		},
		{
			name:          "when pause fails",
			request:       &PauseRequest{Timeout: 5, Expiry: 0},
			pauserError:   errors.New("no stopping this"),
			waitForResume: false,
			responseError: status.Errorf(codes.Unknown, "unknown error: no stopping this"),
		},
		{
			name:          "when pause times out",
			request:       &PauseRequest{Timeout: 0, Expiry: 0},
			pauserError:   errors.New("context expired"),
			waitForResume: false,
			responseError: status.Errorf(codes.DeadlineExceeded, "exceeded pause timeout"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pauser := new(fakePauser)
			pauser.On("Pause", mock.AnythingOfType("*context.timerCtx")).Return(tc.pauserError)
			defer pauser.AssertExpectations(t)

			clock := new(fakeClock)
			clock.On("Now").Return(time.Now())

			var wg sync.WaitGroup

			if tc.waitForResume {
				wg.Add(1) // we want to wait once a response is returned for the resume to happen

				pauser.
					On("Resume", mock.AnythingOfType("*context.emptyCtx")).Return(nil).
					Run(func(args mock.Arguments) { wg.Done() })

				// We would normally use the clock to defer a resume, but we want this to be
				// instant in tests, so return zero to schedule immediately
				clock.On("Until", mock.AnythingOfType("time.Time")).Return(time.Duration(0))
			}

			server := NewServer(WithPGBouncer(pauser), WithClock(clock))
			_, err := server.Pause(context.Background(), tc.request)

			assert.EqualValues(t, tc.responseError, err)

			resumeCalled := make(chan struct{}, 1)
			go func() { wg.Wait(); close(resumeCalled) }()

			select {
			case <-time.After(5 * time.Second):
				assert.Fail(t, "timed out waiting for resume to be called")
			case <-resumeCalled:
				// we're good, proceed
			}
		})
	}
}

func createElement(uname string) *etree.Element {
	return &etree.Element{
		Attr: []etree.Attr{
			etree.Attr{"", "uname", uname},
			etree.Attr{"", "id", "1"},
		},
	}
}

func TestMigrate(t *testing.T) {
	testCases := []struct {
		name                string
		crmError            error
		syncElement         *etree.Element
		resolveAddressValue string
		resolveAddressError error
		shouldMigrate       bool
		migrateTo           string // empty string means don't migrate
		migrateAddress      string
		migrateError        error
		responseError       error
	}{
		{
			name:                "when successfull",
			crmError:            nil,
			syncElement:         createElement("pg03"),
			resolveAddressValue: "172.0.1.1",
			resolveAddressError: nil,
			shouldMigrate:       true,
			migrateTo:           "pg03",
			migrateAddress:      "172.0.1.1",
			migrateError:        nil,
			responseError:       nil,
		},
		{
			name:                "when crm query fails",
			crmError:            errors.New("oops"),
			syncElement:         nil,
			resolveAddressValue: "172.0.1.1",
			resolveAddressError: nil,
			shouldMigrate:       false,
			migrateTo:           "",
			migrateAddress:      "172.0.1.1",
			migrateError:        nil,
			responseError:       status.Errorf(codes.Unknown, "failed to query cib: oops"),
		},
		{
			name:                "when no sync node is found",
			crmError:            nil,
			syncElement:         nil,
			resolveAddressValue: "172.0.1.1",
			resolveAddressError: nil,
			shouldMigrate:       false,
			migrateTo:           "",
			migrateAddress:      "172.0.1.1",
			migrateError:        nil,
			responseError:       status.Errorf(codes.NotFound, "failed to find sync node"),
		},
		{
			name:                "when unable to resolve sync node IP address",
			crmError:            nil,
			syncElement:         nil,
			resolveAddressValue: "",
			resolveAddressError: errors.New("corosync-cfgtool: not in $PATH"),
			shouldMigrate:       false,
			migrateTo:           "",
			migrateAddress:      "",
			migrateError:        nil,
			responseError:       status.Errorf(codes.NotFound, "failed to find sync node"),
		},
		{
			name:                "when crm migration fails",
			crmError:            nil,
			syncElement:         createElement("pg03"),
			resolveAddressValue: "172.0.1.1",
			resolveAddressError: nil,
			shouldMigrate:       true,
			migrateTo:           "pg03",
			migrateAddress:      "172.0.1.1",
			migrateError:        errors.New("cannot find crm in PATH"),
			responseError: status.Errorf(
				codes.Unknown, "'crm resource migrate pg03' failed: cannot find crm in PATH",
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clock := new(fakeClock)
			clock.On("Now").Return(time.Now())

			crm := new(fakeCrm)
			bgCtx := context.Background()

			crm.
				On("Get", bgCtx, []string{pacemaker.SyncXPath}).
				Return([]*etree.Element{tc.syncElement}, tc.crmError)

			crm.
				On("ResolveAddress", bgCtx, "1").
				Return(tc.resolveAddressValue, tc.resolveAddressError)

			crm.On("Migrate", bgCtx, tc.migrateTo).Return(tc.migrateError)

			server := NewServer(WithPacemaker(crm), WithClock(clock))
			_, err := server.Migrate(bgCtx, nil)

			assert.EqualValues(t, tc.responseError, err)
		})
	}
}
