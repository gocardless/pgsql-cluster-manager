package routes

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPause(t *testing.T) {
	testCases := []struct {
		name           string
		path           string
		pauseError     error
		shouldResume   bool
		responseStatus int
		responseBody   string
	}{
		{
			"when pause is successful",
			"/pause?timeout=1&expiry=2",
			nil,
			true,
			http.StatusCreated, `
			{
				"pause": {
					"expires_at": "2017-10-01T15:25:02+0000",
					"created_at": "2017-10-01T15:25:00+0000"
				}
			}`,
		},
		{
			"when pause fails",
			"/pause?timeout=1&expiry=2",
			errors.New("no stopping this"),
			false,
			http.StatusInternalServerError, `
			{
				"error": {
					"status": 500,
					"message": "failed to PAUSE PGBouncer, check server logs"
				}
			}`,
		},
		{
			"when pause times out",
			"/pause?timeout=0&expiry=2",
			errors.New("failed context"),
			false,
			http.StatusRequestTimeout, `
			{
				"error": {
					"status": 408,
					"message": "timed out attempting to PAUSE, gave up and issued RESUME"
				}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bouncer := new(fakePGBouncerPauser)
			bouncer.On("Pause", mock.AnythingOfType("*context.timerCtx")).Return(tc.pauseError)

			clock := new(fakeClock)
			clock.On("Now").Return(time.Date(2017, 10, 1, 15, 25, 0, 0, time.UTC))

			var wg sync.WaitGroup

			if tc.shouldResume {
				wg.Add(1) // we want to wait at the end for resume to finish
				bouncer.
					On("Resume", mock.AnythingOfType("*context.emptyCtx")).Return(nil).
					Run(func(args mock.Arguments) { wg.Done() })

				// We would normally use this to schedule a resume, but ideally this will happen
				// instantly in tests, so return zero wait.
				clock.On("Until", mock.AnythingOfType("time.Time")).Return(time.Duration(0))
			}

			recorder := httptest.NewRecorder()
			req, err := http.NewRequest("POST", tc.path, nil)
			require.Nil(t, err)

			router := Route(WithClock(clock), WithPGBouncer(bouncer))
			router.ServeHTTP(recorder, req)

			resumeCalled := make(chan struct{}, 1)
			go func() { wg.Wait(); close(resumeCalled) }()

			select {
			case <-time.After(5 * time.Second):
				assert.Fail(t, "timed out waiting for resume to be called")
				return
			case <-resumeCalled:
				// we're good, do nothing
			}

			bouncer.AssertExpectations(t)

			assert.Equal(t, tc.responseStatus, recorder.Code)
			assert.Equal(t, []string{"application/json"}, recorder.Result().Header["Content-Type"])
			assert.JSONEq(t, tc.responseBody, string(recorder.Body.Bytes()))
		})
	}
}
