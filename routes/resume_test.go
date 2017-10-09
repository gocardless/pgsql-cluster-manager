package routes

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestResume(t *testing.T) {
	testCases := []struct {
		name           string
		resumeError    error
		responseStatus int
		responseBody   string
	}{
		{
			"when resume is successful",
			nil,
			http.StatusCreated, `
			{
				"resume": {
					"created_at": "2017-10-01T15:25:00+0000"
				}
			}`,
		},
		{
			"when resume fails",
			errors.New("the bees, they're in my eyes!"),
			http.StatusInternalServerError, `
			{
				"error": {
					"status": 500,
					"message": "failed to RESUME PGBouncer, check server logs"
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bouncer := new(fakePGBouncerPauser)
			bouncer.On("Resume", mock.AnythingOfType("*context.emptyCtx")).Return(tc.resumeError)

			recorder := httptest.NewRecorder()
			req, err := http.NewRequest("POST", "/resume", nil)
			require.Nil(t, err)

			clock := new(fakeClock)
			clock.On("Now").Return(time.Date(2017, 10, 1, 15, 25, 0, 0, time.UTC))

			router := Route(WithClock(clock), WithPGBouncer(bouncer))
			router.ServeHTTP(recorder, req)

			bouncer.AssertExpectations(t)

			assert.Equal(t, tc.responseStatus, recorder.Code)
			assert.Equal(t, []string{"application/json"}, recorder.Result().Header["Content-Type"])
			assert.JSONEq(t, tc.responseBody, string(recorder.Body.Bytes()))
		})
	}
}
