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
	"github.com/stretchr/testify/require"
)

func TestSupervise(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("creating cluster...")
	cluster := StartCluster(t, ctx)
	defer cluster.Shutdown()

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

	waitUntilMaster := func(node *docker.Container) {
		defer func(begin time.Time) {
			fmt.Printf("master was migrated in %.2fs\n", time.Since(begin).Seconds())
		}(time.Now())

		timeout := time.After(time.Minute)

		for {
			select {
			case <-timeout:
				require.Fail(t, "timed out waiting for node to become master")
			default:
				if master, _, _ := cluster.Roles(); master == node {
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

	master, sync, async := cluster.Roles()

	conn := connectTo(async)
	connectedAddr := inetServerAddr(conn)

	fmt.Printf("async node PGBouncer is proxying to %s\n", connectedAddr)

	masterAddr := master.NetworkSettings.IPAddress
	masterHostname := master.Config.Hostname

	require.Equal(t, connectedAddr, masterAddr, "expected PGBouncer to connect to master")

	resp := get("/postgres/master")

	require.Equal(t, 1, len(resp.Kvs), "could not find master key in etcd")
	require.Equal(t, masterHostname, string(resp.Kvs[0].Value), "etcd master key to equal host")

	// Now we need to migrate the master to the sync. This will test whether PGBouncer
	// on the other nodes will switch to point at the new master.
	fmt.Printf("issuing 'crm resource migrate msPostgresql %s'\n", sync.Config.Hostname)
	_, err := cluster.Executor().CombinedOutput(
		"crm", "resource", "migrate", "msPostgresql", sync.Config.Hostname,
	)

	require.Nil(t, err, "crm resource migrate failed")

	waitUntilMaster(sync)
	waitUntilConnectedTo(conn, sync)

	fmt.Println("success, proxy moved!")
}
