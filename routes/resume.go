package routes

import (
	"context"
	"net/http"
)

// POST http://pg01/resume HTTP/1.1
//
// HTTP/1.1 201 (Created)
// Content-Type: application/json
type resumeSuccess struct {
	CreatedAt string `json:"created_at"`
}

// HTTP/1.1 500 (Error)
// Content-Type: application/json
var resumeError = apiError{
	Status:  http.StatusInternalServerError,
	Message: "failed to RESUME PGBouncer, check server logs",
}

func (s router) Resume(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	err := s.resumePGBouncer(context.Background())

	if err != nil {
		renderError(w, resumeError)
	} else {
		render(w, http.StatusCreated, "resume", resumeSuccess{iso3339(s.Now())})
	}
}

func (s router) resumePGBouncer(ctx context.Context) error {
	s.logger.Info("Issuing PGBouncer RESUME")
	err := s.PGBouncer.Resume(ctx)

	if err != nil {
		s.logger.WithError(err).Error("Failed to RESUME PGBouncer")
	}

	return err
}
