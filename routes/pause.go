package routes

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

// POST http://pg01/pause?timeout=5&expiry=20 HTTP/1.1
//
// HTTP/1.1 201 (OK)
// Content-Type: application/json
type pauseSuccess struct {
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

// HTTP/1.1 408 (timeout)
// Content-Type: application/json
var pauseTimeoutError = apiError{
	Status:  http.StatusRequestTimeout,
	Message: "timed out attempting to PAUSE, gave up and issued RESUME",
}

// HTTP/1.1 500 (error)
// Content-Type: application/json
var pauseError = apiError{
	Status:  http.StatusInternalServerError,
	Message: "failed to PAUSE PGBouncer, check server logs",
}

func (s router) Pause(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := mux.Vars(r)

	timeout, _ := strconv.Atoi(params["timeout"])
	expiry, _ := strconv.Atoi(params["expiry"])
	createdAt := s.Now()
	expiresAt := createdAt.Add(time.Duration(expiry) * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	err := s.pausePGBouncer(ctx)

	if err != nil {
		if ctx.Err() == nil {
			renderError(w, pauseError)
		} else {
			renderError(w, pauseTimeoutError)
		}

		return
	}

	// We need to ensure we remove the pause at expiry seconds from the moment the request
	// was received. This ensures we don't leave PGBouncer in a paused state if migration
	// goes wrong.
	if expiry > 0 {
		go func() {
			s.logger.Infof("Scheduling RESUME for %s", iso3339(expiresAt))
			<-time.After(s.Until(expiresAt))

			s.resumePGBouncer(context.Background())
		}()
	}

	render(w, http.StatusCreated, "pause", pauseSuccess{
		ExpiresAt: iso3339(expiresAt),
		CreatedAt: iso3339(createdAt),
	})
}

func (s router) pausePGBouncer(ctx context.Context) error {
	s.logger.Info("Issuing PGBouncer PAUSE")
	err := s.PGBouncer.Pause(ctx)

	if err != nil {
		s.logger.WithError(err).Error("Failed to PAUSE PGBouncer")
	}

	return err
}
