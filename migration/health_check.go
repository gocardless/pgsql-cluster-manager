package migration

import (
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

func (m *migration) HealthCheck() error {
	m.logger.WithField("endpoints", m.endpoints).Info("Health checking cluster endpoints")

	for _, endpoint := range m.endpoints {
		resp, err := m.client.Get(fmt.Sprintf("%s/health_check", endpoint))
		if err != nil {
			return errors.Wrap(err, "failed to health check node")
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("endpoint [%s] responded %d for health check", endpoint, resp.StatusCode)
		}

		m.logger.WithField("endpoint", endpoint).Debug("HealthCheck: OK")
	}

	return nil
}
