package pgbouncer

import (
	"database/sql"
	"time"

	"github.com/stretchr/testify/mock"
)

type FakePGBouncer struct{ mock.Mock }

func (b FakePGBouncer) Config() (map[string]string, error) {
	args := b.Called()
	return args.Get(0).(map[string]string), args.Error(1)
}

func (b FakePGBouncer) GenerateConfig(host string) error {
	args := b.Called(host)
	return args.Error(0)
}

func (b FakePGBouncer) Psql(timeout time.Duration) (PsqlExecutor, error) {
	args := b.Called(timeout)
	return args.Get(0).(PsqlExecutor), args.Error(1)
}

type FakePsqlExecutor struct{ mock.Mock }

func (e FakePsqlExecutor) Query(query string, params ...interface{}) (*sql.Rows, error) {
	args := e.Called(query, params)
	return args.Get(0).(*sql.Rows), args.Error(1)
}

type FakeFieldError struct{ mock.Mock }

func (e FakeFieldError) Error() string {
	args := e.Called()
	return args.String(0)
}

func (e FakeFieldError) Field(f byte) string {
	args := e.Called(f)
	return args.String(0)
}
