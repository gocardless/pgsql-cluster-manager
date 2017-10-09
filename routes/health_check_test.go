package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheck(t *testing.T) {
	testCases := []struct {
		name           string
		responseStatus int
		responseBody   string
	}{
		{
			"when healthy",
			http.StatusOK, `
			{
				"health_check": {
					"healthy": true
				}
			}
			`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/health_check", nil)
			require.Nil(t, err)

			Route().ServeHTTP(recorder, req)

			assert.Equal(t, tc.responseStatus, recorder.Code)
			assert.Equal(t, []string{"application/json"}, recorder.Result().Header["Content-Type"])
			assert.JSONEq(t, tc.responseBody, string(recorder.Body.Bytes()))
		})
	}
}
