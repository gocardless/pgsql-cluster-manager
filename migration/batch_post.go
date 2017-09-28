package migration

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/pkg/errors"
)

func (m *migration) batchPost(path string, expectedStatus int) error {
	var wg sync.WaitGroup
	errChan := make(chan *batchPostError, len(m.endpoints))

	for _, endpoint := range m.endpoints {
		wg.Add(1)

		go func(endpoint string) {
			if err := m.post(endpoint+path, expectedStatus); err != nil {
				errChan <- err
			}

			wg.Done()
		}(endpoint)
	}

	wg.Wait()
	close(errChan)

	collectError := func() error {
		failed := make([]string, 0)
		for err := range errChan {
			failed = append(failed, err.url)
		}

		if len(failed) == 0 {
			return nil
		}

		return fmt.Errorf("POST %s failed for %v", path, failed)
	}

	return collectError()
}

type batchPostError struct {
	url string
	error
}

func (m *migration) post(url string, expectedStatus int) *batchPostError {
	contextLogger := m.logger.WithField("url", url)
	contextLogger.Debugf("POST request, want %s response", expectedStatus)

	resp, err := m.client.Post(url, "application/json", nil)
	defer resp.Body.Close()

	if err != nil {
		contextLogger.WithError(err).Debug("Request failed")
		return &batchPostError{url, errors.Wrap(err, "Request failed")}
	}

	if status := resp.StatusCode; status != http.StatusCreated {
		contextLogger.WithField("status", status).Error("Request returned unexpected status")
		return &batchPostError{url, fmt.Errorf("POST %s responded [%d]", url, status)}
	}

	return nil
}
