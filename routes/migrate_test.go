package routes

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/gocardless/pgsql-cluster-manager/pacemaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrate(t *testing.T) {
	testCases := []struct {
		name           string
		cibError       error
		syncElement    *etree.Element
		shouldMigrate  bool
		migrateError   error
		migrateTo      string // empty string means don't migrate
		responseStatus int
		responseBody   string
	}{
		{
			name:           "when successful",
			cibError:       nil,
			syncElement:    &etree.Element{Attr: []etree.Attr{etree.Attr{"", "uname", "pg03"}}},
			shouldMigrate:  true,
			migrateError:   nil,
			migrateTo:      "pg03",
			responseStatus: http.StatusCreated,
			responseBody: `
			{
				"migration": {
					"to": "pg03",
					"created_at": "2017-10-01T15:25:00+0000"
				}
			}`,
		},
		{
			name:           "when cib query fails",
			cibError:       errors.New("oops"),
			syncElement:    nil,
			shouldMigrate:  false,
			migrateError:   nil,
			migrateTo:      "",
			responseStatus: http.StatusInternalServerError,
			responseBody: `
			{
				"error": {
					"status": 500,
					"message": "failed to issue migration, check server logs"
				}
			}`,
		},
		{
			name:           "when no sync node is found",
			cibError:       nil,
			syncElement:    nil,
			shouldMigrate:  false,
			migrateError:   nil,
			migrateTo:      "",
			responseStatus: http.StatusNotFound,
			responseBody: `
			{
				"error": {
					"status": 404,
					"message": "could not identify sync node, cannot migrate"
				}
			}`,
		},
		{
			name:           "when cib.Migrate fails",
			cibError:       nil,
			syncElement:    &etree.Element{Attr: []etree.Attr{etree.Attr{"", "uname", "pg03"}}},
			shouldMigrate:  true,
			migrateError:   errors.New("cannot find crm in PATH"),
			migrateTo:      "pg03",
			responseStatus: http.StatusInternalServerError,
			responseBody: `
			{
				"error": {
					"status": 500,
					"message": "failed to issue migration, check server logs"
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clock := new(fakeClock)
			clock.On("Now").Return(time.Date(2017, 10, 1, 15, 25, 0, 0, time.UTC))

			cib := new(fakeCib)
			cib.
				On("Get", []string{pacemaker.SyncXPath}).
				Return([]*etree.Element{tc.syncElement}, tc.cibError)

			if tc.shouldMigrate {
				cib.On("Migrate", tc.migrateTo).Return(tc.migrateError)
			}

			recorder := httptest.NewRecorder()
			req, err := http.NewRequest("POST", "/migration", nil)
			require.Nil(t, err)

			router := Route(WithCib(cib), WithClock(clock))
			router.ServeHTTP(recorder, req)

			cib.AssertExpectations(t)

			assert.Equal(t, tc.responseStatus, recorder.Code)
			assert.Equal(t, []string{"application/json"}, recorder.Result().Header["Content-Type"])
			assert.JSONEq(t, tc.responseBody, string(recorder.Body.Bytes()))
		})
	}
}
