package failover

import (
	"context"
	"time"

	"github.com/beevik/etree"
	kitlog "github.com/go-kit/kit/log"
	"github.com/gocardless/pgsql-cluster-manager/pkg/pacemaker"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	uuid "github.com/satori/go.uuid"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the hooks required to provide the failover interface
type Server struct {
	logger  kitlog.Logger
	bouncer pauser
	crm     crm
	clock   clock
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

type pauser interface {
	Pause(context.Context) error
	Resume(context.Context) error
}

type crm interface {
	Get(context.Context, ...string) ([]*etree.Element, error)
	ResolveAddress(context.Context, string) (string, error)
	Migrate(context.Context, string) error
	Unmigrate(context.Context) error
}

func iso3339(t time.Time) string {
	return t.Format("2006-01-02T15:04:05-0700")
}

func NewServer(logger kitlog.Logger, bouncer pauser, crm crm) *Server {
	return &Server{
		logger:  logger,
		bouncer: bouncer,
		crm:     crm,
		clock:   realClock{},
	}
}

// LoggingInterceptor returns a UnaryServerInterceptor that logs all incoming
// requests, both at the start and at the end of their execution.
func (s *Server) LoggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	logger := kitlog.With(s.logger, "method", info.FullMethod, "trace", uuid.NewV4().String())
	logger.Log("msg", "handling request")

	defer func(begin time.Time) {
		if err != nil {
			logger = kitlog.With(logger, "error", err.Error())
		}

		logger.Log("duration", time.Since(begin).Seconds())
	}(time.Now())

	return handler(ctx, req)
}

func (s *Server) HealthCheck(ctx context.Context, _ *Empty) (*HealthCheckResponse, error) {
	return &HealthCheckResponse{
		Status: HealthCheckResponse_HEALTHY,
	}, nil
}

func (s *Server) Pause(ctx context.Context, req *PauseRequest) (resp *PauseResponse, err error) {
	createdAt := s.clock.Now()
	timeoutAt := createdAt.Add(time.Duration(req.Timeout) * time.Second)
	expiresAt := createdAt.Add(time.Duration(req.Expiry) * time.Second)

	timeoutCtx, cancel := context.WithDeadline(ctx, timeoutAt)
	defer cancel()

	if err := s.bouncer.Pause(timeoutCtx); err != nil {
		if timeoutCtx.Err() == nil {
			return nil, status.Error(codes.Unknown, err.Error())
		}

		return nil, status.Errorf(codes.DeadlineExceeded, "exceeded pause timeout")
	}

	// We need to ensure we remove the pause at expiry seconds from the moment the request
	// was received. This ensures we don't leave PgBouncer in a paused state if migration
	// goes wrong.
	if req.Expiry > 0 {
		go func() {
			s.logger.Log("event", "pause", "msg", "scheduling pgbouncer resume", "at", iso3339(expiresAt))
			time.Sleep(s.clock.Until(expiresAt))

			if err := s.bouncer.Resume(context.TODO()); err != nil {
				s.logger.Log("event", "pause", "error", err.Error(), "msg", "failed to resume pgbouncer")
			}
		}()
	}

	return &PauseResponse{
		CreatedAt: s.TimestampProto(createdAt),
		ExpiresAt: s.TimestampProto(expiresAt),
	}, nil
}

func (s *Server) Resume(ctx context.Context, _ *Empty) (*ResumeResponse, error) {
	if err := s.bouncer.Resume(ctx); err != nil {
		return nil, status.Errorf(codes.Unknown, "failed to resume pgbouncer: %s", err.Error())
	}

	return &ResumeResponse{CreatedAt: s.TimestampProto(s.clock.Now())}, nil
}

func (s *Server) Migrate(ctx context.Context, _ *Empty) (*MigrateResponse, error) {
	nodes, err := s.crm.Get(ctx, pacemaker.SyncXPath)
	if err != nil {
		return nil, status.Errorf(codes.Unknown, "failed to query cib: %s", err.Error())
	}

	sync := nodes[0]
	if sync == nil {
		return nil, status.Errorf(codes.NotFound, "failed to find sync node")
	}

	syncHost := sync.SelectAttrValue("uname", "")
	syncID := sync.SelectAttrValue("id", "")
	syncAddress, err := s.crm.ResolveAddress(ctx, syncID)

	if err != nil {
		return nil, status.Errorf(
			codes.Unknown, "failed to resolve sync host IP address: %s", err.Error(),
		)
	}

	if err := s.crm.Migrate(ctx, syncHost); err != nil {
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

func (s *Server) TimestampProto(t time.Time) *tspb.Timestamp {
	ts, err := ptypes.TimestampProto(t)

	if err != nil {
		panic("failed to convert what should have been an entirely safe timestamp")
	}

	return ts
}
