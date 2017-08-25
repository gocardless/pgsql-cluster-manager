package pgbouncer

import (
	"time"

	"github.com/go-pg/pg/orm"
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

func (e FakePsqlExecutor) Exec(query interface{}, params ...interface{}) (orm.Result, error) {
	args := e.Called(query, params)
	return args.Get(0).(orm.Result), args.Error(1)
}

type FakeORMResult struct{ mock.Mock }

func (r FakeORMResult) Model() orm.Model {
	args := r.Called()
	return args.Get(0).(orm.Model)
}

func (r FakeORMResult) RowsAffected() int {
	args := r.Called()
	return args.Int(0)
}

func (r FakeORMResult) RowsReturned() int {
	args := r.Called()
	return args.Int(0)
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
