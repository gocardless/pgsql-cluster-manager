package routes

import (
	"net/http"

	"github.com/gocardless/pgsql-cluster-manager/pacemaker"
)

// POST http://pg01/migration HTTP/1.1
//
// HTTP/1.1 201 (OK)
// Content-Type: application/json
type migration struct {
	To        string `json:"to"`
	CreatedAt string `json:"created_at"`
}

// HTTP/1.1 400 (Bad request)
// Context-Type: application/json
var migrationNoSyncError = apiError{
	Status:  http.StatusNotFound,
	Message: "could not identify sync node, cannot migrate",
}

// HTTP/1.1 500 (Error)
// Content-Type: application/json
var migrationError = apiError{
	Status:  http.StatusInternalServerError,
	Message: "failed to issue migration, check server logs",
}

func (s router) Migrate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	nodes, err := s.cib.Get(pacemaker.SyncXPath)
	sync := nodes[0]

	if err != nil {
		s.logger.WithError(err).Error("Failed to query cib")
		renderError(w, migrationError)

		return
	}

	if sync == nil {
		s.logger.WithError(err).Error("Failed to find sync node")
		renderError(w, migrationNoSyncError)

		return
	}

	syncHost := sync.SelectAttrValue("uname", "")
	err = s.cib.Migrate(sync.SelectAttrValue("uname", ""))

	if err != nil {
		s.logger.WithError(err).Error("crm resource migrate failed")
		renderError(w, migrationError)

		return
	}

	render(w, http.StatusCreated, "migration", migration{
		To:        syncHost,
		CreatedAt: iso3339(s.Now()),
	})
}
