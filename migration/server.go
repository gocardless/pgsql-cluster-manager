package migration

import (
	"context"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/beevik/etree"
	"github.com/gocardless/pgsql-cluster-manager/pacemaker"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type migrationServer struct {
	logger    *logrus.Logger
	PGBouncer pgBouncerPauser
	crm
	clock
}

// Define a slim interface to enable easier testing
type pgBouncerPauser interface {
	Pause(context.Context) error
	Resume(context.Context) error
}

// This allows stubbing of time in tests, but would normally delegate to the time package
type clock interface {
	Now() time.Time
	Until(time.Time) time.Duration
}

type realClock struct{}

func (c realClock) Now() time.Time { return time.Now() }

func (c realClock) Until(t time.Time) time.Duration { return time.Until(t) }

type crm interface {
	Get(context.Context, ...string) ([]*etree.Element, error)
	Migrate(context.Context, string) error
	Unmigrate(context.Context) error
}

func WithServerLogger(logger *logrus.Logger) func(*migrationServer) {
	return func(s *migrationServer) { s.logger = logger }
}

func WithClock(c clock) func(*migrationServer) {
	return func(s *migrationServer) { s.clock = c }
}

func WithPGBouncer(bouncer pgBouncerPauser) func(*migrationServer) {
	return func(s *migrationServer) { s.PGBouncer = bouncer }
}

func WithPacemaker(c crm) func(*migrationServer) {
	return func(s *migrationServer) { s.crm = c }
}

func NewServer(options ...func(*migrationServer)) *migrationServer {
	s := &migrationServer{
		logger: logrus.New(),
		clock:  realClock{},
		crm:    pacemaker.NewPacemaker(),
	}

	for _, option := range options {
		option(s)
	}

	return s
}

func (s *migrationServer) HealthCheck(ctx context.Context, _ *Empty) (*HealthCheckResponse, error) {
	return &HealthCheckResponse{
		Status: HealthCheckResponse_HEALTHY,
	}, nil
}

func (s *migrationServer) Pause(ctx context.Context, req *PauseRequest) (*PauseResponse, error) {
	createdAt := s.Now()
	timeoutAt := createdAt.Add(time.Duration(req.Timeout) * time.Second)
	expiresAt := createdAt.Add(time.Duration(req.Expiry) * time.Second)

	timeoutCtx, cancel := context.WithDeadline(ctx, timeoutAt)
	defer cancel()

	err := s.execAndLog("PGBouncer pause", func() error { return s.PGBouncer.Pause(timeoutCtx) })

	if err != nil {
		if timeoutCtx.Err() == nil {
			return nil, status.Errorf(codes.Unknown, "unknown error: %s", err.Error())
		} else {
			return nil, status.Errorf(codes.DeadlineExceeded, "exceeded pause timeout")
		}
	}

	// We need to ensure we remove the pause at expiry seconds from the moment the request
	// was received. This ensures we don't leave PGBouncer in a paused state if migration
	// goes wrong.
	if req.Expiry > 0 {
		go func() {
			s.logger.Infof("Scheduling RESUME for %s", iso3339(expiresAt))
			<-time.After(s.Until(expiresAt))

			s.execAndLog("PGBouncer resume", func() error { return s.PGBouncer.Resume(context.Background()) })
		}()
	}

	return &PauseResponse{
		CreatedAt: s.TimestampProto(createdAt),
		ExpiresAt: s.TimestampProto(expiresAt),
	}, nil
}

func (s *migrationServer) Resume(ctx context.Context, _ *Empty) (*ResumeResponse, error) {
	err := s.execAndLog("PGBouncer resume", func() error { return s.PGBouncer.Resume(ctx) })

	if err != nil {
		return nil, status.Errorf(codes.Unknown, "unknown error: %s", err.Error())
	}

	return &ResumeResponse{CreatedAt: s.TimestampProto(s.Now())}, nil
}

func (s *migrationServer) Migrate(ctx context.Context, _ *Empty) (*MigrateResponse, error) {
	nodes, err := s.crm.Get(ctx, pacemaker.SyncXPath)

	if err != nil {
		s.logger.WithError(err).Error("Failed to query cib")
		return nil, status.Errorf(codes.Unknown, "failed to query cib: %s", err.Error())
	}

	sync := nodes[0]
	if sync == nil {
		s.logger.Error("Failed to find sync node")
		return nil, status.Errorf(codes.NotFound, "failed to find sync node")
	}

	syncHost := sync.SelectAttrValue("uname", "")
	err = s.crm.Migrate(ctx, syncHost)

	if err != nil {
		s.logger.WithError(err).Error("crm resource migrate failed")
		return nil, status.Errorf(
			codes.Unknown, "'crm resource migrate %s' failed: %s", syncHost, err.Error(),
		)
	}

	return &MigrateResponse{
		MigratingTo: syncHost,
		CreatedAt:   s.TimestampProto(s.Now()),
	}, nil
}

func (s *migrationServer) Unmigrate(ctx context.Context, _ *Empty) (*UnmigrateResponse, error) {
	if err := s.crm.Unmigrate(ctx); err != nil {
		return nil, status.Errorf(codes.Unknown, "crm resource unmigrate failed: %s", err.Error())
	}

	return &UnmigrateResponse{CreatedAt: s.TimestampProto(s.Now())}, nil
}

func (s *migrationServer) execAndLog(opName string, op func() error) error {
	s.logger.Infof("Running %s", opName)

	if err := op(); err != nil {
		s.logger.WithError(err).Errorf("Failed %s", opName)
		return err
	}

	return nil
}

func (s *migrationServer) TimestampProto(t time.Time) *tspb.Timestamp {
	ts, err := ptypes.TimestampProto(t)

	if err != nil {
		panic("Failed to convert what should have been an entirely safe timestamp")
	}

	return ts
}
