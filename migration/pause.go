package migration

import (
	"fmt"
	"net/http"
	"time"
)

func (m *migration) Pause() error {
	return m.batchPost(
		fmt.Sprintf(
			"/pause?timeout=%d&expiry=%d",
			int64(m.pauseTimeout/time.Second),
			int64(m.pauseExpiry/time.Second),
		),
		http.StatusCreated,
	)
}
