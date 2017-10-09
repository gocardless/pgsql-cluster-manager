package migration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResume(t *testing.T) {
	testCases := []struct {
		name        string
		endpoints   []string
		responses   map[string]*http.Response
		resumeError error
	}{
		{
			"when all nodes resume successfully",
			[]string{"http://pg01", "http://pg02"},
			map[string]*http.Response{
				"http://pg01/resume": mockResponse(http.StatusCreated),
				"http://pg02/resume": mockResponse(http.StatusCreated),
			},
			nil,
		},
		{
			"when any node fails to resume, returns error",
			[]string{"http://pg01", "http://pg02"},
			map[string]*http.Response{
				"http://pg01/resume": mockResponse(http.StatusCreated),
				"http://pg02/resume": mockResponse(http.StatusInternalServerError),
			},
			fmt.Errorf("POST /resume failed for [http://pg02/resume]"),
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
			).Resume()

			client.AssertExpectations(t)
			assert.EqualValues(t, tc.resumeError, err)
		})
	}
}
