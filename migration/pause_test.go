package migration

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPause(t *testing.T) {
	mockResponse := func(status int) *http.Response {
		return &http.Response{
			StatusCode: status,
			Body:       ioutil.NopCloser(bytes.NewBufferString("")),
		}
	}

	testCases := []struct {
		name       string
		endpoints  []string
		responses  map[string]*http.Response
		pauseError error
	}{
		{
			"when all nodes pause successfully",
			[]string{"http://pg01", "http://pg02"},
			map[string]*http.Response{
				"http://pg01/pause?timeout=5&expiry=25": mockResponse(http.StatusCreated),
				"http://pg02/pause?timeout=5&expiry=25": mockResponse(http.StatusCreated),
			},
			nil,
		},
		{
			"when any node fails to pause, returns error",
			[]string{"http://pg01", "http://pg02"},
			map[string]*http.Response{
				"http://pg01/pause?timeout=5&expiry=25": mockResponse(http.StatusCreated),
				"http://pg02/pause?timeout=5&expiry=25": mockResponse(http.StatusInternalServerError),
			},
			fmt.Errorf("POST /pause?timeout=5&expiry=25 failed for [http://pg02/pause?timeout=5&expiry=25]"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := new(fakeHttpClient)
			for url, resp := range tc.responses {
				client.On("Post", url, "application/json", nil).Return(resp, nil)
			}

			err := New(
				WithEndpoints(tc.endpoints),
				WithHttpClient(client),
				WithPause(5*time.Second, 25*time.Second),
			).Pause()

			client.AssertExpectations(t)
			assert.EqualValues(t, tc.pauseError, err)
		})
	}
}
