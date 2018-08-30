package failover

import (
	"context"
	"fmt"
	"time"

	"github.com/beevik/etree"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pacemaker"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	logger    kitlog.Logger
	pgBouncer pgBouncerPauser
	crm       crm
	clock     clock
}

// This allows stubbing of time in tests, but would normally delegate to the time package
type clock interface {
	Now() time.Time
	Until(time.Time) time.Duration
}

type realClock struct{}

func (c realClock) Now() time.Time {
	return time.Now()
}

func (c realClock) Until(t time.Time) time.Duration {
	return time.Until(t)
}

type pgBouncerPauser interface {
	Pause(context.Context) error
	Resume(context.Context) error
}

type crm interface {
	Get(context.Context, ...string) ([]*etree.Element, error)
	ResolveAddress(context.Context, string) (string, error)
	Migrate(context.Context, string) error
	Unmigrate(context.Context) error
}

func NewServer(logger kitlog.Logger, pgBouncer pgBouncerPauser, crm crm) *Server {
	return &Server{
		logger:    logger,
		pgBouncer: pgBouncer,
		crm:       crm,
		clock:     realClock{},
	}
}

func (s *Server) HealthCheck(ctx context.Context, _ *Empty) (*HealthCheckResponse, error) {
	return &HealthCheckResponse{
		Status: HealthCheckResponse_HEALTHY,
	}, nil
}

func (s *Server) Pause(ctx context.Context, req *PauseRequest) (*PauseResponse, error) {
	createdAt := s.clock.Now()
	timeoutAt := createdAt.Add(time.Duration(req.Timeout) * time.Second)
	expiresAt := createdAt.Add(time.Duration(req.Expiry) * time.Second)

	timeoutCtx, cancel := context.WithDeadline(ctx, timeoutAt)
	defer cancel()

	err := s.execute("pgbouncer.pause", func() error { return s.pgBouncer.Pause(timeoutCtx) })

	if err != nil {
		if timeoutCtx.Err() == nil {
			return nil, status.Errorf(codes.Unknown, "unknown error: %s", err.Error())
		} else {
			return nil, status.Errorf(codes.DeadlineExceeded, "exceeded pause timeout")
		}
	}

	// We need to ensure we remove the pause at expiry seconds from the moment the request
	// was received. This ensures we don't leave PgBouncer in a paused state if migration
	// goes wrong.
	if req.Expiry > 0 {
		go func() {
			s.logger.Log("event", "pgbouncer.resume.schedule", "at", iso3339(expiresAt))
			time.Sleep(s.clock.Until(expiresAt))

			s.execute("pgbouncer.resume", func() error { return s.pgBouncer.Resume(context.TODO()) })
		}()
	}

	return &PauseResponse{
		CreatedAt: s.TimestampProto(createdAt),
		ExpiresAt: s.TimestampProto(expiresAt),
	}, nil
}

func (s *Server) Resume(ctx context.Context, _ *Empty) (*ResumeResponse, error) {
	err := s.execute("pgbouncer.resume", func() error { return s.pgBouncer.Resume(ctx) })
	if err != nil {
		return nil, status.Errorf(codes.Unknown, "unknown error: %s", err.Error())
	}

	return &ResumeResponse{CreatedAt: s.TimestampProto(s.clock.Now())}, nil
}

func (s *Server) Migrate(ctx context.Context, _ *Empty) (*MigrateResponse, error) {
	nodes, err := s.crm.Get(ctx, pacemaker.SyncXPath)
	if err != nil {
		s.logger.Log("event", "cib.error", "error", err, "msg", "failed to query cib")
		return nil, status.Errorf(codes.Unknown, "failed to query cib: %s", err.Error())
	}

	sync := nodes[0]
	if sync == nil {
		s.logger.Log("event", "cluster.sync.not_found")
		return nil, status.Errorf(codes.NotFound, "failed to find sync node")
	}

	syncHost := sync.SelectAttrValue("uname", "")
	syncID := sync.SelectAttrValue("id", "")
	syncAddress, err := s.crm.ResolveAddress(ctx, syncID)

	if err != nil {
		s.logger.Log("event", "cluster.sync.cannot_resolve", "error", err)
		return nil, status.Errorf(
			codes.Unknown, "failed to resolve sync host IP address: %s", err.Error(),
		)
	}

	err = s.crm.Migrate(ctx, syncHost)

	if err != nil {
		s.logger.Log("event", "crm.migrate.error", "error", err)
		return nil, status.Errorf(
			codes.Unknown, "'crm resource migrate %s' failed: %s", syncHost, err.Error(),
		)
	}

	return &MigrateResponse{
		MigratingTo: syncHost,
		Address:     syncAddress,
		CreatedAt:   s.TimestampProto(s.clock.Now()),
	}, nil
}

func (s *Server) Unmigrate(ctx context.Context, _ *Empty) (*UnmigrateResponse, error) {
	if err := s.crm.Unmigrate(ctx); err != nil {
		return nil, status.Errorf(codes.Unknown, "crm resource unmigrate failed: %s", err.Error())
	}

	return &UnmigrateResponse{CreatedAt: s.TimestampProto(s.clock.Now())}, nil
}

func (s *Server) execute(event string, action func() error) error {
	s.logger.Log("event", fmt.Sprintf("%s.execute", event))

	if err := action(); err != nil {
		s.logger.Log("event", fmt.Sprintf("%s.error", event), "error", err)
		return err
	}

	return nil
}

func (s *Server) TimestampProto(t time.Time) *tspb.Timestamp {
	ts, err := ptypes.TimestampProto(t)

	if err != nil {
		panic("failed to convert what should have been an entirely safe timestamp")
	}

	return ts
}
