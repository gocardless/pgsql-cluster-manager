package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/beevik/etree"
	"github.com/gocardless/pgsql-cluster-manager/pacemaker"
	"github.com/gorilla/mux"
)

type router struct {
	logger    *logrus.Logger
	PGBouncer pgBouncerPauser
	cib
	*mux.Router
	clock
}

type apiError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// Define a slim interface to enable easier testing
type pgBouncerPauser interface {
	Pause(context.Context) error
	Resume(context.Context) error
}

// This allows stubbing of time in tests, but would normally delegate to the time package
type clock interface {
	Now() time.Time
	Until(time.Time) time.Duration
}

type realClock struct{}

func (c realClock) Now() time.Time { return time.Now() }

func (c realClock) Until(t time.Time) time.Duration { return time.Until(t) }

type cib interface {
	Get(...string) ([]*etree.Element, error)
	Migrate(string) error
}

func WithLogger(logger *logrus.Logger) func(*router) {
	return func(r *router) { r.logger = logger }
}

func WithClock(c clock) func(*router) {
	return func(r *router) { r.clock = c }
}

func WithPGBouncer(bouncer pgBouncerPauser) func(*router) {
	return func(r *router) { r.PGBouncer = bouncer }
}

func WithCib(c cib) func(*router) {
	return func(r *router) { r.cib = c }
}

func Route(options ...func(*router)) *mux.Router {
	r := &router{
		logger: logrus.New(),
		clock:  realClock{},
		cib:    pacemaker.NewCib(),
		Router: mux.NewRouter(),
	}

	for _, option := range options {
		option(r)
	}

	r.HandleFunc("/health_check", r.HealthCheck).Methods("GET")

	r.HandleFunc("/pause", r.Pause).Methods("POST").
		Queries("timeout", "{timeout:[0-9]+}").
		Queries("expiry", "{expiry:[0-9]+}")

	r.HandleFunc("/resume", r.Resume).Methods("POST")

	r.HandleFunc("/migration", r.Migrate).Methods("POST")

	return r.Router
}

func renderError(w http.ResponseWriter, err apiError) {
	render(w, err.Status, "error", err)
}

func render(w http.ResponseWriter, status int, envelope string, body interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{envelope: body})
}

func iso3339(t time.Time) string {
	return t.Format("2006-01-02T15:04:05-0700")
}
