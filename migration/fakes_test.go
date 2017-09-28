package migration

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/stretchr/testify/mock"
)

func mockResponse(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       ioutil.NopCloser(bytes.NewBufferString("")),
	}
}

type fakeHttpClient struct{ mock.Mock }

func (c fakeHttpClient) Get(url string) (*http.Response, error) {
	args := c.Called(url)
	return args.Get(0).(*http.Response), args.Error(1)
}

func (c fakeHttpClient) Post(url string, contentType string, body io.Reader) (*http.Response, error) {
	args := c.Called(url, contentType, body)
	return args.Get(0).(*http.Response), args.Error(1)
}
