package migration

import (
	"io"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
)

type migration struct {
	logger    *logrus.Logger
	endpoints []string
	client    httpClient
}

type httpClient interface {
	Get(string) (*http.Response, error)
	Post(string, string, io.Reader) (*http.Response, error)
}

func WithLogger(logger *logrus.Logger) func(*migration) {
	return func(m *migration) { m.logger = logger }
}

func WithEndpoints(es []string) func(*migration) {
	return func(m *migration) { m.endpoints = es }
}

func WithHttpClient(c httpClient) func(*migration) {
	return func(m *migration) { m.client = c }
}

func New(options ...func(*migration)) *migration {
	m := &migration{
		logger:    logrus.StandardLogger(),
		endpoints: []string{},
		client: &http.Client{
			Timeout: time.Second * 30,
		},
	}

	for _, option := range options {
		option(m)
	}

	return m
}

func (m *migration) Migrate() error {
	var err error

	err = m.HealthCheck()
	if err != nil {
		return errors.Wrap(err, "failed to health check API endpoints")
	}

	//	err = m.AcquireLock()
	//	if err != nil {
	//		return errors.Wrap(err, "failed to acquire lock in etcd")
	//	}
	//
	//	defer m.ReleaseLock()
	//
	//	err = m.Pause()
	//	if err != nil {
	//		m.Resume()
	//		return errors.Wrap(err, "failed to pause PGBouncers")
	//	}
	//
	//	resp, err = m.Migrate()
	//	if err != nil {
	//		return errors.Wrap(err, "failed to migrate primary")
	//	}
	//
	//	select {
	//	case <-time.After(m.expiry):
	//		return errors.New("timed out waiting for migration")
	//	case newPrimary := <-m.WatchPrimary():
	//		m.logger.WithField("primary", newPrimary).Info("Postgres primary has migrated")
	//	}
	//
	//	err = m.Resume()
	//	if err != nil {
	//		return errors.Wrap(err, "failed to resume PGBouncers")
	//	}
	//
	//	err = m.Unmigrate()
	//	if err != nil {
	//		return errors.Wrap(err, "failed to unmigrate pacemaker, manual action required")
	//	}

	return nil
}
