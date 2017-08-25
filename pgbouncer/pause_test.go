package pgbouncer

import (
	"errors"
	"testing"
	"time"

	"github.com/gocardless/pgsql-novips/util"
	"github.com/stretchr/testify/assert"
)

func TestPause_WhenSuccessful(t *testing.T) {
	bouncer := new(FakePGBouncer)
	psql := new(FakePsqlExecutor)

	var noParams []interface{}

	bouncer.On("Psql", time.Second).Return(psql, nil)
	psql.On("Exec", "PAUSE;", noParams).Return(FakeORMResult{}, nil)

	err := Pause(bouncer, time.Second)

	bouncer.AssertExpectations(t)
	psql.AssertExpectations(t)

	assert.Nil(t, err, "expected Pause to return no error")
}

func TestPause_WhenFailsReturnsError(t *testing.T) {
	bouncer := new(FakePGBouncer)
	psql := new(FakePsqlExecutor)

	var noParams []interface{}

	bouncer.On("Psql", time.Second).Return(psql, nil)
	psql.On("Exec", "PAUSE;", noParams).Return(FakeORMResult{}, errors.New("timeout"))

	err := Pause(bouncer, time.Second)

	bouncer.AssertExpectations(t)
	psql.AssertExpectations(t)

	assert.IsType(t, util.ErrorWithFields{}, err, "expected error to be ErrorWithFields")
	ferr, _ := err.(util.ErrorWithFields)

	assert.Error(t, ferr, "expected Pause to return error")
	assert.Equal(t, "failed to pause PGBouncer", ferr.Error())
	assert.Equal(t, "timeout", ferr.Fields["error"])
}

// If PGBouncer is already paused then we'll receive a specific error code. Verify that
// the Pause command will succeed in this case, as it has no work to do.
func TestPause_WhenFailedBecauseAlreadyPausedSucceeds(t *testing.T) {
	bouncer := new(FakePGBouncer)
	psql := new(FakePsqlExecutor)
	fieldError := new(FakeFieldError)

	var noParams []interface{}

	bouncer.On("Psql", time.Second).Return(psql, nil)
	psql.On("Exec", "PAUSE;", noParams).Return(FakeORMResult{}, fieldError)
	fieldError.On("Field", byte('C')).Return("08P01")

	err := Pause(bouncer, time.Second)

	bouncer.AssertExpectations(t)
	psql.AssertExpectations(t)
	fieldError.AssertExpectations(t)

	assert.Nil(t, err, "expected Pause to return no error")
}
