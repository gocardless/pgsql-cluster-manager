package integration

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer"
	"github.com/stretchr/testify/require"
)

type PgBouncerProcess struct {
	*pgbouncer.PgBouncer
	ConfigFile, ConfigFileTemplate string
}

func StartPgBouncer(t *testing.T, ctx context.Context) *PgBouncerProcess {
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
	authFile := filepath.Join(workspace, "users.txt")

	// We need to allow the pgbouncer user for our tests
	err = ioutil.WriteFile(authFile, []byte(`
	"pgbouncer" "this_matters_not_as_we_trust"
	`), 0644)

	require.Nil(t, err)

	// Generate a config file template that will place unix socket in our temporary
	// workspace
	for _, file := range []string{configFile, configFileTemplate} {
		err = ioutil.WriteFile(
			file,
			[]byte(fmt.Sprintf(`
[databases]
postgres = host={{.Host}} port=6432 pool_size=6

[pgbouncer]
logfile = /tmp/pgbouncer.log
listen_port = 6432
unix_socket_dir = %s
auth_type = trust
auth_file = %s/users.txt
admin_users = postgres,pgbouncer
pool_mode = session
ignore_startup_parameters = extra_float_digits`, workspace, workspace)),
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

	bouncer := &pgbouncer.PgBouncer{
		ConfigFile:         filepath.Join(workspace, "pgbouncer.ini"),
		ConfigTemplateFile: filepath.Join(workspace, "pgbouncer.ini.template"),
		Executor: pgbouncer.AuthorizedExecutor{
			User:      "pgbouncer",
			Database:  "pgbouncer",
			SocketDir: workspace,
			Port:      "6432",
		},
	}

	if err = pollPgBouncer(bouncer); err != nil {
		require.Fail(t, err.Error())
	}

	return &PgBouncerProcess{
		bouncer,
		configFile,
		configFileTemplate,
	}
}

// pollPgBouncer attempts to execute a Reload against PgBouncer until the reload is
// successful, eventually timing out. This allows us to wait for PgBouncer to become ready
// before proceeding.
func pollPgBouncer(bouncer *pgbouncer.PgBouncer) error {
	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-timeout:
			return errors.New("timed out waiting for PgBouncer to start")
		default:
			if err := bouncer.Reload(context.Background()); err == nil {
				return err
			}

			<-time.After(100 * time.Millisecond)
		}
	}
}
