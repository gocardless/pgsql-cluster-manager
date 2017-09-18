package pgbouncer

import (
	"database/sql"
	"io/ioutil"
	"os"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeFieldError struct{ mock.Mock }

func (e fakeFieldError) Error() string {
	args := e.Called()
	return args.String(0)
}

func (e fakeFieldError) Field(f byte) string {
	args := e.Called(f)
	return args.String(0)
}

type fakePsqlExecutor struct{ mock.Mock }

func (e fakePsqlExecutor) Query(query string, params ...interface{}) (*sql.Rows, error) {
	args := e.Called(query, params)
	return args.Get(0).(*sql.Rows), args.Error(1)
}

func makeTempFile(t *testing.T, prefix string) *os.File {
	tempFile, err := ioutil.TempFile("", prefix)
	require.Nil(t, err, "failed to create temporary file")

	return tempFile
}

func TestGenerateConfig(t *testing.T) {
	t.Run("errors with invalid config template", func(t *testing.T) {
		bouncer := pgBouncer{
			ConfigFile:         "/etc/pgbouncer/pgbouncer.ini",
			ConfigFileTemplate: "/this/does/not/exist",
		}

		err := bouncer.GenerateConfig("curly.db.ams.gc.cx")

		assert.Error(t, err, "expected config generation to fail")
		assert.Equal(t,
			"failed to read PGBouncer config template file: open /this/does/not/exist: no such file or directory",
			err.Error(),
		)
	})

	t.Run("writes config with host when successful", func(t *testing.T) {
		tempConfigFile := makeTempFile(t, "pgbouncer-config-")
		defer os.Remove(tempConfigFile.Name())

		bouncer := pgBouncer{
			ConfigFile:         tempConfigFile.Name(),
			ConfigFileTemplate: "./fixtures/pgbouncer.ini.template",
		}

		err := bouncer.GenerateConfig("curly.db.ams.gc.cx")
		assert.Nil(t, err, "failed to generate config")

		configBuffer, _ := ioutil.ReadFile(tempConfigFile.Name())
		assert.Contains(t, string(configBuffer),
			"postgres = host=curly.db.ams.gc.cx", "expected host to be in generated config")
	})
}

func TestPause(t *testing.T) {
	testCases := []struct {
		name        string
		psqlError   error                   // error returned from PsqlExecutor when Query'ing PAUSE
		assertError func(*testing.T, error) // assertions on the Pause() error
	}{
		{
			"when pause is successful",
			nil,
			func(t *testing.T, err error) {
				assert.Nil(t, err, "expected Pause to return no error")
			},
		},
		{
			"when pause fails",
			errors.New("timeout"),
			func(t *testing.T, err error) {
				assert.Equal(t, "failed to pause PGBouncer: timeout", err.Error())
			},
		},
		// If PGBouncer is already paused then we'll receive a specific error code. Verify that
		// the Pause command will succeed in this case, as it has no work to do.
		{
			"when already paused",
			func() error {
				fieldError := new(fakeFieldError)
				fieldError.On("Field", byte('C')).Return("08P01")

				return fieldError
			}(),
			func(t *testing.T, err error) {
				assert.Nil(t, err, "expected Pause to return no error")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var noParams []interface{}
			psql := new(fakePsqlExecutor)
			bouncer := pgBouncer{PsqlExecutor: psql}

			psql.On("Query", "PAUSE;", noParams).Return(&sql.Rows{}, tc.psqlError)
			err := bouncer.Pause()

			psql.AssertExpectations(t)
			tc.assertError(t, err)
		})
	}
}

func TestReload(t *testing.T) {
	testCases := []struct {
		name        string
		psqlError   error                   // error returned from PsqlExecutor when Query'ing RELOAD
		assertError func(*testing.T, error) // assertions on the Reload() error
	}{
		{
			"when reload is successful",
			nil,
			func(t *testing.T, err error) {
				assert.Nil(t, err, "expected Reload to return no error")
			},
		},
		{
			"when reload is successful",
			errors.New("timeout"),
			func(t *testing.T, err error) {
				assert.Equal(t, "failed to reload PGBouncer: timeout", err.Error())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var noParams []interface{}
			psql := new(fakePsqlExecutor)
			bouncer := pgBouncer{PsqlExecutor: psql}

			psql.On("Query", "RELOAD;", noParams).Return(&sql.Rows{}, tc.psqlError)
			err := bouncer.Reload()

			psql.AssertExpectations(t)
			tc.assertError(t, err)
		})
	}
}
