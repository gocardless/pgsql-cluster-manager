package integration

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/coreos/etcd/clientv3"
)

var seededRandom = rand.New(rand.NewSource(time.Now().UnixNano()))
var charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandomKey will create a key that can be used in each of our etcd tests, ensuring we
// test against different keys for each test even if re-using the same etcd instance.
func RandomKey() string {
	keyBytes := make([]byte, 20)
	for idx := range keyBytes {
		keyBytes[idx] = charset[seededRandom.Intn(len(charset))]
	}

	return fmt.Sprintf("/%s", keyBytes)
}

// StartEtcd spins up a single-node etcd cluster in a temporary directory
func StartEtcd() (client *clientv3.Client, cleanup func(), err error) {
	var proc *exec.Cmd
	var workspace string

	cleanup = func() {
		if proc != nil {
			proc.Process.Kill()
		}
		os.RemoveAll(workspace)
	}

	workspace, err = ioutil.TempDir("", "etcd")
	if err != nil {
		return
	}

	endpointAddress, err := nextAvailableAddress()
	if err != nil {
		return
	}

	peerAddress, err := nextAvailableAddress()
	if err != nil {
		return
	}

	etcd := exec.Command(
		"etcd",
		"--data-dir", workspace,
		"--listen-peer-urls", peerAddress,
		"--initial-advertise-peer-urls", peerAddress,
		"--initial-cluster", fmt.Sprintf("default=%s", peerAddress),
		"--listen-client-urls", endpointAddress,
		"--advertise-client-urls", endpointAddress,
	)

	etcd.Dir = workspace

	if err = etcd.Start(); err != nil {
		return
	}

	cfg := clientv3.Config{
		Endpoints:   []string{endpointAddress},
		DialTimeout: 1 * time.Second,
	}

	success := Eventually(
		func() error { client, err = clientv3.New(cfg); return err },
		10*time.Second,
		100*time.Millisecond,
	).Should(
		Succeed(),
	)

	if !success {
		err = errors.Wrap(err, "timed out waiting for etcd to connect")
	}

	return
}

func nextAvailableAddress() (string, error) {
	var address string

	listen, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return address, errors.Wrap(err, "failed to find available port")
	}

	defer listen.Close()
	return fmt.Sprintf("http://%s", listen.Addr().String()), nil
}
