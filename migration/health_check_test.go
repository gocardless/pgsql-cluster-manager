package migration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	testCases := []struct {
		name       string
		endpoints  []string
		responses  map[string]*http.Response
		checkError error
	}{
		{
			"when all endpoints respond successfully",
			[]string{"http://pg01:8080", "http://pg02:8080"},
			map[string]*http.Response{
				"http://pg01:8080/health_check": &http.Response{StatusCode: http.StatusOK},
				"http://pg02:8080/health_check": &http.Response{StatusCode: http.StatusOK},
			},
			nil,
		},
		{
			"when an endpoint fails to respond",
			[]string{"http://pg01:8080", "http://pg02:8080"},
			map[string]*http.Response{
				"http://pg01:8080/health_check": &http.Response{StatusCode: http.StatusOK},
				"http://pg02:8080/health_check": &http.Response{StatusCode: http.StatusInternalServerError},
			},
			fmt.Errorf("endpoint [http://pg02:8080] responded 500 for health check"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := new(fakeHttpClient)
			for url, resp := range tc.responses {
				client.On("Get", url).Return(resp, nil)
			}

			err := New(WithEndpoints(tc.endpoints), WithHttpClient(client)).HealthCheck()

			client.AssertExpectations(t)
			assert.EqualValues(t, tc.checkError, err)
		})
	}
}
