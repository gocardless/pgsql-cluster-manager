package pgbouncer

import (
	"errors"
	"testing"
	"time"

	"github.com/gocardless/pgsql-novips/util"
	"github.com/stretchr/testify/assert"
)

func TestReload_WhenSuccessful(t *testing.T) {
	bouncer := new(FakePGBouncer)
	psql := new(FakePsqlExecutor)

	var noParams []interface{}

	bouncer.On("Psql", time.Second).Return(psql, nil)
	psql.On("Exec", "RELOAD;", noParams).Return(FakeORMResult{}, nil)

	err := Reload(bouncer, time.Second)

	bouncer.AssertExpectations(t)
	psql.AssertExpectations(t)

	assert.Nil(t, err, "expected Reload to return no error")
}

func TestReload_WhenFailsReturnsError(t *testing.T) {
	bouncer := new(FakePGBouncer)
	psql := new(FakePsqlExecutor)

	var noParams []interface{}

	bouncer.On("Psql", time.Second).Return(psql, nil)
	psql.On("Exec", "RELOAD;", noParams).Return(FakeORMResult{}, errors.New("timeout"))

	err := Reload(bouncer, time.Second)

	bouncer.AssertExpectations(t)
	psql.AssertExpectations(t)

	assert.IsType(t, util.ErrorWithFields{}, err, "expected error to be ErrorWithFields")
	ferr, _ := err.(util.ErrorWithFields)

	assert.Error(t, ferr, "expected Reload to return error")
	assert.Equal(t, "failed to reload PGBouncer", ferr.Error())
	assert.Equal(t, "timeout", ferr.Fields["error"])
}
