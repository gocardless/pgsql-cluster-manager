package testHelpers

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/stretchr/testify/require"
)

func StartEtcd(t *testing.T, ctx context.Context) *clientv3.Client {
	workspace, err := ioutil.TempDir("", "etcd")
	if err != nil {
		require.Fail(t, "failed to create etcd workspace: %s", err.Error())
	}

	endpointAddr, err := nextAvailableAddr()
	if err != nil {
		require.Fail(t, "failed to allocate endpoint address: %s", err.Error())
	}

	peerAddr, err := nextAvailableAddr()
	if err != nil {
		require.Fail(t, "failed to allocate peer address: %s", err.Error())
	}

	etcd := exec.CommandContext(
		ctx,
		"etcd",
		"--data-dir", workspace,
		"--listen-peer-urls", peerAddr,
		"--initial-advertise-peer-urls", peerAddr,
		"--initial-cluster", fmt.Sprintf("default=%s", peerAddr),
		"--listen-client-urls", endpointAddr,
		"--advertise-client-urls", endpointAddr,
	)

	etcd.Dir = workspace

	if err = etcd.Start(); err != nil {
		require.Fail(t, "failed to start etcd: %s", err.Error())
	}

	cfg := clientv3.Config{
		Endpoints:   []string{endpointAddr},
		DialTimeout: 1 * time.Second,
	}

	timeout := time.After(5 * time.Second)
	retry := time.Tick(1 * time.Second)

	for {
		select {
		case <-retry:
			if client, err := clientv3.New(cfg); err == nil {
				return client
			}
		case <-timeout:
			require.Fail(t, "timed out waiting for etcd to start")
		}
	}
}

func nextAvailableAddr() (string, error) {
	var addr string

	listen, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return addr, err
	}

	defer listen.Close()
	return fmt.Sprintf("http://%s", listen.Addr().String()), nil
}
