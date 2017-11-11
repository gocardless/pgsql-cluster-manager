// +build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	docker "github.com/fsouza/go-dockerclient"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("creating cluster...")
	cluster := StartCluster(t, ctx)

	outputFile := func(path string) {
		contents, err := cluster.Executor().CombinedOutput(ctx, "cat", path)

		if err == nil {
			fmt.Printf("$ cat %s\n\n%s\n\n", path, string(contents))
		} else {
			fmt.Printf("failed to output file: %s\n", err.Error())
		}
	}

	dumpLogs := func() {
		outputFile("/var/log/start-cluster.log")
		outputFile("/var/log/postgresql/pgbouncer.log")
	}

	defer cluster.Shutdown()
	defer dumpLogs() // print log whatever happens

	client := cluster.EtcdClient(t)

	openPGBouncer := func(container *docker.Container) (*sql.DB, error) {
		host := cluster.Hostname(t)
		port := container.NetworkSettings.Ports["6432/tcp"][0].HostPort

		connStr := fmt.Sprintf("user=postgres dbname=postgres connect_timeout=1 sslmode=disable host=%s port=%s", host, port)
		conn, err := sql.Open("postgres", connStr)

		return conn, err
	}

	connectTo := func(container *docker.Container) *sql.DB {
		defer func(begin time.Time) {
			fmt.Printf("connected to PGBouncer in %.2fs\n", time.Since(begin).Seconds())
		}(time.Now())

		timeout := time.After(time.Minute)
		err := errors.New("")

		for {
			select {
			case <-timeout:
				require.Fail(t, fmt.Sprintf("timed out connecting to PGBouncer: %s", err.Error()))
			default:
				if conn, err := openPGBouncer(container); err == nil {
					if _, err = conn.QueryContext(ctx, `SELECT now();`); err == nil {
						return conn
					}
				}

				<-time.After(time.Second)
			}
		}
	}

	inetServerAddr := func(conn *sql.DB) string {
		rows, err := conn.Query(`SELECT inet_server_addr();`)
		require.Nil(t, err)

		defer rows.Close()

		var addr sql.NullString

		require.Equal(t, true, rows.Next())
		require.Nil(t, rows.Scan(&addr))

		return addr.String
	}

	get := func(key string) *clientv3.GetResponse {
		resp, err := client.Get(context.Background(), key)
		require.Nil(t, err)
		return resp
	}

	runMigration := func(finish chan interface{}) {
		defer func(begin time.Time) {
			fmt.Printf("migrate command executed in %.2fs\n", time.Since(begin).Seconds())
		}(time.Now())

		fmt.Println("running migrate using api...")
		output, err := cluster.Executor().CombinedOutput(
			ctx,
			"pgsql-cluster-manager", "migrate",
			"--log-level", "debug",
			"--etcd-namespace", "/postgres",
			"--etcd-endpoints", "pg01:2379,pg02:2379,pg03:2379",
			"--migration-api-endpoints", "pg01:8080,pg02:8080,pg03:8080",
		)

		fmt.Println(string(output))
		assert.Nil(t, err, "migration exited with error")

		finish <- struct{}{}
	}

	waitUntilMaster := func(node *docker.Container) {
		defer func(begin time.Time) {
			fmt.Printf("became master after %.2fs\n", time.Since(begin).Seconds())
		}(time.Now())

		timeout := time.After(time.Minute)

		for {
			select {
			case <-timeout:
				require.Fail(t, "timed out waiting for node to become master")
			default:
				if master, _, _ := cluster.Roles(ctx); master == node {
					return
				}

				<-time.After(time.Second)
			}
		}
	}

	waitUntilConnectedTo := func(conn *sql.DB, node *docker.Container) {
		defer func(begin time.Time) {
			fmt.Printf("proxy responded to master change in %.2fs\n", time.Since(begin).Seconds())
		}(time.Now())

		timeout := time.After(time.Minute)

		for {
			select {
			case <-timeout:
				require.Fail(t, "timed out waiting for PGBouncer to point at new master")
			default:
				if inetServerAddr(conn) == node.NetworkSettings.IPAddress {
					return
				}

				<-time.After(500 * time.Millisecond)
			}
		}
	}

	master, sync, async := cluster.Roles(ctx)

	conn := connectTo(async)
	connectedAddr := inetServerAddr(conn)

	fmt.Printf("async node PGBouncer is proxying to %s\n", connectedAddr)

	masterAddr := master.NetworkSettings.IPAddress

	require.Equal(t, connectedAddr, masterAddr, "expected PGBouncer to connect to master")

	resp := get("/postgres/master")

	require.Equal(t, 1, len(resp.Kvs), "could not find master key in etcd")
	require.Equal(t, masterAddr, string(resp.Kvs[0].Value), "etcd master key does not equal host IP")

	// Now we need to migrate the master to the sync. This will test whether PGBouncer
	// on the other nodes will switch to point at the new master. We do this asynchronously
	// because the execution will block, and we want to be monitoring the cluster during
	// this period.
	migrationFinished := make(chan interface{})
	go runMigration(migrationFinished)

	waitUntilMaster(sync)
	waitUntilConnectedTo(conn, sync)

	select {
	case <-time.After(10 * time.Second):
		assert.Fail(t, "timed out waiting for the migration script to finish")
	case <-migrationFinished:
		// success!
	}
}
