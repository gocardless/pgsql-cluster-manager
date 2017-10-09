package migration

import "net/http"

func (m *migration) Resume() error {
	return m.batchPost("/resume", http.StatusCreated)
}
