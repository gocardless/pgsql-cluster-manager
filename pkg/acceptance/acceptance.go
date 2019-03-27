package acceptance

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	kitlog "github.com/go-kit/kit/log"
	"github.com/jackc/pgx"

	. "github.com/onsi/gomega"
)

type AcceptanceOptions struct {
	BinaryPath  string
	DockerImage string
}

func RunAcceptance(ctx context.Context, logger kitlog.Logger, opt ClusterOptions) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cluster := StartCluster(ctx, logger, opt)
	defer cluster.Shutdown()

	// Attempt a connection to the PgBouncer on the given container, but accessing the
	// postgres database which should be proxied to our primary node.
	pgTryConnect := func(container *docker.Container) (*pgx.Conn, error) {
		cfg, err := pgx.ParseConnectionString(
			fmt.Sprintf(
				"user=postgres dbname=postgres host=%s port=%s "+
					"connect_timeout=1 sslmode=disable",
				cluster.Hostname(),
				container.NetworkSettings.Ports["6432/tcp"][0].HostPort,
			),
		)

		Expect(err).NotTo(HaveOccurred())
		return pgx.Connect(cfg)
	}

	// Repeatedly attempt to connect to PgBouncer proxied postgres, timing out after a given
	// limit.
	pgConnect := func(container *docker.Container) (conn *pgx.Conn) {
		defer func(begin time.Time) {
			logger.Log("event", "pg.connect", "msg", "connected to pg via pgBouncer",
				"container", container.Name, "elapsed", time.Since(begin).Seconds())
		}(time.Now())

		Eventually(
			func() (err error) { conn, err = pgTryConnect(container); return },
		).Should(
			Succeed(),
		)

		return conn
	}

	// Given a database connection, attempt to query the inet_server_addr, which can be used
	// to identify which machine we're talking to. This is necessary to identify whether
	// PgBouncer has routed our connection correctly.
	inetServerAddr := func(conn *pgx.Conn) string {
		rows, err := conn.Query(`SELECT inet_server_addr();`)
		Expect(err).NotTo(HaveOccurred())

		defer rows.Close()

		var addr sql.NullString

		Expect(rows.Next()).To(BeTrue())
		Expect(rows.Scan(&addr)).To(Succeed())

		// Remove any network suffix from the IP (e.g., 172.17.0.3/32)
		return strings.SplitN(addr.String, "/", 2)[0]
	}

	etcdGetValue := func(key string) string {
		resp, err := cluster.EtcdClient().Get(ctx, key)
		Expect(err).NotTo(HaveOccurred())

		if len(resp.Kvs) > 0 {
			return string(resp.Kvs[0].Value)
		}

		return ""
	}

	addrOf := func(container *docker.Container) string {
		return strings.SplitN(container.NetworkSettings.IPAddress, "/", 2)[0]
	}

	printOutput := func(source string, output []byte) {
		fmt.Println(strings.Replace("\n"+string(output), "\n", fmt.Sprintf("\n%s >", source), 0))
	}

	failover := func(result chan error, args ...string) {
		logger.Log("msg", "running failover using api")
		defer func(begin time.Time) {
			logger.Log("msg", "ran failover", "elapsed", time.Since(begin).Seconds())
		}(time.Now())

		output, err := cluster.Executor().CombinedOutput(
			ctx,
			"pgcm",
			append(
				[]string{
					"failover",
					"--config-file",
					"/etc/pgsql-cluster-manager/config.toml",
				},
				args...,
			)...,
		)

		printOutput("$ pgcm failover", output)
		result <- err
	}

	master, sync, async := cluster.Roles()
	masterAddr := addrOf(master)

	logger.Log("master", master.Name, "sync", sync.Name, "async", async.Name)

	logger.Log("expect", "etcd has IP of master at /postgres/master", "addr", masterAddr)
	Eventually(func() string { return etcdGetValue("/postgres/master") }).Should(
		Equal(masterAddr),
	)

	logger.Log("expect", "all PgBouncers to proxy to master", "masterAddr", masterAddr)
	for _, member := range cluster.members {
		conn := pgConnect(member)
		connectedAddr := inetServerAddr(conn)

		Expect(connectedAddr).To(
			Equal(masterAddr),
		)
	}

	// Use the async node for issuing Postgres connections
	conn := pgConnect(async)

	// We're going to run a couple of failovers, and we want to validate that the failover
	// behaved as we expected by inspecting the error returned from the exec.
	failoverResults := make(chan error)

	logger.Log("msg", "start transaction, preventing PgBouncer pause")
	txact, err := conn.Begin()
	Expect(err).NotTo(HaveOccurred())

	logger.Log("msg", "this failover should fail, due to the PgBouncer pause timeout")
	go failover(failoverResults, "--pause-timeout", "1s", "--pause-expiry", "1s")

	Eventually(failoverResults).Should(
		Receive(
			HaveOccurred(), // expect a non-nil error
		),
	)

	logger.Log("msg", "rollback transaction, allowing PgBouncer pause")
	Expect(txact.Rollback()).To(Succeed())

	logger.Log("msg", "triggering failover we expect to succeed")
	go failover(failoverResults)

	logger.Log("msg", "expecting the old sync node should become master")
	Eventually(cluster.Master).Should(
		Equal(sync),
	)

	logger.Log("msg", "our connection via the async PgBouncer should proxy to new master")
	Eventually(func() string { return inetServerAddr(conn) }).Should(
		Equal(addrOf(sync)), // the old sync is now the new master
	)

	Expect(failoverResults).Should(
		Receive(
			Succeed(), // nil error, failover was successful
		),
	)
}
