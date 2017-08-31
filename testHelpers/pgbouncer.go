package testHelpers

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gocardless/pgsql-novips/pgbouncer"
	"github.com/stretchr/testify/require"
)

type PGBouncerProcess struct {
	pgbouncer.PGBouncer
	ConfigFile, ConfigFileTemplate string
}

func StartPGBouncer(t *testing.T, ctx context.Context) *PGBouncerProcess {
	workspace, err := ioutil.TempDir("", "pgbouncer")
	if err != nil {
		require.Fail(t, "failed to create pgbouncer workspace: %s", err.Error())
	}

	pgbouncerBinary, err := exec.LookPath("pgbouncer")
	if err != nil {
		require.Fail(t, "failed to find pgbouncer binary: %s", err.Error())
	}

	configFile := filepath.Join(workspace, "pgbouncer.ini")
	configFileTemplate := filepath.Join(workspace, "pgbouncer.ini.template")

	// Generate a config file template that will place unix socket in our temporary
	// workspace
	for _, file := range []string{configFile, configFileTemplate} {
		err = ioutil.WriteFile(
			file,
			[]byte(fmt.Sprintf(`
[databases]
postgres = host={{.Host}} port=6432 pool_size=6

[pgbouncer]
listen_port = 6432
unix_socket_dir = %s
auth_type = trust
pool_mode = session
ignore_startup_parameters = extra_float_digits`, workspace)),
			0644,
		)
		if err != nil {
			require.Fail(t, "failed to generate pgbouncer config file: %s", err.Error())
		}
	}

	proc := exec.CommandContext(
		ctx,
		pgbouncerBinary,
		filepath.Join(workspace, "pgbouncer.ini"),
	)

	proc.Dir = workspace

	if err = proc.Start(); err != nil {
		require.Fail(t, "failed to start pgbouncer: %s", err.Error())
	}

	bouncer := pgbouncer.NewPGBouncer(
		filepath.Join(workspace, "pgbouncer.ini"),
		filepath.Join(workspace, "pgbouncer.ini.template"),
		time.Second,
	)

	if err = pollPGBouncer(bouncer); err != nil {
		require.Fail(t, err.Error())
	}

	return &PGBouncerProcess{
		bouncer,
		configFile,
		configFileTemplate,
	}
}

// pollPGBouncer attempts to execute a Reload against PGBouncer until the reload is
// successful, eventually timing out. This allows us to wait for PGBouncer to become ready
// before proceeding.
func pollPGBouncer(bouncer pgbouncer.PGBouncer) error {
	timeout := time.After(5 * time.Second)
	retry := time.Tick(100 * time.Millisecond)

	for {
		select {
		case <-retry:
			if err := bouncer.Reload(); err == nil {
				return nil
			}
		case <-timeout:
			return errors.New("timed out waiting for PGBouncer to start")
		}
	}
}
