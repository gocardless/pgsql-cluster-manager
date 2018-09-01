package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/gomega"

	"github.com/gocardless/pgsql-cluster-manager/pkg/pgbouncer"
)

// StartPgBouncer spins up a new PgBouncer instance in a temporary directory.
func StartPgBouncer(database, user, port string) (bouncer *pgbouncer.PgBouncer, cleanup func(), err error) {
	var proc *exec.Cmd
	var workspace string

	cleanup = func() {
		if proc != nil {
			proc.Process.Kill()
		}
		os.RemoveAll(workspace)
	}

	workspace, err = ioutil.TempDir("", "pgbouncer")
	if err != nil {
		return
	}

	pgbouncerBinary, err := exec.LookPath("pgbouncer")
	if err != nil {
		return
	}

	configFile := filepath.Join(workspace, "pgbouncer.ini")
	configFileTemplate := filepath.Join(workspace, "pgbouncer.ini.template")
	authFile := filepath.Join(workspace, "users.txt")

	// We need to allow the pgbouncer user for our tests
	err = ioutil.WriteFile(
		authFile,
		[]byte(fmt.Sprintf(
			"\"postgres\" \"trusted\"\n\"pgbouncer\" \"trusted\"\n\"%s\" \"trusted\"\n",
			user,
		)),
		0644,
	)

	if err != nil {
		return
	}

	// Generate a config file template that will place unix socket in our temporary
	// workspace
	for _, file := range []string{configFile, configFileTemplate} {
		err = ioutil.WriteFile(
			file,
			[]byte(fmt.Sprintf(`[databases]
%s = host={{.Host}} port=%s pool_size=6

[pgbouncer]
logfile = %s/pgbouncer.log
listen_port = 6432
unix_socket_dir = %s
auth_type = trust
auth_file = %s/users.txt
admin_users = postgres,pgbouncer
pool_mode = session
ignore_startup_parameters = extra_float_digits`, database, port, workspace, workspace, workspace)),
			0644,
		)
		if err != nil {
			return
		}
	}

	proc = exec.Command(pgbouncerBinary, filepath.Join(workspace, "pgbouncer.ini"))
	proc.Dir = workspace

	if err = proc.Start(); err != nil {
		return
	}

	bouncer = &pgbouncer.PgBouncer{
		ConfigFile:         filepath.Join(workspace, "pgbouncer.ini"),
		ConfigTemplateFile: filepath.Join(workspace, "pgbouncer.ini.template"),
		Executor: pgbouncer.AuthorizedExecutor{
			User:      "pgbouncer",
			Database:  "pgbouncer",
			SocketDir: workspace,
			Port:      "6432",
		},
	}

	success := Eventually(
		func() error { return bouncer.Connect(context.TODO()) },
		10*time.Second,
		100*time.Millisecond,
	).Should(
		Succeed(),
	)

	if !success {
		err = fmt.Errorf("timed out waiting for successful PgBouncer connection")
	}

	return
}
