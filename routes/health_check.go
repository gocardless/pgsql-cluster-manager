package routes

import (
	"net/http"
)

// GET http://pg01/health_check HTTP/1.1
//
// HTTP/1.1 200 (OK)
type healthCheck struct {
	Healthy bool `json:"healthy"`
}

func (s router) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	render(w, http.StatusOK, "health_check", healthCheck{true})
}
